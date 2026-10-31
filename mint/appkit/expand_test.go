package appkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

func TestExpandTildeVarAndQuotes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MINT_VAR", "hello")
	t.Setenv("MINT_SP", "a b")
	os.Unsetenv("MINT_NOT_SET_ZZ")

	cases := []struct {
		in, want string
		unset    bool
	}{
		{"~", home, false},
		{"~/x", filepath.Join(home, "x"), false},
		{"$HOME", home, false},
		{"${MINT_VAR}", "hello", false},
		{"$MINT_NOT_SET_ZZ", "$MINT_NOT_SET_ZZ", true},
		{"'~'", "~", false},
		{"'$HOME'", "$HOME", false},
		{`"$HOME"`, home, false},
		{`"~/x"`, "~/x", false},
		{"$MINT_SP", "'a b'", false},
		{"~user", "~user", false},
		{"*.log", "*.log", false},
		{"foo && bar", "foo && bar", false},
		{"foo;bar", "foo;bar", false},
		{"echo | cat", "echo | cat", false},
		{"$(echo hi)", "$(echo hi)", false},
		{"`id`", "`id`", false},
		{"--output=~/report.txt", "--output=" + filepath.Join(home, "report.txt"), false},
	}
	for _, c := range cases {
		got, _, unset := expandInput(c.in)
		if got != c.want {
			t.Errorf("expand %q = %q, want %q", c.in, got, c.want)
		}
		if c.unset != (len(unset) > 0) {
			t.Errorf("expand %q unset %v, want %v", c.in, unset, c.unset)
		}
	}
	if words := splitShellWords(expandInputMust(t, "$MINT_SP")); len(words) != 1 || words[0] != "a b" {
		t.Fatalf("re-quoted word splits to %q", words)
	}
}

func expandInputMust(t *testing.T, s string) string {
	t.Helper()
	got, _, _ := expandInput(s)
	return got
}

func TestExpandDoesNotRunAnything(t *testing.T) {
	sentinel := filepath.Join(t.TempDir(), "pwned")
	in := "$(touch " + sentinel + ")"
	got, _, _ := expandInput(in)
	if got != in {
		t.Fatalf("expand %q = %q", in, got)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatal("command substitution ran")
	}
	back := "`touch " + sentinel + "`"
	if got, _, _ := expandInput(back); got != back {
		t.Fatalf("backticks expanded to %q", got)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatal("backticks ran")
	}
}

