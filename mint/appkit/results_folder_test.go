package appkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResultsGoInTheAppsOwnFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	appDir := filepath.Join(t.TempDir(), "apps", "mytool")
	os.MkdirAll(appDir, 0o755)
	for _, f := range []string{"go.mod", "main.go", "spec.go", ".mint-gen", "mytool-mint", "mytool-mint.el"} {
		os.WriteFile(filepath.Join(appDir, f), []byte("x"), 0o644)
	}

	a := New(ToolSpec{Name: "mytool", Bin: "echo", DefaultResultsDir: appDir})
	defer a.Close()
	if a.ResultsDir != appDir {
		t.Fatalf("results folder %q, want the app's own %q", a.ResultsDir, appDir)
	}
	// The app's own files are not listed as results; a result is.
	os.WriteFile(filepath.Join(appDir, "mytool-20261104.txt"), []byte("out"), 0o644)
	entries, err := a.ListResultsDir()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "mytool-20261104.txt" {
		t.Fatalf("listed %+v, want only the result", entries)
	}

	// An app whose folder is gone uses ~/mint-results/<tool>.
	gone := New(ToolSpec{Name: "mytool", Bin: "echo", DefaultResultsDir: filepath.Join(home, "moved", "apps", "mytool")})
	defer gone.Close()
	if want := filepath.Join(home, "mint-results", "mytool"); gone.ResultsDir != want {
		t.Fatalf("moved app: results folder %q, want %q", gone.ResultsDir, want)
	}
}

// Settings saved when the default was ~/mint-results/<tool> move to the
// app's own folder; a folder the user chose stays.
func TestOldDefaultResultsFolderMoves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	appDir := filepath.Join(t.TempDir(), "apps", "mytool")
	os.MkdirAll(appDir, 0o755)

	old := New(ToolSpec{Name: "mytool", Bin: "echo"}) // no folder of its own
	old.ResultsDir = filepath.Join(home, "mint-results", "mytool")
	if err := old.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	old.Close()
	a := New(ToolSpec{Name: "mytool", Bin: "echo", DefaultResultsDir: appDir})
	if a.ResultsDir != appDir {
		t.Fatalf("old default kept: %q", a.ResultsDir)
	}
	chosen := filepath.Join(home, "Documents", "scans")
	a.ResultsDir = chosen
	a.SaveSettings()
	a.Close()
	b := New(ToolSpec{Name: "mytool", Bin: "echo", DefaultResultsDir: appDir})
	defer b.Close()
	if b.ResultsDir != chosen {
		t.Fatalf("the user's own folder was replaced: %q", b.ResultsDir)
	}
}

// Sensitive results saved in the app's own folder: the folder is made
// private, as a new results folder would be.
func TestSensitiveResultsMakeTheAppsFolderPrivate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	appDir := filepath.Join(t.TempDir(), "apps", "tcpdump")
	os.MkdirAll(appDir, 0o755)
	a := New(ToolSpec{Name: "tcpdump", Bin: "echo", Data: DataSensitive, DefaultResultsDir: appDir})
	defer a.Close()
	if err := a.makeResultsDir(a.ResultsDir); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(appDir); fi.Mode().Perm() != 0o700 {
		t.Fatalf("app folder is %v, want 0700 for sensitive results", fi.Mode().Perm())
	}
	// A folder the user chose is left as it is.
	mine := filepath.Join(t.TempDir(), "shared")
	os.MkdirAll(mine, 0o755)
	if err := a.makeResultsDir(mine); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(mine); fi.Mode().Perm() != 0o755 {
		t.Fatalf("the user's folder was changed to %v", fi.Mode().Perm())
	}
}
