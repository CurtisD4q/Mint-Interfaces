package secure

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPBKDF2Vectors(t *testing.T) {
	// RFC 7914 §11 / the widely published PBKDF2-HMAC-SHA256 vectors.
	for iter, want := range map[int]string{
		1:    "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b",
		2:    "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43",
		4096: "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a",
	} {
		if got := hex.EncodeToString(pbkdf2SHA256([]byte("password"), []byte("salt"), iter, 32)); got != want {
			t.Errorf("iter %d: %s", iter, got)
		}
	}
}

func TestSealRoundTrip(t *testing.T) {
	data := []byte("BEGIN PRIVATE KEY\nabc\n")
	sealed, err := SealBytes(data, []byte("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("PRIVATE")) || !IsSealed(sealed) {
		t.Fatal("sealed output holds the plaintext or lacks the header")
	}
	if name, _ := SealerOf(sealed); name != "aes-256-gcm" {
		t.Fatalf("sealer %q", name)
	}
	got, err := OpenSealed(sealed, []byte("correct horse"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := OpenSealed(sealed, []byte("wrong")); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 1
	if _, err := OpenSealed(tampered, []byte("correct horse")); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("tampered file opened: %v", err)
	}
	if _, err := SealBytes(data, nil); err == nil {
		t.Fatal("sealed with an empty passphrase")
	}
}

// xorSealer stands in for a user's own algorithm (it is NOT secure).
type xorSealer struct{}

func (xorSealer) Name() string { return "test-xor" }
func (xorSealer) Seal(key, p []byte) ([]byte, error) {
	out := make([]byte, len(p)+1)
	out[0] = 'X'
	for i, b := range p {
		out[i+1] = b ^ key[i%len(key)]
	}
	return out, nil
}
func (xorSealer) Open(key, s []byte) ([]byte, error) {
	if len(s) == 0 || s[0] != 'X' {
		return nil, errors.New("bad")
	}
	out := make([]byte, len(s)-1)
	for i, b := range s[1:] {
		out[i] = b ^ key[i%len(key)]
	}
	return out, nil
}

func TestCustomSealer(t *testing.T) {
	RegisterSealer(xorSealer{})
	t.Setenv("MINT_SEALER", "test-xor")
	sealed, err := SealBytes([]byte("hello"), []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if name, _ := SealerOf(sealed); name != "test-xor" {
		t.Fatalf("file names sealer %q", name)
	}
	// The file opens with the sealer that made it, whatever the default is now.
	t.Setenv("MINT_SEALER", "aes-256-gcm")
	if got, err := OpenSealed(sealed, []byte("pw")); err != nil || string(got) != "hello" {
		t.Fatalf("open: %q %v", got, err)
	}
	t.Setenv("MINT_SEALER", "missing")
	if _, err := SealBytes([]byte("x"), []byte("pw")); err == nil {
		t.Fatal("sealed with an unregistered sealer")
	}
}

func TestSealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.sql")
	os.WriteFile(path, []byte("secret rows"), 0o644)
	dest, err := SealFile(path, []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the plaintext file is still there")
	}
	fi, _ := os.Stat(dest)
	if fi.Mode().Perm() != 0o600 || dest != path+SealedExt {
		t.Fatalf("%s mode %v", dest, fi.Mode())
	}
	b, _ := os.ReadFile(dest)
	if got, err := OpenSealed(b, []byte("pw")); err != nil || string(got) != "secret rows" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestPrivateDir(t *testing.T) {
	uid, gid := os.Getuid(), os.Getgid()
	oldEnv, oldRun, oldShm := runtimeDirEnv, runUserRoot, shmRoot
	defer func() { runtimeDirEnv, runUserRoot, shmRoot = oldEnv, oldRun, oldShm }()
	runUserRoot = filepath.Join(t.TempDir(), "none")
	shmRoot = filepath.Join(t.TempDir(), "none")

	// A private runtime folder is used.
	rt := t.TempDir()
	os.Chmod(rt, 0o700)
	runtimeDirEnv = func() string { return rt }
	dir, _, err := PrivateDir("app", uid, gid)
	if err != nil || dir != filepath.Join(rt, "mint", "app") {
		t.Fatalf("%s %v", dir, err)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", fi.Mode())
	}

	// One other users can open is refused.
	open := t.TempDir()
	os.Chmod(open, 0o755)
	runtimeDirEnv = func() string { return open }
	if _, _, err := PrivateDir("app", uid, gid); !errors.Is(err, ErrNoPrivateDir) {
		t.Fatalf("used a folder others can open: %v", err)
	}

	// Bad names are refused.
	runtimeDirEnv = func() string { return rt }
	for _, bad := range []string{"", "..", "a/b"} {
		if _, _, err := PrivateDir(bad, uid, gid); err == nil {
			t.Fatalf("accepted name %q", bad)
		}
	}

	// In a shared folder, a planted symlink or open folder is refused.
	shm := t.TempDir()
	runtimeDirEnv = func() string { return "" }
	runUserRoot = filepath.Join(t.TempDir(), "none")
	shmRoot = shm
	planted := filepath.Join(shm, "mint-"+strconv.Itoa(uid))
	os.Symlink(t.TempDir(), planted)
	if d, err := makePrivate(planted, uid, gid); err == nil {
		t.Fatalf("followed a planted symlink to %s", d)
	}
	os.Remove(planted)
	os.Mkdir(planted, 0o755)
	os.Chmod(planted, 0o755)
	if _, err := makePrivate(planted, uid, gid); err == nil {
		t.Fatal("accepted a planted folder others can open")
	}
}

func TestPrivateDirIsInMemoryHere(t *testing.T) {
	if _, err := os.Stat("/dev/shm"); err != nil || !isMemoryFS("/dev/shm") {
		t.Skip("no tmpfs at /dev/shm")
	}
	dir, mem, err := PrivateDir("mint-test", os.Getuid(), os.Getgid())
	if err != nil || !mem {
		t.Fatalf("%s in memory=%v %v", dir, mem, err)
	}
	os.Remove(dir)
}
