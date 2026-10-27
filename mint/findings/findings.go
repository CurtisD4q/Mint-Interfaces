// Package findings reads the reports of audit and lint tools (shellcheck,
// SARIF producers such as trivy, semgrep and hadolint, or plain text) into
// findings on one severity scale, and groups them for display.
//
// It parses only. Every string in a Finding comes from the tool and is
// untrusted: clean it (mint.CleanLineVisible) before showing it, and
// never use File to open anything without confining it first.
package findings

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Severity is how serious a finding is. Higher is worse.
type Severity int

const (
	Info Severity = iota
	Low
	Medium
	High
	Critical
)

// Severities lists every severity, worst first.
func Severities() []Severity { return []Severity{Critical, High, Medium, Low, Info} }

func (s Severity) String() string {
	switch s {
	case Critical:
		return "critical"
	case High:
		return "high"
	case Medium:
		return "medium"
	case Low:
		return "low"
	}
	return "info"
}

// Title is the severity for headings: "High".
func (s Severity) Title() string {
	n := s.String()
	return strings.ToUpper(n[:1]) + n[1:]
}

// ParseSeverity reads a severity name in any case, with common synonyms.
func ParseSeverity(s string) (Severity, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "crit", "fatal":
		return Critical, true
	case "high", "error", "err":
		return High, true
	case "medium", "moderate", "warning", "warn":
		return Medium, true
	case "low", "note", "suggestion":
		return Low, true
	case "info", "information", "style", "none", "unknown":
		return Info, true
	}
	return Info, false
}

// Finding is one problem a tool reported.
type Finding struct {
	Severity  Severity
	Rule      string // the tool's rule or test ID: "SC2086", "AUTH-9286"
	Message   string
	File      string // as the tool reported it; untrusted
	Line, Col int    // from 1; 0 if not given
	Detail    string // extra text: the fix, a link, details
	Fix       string // a suggested change, if the tool gave one
}

// Location is "file:line:col", leaving out what is not known.
func (f Finding) Location() string {
	if f.File == "" {
		return ""
	}
	switch {
	case f.Line > 0 && f.Col > 0:
		return fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Col)
	case f.Line > 0:
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

var digits = regexp.MustCompile(`\d+`)

// Fingerprint identifies a finding across runs: its rule, file and
// message with numbers taken out, so it survives lines moving and counts
// changing. It is not affected by Line or Col.
func (f Finding) Fingerprint() string {
	h := sha256.New()
	for _, part := range []string{f.Rule, f.File, digits.ReplaceAllString(strings.ToLower(strings.TrimSpace(f.Message)), "#")} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// GroupBy is how findings are grouped.
type GroupBy int

const (
	BySeverity GroupBy = iota
	ByFile
	ByRule
)

func (g GroupBy) String() string {
	switch g {
	case ByFile:
		return "file"
	case ByRule:
		return "rule"
	}
	return "severity"
}

// Group is a set of findings with the same severity, file or rule.
type Group struct {
	Key      string
	Worst    Severity
	Findings []Finding
}

// GroupFindings groups fs, keeping those at least min. Severity groups
// come worst first; file and rule groups come worst first, then by name.
// Findings within a group come worst first, then by location.
func GroupFindings(fs []Finding, by GroupBy, min Severity) []Group {
	idx := map[string]int{}
	var groups []Group
	for _, f := range fs {
		if f.Severity < min {
			continue
		}
		var key string
		switch by {
		case ByFile:
			key = f.File
			if key == "" {
				key = "(no file)"
			}
		case ByRule:
			key = f.Rule
			if key == "" {
				key = "(no rule)"
			}
		default:
			key = f.Severity.Title()
		}
		i, ok := idx[key]
		if !ok {
			i = len(groups)
			idx[key] = i
			groups = append(groups, Group{Key: key, Worst: f.Severity})
		}
		g := &groups[i]
		g.Findings = append(g.Findings, f)
		if f.Severity > g.Worst {
			g.Worst = f.Severity
		}
	}
	for i := range groups {
		sortFindings(groups[i].Findings)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Worst != groups[j].Worst {
			return groups[i].Worst > groups[j].Worst
		}
		return groups[i].Key < groups[j].Key
	})
	return groups
}

func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
}

// Counts returns how many findings there are of each severity.
func Counts(fs []Finding) map[Severity]int {
	c := map[Severity]int{}
	for _, f := range fs {
		c[f.Severity]++
	}
	return c
}

// MaxFindings bounds how many findings a parser returns, so a huge report
// can't make the interface unusable. Parsers stop at this many and report
// that they did.
const MaxFindings = 20000