func TestUnsetVarMarksTheRow(t *testing.T) {
	os.Unsetenv("MINT_NOT_SET_ZZ")
	a := newTestApp(t, ToolSpec{Name: "tree", Bin: "echo", TargetLabel: "Pattern"})
	got := a.acceptText("extra_args", "$MINT_NOT_SET_ZZ", boxExtra, nil)
	if got != "$MINT_NOT_SET_ZZ" {
		t.Fatalf("stored %q", got)
	}
	scr := a.MainScreen()
	row, ok := rowByID(scr, "extra_args")
	if !ok {
		t.Fatal("no extra arguments row")
	}
	if label := rowField(row, "label"); label != "Extra arguments - File path does not exist!" {
		t.Fatalf("label %q", label)
	}
	if !rowBool(row, "alert") {
		t.Fatal("row is not marked")
	}
	n := 0
	for _, s := range scr.Sections {
		for _, r := range s.Rows {
			if strings.Contains(rowField(r, "label"), "File path does not exist!") {
				n++
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d warning rows; the warning should be the field's own label", n)
	}
	// A later commit with no unset variable clears it.
	a.acceptText("extra_args", "--prune", boxExtra, nil)
	scr = a.MainScreen()
	row, _ = rowByID(scr, "extra_args")
	if rowField(row, "label") != "Extra arguments" || rowBool(row, "alert") {
		t.Fatalf("error stuck: %q alert=%v", rowField(row, "label"), rowBool(row, "alert"))
	}
}

func TestPathChecks(t *testing.T) {
	a := newTestApp(t, ToolSpec{
		Name:        "tree",
		Bin:         "echo",
		TargetLabel: "Pattern",
		OutputFlag:  "-o",
		Flags: []FlagSpec{
			{Flag: "-f", Kind: FlagText, Label: "Config", Hint: "FILE"},
			{Flag: "--output", Kind: FlagText, Label: "Output file", Hint: "FILE", Role: RoleOutputFile},
			{Flag: "-t", Kind: FlagText, Label: "Format", Hint: "FILE", Role: RoleFormat},
		},
	})
	home := os.Getenv("HOME")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(home, "exists.txt")
	if err := os.WriteFile(good, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(home, "no-such-file")
	badParent := filepath.Join(home, "missing-dir", "out.txt")

	a.acceptText("extra_args", missing, boxExtra, nil)
	if !a.boxBad("extra_args") {
		t.Fatal("missing path was not flagged")
	}
	a.acceptText("extra_args", good, boxExtra, nil)
	if a.boxBad("extra_args") {
		t.Fatal("existing path was flagged")
	}
	a.acceptText("extra_args", "pattern *.c --prune", boxExtra, nil)
	if a.boxBad("extra_args") {
		t.Fatal("plain words and flags were flagged")
	}

	a.acceptText("extra_args", "-o "+badParent, boxExtra, nil)
	if !a.boxBad("extra_args") {
		t.Fatal("output path with a missing parent was not flagged")
	}
	a.acceptText("extra_args", "-o "+filepath.Join(home, "new-report.txt"), boxExtra, nil)
	if a.boxBad("extra_args") {
		t.Fatal("output file whose parent exists was flagged")
	}
	got := a.acceptText("extra_args", "--output=~/missing-dir/report.txt", boxExtra, nil)
	if !strings.Contains(got, filepath.Join(home, "missing-dir", "report.txt")) {
		t.Fatalf("assignment not expanded: %q", got)
	}
	if !a.boxBad("extra_args") {
		t.Fatal("--output=~/missing parent was not flagged")
	}
	got = a.acceptText("extra_args", "--output=~/report.txt", boxExtra, nil)
	if !strings.HasPrefix(got, "--output=") || !strings.Contains(got, filepath.Join(home, "report.txt")) {
		t.Fatalf("assignment %q", got)
	}
	if a.boxBad("extra_args") {
		t.Fatal("--output=~/report.txt flagged though its parent exists")
	}

	fileFlag := a.Spec.Flags[0]
	a.acceptText("flag:-f", missing, boxFlag, &fileFlag)
	if !a.boxBad("flag:-f") {
		t.Fatal("FILE option did not flag a missing path")
	}
	outFlag := a.Spec.Flags[1]
	a.acceptText("flag:--output", filepath.Join(home, "fresh.txt"), boxFlag, &outFlag)
	if a.boxBad("flag:--output") {
		t.Fatal("output role flagged a file whose parent exists")
	}
	a.acceptText("flag:--output", badParent, boxFlag, &outFlag)
	if !a.boxBad("flag:--output") {
		t.Fatal("output role did not flag a missing parent")
	}
	fmtFlag := a.Spec.Flags[2]
	a.acceptText("flag:-t", missing, boxFlag, &fmtFlag)
	if a.boxBad("flag:-t") {
		t.Fatal("a known format role used the FILE heuristic")
	}

	a.acceptText("target", "TODO", boxTarget, nil)
	if a.boxBad("target") {
		t.Fatal("pattern word was flagged")
	}
	a.Spec.PositionalRoles = []ArgRole{RoleInputFile}
	a.acceptText("target", missing, boxTarget, nil)
	if !a.boxBad("target") {
		t.Fatal("input-file role did not flag a missing path")
	}
	a.acceptText("target", good, boxTarget, nil)
	if a.boxBad("target") {
		t.Fatal("input-file role flagged a path that exists")
	}
}

func TestRuntimeExpandsWithoutEnter(t *testing.T) {
	a := newTestApp(t, ToolSpec{
		Name: "tree", Bin: "echo",
		Profiles: []ProfileSpec{{
			Name:      "Home",
			ExtraArgs: []string{"--prune", "~/Downloads"},
		}},
	})
	home := os.Getenv("HOME")
	a.Target = "$HOME/Downloads"
	a.ExtraArgs = "--info ~/Downloads"
	args, _ := a.BuildArgs()
	want := filepath.Join(home, "Downloads")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, want) || strings.Contains(joined, "~") || strings.Contains(joined, "$HOME") {
		t.Fatalf("args %q", args)
	}
	preview := a.CommandPreview()
	if !strings.Contains(preview, want) || strings.Contains(preview, "~/") || strings.Contains(preview, "$HOME") {
		t.Fatalf("preview %q", preview)
	}
	if !a.ApplyProfile("Home") {
		t.Fatal("profile")
	}
	args, _ = a.BuildArgs()
	if !strings.Contains(strings.Join(args, " "), want) {
		t.Fatalf("preset args %q", args)
	}
}

func TestOutputFlagFileShownWhenStdoutIsEmpty(t *testing.T) {
	bin, _ := fakeTool(t, "tree", `
prev=
for a in "$@"; do
  if [ "$prev" = "-o" ]; then
    printf '%s\n' 'Downloads' > "$a"
  fi
  prev=$a
done
`)
	a := newTestApp(t, ToolSpec{
		Name: "tree", Bin: bin, OutputsFile: true, OutputFlag: "-o", DefaultExt: "txt",
	})
	a.SaveToFile = true
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, "Downloads"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.Target = "~/Downloads"
	a.ExtraArgs = "--prune --info"
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	if job.State() != runner.Done {
		t.Fatalf("state %s (%s)", job.State(), job.LastError())
	}
	lines := a.ActiveLines()
	if strings.Join(lines, "\n") != "Downloads" {
		t.Fatalf("output pane %q", lines)
	}
	if len(a.History()) == 0 || !strings.Contains(a.History()[0].CmdLine, filepath.Join(home, "Downloads")) {
		t.Fatalf("history %+v", a.History())
	}
	if strings.Contains(a.History()[0].CmdLine, "~/") {
		t.Fatalf("history kept the tilde: %s", a.History()[0].CmdLine)
	}
}

func TestStdoutKeptWhenTheToolAlsoPrints(t *testing.T) {
	bin, _ := fakeTool(t, "tree", `
echo progress
prev=
for a in "$@"; do
  if [ "$prev" = "-o" ]; then
    printf '%s\n' 'from-file' > "$a"
  fi
  prev=$a
done
`)
	a := newTestApp(t, ToolSpec{
		Name: "tree", Bin: bin, OutputsFile: true, OutputFlag: "-o", DefaultExt: "txt",
	})
	a.SaveToFile = true
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	got := strings.Join(a.ActiveLines(), "\n")
	if !strings.Contains(got, "progress") || strings.Contains(got, "from-file") {
		t.Fatalf("pane %q", got)
	}
}

func rowByID(scr *mint.Screen, id string) (mint.Row, bool) {
	for _, s := range scr.Sections {
		for _, r := range s.Rows {
			if rowField(r, "id") == id {
				return r, true
			}
		}
	}
	for _, p := range scr.Panes {
		if p == nil {
			continue
		}
		for _, s := range p.Sections {
			for _, r := range s.Rows {
				if rowField(r, "id") == id {
					return r, true
				}
			}
		}
	}
	return mint.Row{}, false
}
