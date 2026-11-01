package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The package description is read from dpkg's files: the package that
// installs the program, its summary and description, and how many
// programs it installs. A package of many programs gives no text.
func TestPackageTextFromDpkg(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(filepath.Join(dir, "dpkg", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"viewer9", "util9a"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, "dpkg", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("info/viewer9.list", bin+"/viewer9\n/usr/share/doc/viewer9\n")
	many := ""
	for _, n := range []string{"util9a", "util9b", "util9c", "util9d", "util9e", "util9f", "util9g", "util9h", "util9i", "util9j", "util9k"} {
		many += bin + "/" + n + "\n"
	}
	write("info/utils9:amd64.list", many)
	write("status", "Package: viewer9\nStatus: install ok installed\nDescription: interactive processes viewer\n Shows the processes.\n .\n More text.\n\nPackage: utils9\nDescription: assorted utilities\n Many tools.\n")
	saved, savedPath := dpkgDir, os.Getenv("PATH")
	t.Cleanup(func() { dpkgDir = saved; pkgOnce = sync.Once{}; os.Setenv("PATH", savedPath) })
	dpkgDir, pkgOnce = filepath.Join(dir, "dpkg"), sync.Once{}
	t.Setenv("PATH", bin)
	if s, d := packageText("viewer9"); s != "interactive processes viewer" || d != "Shows the processes. More text." {
		t.Errorf("viewer9: %q %q", s, d)
	}
	if s, _ := packageText("util9a"); s != "" {
		t.Errorf("a package of many programs gave %q", s)
	}
	if s, _ := packageText("nothere9"); s != "" {
		t.Errorf("a program not installed gave %q", s)
	}
}
