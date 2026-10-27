package findings

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Real output from shellcheck 0.11.0 -f json1.
func TestParseShellcheckJSON1(t *testing.T) {
	r, err := ParseShellcheckJSON1(fixture(t, "shellcheck.json1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 4 || r.Capped {
		t.Fatalf("got %d findings", len(r.Findings))
	}
	want := []struct {
		sev        Severity
		rule       string
		line, col  int
		fixPresent bool
	}{
		{Low, "SC2086", 2, 6, true},
		{Medium, "SC2164", 3, 1, true},
		{Medium, "SC2034", 4, 1, false},
		{Info, "SC2006", 4, 3, true},
	}
	for i, w := range want {
		f := r.Findings[i]
		if f.Severity != w.sev || f.Rule != w.rule || f.Line != w.line || f.Col != w.col || f.File != "t.sh" || (f.Fix != "") != w.fixPresent {
			t.Errorf("finding %d = %+v, want %+v", i, f, w)
		}
	}
	if !strings.Contains(r.Findings[1].Fix, `" || exit"`) {
		t.Errorf("fix = %q", r.Findings[1].Fix)
	}
	if r.Findings[0].Detail != "https://www.shellcheck.net/wiki/SC2086" {
		t.Errorf("detail = %q", r.Findings[0].Detail)
	}
	// No problems: shellcheck prints nothing, or an empty list.
	for _, empty := range []string{"", "  \n", `{"comments":[]}`} {
		if r, err := ParseShellcheckJSON1([]byte(empty)); err != nil || len(r.Findings) != 0 {
			t.Errorf("empty %q: %v %v", empty, r, err)
		}
	}
	if _, err := ParseShellcheckJSON1([]byte("t.sh:2:6: note: Double quote")); err == nil {
		t.Error("gcc-format output accepted as json1")
	}
}

func TestParseSARIF(t *testing.T) {
	r, err := ParseSARIF(fixture(t, "trivy.sarif"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 3 {
		t.Fatalf("got %d findings", len(r.Findings))
	}
	cve := r.Findings[0]
	if cve.Severity != Critical || cve.Rule != "CVE-2023-0001" || cve.File != "/srv/app/my lib/package-lock.json" || cve.Line != 12 || cve.Col != 3 {
		t.Errorf("cve %+v", cve)
	}
	if !strings.Contains(cve.Detail, "Upgrade openssl") || !strings.Contains(cve.Detail, "avd.aquasec.com") {
		t.Errorf("cve detail %q", cve.Detail)
	}
	if ds := r.Findings[1]; ds.Severity != Medium || ds.File != "Dockerfile" || ds.Line != 1 {
		t.Errorf("ds002 %+v", ds)
	}
	// Rule found by index, level from the rule's default.
	if low := r.Findings[2]; low.Rule != "LOW-RULE" || low.Severity != Low || low.File != "" {
		t.Errorf("by index %+v", low)
	}
	for _, bad := range []string{"", "{}", "[1,2]", `{"runs": "x"}`, "not json"} {
		if _, err := ParseSARIF([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	// An out-of-range rule index is ignored, not a crash.
	if _, err := ParseSARIF([]byte(`{"runs":[{"tool":{"driver":{}},"results":[{"ruleIndex":7,"message":{"text":"x"}}]}]}`)); err != nil {
		t.Error(err)
	}
}

func TestSarifSeverity(t *testing.T) {
	for _, c := range []struct {
		level, rule string
		props       map[string]any
		want        Severity
	}{
		{"error", "", nil, High},
		{"", "error", nil, High},
		{"", "", nil, Medium},
		{"note", "", nil, Low},
		{"none", "", nil, Info},
		{"note", "", map[string]any{"security-severity": "7.5"}, High},
		{"error", "", map[string]any{"security-severity": 3.1}, Low},
		{"error", "", map[string]any{"security-severity": "junk"}, High},
	} {
		if got := sarifSeverity(c.level, c.rule, c.props); got != c.want {
			t.Errorf("sarifSeverity(%q, %q, %v) = %v, want %v", c.level, c.rule, c.props, got, c.want)
		}
	}
}

func TestParseLynisReport(t *testing.T) {
	r := ParseLynisReport(fixture(t, "lynis-report.dat"))
	if len(r.Findings) != 4 {
		t.Fatalf("got %d: %+v", len(r.Findings), r.Findings)
	}
	if f := r.Findings[0]; f.Severity != High || f.Rule != "AUTH-9286" || !strings.HasPrefix(f.Message, "Configure minimum") || f.Detail != "" {
		t.Errorf("warning %+v", f)
	}
	if f := r.Findings[1]; f.Detail != "Check your /etc/resolv.conf file" {
		t.Errorf("solution %+v", f)
	}
	if f := r.Findings[3]; f.Severity != Low || f.Detail != "sysctl key=fs.suid_dumpable" {
		t.Errorf("suggestion %+v", f)
	}
}

func TestParseLines(t *testing.T) {
	patterns := []Pattern{
		{Severity: High, Re: regexp.MustCompile(`^(?P<file>[^:]+):(?P<line>\d+):(?P<col>\d+): error: (?P<message>.*)$`)},
		{Severity: Medium, Re: regexp.MustCompile(`^(?P<file>[^:]+):(?P<line>\d+):\d+: warning: (?P<message>.*) \[(?P<rule>[^\]]+)\]$`)},
		{Severity: Low, Re: regexp.MustCompile(`(?i)^hint:`)},
	}
	r := ParseLines([]string{
		"a.c:3:7: error: expected ';'",
		"b.c:9:1: warning: unused variable [-Wunused]",
		"hint: try harder",
		"unrelated text",
	}, patterns)
	if len(r.Findings) != 3 {
		t.Fatalf("%+v", r.Findings)
	}
	if f := r.Findings[0]; f.Severity != High || f.File != "a.c" || f.Line != 3 || f.Col != 7 || f.Message != "expected ';'" {
		t.Errorf("error %+v", f)
	}
	if f := r.Findings[1]; f.Rule != "-Wunused" || f.Message != "unused variable" {
		t.Errorf("warning %+v", f)
	}
	if f := r.Findings[2]; f.Message != "hint: try harder" {
		t.Errorf("hint %+v", f)
	}
}

func TestCapped(t *testing.T) {
	var lines []string
	for i := 0; i < MaxFindings+10; i++ {
		lines = append(lines, "x")
	}
	r := ParseLines(lines, []Pattern{{Severity: Low, Re: regexp.MustCompile("x")}})
	if len(r.Findings) != MaxFindings || !r.Capped {
		t.Fatalf("%d findings, capped %v", len(r.Findings), r.Capped)
	}
	var b strings.Builder
	b.WriteString(`{"comments":[`)
	for i := 0; i < MaxFindings+1; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"file":"f","line":%d,"column":1,"level":"info","code":1,"message":"m"}`, i+1)
	}
	b.WriteString(`]}`)
	if r, _ := ParseShellcheckJSON1([]byte(b.String())); !r.Capped {
		t.Fatal("shellcheck not capped")
	}
}

func TestGroupFindings(t *testing.T) {
	fs := []Finding{
		{Severity: Low, Rule: "R1", File: "b.sh", Line: 2},
		{Severity: Critical, Rule: "R2", File: "a.sh", Line: 9},
		{Severity: Low, Rule: "R1", File: "a.sh", Line: 1},
		{Severity: Info, Rule: "R3", File: "a.sh", Line: 5},
		{Severity: High, Rule: "R1", File: "c.sh"},
	}
	g := GroupFindings(fs, BySeverity, Info)
	keys := []string{}
	for _, x := range g {
		keys = append(keys, fmt.Sprintf("%s:%d", x.Key, len(x.Findings)))
	}
	if strings.Join(keys, " ") != "Critical:1 High:1 Low:2 Info:1" {
		t.Fatalf("by severity %v", keys)
	}
	if g[2].Findings[0].File != "a.sh" {
		t.Errorf("within group not sorted by file: %+v", g[2].Findings)
	}

	g = GroupFindings(fs, ByFile, Low)
	keys = keys[:0]
	for _, x := range g {
		keys = append(keys, fmt.Sprintf("%s:%d:%s", x.Key, len(x.Findings), x.Worst))
	}
	if strings.Join(keys, " ") != "a.sh:2:critical c.sh:1:high b.sh:1:low" {
		t.Fatalf("by file %v", keys)
	}

	g = GroupFindings(fs, ByRule, Medium)
	if len(g) != 2 || g[0].Key != "R2" || g[1].Key != "R1" || len(g[1].Findings) != 1 {
		t.Fatalf("by rule, min medium: %+v", g)
	}
	if c := Counts(fs); c[Low] != 2 || c[Critical] != 1 || c[Medium] != 0 {
		t.Fatalf("counts %v", c)
	}
}

func TestFingerprint(t *testing.T) {
	a := Finding{Rule: "SC2086", File: "x.sh", Line: 3, Message: "Double quote $1 at 3"}
	b := a
	b.Line, b.Col, b.Message = 40, 2, "double quote $1 at 41 "
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("fingerprint changed when the finding moved")
	}
	c := a
	c.File = "y.sh"
	d := a
	d.Rule = "SC2087"
	if a.Fingerprint() == c.Fingerprint() || a.Fingerprint() == d.Fingerprint() {
		t.Error("different findings share a fingerprint")
	}
	// Fields are separated, so moving text between them changes it.
	e := Finding{Rule: "ab", File: "c"}
	f := Finding{Rule: "a", File: "bc"}
	if e.Fingerprint() == f.Fingerprint() {
		t.Error("field boundary ignored")
	}
}

func TestParseSeverity(t *testing.T) {
	for in, want := range map[string]Severity{"CRITICAL": Critical, " Error ": High, "warning": Medium, "note": Low, "style": Info} {
		if got, ok := ParseSeverity(in); !ok || got != want {
			t.Errorf("ParseSeverity(%q) = %v %v", in, got, ok)
		}
	}
	if _, ok := ParseSeverity("purple"); ok {
		t.Error("purple accepted")
	}
}

func FuzzParsers(f *testing.F) {
	f.Add([]byte(`{"comments":[{"file":"a","line":1}]}`))
	f.Add([]byte(`{"runs":[{"results":[{"ruleIndex":-1}]}]}`))
	f.Add([]byte("warning[]=A|b|c|d|\nsuggestion[]="))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseShellcheckJSON1(data)
		_, _ = ParseSARIF(data)
		_ = ParseLynisReport(data)
	})
}
