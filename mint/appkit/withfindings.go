package appkit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/findings"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

// MixinFindings groups an audit or lint tool's findings by severity.
const MixinFindings MixinName = "WithFindings"

// Findings adapters (FindingsSpec.Adapter).
const (
	FindingsShellcheck = "shellcheck-json1" // shellcheck -f json1
	FindingsSARIF      = "sarif"            // SARIF 2.1: trivy, semgrep, hadolint, CodeQL…
	FindingsLynis      = "lynis-report"     // lynis's report file
	FindingsLines      = "lines"            // plain text, matched with Patterns
)

// FindingsSpec configures WithFindings.
type FindingsSpec struct {
	Adapter string `json:"adapter"`
	// ForceArgs make the tool print the adapter's format ({"-f",
	// "json1"}), unless the user chose a format with one of FormatFlags.
	// ReadableFlags are the user's choices that are still readable: a
	// flag ("--sarif", "--format=sarif") or a flag and value ("-f sarif").
	ForceArgs     []string `json:"force_args,omitempty"`
	FormatFlags   []string `json:"format_flags,omitempty"`
	ReadableFlags []string `json:"readable_flags,omitempty"`
	// ReportFile is where FindingsLynis reads from (default
	// /var/log/lynis-report.dat). It must be an absolute path.
	ReportFile string `json:"report_file,omitempty"`
	// Patterns are FindingsLines's rules, tried in order.
	Patterns []FindingPattern `json:"patterns,omitempty"`
}

// FindingPattern is one rule for FindingsLines: lines matching Regex are
// findings of Severity. Named groups file, line, col, rule and message
// fill in the finding.
type FindingPattern struct {
	Severity string `json:"severity"`
	Regex    string `json:"regex"`
}

const defaultLynisReport = "/var/log/lynis-report.dat"

// rawStdoutLimit bounds the machine-readable output kept for parsing.
const rawStdoutLimit = 64 << 20

// maxReportFile bounds a report file read from disk.
const maxReportFile = 16 << 20

var findingsDefaults = map[string]FindingsSpec{
	"shellcheck": {Adapter: FindingsShellcheck, ForceArgs: []string{"-f", "json1"},
		FormatFlags: []string{"-f", "--format"}, ReadableFlags: []string{"-f json1", "--format json1", "-fjson1", "--format=json1"}},
	"hadolint": {Adapter: FindingsSARIF, ForceArgs: []string{"-f", "sarif"},
		FormatFlags: []string{"-f", "--format"}, ReadableFlags: []string{"-f sarif", "--format sarif", "-fsarif", "--format=sarif"}},
	"trivy": {Adapter: FindingsSARIF, ForceArgs: []string{"--format", "sarif"},
		FormatFlags: []string{"-f", "--format"}, ReadableFlags: []string{"-f sarif", "--format sarif", "-fsarif", "--format=sarif"}},
	"semgrep": {Adapter: FindingsSARIF, ForceArgs: []string{"--sarif"},
		FormatFlags:   []string{"--sarif", "--json", "--text", "--emacs", "--vim", "--junit-xml", "--gitlab-sast", "--gitlab-secrets"},
		ReadableFlags: []string{"--sarif"}},
	"lynis": {Adapter: FindingsLynis, ReportFile: defaultLynisReport},
}

// KnownFindingsSpec returns WithFindings's settings for a tool Mint knows.
func KnownFindingsSpec(bin string) (FindingsSpec, bool) {
	f, ok := findingsDefaults[toolBase(bin)]
	if ok {
		f.ForceArgs = append([]string(nil), f.ForceArgs...)
		f.FormatFlags = append([]string(nil), f.FormatFlags...)
		f.ReadableFlags = append([]string(nil), f.ReadableFlags...)
	}
	return f, ok
}

func validateFindings(s *ToolSpec) error {
	f := s.Findings
	switch f.Adapter {
	case FindingsShellcheck, FindingsSARIF:
	case FindingsLynis:
		if f.ReportFile != "" && (!filepath.IsAbs(f.ReportFile) || filepath.Clean(f.ReportFile) != f.ReportFile) {
			return fmt.Errorf("report_file must be a clean absolute path, got %q", f.ReportFile)
		}
	case FindingsLines:
		if len(f.Patterns) == 0 {
			return errors.New("the lines adapter needs at least one pattern")
		}
		if _, err := compilePatterns(f.Patterns); err != nil {
			return err
		}
	case "":
		return errors.New("adapter is required")
	default:
		return fmt.Errorf("unknown adapter %q", f.Adapter)
	}
	for _, a := range f.ForceArgs {
		if a == "--" {
			return errors.New(`force_args can't contain "--"`)
		}
	}
	return nil
}

