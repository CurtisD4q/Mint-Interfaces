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

func TestSafetyReportOutput(t *testing.T) {
	help := `Usage: login [options] host
Options:
  -p, --password PASS      password to log in with
      --password-stdin     read the password from standard input
      --force              overwrite the saved session without asking
`
	var b strings.Builder
	printSafetyReport(&b, ParseManAndHelp("login", "", help))
	out := b.String()
	for _, want := range []string{"Secrets:     2 options", "-p", "command line: visible to other users",
		"--password-stdin", "standard input: never visible", "Destructive: 1 option to review", "--force"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

func TestEnvironmentSecretsFromManPage(t *testing.T) {
	man := `NAME
       vault - store things

OPTIONS
       --password PASS
              password for the store
       --api-token TOKEN
              API token

ENVIRONMENT
       VAULT_PASSWORD
              Password for the store, instead of --password.
       VAULT_PASSWORD_FILE
              File holding the password.
       VAULT_HOME
              Where the store lives.
`
	help := "Usage: vault [options]\n\nThe token can also be given in the environment variable `VAULT_TOKEN`.\n"
	spec := ParseManAndHelp("vault", man, help)
	byEnv := map[string]appkit.FlagSpec{}
	for _, f := range spec.Flags {
		if f.SecretEnv != "" {
			byEnv[f.SecretEnv] = f
		}
	}
	if f, ok := byEnv["VAULT_PASSWORD"]; !ok || f.Flag != "--password" {
		t.Errorf("VAULT_PASSWORD should replace --password: %+v", byEnv)
	}
	if f, ok := byEnv["VAULT_TOKEN"]; !ok || f.Flag != "--api-token" {
		t.Errorf("VAULT_TOKEN should replace --api-token: %+v", byEnv)
	}
	for _, name := range []string{"VAULT_PASSWORD_FILE", "VAULT_HOME"} {
		if _, ok := byEnv[name]; ok {
			t.Errorf("%s is not a secret", name)
		}
	}
	var b strings.Builder
	printSafetyReport(&b, spec)
	if out := b.String(); !strings.Contains(out, "environment variable VAULT_PASSWORD (man page)") {
		t.Errorf("report:\n%s", out)
	}
	if src, _ := renderSpecGo(spec); !strings.Contains(string(src), `SecretEnv: "VAULT_PASSWORD"`) {
		t.Errorf("spec.go lost SecretEnv:\n%s", src)
	}
}

func TestStartAsRootQuestion(t *testing.T) {
	oldEuid, oldTTY, oldIn, oldOut := genEuid, stdinIsTTY, promptIn, promptOut
	defer func() { genEuid, stdinIsTTY, promptIn, promptOut = oldEuid, oldTTY, oldIn, oldOut }()
	genEuid = func() int { return 1000 }
	stdinIsTTY = func() bool { return true }
	var out strings.Builder
	promptOut = &out

	ask := func(answer, program string, root, noRoot bool) bool {
		promptIn = strings.NewReader(answer)
		out.Reset()
		return startAsRoot(program, nil, root, noRoot)
	}
	if !ask("y\n", "tcpdump", false, false) || !strings.Contains(out.String(), "tcpdump usually needs root access. Start it with sudo? [y/N]") {
		t.Fatalf("yes: %q", out.String())
	}
	if ask("\n", "tcpdump", false, false) {
		t.Fatal("the default answer must be no")
	}
	if ask("y\n", "curl", false, false) || out.String() != "" {
		t.Fatalf("curl does not need root, so no question: %q", out.String())
	}
	if !ask("", "curl", true, false) || ask("y\n", "tcpdump", false, true) {
		t.Fatal("--sudo and --no-sudo decide without asking")
	}
	stdinIsTTY = func() bool { return false }
	if ask("y\n", "tcpdump", false, false) || out.String() != "" {
		t.Fatal("no question without a terminal")
	}
	stdinIsTTY = func() bool { return true }
	genEuid = func() int { return 0 }
	if ask("y\n", "tcpdump", false, false) || out.String() != "" {
		t.Fatal("no question when already root")
	}
}

func TestLaunchThroughSudo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sudo"), "#!/bin/sh\n")
	os.Chmod(filepath.Join(dir, "sudo"), 0o755)
	t.Setenv("PATH", dir)
	cmd, err := launchCommand("/apps/tcpdump/tcpdump-mint", []string{"--screen", "Run"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(dir, "sudo"), "--", "/apps/tcpdump/tcpdump-mint", "--screen", "Run"}; strings.Join(cmd.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("args %q", cmd.Args)
	}
	if cmd, _ := launchCommand("/apps/x/x-mint", nil, false); cmd.Args[0] != "/apps/x/x-mint" {
		t.Fatalf("without root: %q", cmd.Args)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := launchCommand("/apps/x/x-mint", nil, true); err == nil {
		t.Fatal("missing sudo should be an error")
	}
}

func TestListApps(t *testing.T) {
	root := t.TempDir()
	var out, errOut strings.Builder
	if code := printApps(&out, &errOut, root); code != 0 || !strings.Contains(errOut.String(), "No apps yet") {
		t.Fatalf("no apps folder: %d %q", code, errOut.String())
	}
	for _, d := range []string{"tcpdump", "curl", ".hidden"} {
		os.MkdirAll(filepath.Join(root, "apps", d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "apps", "README"), nil, 0o644)
	out.Reset()
	errOut.Reset()
	if code := printApps(&out, &errOut, root); code != 0 || out.String() != "curl\ntcpdump\n" {
		t.Fatalf("got %q (%d)", out.String(), code)
	}
}

// The Emacs launcher uses mint.el's real API and stays a valid file.
func TestRenderElisp(t *testing.T) {
	el := renderElisp("nmap", strings.Repeat("Network exploration tool and security / port scanner. ", 3)+`with "quotes"`)
	for _, gone := range []string{"mint-start", "mint-register"} {
		if strings.Contains(el, gone) {
			t.Fatalf("uses %s, which mint.el does not define", gone)
		}
	}
	if !strings.Contains(el, "(mint-define-app nmap-mint \"nmap\" nmap-mint-binary") || !strings.Contains(el, "(ansi-term nmap-mint-binary") {
		t.Fatalf("launcher or fallback missing:\n%s", el)
	}
	if strings.Count(el, "(") != strings.Count(el, ")") {
		t.Fatal("unbalanced parentheses")
	}
	doc := elispDocDesc(strings.Repeat("word ", 40))
	for _, line := range strings.Split(doc, "\n") {
		if len(line) > 79 {
			t.Fatalf("docstring line too wide (%d): %q", len(line), line)
		}
	}
}
