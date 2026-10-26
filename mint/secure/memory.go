package secure

import "syscall"

// lockedBuf is memory outside Go's heap, so the garbage collector never
// copies it. It is locked (never written to swap) when the system allows,
// marked to stay out of core dumps where supported, and wiped before it is
// released.
type lockedBuf struct {
	mem      []byte // the whole mapping, as returned by Mmap
	b        []byte // the usable part
	locked   bool
	noDump   bool
	released bool
}

// forceNoLock makes newLocked behave as if locking were refused (tests).
var forceNoLock bool

func newLocked(n int) (*lockedBuf, error) {
	page := syscall.Getpagesize()
	size := (n + page - 1) / page * page
	if size == 0 {
		size = page
	}
	b, err := syscall.Mmap(-1, 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	lb := &lockedBuf{mem: b, b: b[:n]}
	if !forceNoLock {
		lb.locked = mlock(b) == nil
	}
	lb.noDump = excludeFromDumps(b) == nil
	return lb, nil
}

// bytes is the usable part of the buffer.
func (l *lockedBuf) bytes() []byte { return l.b }

// release wipes the memory, unlocks it and gives it back.
func (l *lockedBuf) release() {
	if l == nil || l.released {
		return
	}
	wipe(l.mem)
	if l.locked {
		_ = munlock(l.mem)
	}
	_ = syscall.Munmap(l.mem)
	l.mem, l.b, l.released = nil, nil, true
}

// wipe overwrites b with zeros.
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// disableCoreDumps sets this process's core-file size limit to zero, the
// portable way to keep its memory out of crash dumps.
func disableCoreDumps() error {
	return syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{})
}
