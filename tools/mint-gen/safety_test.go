package main

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Problem 3: --remove must never delete a folder mint-gen did not create.
func TestRemoveRefusesFoldersMintGenDidNotCreate(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "go.mod"), "module example.com/mine\n")
	writeFile(t, filepath.Join(project, "main.go"), "package main\nfunc main() {}\n")
	if code := runCLI([]string{"--remove", "anything", "--out", project}); code == 0 {
		t.Fatal("--remove succeeded on a project mint-gen did not create")
	}
	for _, name := range []string{"go.mod", "main.go"} {
		if _, err := os.Stat(filepath.Join(project, name)); err != nil {
			t.Fatalf("%s was deleted", name)
		}
	}
}

func TestRemoveDeletesOnlyGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "apps", "demo")
	spec := appkit.ToolSpec{Name: "demo", Title: "Demo", Desc: "demo"}
	if _, err := ScaffoldApp(spec, out, root); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(out, "my-notes.txt"), "keep me")
	if code := runCLI([]string{"--remove", "--out", out, "demo"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(out, "my-notes.txt")); err != nil {
		t.Fatal("a file mint-gen did not create was deleted")
	}
	if _, err := os.Stat(filepath.Join(out, "spec.go")); !os.IsNotExist(err) {
		t.Fatal("generated spec.go was not removed")
	}
	os.Remove(filepath.Join(out, "my-notes.txt"))
	if _, err := ScaffoldApp(spec, out, root); err != nil {
		t.Fatal(err)
	}
	if code := runCLI([]string{"--remove", "--out", out, "demo"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("an app folder holding only generated files should be removed")
	}
}

func TestScaffoldWillNotOverwriteAProject(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "main.go"), "package main // mine\n")
	if _, err := ScaffoldApp(appkit.ToolSpec{Name: "demo"}, project, ""); err == nil {
		t.Fatal("scaffolded over an existing project")
	}
	if b, _ := os.ReadFile(filepath.Join(project, "main.go")); !strings.Contains(string(b), "mine") {
		t.Fatal("main.go was overwritten")
	}
}

func TestFlagsAfterTheProgramName(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.Bool("remove", false, "")
	fs.Bool("run", false, "")
	fs.String("out", "", "")
	got := flagsFirst(fs, []string{"--remove", "nmap", "--out", "dir"})
	if want := []string{"--remove", "--out", "dir", "nmap"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	// Arguments for the app itself are left after the program name.
	got = flagsFirst(fs, []string{"--run", "nmap", "--screen", "History"})
	if want := []string{"--run", "nmap", "--screen", "History"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Problem 4: password and token options become secrets.
func TestSecretOptionsAreDetected(t *testing.T) {
	help := `Usage: login [options] host
Options:
  -u, --user NAME          user name
  -p, --password PASS      password to log in with
      --password-stdin     read the password from standard input
      --password-file FILE read the password from FILE
      --token TOKEN        API token
`
	spec := ParseManAndHelp("login", "", help)
	kinds := map[string]appkit.FlagSpec{}
	for _, f := range spec.Flags {
		kinds[f.Flag] = f
	}
	if f := kinds["-p"]; f.Kind != appkit.FlagSecret || f.SecretStdin {
		t.Errorf("-p: %+v", f)
	}
	if f := kinds["--password-stdin"]; f.Kind != appkit.FlagSecret || !f.SecretStdin {
		t.Errorf("--password-stdin: %+v", f)
	}
	if f := kinds["--password-file"]; f.Kind == appkit.FlagSecret {
		t.Errorf("--password-file is a path, not a secret: %+v", f)
	}
	if f := kinds["--token"]; f.Kind != appkit.FlagSecret {
		t.Errorf("--token: %+v", f)
	}
	if f := kinds["-u"]; f.Kind == appkit.FlagSecret {
		t.Errorf("-u is not secret: %+v", f)
	}
	src, err := renderSpecGo(spec)
	if err != nil || !strings.Contains(string(src), "appkit.FlagSecret") || !strings.Contains(string(src), "SecretStdin: true") {
		t.Fatalf("spec.go does not keep the secret kinds (%v):\n%s", err, src)
	}
}
