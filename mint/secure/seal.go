package secure

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// A Sealer encrypts data for storage in a file. Mint ships "aes-256-gcm";
// register your own with RegisterSealer and choose it with $MINT_SEALER or
// "sealer" in security.json.
//
// Mint derives the key from the user's passphrase (PBKDF2-HMAC-SHA256 with
// a random salt) and records the sealer's name in the file, so a file always
// opens with the sealer that made it. A Sealer only turns a 32-byte key and
// plaintext into sealed bytes and back. Open must fail (not return garbage)
// when the key is wrong or the data was changed.
type Sealer interface {
	Name() string
	Seal(key, plaintext []byte) ([]byte, error)
	Open(key, sealed []byte) ([]byte, error)
}

// SealedExt is the file extension of sealed files.
const SealedExt = ".mintseal"

// ErrWrongPassphrase is returned when a sealed file does not open: the
// passphrase is wrong or the file was changed.
var ErrWrongPassphrase = errors.New("wrong passphrase, or the file was changed")

var (
	sealersMu sync.Mutex
	sealers   = map[string]Sealer{}
)

func init() { RegisterSealer(aesGCMSealer{}) }

// RegisterSealer makes s available by its name (replacing one with the
// same name). Call it from an init function or main before the app runs.
func RegisterSealer(s Sealer) {
	name := strings.TrimSpace(s.Name())
	if name == "" || len(name) > 255 {
		panic("secure: a sealer needs a name of 1-255 bytes")
	}
	sealersMu.Lock()
	sealers[name] = s
	sealersMu.Unlock()
}

// Sealers lists the registered sealers' names.
func Sealers() []string {
	sealersMu.Lock()
	defer sealersMu.Unlock()
	out := make([]string, 0, len(sealers))
	for n := range sealers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func lookupSealer(name string) (Sealer, bool) {
	sealersMu.Lock()
	defer sealersMu.Unlock()
	s, ok := sealers[name]
	return s, ok
}

// DefaultSealer is the sealer used for new files: the one named by
// $MINT_SEALER or security.json, else "aes-256-gcm".
func DefaultSealer() (Sealer, error) {
	name := sealerSetting()
	if name == "" {
		name = "aes-256-gcm"
	}
	s, ok := lookupSealer(name)
	if !ok {
		return nil, fmt.Errorf("secure: sealer %q is not registered (have %s)", name, strings.Join(Sealers(), ", "))
	}
	return s, nil
}

// aesGCMSealer is the built-in sealer: AES-256-GCM with a random nonce.
type aesGCMSealer struct{}

func (aesGCMSealer) Name() string { return "aes-256-gcm" }

func (aesGCMSealer) Seal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (aesGCMSealer) Open(key, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, ErrWrongPassphrase
	}
	n := gcm.NonceSize()
	return gcm.Open(nil, sealed[:n], sealed[n:], nil)
}

// File layout: "MINTSEAL" | version 1 | name length | sealer name |
// 16-byte salt | PBKDF2 iterations (uint32, big endian) | sealed data.
const (
	sealMagic      = "MINTSEAL"
	sealVersion    = 1
	sealSaltLen    = 16
	sealKeyLen     = 32
	sealIterations = 600_000 // OWASP's 2023 figure for PBKDF2-HMAC-SHA256
)

// SealBytes encrypts data with the default sealer under a key derived from
// passphrase. The result is the content of a sealed file.
func SealBytes(data, passphrase []byte) ([]byte, error) {
	s, err := DefaultSealer()
	if err != nil {
		return nil, err
	}
	return sealWith(s, data, passphrase, sealIterations)
}

func sealWith(s Sealer, data, passphrase []byte, iter uint32) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("secure: empty passphrase")
	}
	salt := make([]byte, sealSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := pbkdf2SHA256(passphrase, salt, int(iter), sealKeyLen)
	defer wipe(key)
	body, err := s.Seal(key, data)
	if err != nil {
		return nil, fmt.Errorf("secure: sealer %s: %w", s.Name(), err)
	}
	var out bytes.Buffer
	out.WriteString(sealMagic)
	out.WriteByte(sealVersion)
	out.WriteByte(byte(len(s.Name())))
	out.WriteString(s.Name())
	out.Write(salt)
	_ = binary.Write(&out, binary.BigEndian, iter)
	out.Write(body)
	return out.Bytes(), nil
}

