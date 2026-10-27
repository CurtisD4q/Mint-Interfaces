package appkit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/findings"
)

func TestScanScopeConfinesFiles(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	other := filepath.Join(root, "other")
	os.MkdirAll(filepath.Join(proj, "sub"), 0o755)
	os.MkdirAll(other, 0o755)
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(proj, "sub", "a.sh"), "1\n2\n3\n")
	write(filepath.Join(other, "secret"), "password\n")
	single := filepath.Join(root, "single.sh")
	write(single, "x\n")
	write(filepath.Join(root, "sibling.sh"), "y\n")
	// A symlink inside the project pointing out of it.
	os.Symlink(filepath.Join(other, "secret"), filepath.Join(proj, "escape"))
	// A symlink inside pointing inside is fine.
	os.Symlink(filepath.Join(proj, "sub", "a.sh"), filepath.Join(proj, "alias.sh"))

	s := newScanScope(root, []string{"proj", single, "-x", "not-there", "*.sh"})
	ok := []string{
		filepath.Join(proj, "sub", "a.sh"),
		"proj/sub/a.sh", // relative to the run's folder
		filepath.Join(proj, "sub", "..", "sub", "a.sh"),
		filepath.Join(proj, "alias.sh"),
		single,
	}
	for _, p := range ok {
		if _, err := s.resolve(p); err != nil {
			t.Errorf("resolve(%q): %v", p, err)
		}
	}
	bad := []string{
		filepath.Join(proj, "escape"),                // symlink out
		filepath.Join(proj, "..", "other", "secret"), // .. out
		filepath.Join(root, "sibling.sh"),            // next to a scanned file
		"/etc/passwd",
		"proj/../other/secret",
		filepath.Join(root, "proj-evil", "x"), // prefix but not inside
		"",
	}
	os.MkdirAll(filepath.Join(root, "proj-evil"), 0o755)
	write(filepath.Join(root, "proj-evil", "x"), "z\n")
	for _, p := range bad {
		if _, err := s.resolve(p); err == nil {
			t.Errorf("resolve(%q) allowed a file outside the scan", p)
		}
	}
	// Nothing named: nothing allowed.
	if _, err := newScanScope(root, nil).resolve(single); err == nil {
		t.Error("empty scope allowed a file")
	}
}

func TestReadContext(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, "line "+string(rune('a'+i%26)))
	}
	f := filepath.Join(dir, "f.sh")
	os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	s := newScanScope(dir, []string{"."})
	src, err := s.readContext("f.sh", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(src) != 11 || src[0].No != 5 || src[10].No != 15 || src[5].Text != lines[9] {
		t.Fatalf("context %+v", src)
	}
	if src, _ := s.readContext("f.sh", 2); src[0].No != 1 || len(src) != 7 {
		t.Fatalf("near the start: %+v", src)
	}
	if _, err := s.readContext("f.sh", 99); err == nil {
		t.Error("line past the end accepted")
	}
	if _, err := s.readContext("f.sh", 0); err == nil {
		t.Error("line 0 accepted")
	}
	// Devices and FIFOs are never read, even inside the scope.
	devScope := newScanScope("/", []string{"/dev"})
	if _, err := devScope.readContext("/dev/zero", 1); err == nil {
		t.Error("read /dev/zero")
	}
	styled := contextStyled(src, 10, 3, 60)
	var text []string
	for _, l := range styled {
		text = append(text, l.Text())
	}
	joined := strings.Join(text, "\n")
	if !strings.Contains(joined, "▶ 10 "+lines[9]) || !strings.Contains(joined, "     ^") {
		t.Fatalf("styled context:\n%s", joined)
	}
}

func TestCompareWithLastRun(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "lint"})
	f := func(rule, msg string) findings.Finding {
		return findings.Finding{Rule: rule, File: "a", Message: msg, Severity: findings.Low}
	}
	run1 := []findings.Finding{f("A", "one"), f("B", "two")}
	if _, _, first := a.compareWithLastRun("k", run1); !first {
		t.Fatal("first run not marked first")
	}
	run2 := []findings.Finding{f("B", "two"), f("C", "three"), f("C", "three")}
	isNew, fixed, first := a.compareWithLastRun("k", run2)
	if first || fixed != 1 || len(isNew) != 1 || !isNew[f("C", "three").Fingerprint()] {
		t.Fatalf("new %v fixed %d first %v", isNew, fixed, first)
	}
	// Another target is separate.
	if _, _, first := a.compareWithLastRun("other", run1); !first {
		t.Fatal("targets share memory")
	}
	// The file is private and holds no finding text.
	fi, err := os.Stat(a.findingsMemoryPath())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("memory file %v %v", fi, err)
	}
	b, _ := os.ReadFile(a.findingsMemoryPath())
	if strings.Contains(string(b), "three") || strings.Contains(string(b), "\"a\"") {
		t.Fatalf("memory holds finding text: %s", b)
	}
	// Old targets are dropped.
	for i := 0; i < maxRememberedTargets+5; i++ {
		a.compareWithLastRun(fmt.Sprint("target-", i), run1)
	}
	if _, _, first := a.compareWithLastRun("k", run1); !first {
		t.Fatal("oldest target was not dropped")
	}
}

func TestTargetKeyHidesArgs(t *testing.T) {
	k := targetKey("tool", []string{"--token", "s3cret", "x"})
	if strings.Contains(k, "s3cret") || k == targetKey("tool", []string{"--token", "s3cret", "y"}) {
		t.Fatalf("key %q", k)
	}
	// Word boundaries matter.
	if targetKey("t", []string{"ab", "c"}) == targetKey("t", []string{"a", "bc"}) {
		t.Fatal("args joined without separators")
	}
}

// End to end: a second run says what's new and fixed, and the detail view
// shows the source of the scanned script but not of a file outside it.
func TestFindingsSecondRun(t *testing.T) {
	bin, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "t.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho $1\ncd /tmp\n"), 0o644)
	fs, _ := KnownFindingsSpec("shellcheck")
	a := newTestApp(t, ToolSpec{Name: "shellcheck", Bin: bin, Findings: &fs, ExitMeanings: KnownExitMeanings("shellcheck")})
	a.Target = script
	run := func() *findingsReport {
		job, err := a.StartJob()
		if err != nil {
			t.Fatal(err)
		}
		job.Wait()
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.findings
	}
	if rep := run(); !rep.first || sinceLastRun(rep) != "first run of this target" {
		t.Fatalf("first run: %q", sinceLastRun(rep))
	}
	// Fix the cd; add x=`ls`, which is two new problems (SC2034 unused,
	// SC2006 backticks).
	os.WriteFile(script, []byte("#!/bin/sh\necho $1\ncd /tmp || exit\nx=`ls`\n"), 0o644)
	rep := run()
	if got := sinceLastRun(rep); got != "2 new · 1 fixed" {
		t.Fatalf("second run: %q", got)
	}

	var sc2086 findings.Finding
	for _, f := range rep.res.Findings {
		if f.Rule == "SC2086" {
			sc2086 = f
		}
	}
	src, err := rep.scope.readContext(sc2086.File, sc2086.Line)
	if err != nil || len(src) != 4 || src[1].Text != "echo $1" {
		t.Fatalf("context %+v %v", src, err)
	}
	outside := sc2086
	outside.File = "/etc/passwd"
	if _, err := rep.scope.readContext(outside.File, 1); err == nil {
		t.Fatal("a finding naming /etc/passwd was read")
	}
}
