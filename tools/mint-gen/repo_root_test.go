package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestFindsTheMintInterfacesFolder(t *testing.T) {
	repo, _ := filepath.Abs("../..")
	if !isRepoRoot(repo) {
		t.Fatalf("%s is not recognised as the Mint-Interfaces folder", repo)
	}
	if got := rootAbove(filepath.Join(repo, "tools", "mint-gen")); got != repo {
		t.Fatalf("rootAbove from tools/mint-gen = %q", got)
	}
	// An app's own go.mod (module …/apps/ps) is not the root.
	app := t.TempDir()
	os.WriteFile(filepath.Join(app, "go.mod"), []byte("module github.com/CurtisD4q/Mint-Interfaces/apps/ps\n"), 0o644)
	if isRepoRoot(app) {
		t.Fatal("an app's go.mod was taken for the root")
	}
}

// Run from a folder outside Mint-Interfaces, with no install to fall back
// on, mint-gen stops rather than writing apps/ into that folder.
func TestNoAppsWrittenOutsideMintInterfaces(t *testing.T) {
	away := t.TempDir()
	oldWD, _ := os.Getwd()
	oldInstalled := installedRoot
	t.Cleanup(func() { os.Chdir(oldWD); installedRoot = oldInstalled })
	installedRoot = filepath.Join(away, "not-installed")
	if err := os.Chdir(away); err != nil {
		t.Fatal(err)
	}
	// The test binary is not inside the repository either.
	if exe, _ := os.Executable(); rootAbove(filepath.Dir(exe)) != "" {
		t.Skip("the test binary is inside the repository")
	}
	for _, args := range [][]string{{"ls"}, {"--list"}, {"--purge"}, {"--remove", "ls"}} {
		if code := runCLI(args); code != 2 {
			t.Errorf("mint-gen %v outside Mint-Interfaces: exit %d, want 2", args, code)
		}
	}
	if _, err := os.Stat(filepath.Join(away, "apps")); err == nil {
		t.Fatal("mint-gen wrote apps/ into the folder it was run from")
	}
	// --root must be the real folder.
	if code := runCLI([]string{"--root", away, "ls"}); code != 2 {
		t.Errorf("--root to a folder that is not Mint-Interfaces: exit %d", code)
	}
}

// A scaffolded app saves its output in its own folder.
func TestScaffoldedAppKeepsResultsInItsFolder(t *testing.T) {
	repo, _ := filepath.Abs("../..")
	out := filepath.Join(t.TempDir(), "apps", "ls")
	res, err := ScaffoldApp(appkit.ToolSpec{Name: "ls", Bin: "ls"}, out, repo)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := os.ReadFile(filepath.Join(res.AppDir, "spec.go"))
	if !strings.Contains(string(src), `DefaultResultsDir: "`+res.AppDir+`",`) {
		t.Fatalf("spec.go does not keep results in %s:\n%s", res.AppDir, src)
	}
}

func TestRootFromTheEnvironment(t *testing.T) {
	repo, _ := filepath.Abs("../..")
	away := t.TempDir()
	oldWD, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldWD) })
	os.Chdir(away)
	t.Setenv("MINT_INTERFACES_ROOT", repo)
	if got := detectRepoRoot(); got != repo {
		t.Fatalf("detectRepoRoot = %q, want %q from MINT_INTERFACES_ROOT", got, repo)
	}
	t.Setenv("MINT_INTERFACES_ROOT", away) // not the folder: ignored
	if got := detectRepoRoot(); got == away {
		t.Fatal("a folder that is not Mint-Interfaces was used")
	}
}
