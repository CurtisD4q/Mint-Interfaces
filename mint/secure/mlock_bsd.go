//go:build freebsd || openbsd || netbsd || dragonfly

package secure

import (
	"syscall"
	"unsafe"
)

// The syscall package has no Mlock wrapper on the BSDs.
func mlock(b []byte) error   { return memcall(syscall.SYS_MLOCK, b) }
func munlock(b []byte) error { return memcall(syscall.SYS_MUNLOCK, b) }

func memcall(trap uintptr, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	_, _, errno := syscall.Syscall(trap, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
	if errno != 0 {
		return errno
	}
	return nil
}
