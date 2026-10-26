//go:build linux || darwin

package secure

import "syscall"

func mlock(b []byte) error   { return syscall.Mlock(b) }
func munlock(b []byte) error { return syscall.Munlock(b) }
