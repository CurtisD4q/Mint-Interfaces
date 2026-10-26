package secure

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// ErrNoPrivateDir means there is no private, memory-backed folder here.
var ErrNoPrivateDir = errors.New("no private in-memory folder on this system")

// Roots for tests.
var (
	runtimeDirEnv = func() string { return os.Getenv("XDG_RUNTIME_DIR") }
	runUserRoot   = "/run/user"
	shmRoot       = "/dev/shm"
)

// PrivateDir returns a folder for name that only user uid can open, kept in
// memory where the system allows it, so files there never reach the disk
// and disappear at logout or reboot. It tries, in order:
//
//   - $XDG_RUNTIME_DIR (when it belongs to uid): systemd's per-user tmpfs,
//     removed when the user logs out;
//   - /run/user/<uid>, the same folder when the environment lost it (sudo);
//   - /dev/shm/mint-<uid>, a folder in the shared tmpfs, removed at reboot.
//
// inMemory reports whether the folder is on tmpfs or ramfs (Linux only;
// false elsewhere). Folders are created with mode 0700 and owned by uid:gid,
// and each step refuses symlinks and folders owned by anyone else.
//
// tmpfs pages can still be swapped out: use encrypted swap, or seal files
// that hold secrets.
func PrivateDir(name string, uid, gid int) (dir string, inMemory bool, err error) {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", false, fmt.Errorf("secure: bad folder name %q", name)
	}
	var bases []string
	if d := runtimeDirEnv(); d != "" && uid == os.Geteuid() {
		bases = append(bases, d)
	}
	bases = append(bases, filepath.Join(runUserRoot, strconv.Itoa(uid)))

	for _, base := range bases {
		if ownedPrivateDir(base, uid) != nil {
			continue
		}
		if dir, err := makePrivate(filepath.Join(base, "mint"), uid, gid); err == nil {
			if dir, err = makePrivate(filepath.Join(dir, name), uid, gid); err == nil {
				return dir, isMemoryFS(dir), nil
			}
		}
	}

	// The shared tmpfs: anyone can create folders there, so ours must be
	// created by us (or already be ours, private and not a symlink).
	if fi, err := os.Stat(shmRoot); err == nil && fi.IsDir() && isMemoryFS(shmRoot) {
		if dir, err := makePrivate(filepath.Join(shmRoot, "mint-"+strconv.Itoa(uid)), uid, gid); err == nil {
			if dir, err = makePrivate(filepath.Join(dir, name), uid, gid); err == nil {
				return dir, true, nil
			}
		}
	}
	return "", false, ErrNoPrivateDir
}

// makePrivate creates dir (mode 0700, owned by uid:gid) or checks that an
// existing one is a real folder, owned by uid and closed to everyone else.
func makePrivate(dir string, uid, gid int) (string, error) {
	err := os.Mkdir(dir, 0o700)
	if err == nil {
		if uid != os.Geteuid() {
			if err := os.Lchown(dir, uid, gid); err != nil {
				_ = os.Remove(dir)
				return "", err
			}
		}
		if err := os.Chmod(dir, 0o700); err != nil { // the umask may have removed bits; never added
			return "", err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if err := ownedPrivateDir(dir, uid); err != nil {
		return "", err
	}
	return dir, nil
}

// ownedPrivateDir checks that dir is a folder (not a symlink), owned by uid,
// that no one else can open.
func ownedPrivateDir(dir string, uid int) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("secure: %s is not a folder", dir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("secure: %s can be opened by other users", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != uid {
		return fmt.Errorf("secure: %s belongs to another user", dir)
	}
	return nil
}
