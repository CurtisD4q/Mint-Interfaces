//go:build !linux

package secure

import "errors"

var errNoKeyring = errors.New("no kernel keyring on this system")

// Other systems have no kernel keyring: the store keeps its key in locked
// memory instead (see Store).
func keyringAdd(string, []byte) (int, error) { return 0, errNoKeyring }
func keyringRead(int, []byte) (int, error)   { return 0, errNoKeyring }
func keyringTimeout(int, uint) error         { return errNoKeyring }
func keyringRevoke(int)                      {}
func keyringAlive(int) bool                  { return false }
func keyringAvailable() bool                 { return false }
func excludeFromDumps([]byte) error          { return errors.New("not supported") }
func setNotDumpable() error                  { return nil }
func isDumpable() bool                       { return true }
