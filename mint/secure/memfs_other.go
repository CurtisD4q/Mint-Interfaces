//go:build !linux

package secure

// isMemoryFS cannot tell on this system, so it says no.
func isMemoryFS(string) bool { return false }
