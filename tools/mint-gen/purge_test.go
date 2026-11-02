package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeScaffold writes the files mint-gen writes for tool in dir.
func makeScaffold(t *testing.T, dir, tool string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range scaffoldFiles(tool) {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPurgeAsksAndRemovesOnlyMintGenApps(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "apps")
	makeScaffold(t, filepath.Join(apps, "nmap"), "nmap")
	makeScaffold(t, filepath.Join(apps, "ps"), "ps")
	// A file of the user's inside an app folder survives, and so the folder.
	os.WriteFile(filepath.Join(apps, "ps", "notes.txt"), []byte("mine"), 0o644)
	// A folder mint-gen did not create.
	os.MkdirAll(filepath.Join(apps, "mine"), 0o755)
	os.WriteFile(filepath.Join(apps, "mine", "main.go"), []byte("package main"), 0o644)
	// A link to an app somewhere else is not followed.
	elsewhere := filepath.Join(t.TempDir(), "df")
	makeScaffold(t, elsewhere, "df")
	if err := os.Symlink(elsewhere, filepath.Join(apps, "df")); err != nil {
		t.Fatal(err)
	}

	purge := func(answer string) (int, string) {
		var out, errOut bytes.Buffer
		code := purgeAllApps(strings.NewReader(answer), &out, &errOut, root)
		return code, out.String() + errOut.String()
	}
	exists := func(p string) bool { _, err := os.Lstat(p); return err == nil }

	// Anything but y or yes removes nothing, and so does no answer.
	for _, answer := range []string{"n\n", "\n", "", "maybe\n", "yy\n"} {
		code, out := purge(answer)
		if code == 0 || !strings.Contains(out, "Nothing removed.") || !exists(filepath.Join(apps, "nmap", "spec.go")) {
			t.Fatalf("answer %q: code %d, removed something:\n%s", answer, code, out)
		}
	}
	code, out := purge("n\n")
	_ = code
	for _, want := range []string{"nmap", "ps", "(y/n)", "Left alone (mint-gen did not create them): df, mine"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the question does not show %q:\n%s", want, out)
		}
	}

	if code, out := purge("Yes\n"); code != 0 {
		t.Fatalf("yes: code %d\n%s", code, out)
	}
	if exists(filepath.Join(apps, "nmap")) {
		t.Fatal("nmap was not removed")
	}
	if exists(filepath.Join(apps, "ps", "spec.go")) || !exists(filepath.Join(apps, "ps", "notes.txt")) {
		t.Fatal("ps: generated files should go and the user's file stay")
	}
	if !exists(filepath.Join(apps, "mine", "main.go")) {
		t.Fatal("a folder mint-gen did not create was touched")
	}
	if !exists(filepath.Join(elsewhere, "spec.go")) || !exists(filepath.Join(apps, "df")) {
		t.Fatal("a linked app was followed")
	}

	// With nothing left to remove it says so and does not ask.
	if code, out := purge(""); code != 0 || strings.Contains(out, "(y/n)") {
		t.Fatalf("nothing to remove: code %d\n%s", code, out)
	}
	// No apps folder at all.
	var o, e bytes.Buffer
	if code := purgeAllApps(strings.NewReader(""), &o, &e, t.TempDir()); code != 0 || !strings.Contains(o.String(), "No apps to remove") {
		t.Fatalf("no apps folder: code %d %q", code, o.String())
	}
}
