package util

import (
	"path/filepath"
	"testing"
)

// Under sudo, Mint's own settings stay in root's home even when sudo kept
// HOME and XDG_CONFIG_HOME pointing at the user's, so nothing root writes
// lands in the user's folder.
func TestConfigDirUnderSudo(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(user, ".config"))
	oldE, oldR := geteuid, lookupRootID
	defer func() { geteuid, lookupRootID = oldE, oldR }()
	lookupRootID = func() (string, error) { return "/root", nil }

	geteuid = func() int { return 1000 }
	t.Setenv("SUDO_USER", "")
	if got := ConfigDir(); got != filepath.Join(user, ".config", "mint-interfaces") {
		t.Fatalf("normal run: %s", got)
	}

	geteuid = func() int { return 0 }
	t.Setenv("SUDO_USER", "alice")
	if got := ConfigDir(); got != "/root/.config/mint-interfaces" {
		t.Fatalf("sudo run: %s", got)
	}

	// Plain root (no sudo) keeps its own HOME as before.
	t.Setenv("SUDO_USER", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/root")
	if got := ConfigDir(); got != "/root/.config/mint-interfaces" {
		t.Fatalf("root run: %s", got)
	}
}