func compilePatterns(ps []FindingPattern) ([]findings.Pattern, error) {
	var out []findings.Pattern
	for i, p := range ps {
		sev, ok := findings.ParseSeverity(p.Severity)
		if !ok {
			return nil, fmt.Errorf("pattern %d: unknown severity %q", i+1, p.Severity)
		}
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			return nil, fmt.Errorf("pattern %d: %w", i+1, err)
		}
		out = append(out, findings.Pattern{Severity: sev, Re: re})
	}
	return out, nil
}

func init() {
	registerMixin(MixinInfo{
		Name: MixinFindings, Order: 102,
		Summary:  "group findings by severity (Audit: shellcheck, lynis, trivy, semgrep)",
		Fields:   "Findings *FindingsSpec",
		Enabled:  func(s *ToolSpec) bool { return s.Findings != nil },
		Validate: validateFindings,
	})
}

// findingsArgs adds WithFindings's ForceArgs, like diffArgs.
func (a *ToolApp) findingsArgs(args []string, at int) ([]string, bool) {
	f := a.Spec.Findings
	if f == nil {
		return args, false
	}
	if f.Adapter == FindingsLynis || f.Adapter == FindingsLines {
		return args, true // reads a file, or any text
	}
	return forceFormat(args, at, f.ForceArgs, f.FormatFlags, f.ReadableFlags)
}

// findingsRawLimit is how much stdout the runner keeps for parsing.
func (a *ToolApp) findingsRawLimit() int {
	if f := a.Spec.Findings; f != nil && (f.Adapter == FindingsShellcheck || f.Adapter == FindingsSARIF) {
		return rawStdoutLimit
	}
	return 0
}

// findingsReport is the parsed result of the last run.
type findingsReport struct {
	res     findings.Result
	err     error
	cutOff  bool // the output was longer than rawStdoutLimit
	minimum findings.Severity
	groupBy findings.GroupBy
}

// ParseFindings reads the findings of job's run.
func (a *ToolApp) ParseFindings(job *runner.Job) (findings.Result, error) {
	f := a.Spec.Findings
	switch f.Adapter {
	case FindingsShellcheck, FindingsSARIF:
		raw, cut := job.RawStdout()
		if cut {
			return findings.Result{}, fmt.Errorf("the report is larger than %d MB; run on fewer files", rawStdoutLimit>>20)
		}
		if f.Adapter == FindingsShellcheck {
			return findings.ParseShellcheckJSON1(raw)
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			return findings.Result{}, errors.New("the tool printed no report")
		}
		return findings.ParseSARIF(raw)
	case FindingsLynis:
		path := f.ReportFile
		if path == "" {
			path = defaultLynisReport
		}
		data, err := readReportFile(path)
		if err != nil {
			return findings.Result{}, err
		}
		return findings.ParseLynisReport(data), nil
	case FindingsLines:
		ps, err := compilePatterns(f.Patterns)
		if err != nil {
			return findings.Result{}, err
		}
		var text []string
		for _, l := range job.Lines() {
			text = append(text, l.Text)
		}
		return findings.ParseLines(text, ps), nil
	}
	return findings.Result{}, fmt.Errorf("unknown adapter %q", f.Adapter)
}

