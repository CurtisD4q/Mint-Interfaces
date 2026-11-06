package appkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fuzz tests for what an app reads back or builds from what you type:
// settings files (which may be old, hand-edited or damaged) and the command
// line made from the boxes. Run one for longer with
//
//	go test ./mint/appkit -run '^$' -fuzz FuzzLoadSettings -fuzztime 60s

func fuzzSpec() ToolSpec {
	return ToolSpec{
		Name: "fz", Bin: "echo", ArchetypeID: "toolkit", OperandSeparator: true, TargetLabel: "Target",
		Flags: []FlagSpec{
			{Flag: "-v", Kind: FlagBool, Label: "Verbose"},
			{Flag: "-n", Kind: FlagInt, Label: "Count", Min: 1, Max: 10},
			{Flag: "--name=", Kind: FlagText, Label: "Name"},
			{Flag: "--pass", Kind: FlagSecret, Label: "Password"},
		},
		Subcommands: []SubcommandSpec{
			{Name: "run", Args: []string{"run"}, Flags: []FlagSpec{{Flag: "--rm", Kind: FlagBool, Label: "Remove"}}},
			{Name: "ls", Args: []string{"ls"}},
		},
	}
}

// A settings file of any shape loads without panicking, and the app still
// builds a command line from it: the program first, every operand after "--".
func FuzzLoadSettings(f *testing.F) {
	f.Add([]byte(`{"target":"x","values":{"-v":"true","-n":"3"},"active_subcommand":"run"}`))
	f.Add([]byte(`{"values":{"-n":"9999999999999999999999"},"active_subcommand":"nope"}`))
	f.Add([]byte(`{"values":null,"in_memory":false,"results_dir":"../../etc"}`))
	f.Add([]byte(`{"values":{"-v":1},"target":["a"]}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(`{"extra_args":"-- --pass hunter22 $HOME ~"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		a := newTestApp(t, fuzzSpec())
		if err := os.MkdirAll(a.ConfigDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(a.ConfigDir, "settings.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		_ = a.LoadSettings()
		args, _ := a.BuildArgs()
		checkArgs(t, a, args)
		_ = a.CommandPreview()
	})
}

// Whatever is typed into the boxes, the command line keeps operands after
// "--", never carries a NUL (which no program can receive), and the preview
// never shows the value of an environment variable that was typed as $NAME.
func FuzzBuildArgs(f *testing.F) {
	f.Add("target file", "-x --y=z", "name", "3", true, "run")
	f.Add("$FUZZ_SECRET", "$FUZZ_SECRET", "$FUZZ_SECRET", "", false, "ls")
	f.Add("-rf /", "--pass hunter22", "a\x00b", "-5", true, "nope")
	f.Add("~/x 'quoted words' \"dq\"", "\\ ", "\n", "10", false, "")
	f.Fuzz(func(t *testing.T, target, extra, name, count string, verbose bool, sub string) {
		t.Setenv("FUZZ_SECRET", "s3cr3t-value")
		a := newTestApp(t, fuzzSpec())
		a.useSubcommand(sub)
		a.Target, a.ExtraArgs = target, extra
		a.Values["--name="] = name
		a.Values["-n"] = count
		if verbose {
			a.Values["-v"] = "true"
		}
		args, _ := a.BuildArgs()
		checkArgs(t, a, args)
		preview := a.CommandPreview()
		if strings.Contains(preview, "s3cr3t-value") && !strings.Contains(target+extra+name+count, "s3cr3t-value") {
			t.Fatalf("the preview shows an environment variable's value: %q", preview)
		}
	})
}

func checkArgs(t *testing.T, a *ToolApp, args []string) {
	t.Helper()
	for _, w := range args {
		if strings.ContainsRune(w, 0) {
			t.Fatalf("an argument holds a NUL: %q", args)
		}
	}
	// With OperandSeparator, every operand comes after "--": nothing the
	// target box holds can be read as an option.
	if len(expandArgWords(strings.TrimSpace(a.Target))) > 0 {
		sep := -1
		for i, w := range args {
			if w == "--" {
				sep = i
			}
		}
		if sep < 0 {
			t.Fatalf("no -- before the operands: %q", args)
		}
	}
}

// Words are split and shown without ever revealing a variable's value in
// the shown form.
func FuzzArgWords(f *testing.F) {
	for _, s := range []string{"a b", "'x y' z", "$FUZZ_SECRET", "${FUZZ_SECRET}x", "\\$FUZZ_SECRET", "~/a ~b", "\"$FUZZ_SECRET\"", "'unterminated", "\\"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		t.Setenv("FUZZ_SECRET", "s3cr3t-value")
		words := expandArgWords(s)
		shown := shownArgWords(s)
		if len(words) != len(shown) {
			t.Fatalf("%d words, %d shown for %q", len(words), len(shown), s)
		}
		if joined := strings.Join(shown, " "); strings.Contains(joined, "s3cr3t-value") && !strings.Contains(s, "s3cr3t-value") {
			t.Fatalf("shown form reveals the value: %q from %q", joined, s)
		}
	})
}
