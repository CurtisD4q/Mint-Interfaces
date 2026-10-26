//go:build linux

package secure

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// Linux keeps the process keyring in each thread's credentials: a thread
// that existed before the keyring was created cannot see it, and reading
// the key from such a thread fails with "permission denied". Go moves
// goroutines between threads freely, so every keyring call runs on one
// dedicated thread that never changes.
var (
	keyThreadOnce sync.Once
	keyThreadCh   chan func()
)

func onKeyThread(f func()) {
	keyThreadOnce.Do(func() {
		keyThreadCh = make(chan func())
		go func() {
			runtime.LockOSThread() // never unlocked: this thread serves only the keyring
			for call := range keyThreadCh {
				call()
			}
		}()
	})
	done := make(chan struct{})
	keyThreadCh <- func() { f(); close(done) }
	<-done
}

const (
	keySpecProcessKeyring = -2 // KEY_SPEC_PROCESS_KEYRING
	keyctlRevoke          = 3  // KEYCTL_REVOKE
	keyctlRead            = 11 // KEYCTL_READ
	keyctlSetTimeout      = 15 // KEYCTL_SET_TIMEOUT
	madvDontDump          = 16 // MADV_DONTDUMP
	prSetDumpable         = 4  // PR_SET_DUMPABLE
	prGetDumpable         = 3  // PR_GET_DUMPABLE
)

// keyringAdd stores payload as a "user" key in this process's own keyring,
// which only this process can use and which the kernel deletes when the
// process exits.
func keyringAdd(desc string, payload []byte) (int, error) {
	typ, err := syscall.BytePtrFromString("user")
	if err != nil {
		return 0, err
	}
	d, err := syscall.BytePtrFromString(desc)
	if err != nil {
		return 0, err
	}
	var p unsafe.Pointer
	if len(payload) > 0 {
		p = unsafe.Pointer(&payload[0])
	}
	ring := int32(keySpecProcessKeyring) // a negative special id
	var id uintptr
	var errno syscall.Errno
	onKeyThread(func() {
		id, _, errno = syscall.Syscall6(syscall.SYS_ADD_KEY, uintptr(unsafe.Pointer(typ)), uintptr(unsafe.Pointer(d)),
			uintptr(p), uintptr(len(payload)), uintptr(ring), 0)
	})
	runtime.KeepAlive(typ)
	runtime.KeepAlive(d)
	runtime.KeepAlive(payload)
	if errno != 0 {
		return 0, errno
	}
	return int(id), nil
}

// keyringRead copies the key's payload into buf and returns its length.
func keyringRead(id int, buf []byte) (int, error) {
	var n uintptr
	var errno syscall.Errno
	onKeyThread(func() {
		n, _, errno = syscall.Syscall6(syscall.SYS_KEYCTL, keyctlRead, uintptr(id),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
	})
	if errno != 0 {
		return 0, errno
	}
	return int(n), nil
}

// keyringAlive reports whether the key still exists (not expired or
// revoked), without copying its payload.
func keyringAlive(id int) bool {
	var errno syscall.Errno
	onKeyThread(func() {
		_, _, errno = syscall.Syscall6(syscall.SYS_KEYCTL, keyctlRead, uintptr(id), 0, 0, 0, 0)
	})
	return errno == 0
}

// keyringTimeout makes the kernel expire the key after secs seconds
// (0 removes the timeout). Setting it again restarts the countdown.
func keyringTimeout(id int, secs uint) error {
	var errno syscall.Errno
	onKeyThread(func() {
		_, _, errno = syscall.Syscall(syscall.SYS_KEYCTL, keyctlSetTimeout, uintptr(id), uintptr(secs))
	})
	if errno != 0 {
		return errno
	}
	return nil
}

func keyringRevoke(id int) {
	onKeyThread(func() {
		_, _, _ = syscall.Syscall(syscall.SYS_KEYCTL, keyctlRevoke, uintptr(id), 0)
	})
}

func keyringAvailable() bool { return true }

// excludeFromDumps keeps a memory region out of core dumps.
func excludeFromDumps(b []byte) error { return syscall.Madvise(b, madvDontDump) }

// setNotDumpable stops core dumps of this process and stops other
// processes running as the same user from reading its memory.
func setNotDumpable() error {
	_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func isDumpable() bool {
	r, _, _ := syscall.RawSyscall(syscall.SYS_PRCTL, prGetDumpable, 0, 0)
	return r != 0
}