// readReportFile reads a report a tool wrote, refusing anything but a
// regular file (a symlink, a FIFO that would block, a device) and anything
// over maxReportFile.
func readReportFile(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%s can only be read as root", path)
		}
		return nil, fmt.Errorf("no report at %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file; not reading it", path)
	}
	fh, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%s can only be read as root", path)
		}
		return nil, err
	}
	defer fh.Close()
	if fi2, err := fh.Stat(); err != nil || !os.SameFile(fi, fi2) {
		return nil, fmt.Errorf("%s changed while opening it; not reading it", path)
	}
	data, err := io.ReadAll(io.LimitReader(fh, maxReportFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReportFile {
		return nil, fmt.Errorf("%s is larger than %d MB", path, maxReportFile>>20)
	}
	return data, nil
}

// findingsActionRow is the run screen's "View findings" action.
func (a *ToolApp) findingsActionRow() (mint.Row, bool) {
	if a.Spec.Findings == nil {
		return mint.Row{}, false
	}
	return mint.Action(mint.ActionOpts{
		Label: "View findings by severity…",
		Hint:  "findings",
		Get: func() string {
			a.mu.Lock()
			rep := a.findings
			a.mu.Unlock()
			if rep == nil {
				return ""
			}
			if rep.err != nil {
				return "could not read"
			}
			return countsSummary(rep.res.Findings)
		},
		OnActivate: func(c *mint.Ctx) {
			j := a.ActiveJob()
			switch {
			case j == nil:
				c.ToastWarn("Run " + a.Spec.Name + " first")
				return
			case j.Running():
				c.ToastWarn("Still running; try again when it finishes")
				return
			}
			a.mu.Lock()
			rep, ok := a.findings, a.findingsOK
			a.mu.Unlock()
			switch {
			case !ok:
				c.ToastWarn("The output format you chose can't be read as findings; see the log instead")
			case rep == nil:
				c.ToastWarn("No findings were read from this run")
			case rep.err != nil:
				c.ToastError("Could not read the findings: " + rep.err.Error())
			default:
				c.Push(a.FindingsScreen())
			}
		},
	}), true
}

// countsSummary is "2 high · 5 medium · 1 low", or "none".
func countsSummary(fs []findings.Finding) string {
	c := findings.Counts(fs)
	var parts []string
	for _, s := range findings.Severities() {
		if c[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c[s], s))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " · ")
}

// readFindings parses job's findings once it has finished; StartJob's
// OnExit calls it.
func (a *ToolApp) readFindings(job *runner.Job, readable bool) {
	if a.Spec.Findings == nil {
		return
	}
	rep := &findingsReport{minimum: findings.Info}
	if readable {
		rep.res, rep.err = a.ParseFindings(job)
	}
	a.mu.Lock()
	if old := a.findings; old != nil {
		rep.minimum, rep.groupBy = old.minimum, old.groupBy // keep the view's choices
	}
	a.findings = rep
	a.findingsOK = readable
	a.mu.Unlock()
}

