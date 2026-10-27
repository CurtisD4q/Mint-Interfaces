package appkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/findings"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

func TestForceFormat(t *testing.T) {
	force := []string{"-f", "json1"}
	formats := []string{"-f", "--format"}
	readable := []string{"-f json1", "--format json1", "-fjson1", "--format=json1"}
	for _, c := range []struct {
		args     string
		at       int
		want     string
		readable bool
	}{
		{"-x a.sh", 0, "-f json1 -x a.sh", true},
		{"-f gcc a.sh", 0, "-f gcc a.sh", false},
		{"-f json1 a.sh", 0, "-f json1 a.sh", true},
		{"--format=json1 a.sh", 0, "--format=json1 a.sh", true},
		{"--format json1 a.sh", 0, "--format json1 a.sh", true},
		{"-fgcc a.sh", 0, "-fgcc a.sh", false},
		{"-- -f", 0, "-f json1 -- -f", true}, // "-f" after -- is a file name
		{"image -q alpine", 1, "image -f json1 -q alpine", true},
		{"image", 5, "image -f json1", true},
	} {
		out, ok := forceFormat(strings.Fields(c.args), c.at, force, formats, readable)
		if strings.Join(out, " ") != c.want || ok != c.readable {
			t.Errorf("forceFormat(%q, %d) = %q %v, want %q %v", c.args, c.at, strings.Join(out, " "), ok, c.want, c.readable)
		}
	}
}

// For a suite, forced options go after the subcommand words.
func TestFindingsArgsAfterSubcommand(t *testing.T) {
	f, _ := KnownFindingsSpec("trivy")
	a := newTestApp(t, ToolSpec{Name: "trivy", Archetype: ArchetypeSuite, Findings: &f,
		Subcommands: []SubcommandSpec{{Name: "Image", Args: []string{"image"}}}})
	a.Target = "alpine:3"
	args, _ := a.BuildArgs()
	if got := strings.Join(args, " "); got != "image --format sarif alpine:3" {
		t.Fatalf("args = %q", got)
	}
}

func TestValidateFindings(t *testing.T) {
	bad := []FindingsSpec{
		{},
		{Adapter: "xml"},
		{Adapter: FindingsLines},
		{Adapter: FindingsLines, Patterns: []FindingPattern{{Severity: "loud", Regex: "x"}}},
		{Adapter: FindingsLines, Patterns: []FindingPattern{{Severity: "high", Regex: "("}}},
		{Adapter: FindingsLynis, ReportFile: "relative.dat"},
		{Adapter: FindingsLynis, ReportFile: "/var/log/../../etc/shadow"},
		{Adapter: FindingsSARIF, ForceArgs: []string{"--"}},
	}
	for _, f := range bad {
		f := f
		if err := (ToolSpec{Name: "x", Findings: &f}).Validate(); err == nil {
			t.Errorf("Validate accepted %+v", f)
		}
	}
	for _, bin := range []string{"shellcheck", "hadolint", "trivy", "semgrep", "lynis"} {
		f, ok := KnownFindingsSpec(bin)
		if !ok {
			t.Fatalf("%s not known", bin)
		}
		if err := (ToolSpec{Name: bin, Findings: &f}).Validate(); err != nil {
			t.Errorf("%s: %v", bin, err)
		}
	}
}

func TestReadReportFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "report.dat")
	os.WriteFile(good, []byte("warning[]=A-1|x|-|-|\n"), 0o600)
	if data, err := readReportFile(good); err != nil || !strings.Contains(string(data), "A-1") {
		t.Fatalf("regular file: %q %v", data, err)
	}
	link := filepath.Join(dir, "link.dat")
	os.Symlink(good, link)
	if _, err := readReportFile(link); err == nil {
		t.Error("followed a symlink")
	}
	fifo := filepath.Join(dir, "fifo.dat")
	if err := syscall.Mkfifo(fifo, 0o600); err == nil {
		if _, err := readReportFile(fifo); err == nil {
			t.Error("read a FIFO")
		}
	}
	if _, err := readReportFile(dir); err == nil {
		t.Error("read a directory")
	}
	if _, err := readReportFile(filepath.Join(dir, "missing")); err == nil || !strings.Contains(err.Error(), "no report") {
		t.Errorf("missing: %v", err)
	}
	big := filepath.Join(dir, "big.dat")
	fh, _ := os.Create(big)
	fh.Truncate(maxReportFile + 1)
	fh.Close()
	if _, err := readReportFile(big); err == nil {
		t.Error("read a file over the limit")
	}
}

