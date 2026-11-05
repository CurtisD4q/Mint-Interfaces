package appkit

import (
	"os"
	"path/filepath"
	"testing"
)

func newInHome(t *testing.T, home, name string) *ToolApp {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	return New(ToolSpec{Name: name, Bin: "/bin/true", DefaultResultsDir: t.TempDir()})
}

func TestDataDirIsUnderInterfacesApps(t *testing.T) {
	home := t.TempDir()
	a := newInHome(t, home, "nmap")
	want := filepath.Join(home, ".local", "share", "mint-interfaces-apps", "nmap")
	if a.DataDir != want {
		t.Fatalf("DataDir %q, want %q", a.DataDir, want)
	}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "../x"} {
		got := dataDirFor(home, name)
		parent := filepath.Join(home, ".local", "share", "mint-interfaces-apps")
		if filepath.Dir(got) != parent {
			t.Errorf("%q escapes %s: %s", name, parent, got)
		}
	}
}

// Earlier versions kept the files in ~/.local/share/mint-<tool>. The first
// run moves the ones it knows, once, and never overwrites or follows links.
func TestOldDataIsMoved(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".local", "share", "mint-nmap")
	if err := os.MkdirAll(filepath.Join(old, "rule-backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, s string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(old, "runs.json"), "[]", 0o600)
	write(filepath.Join(old, "presets.json"), `{"a":1}`, 0o600)
	write(filepath.Join(old, "rule-backups", "b1"), "rules", 0o600)

	a := newInHome(t, home, "nmap")
	for name, want := range map[string]string{"runs.json": "[]", "presets.json": `{"a":1}`, "rule-backups/b1": "rules"} {
		b, err := os.ReadFile(filepath.Join(a.DataDir, name))
		if err != nil || string(b) != want {
			t.Errorf("%s: %q %v", name, b, err)
		}
	}
	if fi, _ := os.Stat(filepath.Join(a.DataDir, "runs.json")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode changed: %v", fi.Mode().Perm())
	}
	if _, err := os.Lstat(old); err == nil {
		t.Error("empty old folder kept")
	}

	// Second run: nothing to do, nothing lost.
	a = newInHome(t, home, "nmap")
	if b, _ := os.ReadFile(filepath.Join(a.DataDir, "runs.json")); string(b) != "[]" {
		t.Errorf("second run: %q", b)
	}
}

func TestOldDataMigrationIsCareful(t *testing.T) {
	home := t.TempDir()
	share := filepath.Join(home, ".local", "share")

	// A tool called "interfaces" has the repository's folder as its old
	// folder. Nothing in it moves, and the folder stays.
	repo := filepath.Join(share, "mint-interfaces")
	if err := os.MkdirAll(filepath.Join(repo, "apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi"), 0o644)
	newInHome(t, home, "interfaces")
	for _, f := range []string{"go.mod", "README.md", "apps"} {
		if _, err := os.Lstat(filepath.Join(repo, f)); err != nil {
			t.Fatalf("repository %s touched: %v", f, err)
		}
	}

	// A new file is not overwritten; a file it does not know stays, so
	// the old folder stays; a link is neither moved nor followed.
	old := filepath.Join(share, "mint-curl")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "runs.json"), []byte("OLD"), 0o600)
	os.WriteFile(filepath.Join(old, "notes.txt"), []byte("mine"), 0o600)
	secret := filepath.Join(home, "secret.json")
	os.WriteFile(secret, []byte("secret"), 0o600)
	os.Symlink(secret, filepath.Join(old, "presets.json"))
	newDir := dataDirFor(home, "curl")
	os.MkdirAll(newDir, 0o755)
	os.WriteFile(filepath.Join(newDir, "runs.json"), []byte("NEW"), 0o600)

	newInHome(t, home, "curl")
	if b, _ := os.ReadFile(filepath.Join(newDir, "runs.json")); string(b) != "NEW" {
		t.Errorf("new history overwritten: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(old, "runs.json")); string(b) != "OLD" {
		t.Errorf("old history lost: %q", b)
	}
	if _, err := os.Lstat(filepath.Join(old, "notes.txt")); err != nil {
		t.Error("unknown file moved or old folder removed")
	}
	if _, err := os.Lstat(filepath.Join(newDir, "presets.json")); err == nil {
		t.Error("a link was moved")
	}
	if b, _ := os.ReadFile(secret); string(b) != "secret" {
		t.Error("link target changed")
	}

	// An old folder that is itself a link is left alone.
	target := filepath.Join(home, "elsewhere")
	os.MkdirAll(target, 0o755)
	os.WriteFile(filepath.Join(target, "runs.json"), []byte("X"), 0o600)
	os.Symlink(target, filepath.Join(share, "mint-ping"))
	newInHome(t, home, "ping")
	if _, err := os.Lstat(filepath.Join(target, "runs.json")); err != nil {
		t.Error("followed a linked old folder")
	}
}

