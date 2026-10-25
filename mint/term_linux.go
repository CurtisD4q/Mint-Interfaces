//go:build linux

package mint

import "syscall"

const (
	ioctlGetTermios = syscall.TCGETS
	ioctlSetTermios = 0x5403 // TCSETSW (TCSADRAIN, as stty uses)
)

// makeRaw applies GNU stty's "raw -echo".
func makeRaw(t *syscall.Termios) {
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.IGNPAR | syscall.PARMRK |
		syscall.INPCK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL |
		syscall.IXON | syscall.IXOFF | syscall.IUCLC | syscall.IXANY | syscall.IMAXBEL
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ICANON | syscall.ISIG | syscall.ECHO
}