// End to end with the real shellcheck: forced json1, exit 1 is a result,
// findings are grouped, and the view's rows are cleaned.
func TestFindingsShellcheckRun(t *testing.T) {
	bin, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "t.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho $1\ncd /tmp\nx=`ls`\n# ‮evil\x1b[2J\n"), 0o644)
	f, _ := KnownFindingsSpec("shellcheck")
	a := newTestApp(t, ToolSpec{Name: "shellcheck", Bin: bin, Findings: &f, ExitMeanings: KnownExitMeanings("shellcheck")})
	a.Target = script
	if args, _ := a.BuildArgs(); strings.Join(args, " ") != "-f json1 "+script {
		t.Fatalf("args %q", args)
	}
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	if job.State() != runner.Done || job.ExitMeaning() != "found problems" {
		t.Fatalf("state %v exit %d %q", job.State(), job.ExitCode(), job.Output())
	}
	a.mu.Lock()
	rep, ok := a.findings, a.findingsOK
	a.mu.Unlock()
	if !ok || rep == nil || rep.err != nil {
		t.Fatalf("report %+v ok %v", rep, ok)
	}
	if got := countsSummary(rep.res.Findings); got != "2 medium · 1 low · 1 info" {
		t.Fatalf("counts %q", got)
	}
	scr := a.FindingsScreen()
	var names []string
	for _, s := range scr.Sections {
		names = append(names, s.Name)
	}
	if strings.Join(names, "|") != "Findings|Medium (2)|Low (1)|Info (1)" {
		t.Fatalf("sections %q", names)
	}
	medium := findings.GroupFindings(rep.res.Findings, findings.BySeverity, findings.Medium)[0].Findings[0]
	lines := findingDetailLines(medium, 80)
	var text []string
	for _, l := range lines {
		text = append(text, l.Text())
	}
	if joined := strings.Join(text, "\n"); !strings.Contains(joined, "Severity    Medium") || !strings.Contains(joined, "SC2164") || !strings.Contains(joined, "|| exit") {
		t.Fatalf("detail %q", text)
	}
}

// A user's own non-json format is shown as a log, not misread.
func TestFindingsUserFormatNotParsed(t *testing.T) {
	bin, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "t.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho $1\n"), 0o644)
	f, _ := KnownFindingsSpec("shellcheck")
	a := newTestApp(t, ToolSpec{Name: "shellcheck", Bin: bin, Findings: &f})
	a.ExtraArgs = "-f gcc"
	a.Target = script
	job, _ := a.StartJob()
	job.Wait()
	a.mu.Lock()
	ok := a.findingsOK
	a.mu.Unlock()
	if ok {
		t.Fatal("gcc format marked readable")
	}
}

func TestFindingLabelCleansText(t *testing.T) {
	f := findings.Finding{Severity: findings.High, Rule: "R\x1b[31m1", Message: "bad‮ thing\nsecond line", File: "a\u009b.sh", Line: 3}
	l := findingLabel(f, findings.BySeverity)
	for _, r := range l {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x202E {
			t.Fatalf("control %U in label %q", r, l)
		}
	}
	if strings.Contains(l, "second line") || !strings.Contains(l, "<U+202E>") {
		t.Fatalf("label %q", l)
	}
}

func TestWrap(t *testing.T) {
	got := wrap("one two three four\n\nfive "+strings.Repeat("x", 25), 10)
	want := []string{"one two", "three four", "", "five", "xxxxxxxxxx", "xxxxxxxxxx", "xxxxx"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrap = %q", got)
	}
}
