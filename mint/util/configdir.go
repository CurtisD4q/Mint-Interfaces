package util

import (
	"os"
	"os/user"
	"path/filepath"
)

// Seams for tests.
var (
	geteuid      = os.Geteuid
	lookupRootID = func() (string, error) {
		u, err := user.LookupId("0")
		if err != nil {
			return "", err
		}
		return u.HomeDir, nil
	}
)

// UnderSudo reports whether the program runs as root, started by another
// user through sudo.
func UnderSudo() bool {
	if geteuid() != 0 {
		return false
	}
	name := os.Getenv("SUDO_USER")
	return name != "" && name != "root"
}

// ConfigDir is the folder Mint keeps its own settings in
// (…/.config/mint-interfaces): under $XDG_CONFIG_HOME, else ~/.config.
//
// Under sudo it is always root's own (/root/.config/mint-interfaces),
// whatever HOME and XDG_CONFIG_HOME say: sudo often keeps them pointing at
// the user's home, and a file root wrote there would belong to root, in
// the user's own folder.
func ConfigDir() string {
	if UnderSudo() {
		home, err := lookupRootID()
		if err != nil || home == "" || !filepath.IsAbs(home) {
			home = "/root"
		}
		return filepath.Join(home, ".config", "mint-interfaces")
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mint-interfaces")
}