func TestConfigDirIsUnderInterfaces(t *testing.T) {
	home := t.TempDir()
	a := newInHome(t, home, "nmap")
	if want := filepath.Join(home, ".config", "mint-interfaces", "nmap"); a.ConfigDir != want {
		t.Fatalf("ConfigDir %q, want %q", a.ConfigDir, want)
	}
	shared := filepath.Join(home, ".config", "mint-interfaces")
	// Never a name the shared folder uses itself, and never outside it.
	for _, name := range []string{"theme.conf", "termcaps.json", "security.json", "tools.d", "", "..", "a/b"} {
		got := configDirFor(home, name)
		if filepath.Dir(got) != shared {
			t.Errorf("%q escapes %s: %s", name, shared, got)
		}
		if sharedConfigNames[filepath.Base(got)] {
			t.Errorf("%q gets the shared name %s", name, filepath.Base(got))
		}
	}
}

// Saved settings follow an app to the new folder: the old file moves once,
// and what was typed in is there when the app starts.
func TestOldSettingsAreMoved(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".config", "mint-tree")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte(`{"extra_args":"/home/u/Downloads/Test"}`), 0o600)
	os.WriteFile(filepath.Join(old, "other.txt"), []byte("mine"), 0o600)

	a := newInHome(t, home, "tree")
	if a.ExtraArgs != "/home/u/Downloads/Test" {
		t.Fatalf("extra arguments %q not restored", a.ExtraArgs)
	}
	if _, err := os.Stat(filepath.Join(a.ConfigDir, "settings.json")); err != nil {
		t.Fatal("settings not in the new folder")
	}
	if fi, _ := os.Stat(filepath.Join(a.ConfigDir, "settings.json")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if _, err := os.Lstat(filepath.Join(old, "other.txt")); err != nil {
		t.Fatal("a file that is not settings was moved, or the old folder removed")
	}
}

// A tool named "interfaces" has the shared settings folder as its old
// folder: the theme and the terminal cache in it stay where they are.
func TestSharedConfigFolderIsNotMigrated(t *testing.T) {
	home := t.TempDir()
	shared := filepath.Join(home, ".config", "mint-interfaces")
	os.MkdirAll(filepath.Join(shared, "tools.d"), 0o755)
	os.WriteFile(filepath.Join(shared, "theme.conf"), []byte("theme = nord\n"), 0o644)
	os.WriteFile(filepath.Join(shared, "termcaps.json"), []byte("{}"), 0o644)
	a := newInHome(t, home, "interfaces")
	if a.ConfigDir != filepath.Join(shared, "interfaces") {
		t.Fatalf("ConfigDir %q", a.ConfigDir)
	}
	for _, f := range []string{"theme.conf", "termcaps.json", "tools.d"} {
		if _, err := os.Lstat(filepath.Join(shared, f)); err != nil {
			t.Fatalf("%s touched: %v", f, err)
		}
	}
}

// Under sudo, root writes the settings into the user's home and hands
// the folders back, including the shared one it may have created.
func TestSettingsHandedBackUnderSudo(t *testing.T) {
	home := t.TempDir()
	a := newInHome(t, home, "tree")
	a.owner = &invoker{uid: 1000, gid: 1000, home: home}
	var got []string
	old := chown
	chown = func(p string, uid, gid int) error { got = append(got, p); return nil }
	defer func() { chown = old }()
	if err := a.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		filepath.Join(a.ConfigDir, "settings.json"),
		a.ConfigDir,
		filepath.Join(home, ".config", "mint-interfaces"),
		filepath.Join(home, ".config"),
	} {
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("%s not handed back (got %q)", want, got)
		}
	}
}