// SealerOf returns the name of the sealer that made a sealed file's content.
func SealerOf(sealed []byte) (string, error) {
	name, _, _, _, err := parseSealed(sealed)
	return name, err
}

// IsSealed reports whether data looks like a sealed file.
func IsSealed(data []byte) bool { return bytes.HasPrefix(data, []byte(sealMagic)) }

func parseSealed(b []byte) (name string, salt []byte, iter uint32, body []byte, err error) {
	bad := errors.New("secure: not a sealed file")
	if !IsSealed(b) || len(b) < len(sealMagic)+2 {
		return "", nil, 0, nil, bad
	}
	b = b[len(sealMagic):]
	if b[0] != sealVersion {
		return "", nil, 0, nil, fmt.Errorf("secure: sealed file version %d is not supported", b[0])
	}
	n := int(b[1])
	b = b[2:]
	if len(b) < n+sealSaltLen+4 {
		return "", nil, 0, nil, bad
	}
	name = string(b[:n])
	salt = b[n : n+sealSaltLen]
	iter = binary.BigEndian.Uint32(b[n+sealSaltLen:])
	body = b[n+sealSaltLen+4:]
	if iter == 0 || iter > 100_000_000 {
		return "", nil, 0, nil, bad
	}
	return name, salt, iter, body, nil
}

// OpenSealed decrypts a sealed file's content with the sealer that made it.
func OpenSealed(sealed, passphrase []byte) ([]byte, error) {
	name, salt, iter, body, err := parseSealed(sealed)
	if err != nil {
		return nil, err
	}
	s, ok := lookupSealer(name)
	if !ok {
		return nil, fmt.Errorf("secure: this file was sealed with %q, which this app does not have", name)
	}
	key := pbkdf2SHA256(passphrase, salt, int(iter), sealKeyLen)
	defer wipe(key)
	out, err := s.Open(key, body)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return out, nil
}

// SealFile encrypts the file at path into path+SealedExt (readable only
// by its owner), then overwrites the original with zeros and removes it.
// Overwriting is best effort: journaling and copy-on-write file systems
// and SSDs may keep old copies.
func SealFile(path string, passphrase []byte) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	defer wipe(data)
	sealed, err := SealBytes(data, passphrase)
	if err != nil {
		return "", err
	}
	dest := path + SealedExt
	if err := os.WriteFile(dest, sealed, 0o600); err != nil {
		return "", err
	}
	if err := os.Chmod(dest, 0o600); err != nil { // WriteFile keeps an existing file's mode
		return "", err
	}
	if err := shred(path, len(data)); err != nil {
		return dest, fmt.Errorf("sealed copy written, but the original could not be removed: %w", err)
	}
	return dest, nil
}

// shred overwrites a file with zeros, flushes it, and removes it.
func shred(path string, size int) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		zero := make([]byte, 64*1024)
		for left := size; left > 0; {
			n := len(zero)
			if left < n {
				n = left
			}
			if _, werr := f.Write(zero[:n]); werr != nil {
				break
			}
			left -= n
		}
		_ = f.Sync()
		_ = f.Close()
	}
	return os.Remove(path)
}

// pbkdf2SHA256 is PBKDF2 (RFC 8018) with HMAC-SHA256. (Go's crypto/pbkdf2
// needs Go 1.24; this module supports 1.21.)
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hLen := prf.Size()
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	var buf [4]byte
	u := make([]byte, hLen)
	t := make([]byte, hLen)
	for block := 1; block <= blocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf[:], uint32(block))
		prf.Write(buf[:])
		u = prf.Sum(u[:0])
		copy(t, u)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	wipe(u)
	wipe(t)
	return out[:keyLen]
}
