package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fakeRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/CurtisD4q/Mint-Interfaces\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// `go -C <repo> run` starts mint-gen in the repository; it goes back to
// the caller's folder ($PWD) and remembers the repository.
func TestCallerDir(t *testing.T) {
	repo := fakeRepo(t, filepath.Join(t.TempDir(), "repo"))
	caller := t.TempDir()
	oldE, oldW, oldC := executable, getwd, chdir
	defer func() { executable, getwd, chdir, launchRoot = oldE, oldW, oldC, "" }()
	var moved string
	chdir = func(d string) error { moved = d; return nil }
	getwd = func() (string, error) { return repo, nil }

	for _, tc := range []struct {
		name, exe, pwd, want string
	}{
		{"go run", "/tmp/go-build123/b001/exe/mint-gen", caller, caller},
		{"PWD is the repo", "/tmp/go-build123/b001/exe/mint-gen", repo, ""},
		{"built binary", "/home/u/.local/bin/mint-gen", caller, ""},
		{"test binary", "/tmp/go-build1/b001/mint-gen.test", caller, ""},
		{"PWD missing", "/tmp/go-build123/b001/exe/mint-gen", "/no/such/dir", ""},
		{"PWD relative", "/tmp/go-build123/b001/exe/mint-gen", "x", ""},
	} {
		moved, launchRoot = "", ""
		exe := tc.exe
		executable = func() (string, error) { return exe, nil }
		t.Setenv("PWD", tc.pwd)
		callerDir()
		if moved != tc.want {
			t.Errorf("%s: moved to %q, want %q", tc.name, moved, tc.want)
		}
		if (tc.want != "") != (launchRoot == repo) {
			t.Errorf("%s: launchRoot %q", tc.name, launchRoot)
		}
	}

	// Not in the repository: stays.
	getwd = func() (string, error) { return caller, nil }
	executable = func() (string, error) { return "/tmp/go-build1/b001/exe/mint-gen", nil }
	moved = ""
	t.Setenv("PWD", t.TempDir())
	callerDir()
	if moved != "" {
		t.Fatal("moved when not started in the repository")
	}
}

// The repository is found in the launcher's repository, then in
// ~/.local/share/mint-interfaces ($XDG_DATA_HOME).
func TestDetectUserInstall(t *testing.T) {
	elsewhere := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	oldRoot := installedRoot
	installedRoot = filepath.Join(elsewhere, "none")
	defer func() { installedRoot, launchRoot = oldRoot, "" }()
	t.Setenv("MINT_INTERFACES_ROOT", "")

	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	if got := detectRepoRoot(); got != "" && !strings.Contains(got, "mint-interfaces") {
		t.Fatalf("found %q with nothing installed", got)
	}
	user := fakeRepo(t, filepath.Join(data, "mint-interfaces"))
	if got := detectRepoRoot(); got != user {
		t.Fatalf("user install: got %q, want %q", got, user)
	}
	launched := fakeRepo(t, filepath.Join(t.TempDir(), "clone"))
	launchRoot = launched
	if got := detectRepoRoot(); got != launched {
		t.Fatalf("launcher repo: got %q, want %q", got, launched)
	}

	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/someone")
	if got := userInstallRoot(); got != "/home/someone/.local/share/mint-interfaces" {
		t.Fatalf("default user root %q", got)
	}
	if got := userLauncherDir(); got != "/home/someone/.local/bin/mint-interfaces" {
		t.Fatalf("launcher dir %q", got)
	}
}

func TestInstallUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	oldEuid := genEuid
	genEuid = func() int { return 1000 }
	defer func() { genEuid = oldEuid }()
	repo := fakeRepo(t, filepath.Join(home, "src", "it's mint"))

	var out, errOut bytes.Buffer
	if code := installUser(&out, &errOut, repo); code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	path := filepath.Join(home, ".local", "bin", "mint-interfaces", "mint-gen")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	if !strings.Contains(script, launcherMark) || !strings.Contains(script, `-C '`+strings.ReplaceAll(repo, "'", `'\''`)+`' run ./tools/mint-gen "$@"`) {
		t.Fatalf("launcher:\n%s", script)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if !strings.Contains(out.String(), `export PATH="$HOME/.local/bin/mint-interfaces:$PATH"`) {
		t.Fatalf("no PATH hint:\n%s", out.String())
	}
	manPath := filepath.Join(home, ".local", "share", "man", "man1", "mint-interfaces.1")
	page, err := os.ReadFile(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(page) != manPage(newMintOpts().fs) {
		t.Fatal("installed manual is not what mint-gen --man prints")
	}
	if fi, _ := os.Stat(manPath); fi.Mode().Perm() != 0o644 {
		t.Fatalf("manual mode %v", fi.Mode().Perm())
	}
	if !strings.Contains(out.String(), "man mint-interfaces") || !strings.Contains(out.String(), "MANPATH") {
		t.Fatalf("no manual note:\n%s", out.String())
	}
	// A link is not replaced, and is not followed.
	if err := os.Remove(manPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp/not-the-manual", manPath); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	if code := installUser(&out, &errOut, repo); code == 0 || !strings.Contains(errOut.String(), "not a file") {
		t.Fatalf("replaced a symlink: %d %s", code, errOut.String())
	}
	if dest, _ := os.Readlink(manPath); dest != "/tmp/not-the-manual" {
		t.Fatalf("symlink now %q", dest)
	}
	if err := os.Remove(manPath); err != nil {
		t.Fatal(err)
	}
	// The shell reads the quoted path back as it was.
	if sh, err := exec.LookPath("sh"); err == nil {
		cmd := exec.Command(sh, "-n", path)
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("launcher does not parse: %v %s", err, o)
		}
	}

	// Its own launcher is replaced; anything else is left alone.
	out.Reset()
	t.Setenv("PATH", filepath.Dir(path)+":/usr/bin")
	if code := installUser(&out, &errOut, repo); code != 0 || strings.Contains(out.String(), "not on your PATH") {
		t.Fatalf("reinstall: %d %s", code, out.String())
	}
	if again, err := os.ReadFile(manPath); err != nil || string(again) != manPage(newMintOpts().fs) {
		t.Fatalf("reinstall left the manual missing: %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	if code := installUser(&out, &errOut, repo); code == 0 || !strings.Contains(errOut.String(), "did not write it") {
		t.Fatalf("overwrote a file it did not write: %d %s", code, errOut.String())
	}
	if b, _ := os.ReadFile(path); string(b) != "#!/bin/sh\necho mine\n" {
		t.Fatal("foreign file changed")
	}

	// Not under sudo, and not without a repository.
	genEuid = func() int { return 0 }
	if code := installUser(&out, &errOut, repo); code == 0 {
		t.Fatal("installed as root")
	}
	genEuid = func() int { return 1000 }
	if code := installUser(&out, &errOut, ""); code == 0 {
		t.Fatal("installed with no repository")
	}
}

func TestOthersCanSwap(t *testing.T) {
	// A file in a temporary folder (world-writable /tmp, or a folder
	// owned by the user) can be swapped.
	f := filepath.Join(t.TempDir(), "x-mint")
	if err := os.WriteFile(f, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if !othersCanSwap(f) {
		t.Fatal("temp file not seen as swappable")
	}
	// A root-owned program in root-owned folders cannot.
	if os.Geteuid() == 0 {
		if p, err := exec.LookPath("sh"); err == nil {
			if real, err := filepath.EvalSymlinks(p); err == nil && !othersCanSwap(real) {
				return
			}
			t.Skip("sh is not root-owned here")
		}
	}
}

// When anyone but root could replace the app's program, sudo is not
// offered; --sudo still chooses it, with a note.
func TestNoSudoOfferForSwappableApp(t *testing.T) {
	oldEuid, oldTTY, oldIn, oldOut, oldSwap := genEuid, stdinIsTTY, promptIn, promptOut, othersCanSwap
	defer func() {
		genEuid, stdinIsTTY, promptIn, promptOut, othersCanSwap = oldEuid, oldTTY, oldIn, oldOut, oldSwap
	}()
	genEuid = func() int { return 1000 }
	stdinIsTTY = func() bool { return true }
	othersCanSwap = func(string) bool { return true }
	var out bytes.Buffer
	promptOut = &out
	promptIn = strings.NewReader("y\n")
	if startAsRoot("tcpdump", nil, false, false, "/home/u/apps/tcpdump/tcpdump-mint") {
		t.Fatal("offered sudo for a program others could swap")
	}
	if !strings.Contains(out.String(), "--sudo") || strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("message: %q", out.String())
	}
	out.Reset()
	if !startAsRoot("tcpdump", nil, true, false, "/home/u/apps/tcpdump/tcpdump-mint") || !strings.Contains(out.String(), "Note:") {
		t.Fatalf("--sudo: %q", out.String())
	}
	othersCanSwap = func(string) bool { return false }
	out.Reset()
	promptIn = strings.NewReader("y\n")
	if !startAsRoot("tcpdump", nil, false, false, "/usr/local/share/mint-interfaces/apps/tcpdump/tcpdump-mint") || !strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("root-owned app: %q", out.String())
	}
}
