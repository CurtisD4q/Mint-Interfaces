package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A program name is never run as shell syntax.
func TestShellTypeIsNotInjectable(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "pwned")
	for _, name := range []string{
		"x; touch " + marker,
		"x$(touch " + marker + ")",
		"x`touch " + marker + "`",
		"x && touch " + marker,
		"x' ; touch " + marker + " #",
		"-x",
		"",
		"x;touch${IFS}zz_pwned_marker", // no "/", so filepath.Base keeps it whole; ran on main
	} {
		if _, err := shellType(name); err == nil {
			t.Errorf("shellType(%q) accepted a name that isn't a program", name)
		}
		_ = CheckCompatibility(name)
	}
	if _, err := os.Stat("zz_pwned_marker"); err == nil {
		os.Remove("zz_pwned_marker")
		t.Fatal("a program name was run as a shell command")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a program name was run as a shell command")
	}
	out, err := shellType("cd")
	if err != nil || !strings.Contains(string(out), "builtin") {
		t.Fatalf("shellType(cd) = %q, %v", out, err)
	}
}
