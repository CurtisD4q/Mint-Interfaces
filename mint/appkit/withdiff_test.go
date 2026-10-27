package appkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/diff"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

func diffApp(t *testing.T, bin string) *ToolApp {
	t.Helper()
	d, ok := KnownDiffSpec("diff")
	if !ok {
		t.Fatal("diff not known")
	}
	return newTestApp(t, ToolSpec{Name: "diff", Bin: bin, Diff: &d, Flags: []FlagSpec{
		{Flag: "-i", Kind: FlagBool, Label: "Ignore case"},
		{Flag: "-y", Kind: FlagBool, Label: "Side by side"},
		{Flag: "-U", Kind: FlagText, Label: "Context lines"},
	}})
}

func TestDiffForcesUnified(t *testing.T) {
	a := diffApp(t, "diff")
	a.Target = "old new"
	a.Values["-i"] = "true"
	args, _ := a.BuildArgs()
	if got := strings.Join(args, " "); got != "-i -u -- old new" {
		t.Fatalf("args = %q", got)
	}
	if !strings.Contains(a.CommandPreview(), "diff -i -u -- old new") {
		t.Fatalf("preview = %q", a.CommandPreview())
	}
	if _, ok := a.diffArgs(args); !ok {
		t.Fatal("forced unified output should be readable")
	}
}

func TestDiffRespectsChosenFormat(t *testing.T) {
	a := diffApp(t, "diff")
	a.Target = "old new"

	a.Values["-y"] = "true"
	args, _ := a.BuildArgs()
	if strings.Contains(strings.Join(args, " "), "-u") {
		t.Fatalf("-u added over the user's -y: %q", args)
	}
	if _, ok := a.diffArgs(args); ok {
		t.Fatal("side-by-side output isn't readable by the view")
	}

	a.Values["-y"] = ""
	a.Values["-U"] = "5"
	args, _ = a.BuildArgs()
	if got := strings.Join(args, " "); got != "-U 5 -- old new" {
		t.Fatalf("args = %q", got)
	}
	if _, ok := a.diffArgs(args); !ok {
		t.Fatal("-U 5 is unified and readable")
	}
	a.Values["-U"] = ""
	a.ExtraArgs = "-U3"
	if args, _ = a.BuildArgs(); strings.Count(strings.Join(args, " "), "-u") != 0 {
		t.Fatalf("-u added over -U3: %q", args)
	}
	a.ExtraArgs = "--unified=2"
	if args, _ = a.BuildArgs(); strings.Contains(strings.Join(args, " "), " -u ") {
		t.Fatalf("-u added over --unified=2: %q", args)
	}
}

// A file whose name looks like a format option is a name, not a format.
func TestDiffFormatFlagAfterSeparatorIsAName(t *testing.T) {
	if usesFlag([]string{"--", "-y", "x"}, []string{"-y"}) {
		t.Fatal("operand after -- counted as an option")
	}
}

func TestUsesFlag(t *testing.T) {
	flags := []string{"-u", "-U", "--unified", "-y"}
	for arg, want := range map[string]bool{
		"-u": true, "-U": true, "-U3": true, "--unified": true, "--unified=4": true,
		"-y": true, "--unifiedx": false, "-x": false, "--u": false, "-": false, "--": false,
	} {
		if got := usesFlag([]string{arg}, flags); got != want {
			t.Errorf("usesFlag(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestValidateDiff(t *testing.T) {
	for _, d := range []DiffSpec{{}, {Format: "html"}, {Format: DiffUnified, ForceArgs: []string{"file"}}} {
		d := d
		if err := (ToolSpec{Name: "x", Diff: &d}).Validate(); err == nil {
			t.Errorf("Validate accepted %+v", d)
		}
	}
	d, _ := KnownDiffSpec("cmp")
	if err := (ToolSpec{Name: "cmp", Diff: &d}).Validate(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range (ToolSpec{Name: "cmp", Diff: &d}).EnabledMixinNames() {
		found = found || n == MixinDiff
	}
	if !found {
		t.Fatal("WithDiff not listed as enabled")
	}
}

// End to end with the real diff: a file named "-left" reaches diff as a
// name (after "--"), the forced -u output is readable, exit 1 is a result,
// and the output lays out as rows.
func TestDiffRunAndLayout(t *testing.T) {
	bin, err := exec.LookPath("diff")
	if err != nil {
		t.Skip("diff not installed")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-left"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "right"), []byte("a\nB\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	a := diffApp(t, bin)
	a.Spec.ExitMeanings = KnownExitMeanings("diff")
	a.Target = "-left right"
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	if job.State() != runner.Done || job.ExitMeaning() != "files differ" {
		t.Fatalf("state %v exit %d: %q", job.State(), job.ExitCode(), job.Output())
	}
	if !a.diffReadable() {
		t.Fatal("run not marked readable")
	}
	var all []string
	for _, l := range a.DiffLines(job.Lines(), 120, diff.SideBySide, false) {
		all = append(all, l.Text())
	}
	if len(all) != 6 {
		t.Fatalf("got %d lines:\n%s", len(all), strings.Join(all, "\n"))
	}
	if !strings.HasPrefix(all[0], "-left  ↔  right") || !strings.Contains(all[0], "+2 −1") {
		t.Errorf("header %q", all[0])
	}
	if !strings.Contains(all[3], "2 - b") || !strings.Contains(all[3], "│ 2 + B") {
		t.Errorf("change row %q", all[3])
	}
	if !strings.Contains(all[5], "│ 4 + d") {
		t.Errorf("insert row %q", all[5])
	}
}

// cmp's byte list goes through the same path.
func TestCmpRunAndLayout(t *testing.T) {
	bin, err := exec.LookPath("cmp")
	if err != nil {
		t.Skip("cmp not installed")
	}
	dir := t.TempDir()
	l, r := filepath.Join(dir, "l"), filepath.Join(dir, "r")
	os.WriteFile(l, []byte("abc"), 0o644)
	os.WriteFile(r, []byte("aXcd"), 0o644)
	d, _ := KnownDiffSpec("cmp")
	a := newTestApp(t, ToolSpec{Name: "cmp", Bin: bin, Diff: &d, ExitMeanings: KnownExitMeanings("cmp")})
	a.Target = l + " " + r
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	var all []string
	for _, line := range a.DiffLines(job.Lines(), 120, diff.Auto, false) {
		all = append(all, line.Text())
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{"1 bytes differ", "62 'b'", "58 'X'", "ends after byte 3"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
}