// FindingsScreen lists the last run's findings in groups.
func (a *ToolApp) FindingsScreen() *mint.Screen {
	groupNames := []string{"severity", "file", "rule"}
	sevNames := []string{"info", "low", "medium", "high", "critical"}
	s := &mint.Screen{
		Icon:   "⚑",
		Title:  a.Spec.Name + " — findings",
		Status: a.statusBadges,
	}
	s.Refresh = func(scr *mint.Screen) {
		a.mu.Lock()
		rep := a.findings
		a.mu.Unlock()
		if rep == nil {
			scr.Sections = []mint.Section{{Rows: []mint.Row{mint.Note("No findings yet.")}}}
			return
		}
		all := rep.res.Findings
		controls := []mint.Row{
			mint.Value(mint.ValueOpts{Label: "Found", Get: func() string { return countsSummary(all) }}),
			mint.Picker(mint.PickerOpts{ID: "findings:group", Label: "Group by", Options: groupNames,
				Get: func() string { return groupNames[rep.groupBy] },
				Set: func(v string) {
					for i, n := range groupNames {
						if n == v {
							a.mu.Lock()
							rep.groupBy = findings.GroupBy(i)
							a.mu.Unlock()
						}
					}
				}}),
			mint.Picker(mint.PickerOpts{ID: "findings:min", Label: "Show at least", Options: sevNames,
				Get: func() string { return rep.minimum.String() },
				Set: func(v string) {
					if sev, ok := findings.ParseSeverity(v); ok {
						a.mu.Lock()
						rep.minimum = sev
						a.mu.Unlock()
					}
				}}),
		}
		if rep.res.Capped {
			controls = append(controls, mint.Label(mint.StyleCaption, fmt.Sprintf("Showing the first %d findings; the report has more.", findings.MaxFindings)))
		}
		sections := []mint.Section{{Name: "Findings", Rows: controls}}
		groups := findings.GroupFindings(all, rep.groupBy, rep.minimum)
		if len(groups) == 0 {
			msg := "No findings."
			if len(all) > 0 {
				msg = "No findings at this level or above."
			}
			sections = append(sections, mint.Section{Rows: []mint.Row{mint.Note(msg)}})
		}
		for gi, g := range groups {
			var rows []mint.Row
			for _, f := range g.Findings {
				f := f
				label := findingLabel(f, rep.groupBy)
				rows = append(rows, mint.Action(mint.ActionOpts{
					Label: label,
					Hint:  f.Severity.String(),
					OnActivate: func(c *mint.Ctx) {
						c.Push(a.findingDetail(f))
					},
				}))
			}
			title := mint.CleanLineVisible(g.Key)
			if rep.groupBy != findings.BySeverity {
				title += " · worst " + g.Worst.String()
			}
			// Open the worst groups; fold the rest when there are many.
			collapsed := gi > 0 && len(all) > 40
			sections = append(sections, mint.CollapsibleSection(fmt.Sprintf("%s (%d)", title, len(g.Findings)), collapsed, rows...))
		}
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}

// findingLabel is one finding as a row: a short location first (so it
// survives the row being cut), then the rule and message; the severity is
// the row's hint. The full path is
// in the detail view; grouping by file already names the file.
func findingLabel(f findings.Finding, by findings.GroupBy) string {
	var parts []string // the severity is the row's hint, on the right
	short := f
	if by == findings.ByFile {
		short.File = ""
		if f.Line > 0 {
			parts = append(parts, fmt.Sprintf("line %d", f.Line))
		}
	} else if f.File != "" {
		short.File = filepath.Base(f.File)
	}
	if loc := short.Location(); loc != "" {
		parts = append(parts, loc)
	}
	if by != findings.ByRule && f.Rule != "" {
		parts = append(parts, f.Rule)
	}
	parts = append(parts, firstLine(f.Message))
	return mint.CleanLineVisible(strings.Join(parts, "  "))
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// findingDetail shows everything about one finding.
func (a *ToolApp) findingDetail(f findings.Finding) *mint.Screen {
	return mint.ViewStyled(mint.ViewStyledOpts{
		Title:  a.Spec.Name + " — " + f.Severity.Title() + " " + f.Rule,
		Status: a.statusBadges,
		Lines: func(w int) []mint.StyledLine {
			return findingDetailLines(f, w)
		},
	})
}

func findingDetailLines(f findings.Finding, width int) []mint.StyledLine {
	sevRole := map[findings.Severity]mint.Role{findings.Critical: mint.RoleError, findings.High: mint.RoleError,
		findings.Medium: mint.RoleWarn, findings.Low: mint.RoleText, findings.Info: mint.RoleMuted}[f.Severity]
	var out []mint.StyledLine
	field := func(label, value string, role mint.Role) {
		if value == "" {
			return
		}
		for i, line := range wrap(value, max(20, width-14)) {
			l := ""
			if i == 0 {
				l = label
			}
			out = append(out, mint.StyledLine{Segs: []mint.Seg{
				{Text: fmt.Sprintf("%-12s", l), Role: mint.RoleMuted}, {Text: line, Role: role}}})
		}
	}
	field("Severity", f.Severity.Title(), sevRole)
	field("Rule", f.Rule, mint.RoleAccent)
	field("Where", f.Location(), mint.RoleText)
	out = append(out, mint.StyledLine{})
	field("Message", f.Message, mint.RoleText)
	if f.Fix != "" {
		out = append(out, mint.StyledLine{})
		field("Fix", f.Fix, mint.RoleAdded)
	}
	if f.Detail != "" {
		out = append(out, mint.StyledLine{})
		field("Details", f.Detail, mint.RoleText)
	}
	return out
}

// wrap splits s into lines of at most width cells, breaking at spaces
// where it can; existing line breaks are kept.
func wrap(s string, width int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			for mint.VLen(word) > width { // a word longer than the line
				if line != "" {
					out = append(out, line)
					line = ""
				}
				r := []rune(word)
				n := min(len(r), width)
				out = append(out, string(r[:n]))
				word = string(r[n:])
			}
			switch {
			case word == "":
			case line == "":
				line = word
			case mint.VLen(line)+1+mint.VLen(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return out
}
