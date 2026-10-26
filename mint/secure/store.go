// Package secure keeps passwords and other small secrets safe while a Mint
// app runs.
//
// Each secret is encrypted (AES-256-GCM) and held in locked memory outside
// Go's heap: never written to swap, kept out of core dumps, and wiped when
// it is removed. The encryption key lives in the app's own kernel keyring on
// Linux, where only this process can use it and the kernel deletes it when
// the app closes or the idle timeout passes. Where there is no keyring
// (containers that block it, macOS, the BSDs) the key is kept in its own
// locked page instead and the store enforces the timeout itself.
//
// Secrets are decrypted only for the moment they are used:
//
//	st, _ := secure.Open()
//	defer st.Close()
//	st.Put("db-password", typed)          // typed is wiped by Put
//	st.Use("db-password", func(pw []byte) error {
//		_, err := stdin.Write(append(pw, '\n')) // pw is wiped afterwards
//		return err
//	})
//
// Keep secrets as []byte from the password box to the program: converting
// one to a string makes copies the store cannot wipe.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"
)

const keySize = 32 // AES-256

// Errors returned by the store.
var (
	ErrExpired  = errors.New("secure store: the key expired; unlock again")
	ErrNotFound = errors.New("secure store: no such secret")
	ErrClosed   = errors.New("secure store: closed")
)

// Level says how well the store can protect secrets on this system.
type Level int

const (
	// Full: key in the app's own kernel keyring, secrets in locked,
	// don't-dump memory, app not dumpable.
	Full Level = iota
	// NoKeyring: the key is kept in locked memory instead (containers,
	// macOS, BSD); the store wipes it when the timeout passes.
	NoKeyring
	// NoLock: memory could not be locked (limit too low), so secrets may
	// reach swap. Encrypted swap covers this.
	NoLock
)

func (l Level) String() string {
	switch l {
	case Full:
		return "full"
	case NoKeyring:
		return "reduced: no kernel keyring, key kept in locked memory"
	}
	return "reduced: memory cannot be locked, secrets may be written to swap (use encrypted swap)"
}

type entry struct {
	nonce []byte
	data  *lockedBuf // ciphertext
}

// Store holds secrets for one running app.
type Store struct {
	mu      sync.Mutex
	timeout time.Duration
	keyID   int        // kernel key, or 0
	keyBuf  *lockedBuf // fallback: key in locked memory
	lastUse time.Time
	entries map[string]entry
	level   Level
	notes   []string
	closed  bool
	expired bool
}

// forceNoKeyring makes Open use the fallback (tests).
var forceNoKeyring bool

// Open creates the store with this computer's idle timeout (see
// IdleTimeout). It also hardens the process: core dumps off, and on Linux
// not dumpable, so other processes running as the same user cannot read
// its memory.
func Open() (*Store, error) {
	d, _ := IdleTimeout()
	return OpenWithTimeout(d)
}

// OpenWithTimeout is Open with an explicit idle timeout (0: until Close).
func OpenWithTimeout(timeout time.Duration) (*Store, error) {
	s := &Store{timeout: timeout, entries: map[string]entry{}, lastUse: time.Now()}
	if err := Harden(); err != nil {
		s.notes = append(s.notes, "could not switch off crash dumps: "+err.Error())
	}

	key, err := newLocked(keySize)
	if err != nil {
		return nil, fmt.Errorf("secure store: %w", err)
	}
	if _, err := rand.Read(key.bytes()); err != nil {
		key.release()
		return nil, fmt.Errorf("secure store: %w", err)
	}
	if !key.locked {
		s.level = NoLock
	}

	if keyringAvailable() && !forceNoKeyring {
		if id, err := keyringAdd(fmt.Sprintf("mint-store-%p", s), key.bytes()); err == nil {
			s.keyID = id
			if timeout > 0 {
				_ = keyringTimeout(id, timeoutSecs(timeout))
			}
			key.release() // the kernel holds the only copy now
		} else {
			s.notes = append(s.notes, "kernel keyring unavailable: "+err.Error())
		}
	}
	if s.keyID == 0 {
		s.keyBuf = key
		if s.level == Full {
			s.level = NoKeyring
		}
	}
	return s, nil
}

func timeoutSecs(d time.Duration) uint {
	secs := uint((d + time.Second - 1) / time.Second)
	if secs == 0 {
		secs = 1
	}
	return secs
}

