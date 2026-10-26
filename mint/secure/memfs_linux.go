//go:build linux

package secure

import "syscall"

const (
	tmpfsMagic = 0x01021994
	ramfsMagic = 0x858458f6
)

// isMemoryFS reports whether path is on tmpfs or ramfs.
func isMemoryFS(path string) bool {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return false
	}
	return uint32(st.Type) == tmpfsMagic || uint32(st.Type) == ramfsMagic
}
