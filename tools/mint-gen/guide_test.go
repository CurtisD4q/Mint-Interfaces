package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMintGenHelpIsTheGuide(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			var code int
			out := captureStdout(t, func() {
				code = runCLI([]string{arg})
			})
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			for _, want := range []string{
				"Using mint-gen",
				"mint-gen ps",
				"mint-gen --build ps",
				"mint-gen --run ps",
				"--check",
				"--explain",
				"--list",
				"--list-archetypes",
				"--archetype",
				"--remove",
				"Using a Mint app",
				"home menu",
				"Enter",
				"Tab",
				"Shift-Tab",
				"[",
				"]",
				"|",
				"Click a pane",
				"scroll sideways",
				"H narrows",
				"L widens",
				"Presets",
				"Run History",
				"Manual",
				"--man",
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("%s guide missing %q\n%s", arg, want, out)
				}
			}
			flagsAt := strings.Index(out, "\nFlags:\n")
			if flagsAt < 0 || flagsAt < strings.Index(out, "Using mint-gen") {
				t.Fatalf("flag list is not after the guide:\n%s", out)
			}
			list := out[flagsAt:]
			for _, flagName := range []string{"-build", "-run", "-check", "-explain", "-list", "-list-archetypes", "-archetype", "-remove", "-out", "-force", "-tui"} {
				if !strings.Contains(list, flagName) {
					t.Fatalf("flag list missing %s\n%s", flagName, list)
				}
			}
		})
	}
}

func TestRunFlagKeepsProgramHelp(t *testing.T) {
	fs := flag.NewFlagSet("mint-gen", flag.ContinueOnError)
	fs.Bool("run", false, "")
	fs.SetOutput(io.Discard)
	ordered := flagsFirst(fs, []string{"--run", "ps", "--help"})
	if helpRequested(ordered) {
		t.Fatalf("--help after the program was taken as the guide: %v", ordered)
	}
	if !helpRequested(flagsFirst(fs, []string{"--help"})) {
		t.Fatal("--help was not the guide")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return buf.String()
}
