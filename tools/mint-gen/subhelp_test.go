package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestCommandHelpForm(t *testing.T) {
	cases := []struct {
		name, text string
		want       string // the arguments for "run", or "" for none
	}{
		{"docker", "Run 'docker COMMAND --help' for more information on a command.", "run --help"},
		{"kubectl", `Use "kubectl <command> --help" for more information about a given command.`, "run --help"},
		{"tool", "See 'tool [subcommand] -h' for details", "run -h"},
		{"git", "See 'git help <command>' or 'git help <concept>'", "help run"},
		{"plain", "Usage: plain [options] file", ""},
		{"docker", "Run 'podman COMMAND --help' for more.", ""}, // another program's sentence
	}
	for _, c := range cases {
		form, ok := commandHelpForm(c.name, c.text)
		got := ""
		if ok {
			got = strings.Join(form("run"), " ")
		}
		if got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.name, c.text, got, c.want)
		}
	}
}

func TestCommandFlagsFromKeepsOnlyTheCommandsOwn(t *testing.T) {
	help := `Usage:  box run [OPTIONS] IMAGE

Options:
      --rm                 Automatically remove the container when it exits
  -n, --name string        Assign a name to the container
  -D, --debug              Enable debug mode
  -o, --output string      Write to a file
`
	// The program's own option, as the same parser names it.
	global := []appkit.FlagSpec{{Flag: "-D", Kind: appkit.FlagBool}}
	flags := commandFlagsFrom("box", appkit.SubcommandSpec{Name: "run", Label: "Run"}, help, global)
	var names []string
	for _, f := range flags {
		names = append(names, f.Flag)
		if f.Section != "Run options" {
			t.Errorf("%s section %q", f.Flag, f.Section)
		}
	}
	got := strings.Join(names, " ")
	if !strings.Contains(got, "--rm") || !strings.Contains(got, "-n") || strings.Contains(got, "-D") {
		t.Fatalf("flags %q", got)
	}
	if commandFlagsFrom("box", appkit.SubcommandSpec{Name: "x"}, "Traceback (most recent call last):\n  File x", nil) != nil {
		t.Error("a crash report gave options")
	}
}

func TestSafeCommandWord(t *testing.T) {
	for _, ok := range []string{"run", "image-ls", "v2.1", "compose:up"} {
		if !safeCommandWord(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "--rm", "-x", "../x", "a b", "a;b", "$(x)", strings.Repeat("a", 41)} {
		if safeCommandWord(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

// The program is asked for each command's help only when its own help says
// how, and only for commands that are plain words.
func TestAttachCommandFlagsRunsOnlyWhatTheHelpSays(t *testing.T) {
	// As root, help is read as nobody, which must be able to write the log
	// (t.TempDir's parent folder is closed to other users).
	logDir, err := os.MkdirTemp("", "mint-gen-calls-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(logDir) })
	if err := os.Chmod(logDir, 0o777); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(logDir, "calls")
	body := `echo "$@" >> '` + logFile + `'
case "$1 $2" in
  "run --help") printf 'Usage: box run [OPTIONS]\n\nOptions:\n      --rm   Remove when done\n' ;;
  *) echo "box: nothing" ;;
esac`
	tool := fakeHelpTool(t, "box", body)
	spec := appkit.ToolSpec{Name: "box", Subcommands: []appkit.SubcommandSpec{
		{Name: "run", Label: "Run", Args: []string{"run"}},
		{Name: "--rm", Label: "Odd", Args: []string{"--rm"}}, // never passed as a command word
	}}
	// The help does not say how: nothing is run.
	if notes := attachCommandFlags(tool, &spec, "Usage: box COMMAND"); notes != nil {
		t.Fatalf("notes %v", notes)
	}
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		b, _ := os.ReadFile(logFile)
		t.Fatalf("the program was run without the help asking: %q", b)
	}
	notes := attachCommandFlags(tool, &spec, "Run 'box COMMAND --help' for more information.")
	b, _ := os.ReadFile(logFile)
	if strings.TrimSpace(string(b)) != "run --help" {
		t.Fatalf("calls %q", b)
	}
	if len(spec.Subcommands[0].Flags) != 1 || spec.Subcommands[0].Flags[0].Flag != "--rm" || len(notes) != 1 {
		t.Fatalf("run flags %+v notes %v", spec.Subcommands[0].Flags, notes)
	}
}

func TestScaffoldWritesEachCommandsOptions(t *testing.T) {
	spec := appkit.ToolSpec{Name: "box", Bin: "box", Subcommands: []appkit.SubcommandSpec{
		{Name: "run", Label: "Run", Args: []string{"run"}, Flags: []appkit.FlagSpec{{Flag: "--rm", Kind: appkit.FlagBool, Label: "Remove", Section: "Run options"}}},
	}}
	src, err := renderSpecGo(spec)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, "Subcommands:")
	if i < 0 || !strings.Contains(s[i:], `Flag:    "--rm"`) && !strings.Contains(s[i:], `Flag: "--rm"`) {
		t.Fatalf("command options not written:\n%s", s)
	}
}

func TestOptionsBeforeCommand(t *testing.T) {
	cases := []struct {
		name, text string
		want       bool
	}{
		{"docker", "Usage:  docker [OPTIONS] COMMAND", true},
		{"git", "usage: git [-v | --version] [-h | --help] [-C <path>]\n           [--exec-path[=<path>]]\n           <command> [<args>]", true},
		{"kubectl", "kubectl [flags] [command]", true},
		{"7z", "Usage: 7z <command> [<switches>...] <archive_name>", false},
		{"tool", "Usage: tool [FILE] command", false}, // a bracket, but not options
		{"tool", "Usage: other [OPTIONS] COMMAND", false},
	}
	for _, c := range cases {
		if got := optionsBeforeCommand(c.name, c.text); got != c.want {
			t.Errorf("%s %q: %v, want %v", c.name, c.text, got, c.want)
		}
	}
}
