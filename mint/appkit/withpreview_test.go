package appkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/fileplan"
)

func inDir(t *testing.T, dir string) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
}

func TestPreviewRm(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "build", "obj"), 0o755)
	os.WriteFile(filepath.Join(dir, "build", "a.o"), make([]byte, 2048), 0o644)
	os.WriteFile(filepath.Join(dir, "build", "obj", "b.o"), make([]byte, 1024), 0o644)
	inDir(t, dir)
	p, _ := KnownPreviewSpec("rm")
	a := newTestApp(t, ToolSpec{Name: "rm", Preview: &p, Flags: []FlagSpec{{Flag: "-r", Kind: FlagBool}}})
	a.Values["-r"] = "true"
	a.Target = "build"
	plan, err := a.MakePlan()
	if err != nil {
		t.Fatal(err)
	}
	if got := planSummary(plan); got != "delete 2 files and 2 folders (3.0 KB removed or replaced)" {
		t.Fatalf("summary %q", got)
	}
	scr := a.PreviewScreen(plan, func(*mint.Ctx, fileplan.Plan) {})
	var names []string
	for _, s := range scr.Sections {
		names = append(names, s.Name)
	}
	if strings.Join(names, "|") != "Preview|Actions|Deleted (4)" {
		t.Fatalf("sections %q", names)
	}
	// Without -r: rm will fail on the folder, and the preview says so.
	a.Values["-r"] = ""
	plan, _ = a.MakePlan()
	if got := planSummary(plan); got != "fail 1" {
		t.Fatalf("summary without -r %q", got)
	}
}

// A pattern the shell would have expanded is blocked, and "Expand
// patterns" turns it into names.
func TestPreviewPatterns(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.log", "b c.log", "keep.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	inDir(t, dir)
	p, _ := KnownPreviewSpec("rm")
	a := newTestApp(t, ToolSpec{Name: "rm", Preview: &p})
	a.Target = "*.log"
	plan, _ := a.MakePlan()
	if !plan.Blocked() || !hasPattern(splitShellWords(a.Target)) {
		t.Fatalf("pattern not blocked: %+v", plan.Warnings)
	}
	words, _ := fileplan.ExpandPatterns(splitShellWords(a.Target), dir)
	a.Target = joinShellWords(words)
	if got := splitShellWords(a.Target); strings.Join(got, "|") != "a.log|b c.log" {
		t.Fatalf("expanded target %q splits to %q", a.Target, got)
	}
	plan, _ = a.MakePlan()
	if plan.Blocked() || plan.Totals().Count[fileplan.Delete] != 2 {
		t.Fatalf("after expanding: %+v", plan)
	}
}

func TestJoinShellWordsRoundTrip(t *testing.T) {
	for _, words := range [][]string{
		{"plain", "two words", "it's", `say "hi"`, `back\slash`, "-dash", "tab\there"},
	} {
		if got := splitShellWords(joinShellWords(words)); strings.Join(got, "|") != strings.Join(words, "|") {
			t.Errorf("round trip %q → %q → %q", words, joinShellWords(words), got)
		}
	}
}

func TestPreviewRsyncDryRun(t *testing.T) {
	bin, err := exec.LookPath("rsync")
	if err != nil {
		t.Skip("rsync not installed")
	}
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	os.MkdirAll(src, 0o755)
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(src, "new"), []byte("n"), 0o644)
	os.WriteFile(filepath.Join(dst, "extra"), []byte("e"), 0o644)
	p, _ := KnownPreviewSpec("rsync")
	a := newTestApp(t, ToolSpec{Name: "rsync", Bin: bin, Preview: &p, Flags: []FlagSpec{
		{Flag: "-a", Kind: FlagBool}, {Flag: "--delete", Kind: FlagBool}}})
	a.Values["-a"], a.Values["--delete"] = "true", "true"
	a.Target = src + "/ " + dst + "/"
	plan, err := a.MakePlan()
	if err != nil {
		t.Fatal(err)
	}
	if got := planSummary(plan); got != "delete 1 file · create 1" {
		t.Fatalf("summary %q", got)
	}
	// The dry run changed nothing.
	if _, err := os.Stat(filepath.Join(dst, "extra")); err != nil {
		t.Fatal("dry run deleted a file")
	}
	if _, err := os.Stat(filepath.Join(dst, "new")); err == nil {
		t.Fatal("dry run created a file")
	}
}

func TestValidatePreview(t *testing.T) {
	for _, c := range []struct {
		bin string
		p   PreviewSpec
	}{
		{"rm", PreviewSpec{}},
		{"dd", PreviewSpec{Adapter: PreviewCoreutils}},
		{"rsync", PreviewSpec{Adapter: PreviewRsync}},
		{"rsync", PreviewSpec{Adapter: PreviewRsync, DryRunArgs: []string{"dest"}}},
		{"x", PreviewSpec{Adapter: "magic"}},
	} {
		p := c.p
		if err := (ToolSpec{Name: c.bin, Preview: &p}).Validate(); err == nil {
			t.Errorf("accepted %s %+v", c.bin, c.p)
		}
	}
	for _, bin := range []string{"rm", "cp", "mv", "rsync"} {
		p, _ := KnownPreviewSpec(bin)
		if err := (ToolSpec{Name: bin, Preview: &p}).Validate(); err != nil {
			t.Errorf("%s: %v", bin, err)
		}
	}
}

func TestPlanSummaryFormatting(t *testing.T) {
	if commas(1234567) != "1,234,567" || commas(12) != "12" || humanBytes(3*1024*1024) != "3.0 MB" || humanBytes(10) != "10 B" {
		t.Fatal("number formatting")
	}
	p := fileplan.Plan{Incomplete: true, Effects: []fileplan.Effect{{Kind: fileplan.Delete, Size: 10}}}
	if got := planSummary(p); got != "at least: delete 1 file (10 B removed or replaced)" {
		t.Fatalf("%q", got)
	}
	if planSummary(fileplan.Plan{}) != "no changes" {
		t.Fatal("empty plan")
	}
	line := effectLine(fileplan.Effect{Kind: fileplan.Move, Path: "/w/a‮", Dest: "/w/d/a"}, "/w")
	if line != "a<U+202E> → d/a" {
		t.Fatalf("effect line %q", line)
	}
}
