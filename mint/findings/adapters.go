package findings

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Result is what a parser found.
type Result struct {
	Findings []Finding
	// Capped is set when there were more than MaxFindings.
	Capped bool
}

func (r *Result) add(f Finding) bool {
	if len(r.Findings) >= MaxFindings {
		r.Capped = true
		return false
	}
	r.Findings = append(r.Findings, f)
	return true
}

// ParseShellcheckJSON1 reads shellcheck -f json1 output.
func ParseShellcheckJSON1(data []byte) (Result, error) {
	var doc struct {
		Comments []struct {
			File    string `json:"file"`
			Line    int    `json:"line"`
			Column  int    `json:"column"`
			Level   string `json:"level"`
			Code    int    `json:"code"`
			Message string `json:"message"`
			Fix     *struct {
				Replacements []struct {
					Line, Column, EndLine, EndColumn int
					Replacement                      string
				} `json:"replacements"`
			} `json:"fix"`
		} `json:"comments"`
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return Result{}, nil // no problems: shellcheck may print nothing
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return Result{}, fmt.Errorf("shellcheck output is not json1: %w", err)
	}
	var r Result
	for _, c := range doc.Comments {
		sev := map[string]Severity{"error": High, "warning": Medium, "info": Low, "style": Info}[c.Level]
		f := Finding{Severity: sev, Rule: fmt.Sprintf("SC%d", c.Code), Message: c.Message,
			File: c.File, Line: c.Line, Col: c.Column,
			Detail: fmt.Sprintf("https://www.shellcheck.net/wiki/SC%d", c.Code)}
		if c.Fix != nil && len(c.Fix.Replacements) > 0 {
			var parts []string
			for _, rep := range c.Fix.Replacements {
				parts = append(parts, fmt.Sprintf("line %d col %d–%d: %q", rep.Line, rep.Column, rep.EndColumn, rep.Replacement))
			}
			f.Fix = strings.Join(parts, "; ")
		}
		if !r.add(f) {
			break
		}
	}
	return r, nil
}

// ParseSARIF reads a SARIF 2.1 log, as written by trivy, semgrep, hadolint,
// CodeQL and others.
func ParseSARIF(data []byte) (Result, error) {
	type rule struct {
		ID      string `json:"id"`
		Default struct {
			Level string `json:"level"`
		} `json:"defaultConfiguration"`
		Help struct {
			Text string `json:"text"`
		} `json:"help"`
		HelpURI    string         `json:"helpUri"`
		Properties map[string]any `json:"properties"`
	}
	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []rule `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID    string `json:"ruleId"`
				RuleIndex *int   `json:"ruleIndex"`
				Level     string `json:"level"`
				Message   struct {
					Text string `json:"text"`
				} `json:"message"`
				Locations []struct {
					Physical struct {
						Artifact struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine   int `json:"startLine"`
							StartColumn int `json:"startColumn"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
				Properties map[string]any `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &doc); err != nil {
		return Result{}, fmt.Errorf("not a SARIF log: %w", err)
	}
	if doc.Runs == nil {
		return Result{}, errors.New("not a SARIF log: no runs")
	}
	var r Result
	for _, run := range doc.Runs {
		rules := map[string]rule{}
		for _, ru := range run.Tool.Driver.Rules {
			rules[ru.ID] = ru
		}
		for _, res := range run.Results {
			ru, ok := rules[res.RuleID]
			if !ok && res.RuleIndex != nil && *res.RuleIndex >= 0 && *res.RuleIndex < len(run.Tool.Driver.Rules) {
				ru = run.Tool.Driver.Rules[*res.RuleIndex]
			}
			f := Finding{Rule: res.RuleID, Message: res.Message.Text, Severity: sarifSeverity(res.Level, ru.Default.Level, res.Properties, ru.Properties)}
			if f.Rule == "" {
				f.Rule = ru.ID
			}
			if len(res.Locations) > 0 {
				loc := res.Locations[0].Physical
				f.File = uriPath(loc.Artifact.URI)
				f.Line, f.Col = loc.Region.StartLine, loc.Region.StartColumn
			}
			f.Detail = strings.TrimSpace(ru.Help.Text)
			if ru.HelpURI != "" {
				f.Detail = strings.TrimSpace(f.Detail + "\n" + ru.HelpURI)
			}
			if !r.add(f) {
				return r, nil
			}
		}
	}
	return r, nil
}

// sarifSeverity prefers a numeric security-severity (CVSS-like, 0–10),
// then the result's level, then the rule's default level.
func sarifSeverity(level, ruleLevel string, props ...map[string]any) Severity {
	for _, p := range props {
		if v, ok := p["security-severity"]; ok {
			var n float64
			switch x := v.(type) {
			case string:
				n, _ = strconv.ParseFloat(x, 64)
			case float64:
				n = x
			}
			switch {
			case n >= 9:
				return Critical
			case n >= 7:
				return High
			case n >= 4:
				return Medium
			case n > 0:
				return Low
			}
		}
	}
	if level == "" {
		level = ruleLevel
	}
	switch level {
	case "error":
		return High
	case "warning", "":
		return Medium // SARIF's default level is warning
	case "note":
		return Low
	}
	return Info
}

// uriPath turns a SARIF artifact URI into a path: file:///a/b → /a/b,
// percent-escapes decoded. Other schemes are kept as they are.
func uriPath(uri string) string {
	if u, err := url.Parse(uri); err == nil && (u.Scheme == "file" || u.Scheme == "") {
		if u.Path != "" {
			return u.Path
		}
	}
	return uri
}

// Pattern turns matching lines of plain text output into findings.
type Pattern struct {
	Severity Severity
	// Re must match the line. Named groups fill the finding: file, line,
	// col, rule, message. Without a message group the whole line is the
	// message.
	Re *regexp.Regexp
}

// ParseLines applies patterns to text output, the first that matches a
// line deciding its severity. Lines nothing matches are skipped.
func ParseLines(lines []string, patterns []Pattern) Result {
	var r Result
	for _, line := range lines {
		for _, p := range patterns {
			m := p.Re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			f := Finding{Severity: p.Severity, Message: strings.TrimSpace(line)}
			for i, name := range p.Re.SubexpNames() {
				if i == 0 || name == "" || i >= len(m) {
					continue
				}
				switch name {
				case "file":
					f.File = m[i]
				case "line":
					f.Line, _ = strconv.Atoi(m[i])
				case "col":
					f.Col, _ = strconv.Atoi(m[i])
				case "rule":
					f.Rule = m[i]
				case "message":
					f.Message = strings.TrimSpace(m[i])
				}
			}
			if !r.add(f) {
				return r
			}
			break
		}
	}
	return r
}

// ParseLynisReport reads lynis's report file (lynis-report.dat): its
// warning[] lines become High findings and suggestion[] lines Low ones.
func ParseLynisReport(data []byte) Result {
	var r Result
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		var sev Severity
		switch key {
		case "warning[]":
			sev = High
		case "suggestion[]":
			sev = Low
		default:
			continue
		}
		// TEST-ID|text|details|solution|
		parts := strings.Split(val, "|")
		for len(parts) < 4 {
			parts = append(parts, "")
		}
		f := Finding{Severity: sev, Rule: parts[0], Message: parts[1]}
		var extra []string
		for _, p := range parts[2:4] {
			if p = strings.TrimSpace(p); p != "" && p != "-" {
				extra = append(extra, p)
			}
		}
		f.Detail = strings.Join(extra, "\n")
		if !r.add(f) {
			break
		}
	}
	return r
}