// Level reports how well secrets are protected here; Notes gives reasons.
func (s *Store) Level() Level { return s.level }

// Notes explains anything that reduced the protection level.
func (s *Store) Notes() []string { return append([]string(nil), s.notes...) }

// Put encrypts secret into the store under name and wipes secret.
func (s *Store) Put(name string, secret []byte) error {
	defer wipe(secret)
	s.mu.Lock()
	defer s.mu.Unlock()
	aead, err := s.cipherLocked()
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	buf, err := newLocked(len(secret) + aead.Overhead())
	if err != nil {
		return err
	}
	aead.Seal(buf.bytes()[:0], nonce, secret, []byte(name))
	if old, ok := s.entries[name]; ok {
		old.data.release()
	}
	s.entries[name] = entry{nonce: nonce, data: buf}
	return nil
}

// Use decrypts the secret into locked memory, passes it to fn, and wipes
// it when fn returns. fn must not keep the slice or copy it elsewhere.
func (s *Store) Use(name string, fn func(secret []byte) error) error {
	s.mu.Lock()
	aead, err := s.cipherLocked()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	e, ok := s.entries[name]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	plain, err := newLocked(len(e.data.bytes()) - aead.Overhead())
	if err != nil {
		s.mu.Unlock()
		return err
	}
	_, err = aead.Open(plain.bytes()[:0], e.nonce, e.data.bytes(), []byte(name))
	s.mu.Unlock()
	defer plain.release()
	if err != nil {
		return fmt.Errorf("secure store: %w", err)
	}
	return fn(plain.bytes())
}

// Has reports whether a secret is stored under name.
func (s *Store) Has(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.entries[name]
	return ok && !s.closed
}

// Delete removes and wipes one secret.
func (s *Store) Delete(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[name]; ok {
		e.data.release()
		delete(s.entries, name)
	}
}

// Close wipes every secret and destroys the key. Call it when the app exits.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropLocked()
	s.closed = true
}

// dropLocked wipes all secrets and the key.
func (s *Store) dropLocked() {
	for name, e := range s.entries {
		e.data.release()
		delete(s.entries, name)
	}
	if s.keyID != 0 {
		keyringRevoke(s.keyID)
		s.keyID = 0
	}
	s.keyBuf.release()
	s.keyBuf = nil
}

// cipherLocked returns the AEAD for the current key, resetting the idle
// timeout, or ErrExpired once the key has gone (all secrets are wiped then).
func (s *Store) cipherLocked() (cipher.AEAD, error) {
	if s.closed {
		return nil, ErrClosed
	}
	if s.expired {
		return nil, ErrExpired
	}
	key, err := newLocked(keySize)
	if err != nil {
		return nil, err
	}
	defer key.release()

	switch {
	case s.keyID != 0:
		n, err := keyringRead(s.keyID, key.bytes())
		if err != nil || n != keySize {
			// The kernel expired (or revoked) the key.
			s.expire()
			return nil, ErrExpired
		}
		if s.timeout > 0 {
			_ = keyringTimeout(s.keyID, timeoutSecs(s.timeout))
		}
	case s.keyBuf != nil:
		if s.timeout > 0 && time.Since(s.lastUse) > s.timeout {
			s.expire()
			return nil, ErrExpired
		}
		copy(key.bytes(), s.keyBuf.bytes())
	default:
		s.expire()
		return nil, ErrExpired
	}
	s.lastUse = time.Now()

	block, err := aes.NewCipher(key.bytes())
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Renew starts afresh after the key expired: a new key, no secrets. The
// app then asks the user again for whatever it needs.
func (s *Store) Renew() error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return ErrClosed
	}
	fresh, err := OpenWithTimeout(s.timeout)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropLocked()
	s.keyID, s.keyBuf, s.level, s.notes = fresh.keyID, fresh.keyBuf, fresh.level, fresh.notes
	s.entries, s.expired, s.lastUse = map[string]entry{}, false, time.Now()
	return nil
}

func (s *Store) expire() {
	s.dropLocked()
	s.expired = true
}

// Harden switches off core dumps for this process and, on Linux, marks it
// not dumpable, so other processes running as the same user cannot attach
// to it and read its memory. Open calls it; apps may call it at startup.
func Harden() error {
	err1 := disableCoreDumps()
	err2 := setNotDumpable()
	if err1 != nil {
		return err1
	}
	return err2
}
