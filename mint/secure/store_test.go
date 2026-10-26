package secure

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// mappingFlags returns the smaps entry ("Locked:" kB and VmFlags) of the
// mapping that contains b.
func mappingFlags(t *testing.T, b []byte) (lockedKB int, flags string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("smaps is Linux-only")
	}
	addr := uint64(uintptr(unsafe.Pointer(&b[0])))
	f, err := os.Open("/proc/self/smaps")
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	in := false
	for sc.Scan() {
		line := sc.Text()
		if fields := strings.Fields(line); len(fields) > 0 && strings.Contains(fields[0], "-") && !strings.HasSuffix(fields[0], ":") {
			r := strings.SplitN(fields[0], "-", 2)
			lo, _ := strconv.ParseUint(r[0], 16, 64)
			hi, _ := strconv.ParseUint(r[1], 16, 64)
			in = addr >= lo && addr < hi
			continue
		}
		if !in {
			continue
		}
		if strings.HasPrefix(line, "Locked:") {
			lockedKB, _ = strconv.Atoi(strings.Fields(line)[1])
		}
		if strings.HasPrefix(line, "VmFlags:") {
			return lockedKB, line
		}
	}
	t.Fatalf("mapping for %x not found", addr)
	return 0, ""
}

func TestPutUseRoundTrip(t *testing.T) {
	st, err := OpenWithTimeout(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	in := []byte("hunter2")
	if err := st.Put("pw", in); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, make([]byte, len(in))) {
		t.Fatalf("Put did not wipe the caller's copy: %q", in)
	}
	var got string
	if err := st.Use("pw", func(s []byte) error { got = string(s); return nil }); err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Fatalf("got %q", got)
	}
	if err := st.Use("missing", func([]byte) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	t.Logf("protection level here: %s %v", st.Level(), st.Notes())
}

func TestSecretsAreEncryptedLockedAndNotDumped(t *testing.T) {
	st, err := OpenWithTimeout(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Put("pw", []byte("correct horse battery staple")); err != nil {
		t.Fatal(err)
	}
	stored := st.entries["pw"].data.bytes()
	if bytes.Contains(stored, []byte("horse")) {
		t.Fatal("secret stored in plain text")
	}
	locked, flags := mappingFlags(t, stored)
	if !strings.Contains(flags, " dd") {
		t.Errorf("not excluded from core dumps: %s", flags)
	}
	if st.entries["pw"].data.locked && (locked == 0 || !strings.Contains(flags, " lo")) {
		t.Errorf("reported locked but smaps says Locked: %d kB, %s", locked, flags)
	}
	// The plaintext handed to Use is in locked, don't-dump memory too.
	st.Use("pw", func(s []byte) error {
		if _, fl := mappingFlags(t, s); !strings.Contains(fl, " dd") {
			t.Errorf("plaintext buffer not excluded from dumps: %s", fl)
		}
		return nil
	})
}

func TestKeyLivesInTheKernelKeyring(t *testing.T) {
	if !keyringAvailable() {
		t.Skip("no kernel keyring")
	}
	st, err := OpenWithTimeout(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.keyID == 0 {
		t.Skipf("keyring blocked here: %v", st.Notes())
	}
	if st.keyBuf != nil {
		t.Fatal("key also kept in process memory")
	}
	if st.Level() != Full && st.Level() != NoLock {
		t.Fatalf("level %v", st.Level())
	}
	if isDumpable() {
		t.Error("process still dumpable after Open")
	}
}

func TestExpiry(t *testing.T) {
	for _, mode := range []string{"keyring", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			forceNoKeyring = mode == "fallback"
			defer func() { forceNoKeyring = false }()
			st, err := OpenWithTimeout(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if mode == "keyring" && st.keyID == 0 {
				t.Skip("no kernel keyring here")
			}
			if err := st.Put("pw", []byte("x")); err != nil {
				t.Fatal(err)
			}
			time.Sleep(2200 * time.Millisecond)
			if err := st.Use("pw", func([]byte) error { return nil }); !errors.Is(err, ErrExpired) {
				t.Fatalf("after the timeout: %v, want ErrExpired", err)
			}
			if st.Has("pw") {
				t.Fatal("secrets kept after expiry")
			}
			if err := st.Renew(); err != nil {
				t.Fatal(err)
			}
			if err := st.Put("pw", []byte("y")); err != nil {
				t.Fatalf("after Renew: %v", err)
			}
		})
	}
}

func TestUseKeepsTheKeyAlive(t *testing.T) {
	forceNoKeyring = true
	defer func() { forceNoKeyring = false }()
	st, _ := OpenWithTimeout(time.Second)
	defer st.Close()
	st.Put("pw", []byte("x"))
	for i := 0; i < 4; i++ {
		time.Sleep(600 * time.Millisecond)
		if err := st.Use("pw", func([]byte) error { return nil }); err != nil {
			t.Fatalf("use %d: %v (the idle timer should restart on each use)", i, err)
		}
	}
}

func TestNoLockLevel(t *testing.T) {
	forceNoLock, forceNoKeyring = true, true
	defer func() { forceNoLock, forceNoKeyring = false, false }()
	st, _ := OpenWithTimeout(0)
	defer st.Close()
	if st.Level() != NoLock || !strings.Contains(st.Level().String(), "swap") {
		t.Fatalf("level %v", st.Level())
	}
}

func TestCloseWipes(t *testing.T) {
	st, _ := OpenWithTimeout(0)
	st.Put("pw", []byte("x"))
	st.Close()
	if err := st.Use("pw", func([]byte) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("after Close: %v", err)
	}
	if err := st.Put("pw", []byte("x")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Put after Close: %v", err)
	}
}

func TestIdleTimeoutSettings(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.json")
	system := filepath.Join(dir, "system.json")
	oldUser, oldSys := userSettingsPath, systemSettingsPath
	userSettingsPath, systemSettingsPath = func() string { return user }, system
	defer func() { userSettingsPath, systemSettingsPath = oldUser, oldSys }()
	t.Setenv("MINT_KEY_TIMEOUT", "")

	if d, src := IdleTimeout(); d != DefaultIdleTimeout || src != "default" {
		t.Fatalf("default: %v %s", d, src)
	}
	os.WriteFile(system, []byte(`{"key_timeout":"30m"}`), 0o644)
	if d, src := IdleTimeout(); d != 30*time.Minute || src != system {
		t.Fatalf("system: %v %s", d, src)
	}
	os.WriteFile(user, []byte(`{"key_timeout":"5m"}`), 0o644)
	if d, _ := IdleTimeout(); d != 5*time.Minute {
		t.Fatalf("user file should win over the system file: %v", d)
	}
	t.Setenv("MINT_KEY_TIMEOUT", "0")
	if d, src := IdleTimeout(); d != 0 || src != "MINT_KEY_TIMEOUT" {
		t.Fatalf("env: %v %s", d, src)
	}
	t.Setenv("MINT_KEY_TIMEOUT", "nonsense")
	if d, _ := IdleTimeout(); d != 5*time.Minute {
		t.Fatalf("a bad value should be ignored: %v", d)
	}
}
