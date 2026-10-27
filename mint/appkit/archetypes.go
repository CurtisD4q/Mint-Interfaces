package appkit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/fileplan"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// SubcommandSpec describes one tab or subcommand mode in an ArchetypeSuite tool
// (e.g., git status/log/branch/diff, docker ps/images/volumes, tar list/extract/create).
type SubcommandSpec struct {
	Name        string     `json:"name"`
	Label       string     `json:"label"`
	Desc        string     `json:"desc,omitempty"`
	Args        []string   `json:"args,omitempty"`
	TargetLabel string     `json:"target_label,omitempty"`
	Flags       []FlagSpec `json:"flags,omitempty"`
	Confirm     bool       `json:"confirm,omitempty"`
	ReadOnly    bool       `json:"read_only,omitempty"`
}

// RowActionSpec describes an action that can be invoked when the user presses Enter
// on a row in a Monitor, Config, or Explorer table (e.g., Restart unit, Kill PID).
type RowActionSpec struct {
	Label      string   `json:"label"`
	Hint       string   `json:"hint,omitempty"`
	Subcommand []string `json:"subcommand,omitempty"` // e.g. ["restart"] or ["kill", "-9"]
	Column     int      `json:"column,omitempty"`     // column index in the selected row (default 0)
	Confirm    bool     `json:"confirm,omitempty"`
}

var sectionHeadingRe = regexp.MustCompile(`^([A-Z][A-Z0-9 _/\-&()]{1,48}:?|\[\+\]|\[!\]|\[-\]|===+.*===+)$`)

// SplitSections implements the WithCollapsibleOutput mixin helper: it inspects lines
// for natural section headings (such as man-page ALL-CAPS headings or security audit
// markers) and groups them into foldable mint.Collapsible rows. If fewer than
// 2 headings are found, it falls back to chunked mint.CollapsibleLines.
func SplitSections(lines []string, chunkSize int) []mint.Row {
	if chunkSize <= 0 {
		chunkSize = 100
	}
	if len(lines) == 0 {
		return []mint.Row{
			mint.Note("  (no output yet)"),
		}
	}

	type secGroup struct {
		title string
		lines []string
	}
	var groups []secGroup
	var cur *secGroup

	for _, raw := range lines {
		cleaned := mint.CleanLine(raw)
		trimmed := strings.TrimSpace(cleaned)
		if len(cleaned) > 0 && cleaned[0] != ' ' && cleaned[0] != '\t' && sectionHeadingRe.MatchString(trimmed) && len(trimmed) >= 2 {
			groups = append(groups, secGroup{title: strings.TrimSuffix(trimmed, ":")})
			cur = &groups[len(groups)-1]
			continue
		}
		if cur == nil {
			groups = append(groups, secGroup{title: "Overview"})
			cur = &groups[len(groups)-1]
		}
		cur.lines = append(cur.lines, cleaned)
	}

	if len(groups) >= 2 {
		rows := make([]mint.Row, 0, len(groups))
		for i, g := range groups {
			bodyLines := append([]string(nil), g.lines...)
			openByDefault := i == 0
			rows = append(rows, mint.Collapsible(mint.CollapsibleOpts{
				ID:        "sec:" + g.title,
				Title:     g.title,
				Badge:     fmt.Sprintf("%d lines", len(bodyLines)),
				Collapsed: !openByDefault,
				Lines:     func() []string { return bodyLines },
				ChunkSize: chunkSize,
			}))
		}
		return rows
	}

	copied := append([]string(nil), lines...)
	return []mint.Row{
		mint.Collapsible(mint.CollapsibleOpts{
			ID:        "output:main",
			Title:     "Output",
			Badge:     fmt.Sprintf("%d lines", len(copied)),
			Collapsed: false,
			Lines:     func() []string { return copied },
			ChunkSize: chunkSize,
		}),
	}
}

// ResolveDynamicChoices runs ChoicesCmd for any FlagSpec that uses WithDynamicChoices
// and populates its Choices slice at runtime.
func (a *ToolApp) ResolveDynamicChoices() {
	for i := range a.Spec.Flags {
		f := &a.Spec.Flags[i]
		if len(f.ChoicesCmd) == 0 {
			continue
		}
		out, err := util.RunTimeout(choicesTimeout, f.ChoicesCmd[0], f.ChoicesCmd[1:]...)
		if err != nil || strings.TrimSpace(out) == "" {
			continue
		}
		choices := []Choice{{Label: "Default", Value: ""}}
		seen := map[string]bool{}
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) == 0 {
				continue
			}
			tok := strings.TrimSuffix(fields[0], ":")
			if tok != "" && !seen[tok] {
				seen[tok] = true
				choices = append(choices, Choice{Label: tok, Value: tok})
			}
		}
		if len(choices) > 1 {
			f.Choices = choices
			f.Kind = FlagChoice
		}
	}
}

// ErrNoStatusCommand is returned by RefreshState when the tool has no
// read-only way to show its state (no StatusArgs, and not ReadOnly).
var ErrNoStatusCommand = errors.New("no read-only status command is set for this tool (StatusArgs)")

// statusArgs returns the arguments of the command that only reads state:
// StatusArgs (plus the view flags and target for monitors; a config tool's
// flags are changes, so they are left out), or the tool's own command line
// when the tool is ReadOnly. ok is false when there is no such command, and
// then nothing may run without the user asking.
func (a *ToolApp) statusArgs() (args []string, ok bool) {
	switch {
	case len(a.Spec.StatusArgs) > 0:
		args = append(args, a.Spec.StatusArgs...)
		if a.Spec.Archetype != ArchetypeConfig {
			flagArgs, _ := a.buildFlagAndTargetArgs()
			args = append(args, flagArgs...)
		}
		return args, true
	case a.Spec.ReadOnly:
		args, _ = a.BuildArgs()
		return args, true
	}
	return nil, false
}

// RefreshState runs the tool's read-only status command (or extraArgs, when
// given) with a time limit and stores its output in SnapshotLines. It never
// falls back to the tool's main command, which may change things.
func (a *ToolApp) RefreshState(extraArgs ...string) error {
	args := extraArgs
	if len(args) == 0 {
		var ok bool
		if args, ok = a.statusArgs(); !ok {
			return ErrNoStatusCommand
		}
	}
	return a.runStatus(args)
}

func (a *ToolApp) runStatus(args []string) error {
	out, err := util.RunTimeout(statusTimeout, a.Spec.BinaryName(), args...)
	a.SetSnapshotText(out)
	if errors.Is(err, util.ErrTimeout) {
		return fmt.Errorf("%s did not finish within %s", a.Spec.Name, statusTimeout)
	}
	return err
}

// refreshAsync reads the state in the background so a slow (or never-ending)
// status command cannot freeze the screen. The arguments are worked out here,
// on the screen's goroutine; only the command runs in the background.
func (a *ToolApp) refreshAsync() {
	args, ok := a.statusArgs()
	if !ok {
		return
	}
	a.mu.Lock()
	if a.refreshing {
		a.mu.Unlock()
		return
	}
	a.refreshing = true
	a.mu.Unlock()
	go func() {
		err := a.runStatus(args)
		a.mu.Lock()
		a.refreshing, a.refreshed = false, true
		a.refreshErr = ""
		if err != nil {
			a.refreshErr = err.Error()
		}
		a.mu.Unlock()
	}()
}

// takeRefreshed reports whether a background refresh finished since the last
// call, so the screen knows to repaint.
func (a *ToolApp) takeRefreshed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.refreshed
	a.refreshed = false
	return r
}

// refreshRow is the "read the state now" action, or a note saying why there
// is none.
func (a *ToolApp) refreshRow(label string) mint.Row {
	if !a.Spec.CanReadState() {
		return mint.Note("No read-only status command is set for " + a.Spec.Name + " (StatusArgs), so its state is not read.")
	}
	return mint.Action(mint.ActionOpts{
		Label: label,
		Hint:  "refresh",
		OnActivate: func(c *mint.Ctx) {
			var err error
			c.Busy("Reading "+a.Spec.Name+" state…", func() { err = a.RefreshState() })
			if err != nil {
				c.ToastError(err.Error())
				return
			}
			c.Toast("Refreshed")
		},
	})
}

// SetSnapshotText stores raw multi-line command output into SnapshotLines.
func (a *ToolApp) SetSnapshotText(out string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.LastRefresh = time.Now()
	trimmed := strings.TrimRight(out, "\n")
	if trimmed == "" {
		a.SnapshotLines = nil
		return
	}
	rawLines := strings.Split(trimmed, "\n")
	lines := make([]string, len(rawLines))
	for i, l := range rawLines {
		lines[i] = mint.CleanLine(l)
	}
	a.SnapshotLines = lines
}

// ActiveLines returns the current job's text lines if a job has run, or the
// snapshot lines captured by RefreshState.
func (a *ToolApp) ActiveLines() []string {
	a.mu.Lock()
	job := a.activeJob
	snap := append([]string(nil), a.SnapshotLines...)
	a.mu.Unlock()
	if job != nil {
		if ll := job.Lines(); len(ll) > 0 {
			out := make([]string, len(ll))
			for i, l := range ll {
				out[i] = l.Text
			}
			return out
		}
	}
	return snap
}

func (a *ToolApp) buildOutputDisplaySection(title string) mint.Section {
	lines := a.ActiveLines()
	var rows []mint.Row

	if a.Spec.TableOutput && len(lines) > 0 {
		headers, tblRows := mint.ParseTable(lines)
		if len(a.Spec.TableColumns) > 0 && len(headers) == 0 {
			headers = a.Spec.TableColumns
		}
		if len(headers) > 0 && len(tblRows) > 0 {
			rows = append(rows, mint.Table(mint.TableOpts{
				ID:        "tbl:" + a.Spec.Name,
				Headers:   headers,
				Rows:      tblRows,
				MaxHeight: 20,
				OnSelect: func(c *mint.Ctx, rowIdx int, row []string) {
					a.handleTableRowSelect(c, rowIdx, headers, row)
				},
			}))
		}
	}

	if a.Spec.CollapsibleOutput && len(lines) > 0 {
		rows = append(rows, SplitSections(lines, a.Spec.ChunkSize)...)
	}

	if len(rows) == 0 {
		if len(lines) == 0 {
			rows = append(rows, mint.Note("  No output captured yet."))
		} else {
			rows = append(rows, SplitSections(lines, a.Spec.ChunkSize)...)
		}
	}

	rows = append(rows, mint.Action(mint.ActionOpts{
		Label: "Open full output viewer (search / scroll / save)…",
		Hint:  "view",
		OnActivate: func(c *mint.Ctx) {
			cur := a.ActiveLines()
			c.Push(mint.ViewLog(a.Spec.Name+" output", func() []mint.LogLine {
				ll := make([]mint.LogLine, len(cur))
				for i, s := range cur {
					ll[i] = mint.LogLine{Text: s}
				}
				return ll
			}))
		},
	}))

	return mint.Section{
		Name: title,
		Rows: rows,
	}
}

func (a *ToolApp) handleTableRowSelect(c *mint.Ctx, rowIdx int, headers, row []string) {
	summary := strings.Join(row, " · ")
	if len(a.Spec.RowActions) == 0 {
		var parts []string
		for i, cell := range row {
			if i < len(headers) && headers[i] != "" {
				parts = append(parts, headers[i]+": "+cell)
			} else {
				parts = append(parts, cell)
			}
		}
		c.Message(fmt.Sprintf("Row %d", rowIdx+1), strings.Join(parts, "\n"))
		return
	}

	var actionRows []mint.Row
	actionRows = append(actionRows, mint.Value(mint.ValueOpts{
		Label: "Selected row",
		Get:   func() string { return summary },
	}))
	for _, act := range a.Spec.RowActions {
		act := act
		actionRows = append(actionRows, mint.Action(mint.ActionOpts{
			Label: act.Label,
			Hint:  act.Hint,
			OnActivate: func(c *mint.Ctx) {
				if act.Confirm || a.Spec.Irreversible {
					if !c.Confirm("Confirm "+act.Label, fmt.Sprintf("Run %s on %s?", act.Label, summary)) {
						return
					}
				}
				col := act.Column
				if col < 0 || col >= len(row) {
					col = 0
				}
				target := ""
				if len(row) > col {
					target = row[col]
				}
				cmdArgs := append([]string(nil), act.Subcommand...)
				if target != "" {
					cmdArgs = append(cmdArgs, target)
				}
				var out string
				var err error
				c.Busy(act.Label+"…", func() {
					out, err = util.RunTimeout(actionTimeout, a.Spec.BinaryName(), cmdArgs...)
				})
				if err != nil {
					c.ToastError(fmt.Sprintf("%s failed: %v", act.Label, err))
				} else {
					c.Toast(act.Label + " completed")
				}
				if out != "" {
					a.SetSnapshotText(out)
				}
				c.Pop()
			},
		}))
	}

	c.Push(&mint.Screen{
		Title:    "Row Actions",
		Sections: []mint.Section{{Name: "Actions", Rows: actionRows}},
	})
}

func (a *ToolApp) rootAuthSection() *mint.Section {
	if !a.Spec.NeedsRoot {
		return nil
	}
	var rows []mint.Row
	if runningAsRoot() {
		rows = append(rows, mint.Note("Running as root: commands run with full rights."))
		if a.owner != nil {
			rows = append(rows, mint.Note("Settings, history and results are kept in your home and belong to you."))
		}
	} else {
		rows = append(rows,
			mint.Label(mint.StyleCaption, a.Spec.Name+" may need root for some actions. Mint never asks for your password: to run as root, quit and start the app with sudo, and sudo will ask."),
			mint.Value(mint.ValueOpts{Label: "Start as root with", Value: restartHint()}),
		)
	}
	return &mint.Section{Name: "Root access", Rows: rows}
}

func (a *ToolApp) executeFromUI(c *mint.Ctx) {
	if a.Spec.Preview != nil {
		var plan fileplan.Plan
		var err error
		c.Busy("Working out what will change…", func() { plan, err = a.MakePlan() })
		if err != nil {
			if !c.Confirm("Run without a preview?", "Mint couldn't work out what this run will change ("+err.Error()+"). Run it anyway?") {
				c.ToastWarn("Cancelled")
				return
			}
			a.confirmAndRun(c, "")
			return
		}
		c.Push(a.PreviewScreen(plan, func(c *mint.Ctx, plan fileplan.Plan) {
			a.confirmAndRun(c, planSummary(plan))
		}))
		return
	}
	a.confirmAndRun(c, "")
}

// confirmAndRun asks whatever the run needs (typed confirmation, a
// secret on the command line…) and starts it. summary, when a preview
// produced one, says what the run will change.
func (a *ToolApp) confirmAndRun(c *mint.Ctx, summary string) {
	needConfirm := a.Spec.ConfirmRun
	if a.Spec.Archetype == ArchetypeSuite {
		if sub := a.activeSubcommandSpec(); sub != nil && sub.Confirm {
			needConfirm = true
		}
	}
	switch {
	case a.Spec.Irreversible:
		// Destroys data or processes: make the user type what is affected.
		phrase := strings.TrimSpace(a.Target)
		if phrase == "" {
			phrase = a.Spec.BinaryName()
		}
		warning := "This cannot be undone: " + a.CommandPreview()
		if summary != "" {
			warning = "This cannot be undone. It will " + summary + ".\n" + a.CommandPreview()
		}
		if !c.TypeToConfirm("Run "+a.Spec.Name+"?", warning, phrase) {
			c.ToastWarn("Cancelled")
			return
		}
	case needConfirm:
		prompt := a.Spec.ConfirmPrompt
		if prompt == "" {
			prompt = "Execute: " + a.CommandPreview()
		}
		if !c.Confirm("Confirm Execution", prompt) {
			c.ToastWarn("Cancelled")
			return
		}
	}
	if labels := a.argvSecrets(); len(labels) > 0 && !a.argvOK {
		if !c.Confirm("Put the secret on the command line?", a.Spec.Name+" only accepts "+strings.Join(labels, ", ")+
			" as a command-line argument, where other users on this computer can see it (for example with ps) while it runs.") {
			c.ToastWarn("Cancelled")
			return
		}
		a.argvOK = true
	}
	if found := a.FreeTextSecrets(); len(found) > 0 {
		if fp := freeTextFingerprint(found); fp != a.freeTextOK {
			if !c.Confirm("Password in a text box?", a.freeTextWarning(found)) {
				c.ToastWarn("Cancelled")
				return
			}
			a.freeTextOK = fp
		}
	}
	if a.Spec.Transcript != nil {
		// The session is the job: its screen starts the program.
		c.Push(a.TranscriptScreen())
		return
	}
	if _, err := a.StartJob(); err != nil {
		c.ToastError(err.Error())
		return
	}
	if a.Spec.Service != nil {
		c.Push(a.ServiceScreen())
		return
	}
	if a.Spec.Follow != nil {
		c.Push(a.FollowScreen())
		return
	}
	c.Push(a.RunScreen())
}

// freeTextWarning explains the risk of a secret typed into a free-text box.
func (a *ToolApp) freeTextWarning(found []FreeTextSecret) string {
	kinds := make([]string, 0, len(found))
	seen := map[string]bool{}
	for _, s := range found {
		if !seen[s.Kind] {
			seen[s.Kind] = true
			kinds = append(kinds, s.Kind)
		}
	}
	msg := "The text you typed contains what looks like a secret (" + strings.Join(kinds, ", ") + "). " +
		"It goes on the command line, where other users on this computer can see it (for example with ps) while it runs. " +
		"Mint hides it on screen, in the output, and in saved settings and history."
	var safer []string
	for _, f := range a.Spec.Flags {
		if f.Kind == FlagSecret {
			label := f.Label
			if label == "" {
				label = f.Flag
			}
			safer = append(safer, label)
		}
	}
	if len(safer) > 0 {
		msg += " Safer: use the " + strings.Join(safer, " or ") + " option, which keeps it hidden."
	}
	return msg + " Run anyway?"
}

// ConfigScreen builds the ArchetypeConfig ("config") State Editor screen.
func (a *ToolApp) ConfigScreen() *mint.Screen {
	s := &mint.Screen{
		Icon:  "⚙",
		Title: a.Spec.Name + " — Configuration",
		Status: func() []mint.Badge {
			return a.statusBadges()
		},
	}
	s.Refresh = func(scr *mint.Screen) {
		var sections []mint.Section
		sections = append(sections, a.targetAndPresetSection("Configuration Target"))
		sections = append(sections, a.flagSections()...)
		if rootSec := a.rootAuthSection(); rootSec != nil {
			sections = append(sections, *rootSec)
		}
		sections = append(sections, mint.Section{
			Name: "Apply & Inspect",
			Rows: []mint.Row{
				mint.Value(mint.ValueOpts{
					Label: "Command preview",
					Get:   func() string { return a.CommandPreview() },
				}),
				mint.Action(mint.ActionOpts{
					Label: "Apply configuration (" + a.Spec.Name + ")",
					Hint:  "apply",
					OnActivate: func(c *mint.Ctx) {
						a.executeFromUI(c)
					},
				}),
				a.refreshRow("Refresh live state"),
			},
		})
		sections = append(sections, a.buildOutputDisplaySection("Current State"))
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}

// ScannerScreen builds the ArchetypeScanner ("scanner") Target -> Live Table + Log screen.
func (a *ToolApp) ScannerScreen() *mint.Screen {
	s := a.RunnerScreen()
	s.Icon = "◎"
	s.Title = a.Spec.Name + " — Scanner"
	origRefresh := s.Refresh
	s.Refresh = func(scr *mint.Screen) {
		origRefresh(scr)
		scr.Sections = append(scr.Sections, a.buildOutputDisplaySection("Discovered Targets & Ports"))
	}
	s.Refresh(s)
	return s
}

// MonitorScreen builds the ArchetypeMonitor ("monitor") Live Poller & Controller screen.
//
// It polls only when the tool has a read-only way to show its state
// (StatusArgs, or a ReadOnly tool such as ps or df). For anything else
// (pkill, kill, killall…) polling would repeat an action every few seconds,
// so the screen becomes a normal run screen with the usual confirmations.
// Polling runs in the background, so a slow command never freezes the screen.
func (a *ToolApp) MonitorScreen() *mint.Screen {
	interval := a.Spec.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	canPoll := a.Spec.CanReadState()
	var lastPoll time.Time

	s := &mint.Screen{
		Icon:  "◧",
		Title: a.Spec.Name + " — Live Monitor",
		Status: func() []mint.Badge {
			switch {
			case !canPoll:
				return a.statusBadges()
			case a.MonitorPaused:
				return []mint.Badge{{Text: "paused", Kind: mint.BadgeWarn}}
			}
			a.mu.Lock()
			errText := a.refreshErr
			a.mu.Unlock()
			if errText != "" {
				return []mint.Badge{{Text: "last poll failed", Kind: mint.BadgeError}}
			}
			return []mint.Badge{{Text: "polling " + interval.String(), Kind: mint.BadgeOK}}
		},
		Enter: func(scr *mint.Screen) {
			if canPoll && !a.MonitorPaused {
				a.refreshAsync()
				lastPoll = time.Now()
			}
		},
		Poll: func(scr *mint.Screen) bool {
			if canPoll && !a.MonitorPaused && time.Since(lastPoll) >= interval {
				lastPoll = time.Now()
				a.refreshAsync()
			}
			return a.takeRefreshed()
		},
	}

	s.Refresh = func(scr *mint.Screen) {
		controls := []mint.Row{
			mint.Text(mint.TextOpts{
				ID:          "monitor:target",
				Label:       a.Spec.TargetLabel,
				Placeholder: a.Spec.TargetPlaceholder,
				Get:         func() string { return a.Target },
				Set:         func(v string) { a.Target = strings.TrimSpace(v) },
			}),
		}
		if canPoll {
			controls = append(controls,
				mint.Toggle(mint.ToggleOpts{
					ID:    "monitor:pause",
					Label: "Pause live polling",
					Get:   func() bool { return a.MonitorPaused },
					Set:   func(v bool) { a.MonitorPaused = v },
				}),
				mint.Value(mint.ValueOpts{
					Label: "Poll interval",
					Get:   func() string { return interval.String() },
				}),
				mint.Value(mint.ValueOpts{
					Label: "Last snapshot",
					Get: func() string {
						a.mu.Lock()
						defer a.mu.Unlock()
						switch {
						case a.refreshErr != "":
							return a.refreshErr
						case a.LastRefresh.IsZero():
							return "never"
						}
						return a.LastRefresh.Format("15:04:05")
					},
				}),
				a.refreshRow("Poll now"),
			)
		} else {
			controls = append(controls,
				mint.Note(a.Spec.Name+" changes things when it runs, so it is not polled. Run it when you choose."),
				mint.Value(mint.ValueOpts{
					Label: "Command preview",
					Get:   func() string { return a.CommandPreview() },
				}),
				mint.Action(mint.ActionOpts{
					Label:      "Run " + a.Spec.Name,
					Hint:       "run",
					OnActivate: func(c *mint.Ctx) { a.executeFromUI(c) },
				}),
			)
		}
		var sections []mint.Section
		sections = append(sections, mint.Section{Name: "Monitor Controls", Rows: controls})
		sections = append(sections, a.flagSections()...)
		if a.Spec.OutputsFile {
			sections = append(sections, a.outputFileSection())
		}
		if rootSec := a.rootAuthSection(); rootSec != nil {
			sections = append(sections, *rootSec)
		}
		sections = append(sections, a.buildOutputDisplaySection("Live Telemetry & Rows"))
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}

// ExplorerScreen builds the ArchetypeExplorer ("explorer") Query -> Results -> Detail screen.
func (a *ToolApp) ExplorerScreen() *mint.Screen {
	s := &mint.Screen{
		Icon:  "⌕",
		Title: a.Spec.Name + " — Explorer",
		Status: func() []mint.Badge {
			return a.statusBadges()
		},
	}
	s.Refresh = func(scr *mint.Screen) {
		var sections []mint.Section
		sections = append(sections, mint.Section{
			Name: "Search & Query",
			Rows: []mint.Row{
				mint.Text(mint.TextOpts{
					ID:          "explorer:target",
					Label:       a.Spec.TargetLabel,
					Placeholder: a.Spec.TargetPlaceholder,
					Get:         func() string { return a.Target },
					Set:         func(v string) { a.Target = strings.TrimSpace(v) },
				}),
				mint.Value(mint.ValueOpts{
					Label: "Command preview",
					Get:   func() string { return a.CommandPreview() },
				}),
				mint.Action(mint.ActionOpts{
					Label: "Run query (" + a.Spec.Name + ")",
					Hint:  "query",
					OnActivate: func(c *mint.Ctx) {
						a.executeFromUI(c)
					},
				}),
			},
		})
		sections = append(sections, a.flagSections()...)
		if a.Spec.OutputsFile {
			sections = append(sections, a.outputFileSection())
		}
		if rootSec := a.rootAuthSection(); rootSec != nil {
			sections = append(sections, *rootSec)
		}
		sections = append(sections, a.buildOutputDisplaySection("Query Results"))
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}

func (a *ToolApp) activeSubcommandSpec() *SubcommandSpec {
	if len(a.Spec.Subcommands) == 0 {
		return nil
	}
	for i := range a.Spec.Subcommands {
		if a.Spec.Subcommands[i].Name == a.ActiveSubcommand {
			return &a.Spec.Subcommands[i]
		}
	}
	return &a.Spec.Subcommands[0]
}

// SuiteScreen builds the ArchetypeSuite ("suite") Multi-Subcommand Manager screen.
func (a *ToolApp) SuiteScreen() *mint.Screen {
	s := &mint.Screen{
		Icon:  "❖",
		Title: a.Spec.Name + " — Suite",
		Status: func() []mint.Badge {
			return a.statusBadges()
		},
	}
	s.Refresh = func(scr *mint.Screen) {
		subs := a.Spec.Subcommands
		if len(subs) == 0 {
			subs = []SubcommandSpec{{Name: "default", Label: "Default", Desc: "Run " + a.Spec.Name}}
		}
		subNames := make([]string, len(subs))
		for i, sub := range subs {
			subNames[i] = sub.Name
		}
		if a.ActiveSubcommand == "" {
			a.ActiveSubcommand = subs[0].Name
		}

		var sections []mint.Section
		sections = append(sections, mint.Section{
			Name: "Subcommand Tabs (ArchetypeSuite)",
			Rows: []mint.Row{
				mint.Picker(mint.PickerOpts{
					ID:      "suite:subcommand",
					Label:   "Subcommand mode",
					Options: subNames,
					Get:     func() string { return a.ActiveSubcommand },
					Set:     func(v string) { a.ActiveSubcommand = v },
				}),
				mint.Value(mint.ValueOpts{
					Label: "Mode description",
					Get: func() string {
						if sub := a.activeSubcommandSpec(); sub != nil && sub.Desc != "" {
							return sub.Desc
						}
						return a.Spec.Desc
					},
				}),
				mint.Text(mint.TextOpts{
					ID:          "suite:target",
					Label:       a.Spec.TargetLabel,
					Placeholder: a.Spec.TargetPlaceholder,
					Get:         func() string { return a.Target },
					Set:         func(v string) { a.Target = strings.TrimSpace(v) },
				}),
			},
		})

		sections = append(sections, a.flagSections()...)
		if a.Spec.OutputsFile {
			sections = append(sections, a.outputFileSection())
		}
		if rootSec := a.rootAuthSection(); rootSec != nil {
			sections = append(sections, *rootSec)
		}
		sections = append(sections, mint.Section{
			Name: "Execute Subcommand",
			Rows: []mint.Row{
				mint.Value(mint.ValueOpts{
					Label: "Command preview",
					Get:   func() string { return a.CommandPreview() },
				}),
				mint.Action(mint.ActionOpts{
					Label: "Run subcommand",
					Hint:  "run",
					OnActivate: func(c *mint.Ctx) {
						a.executeFromUI(c)
					},
				}),
			},
		})
		sections = append(sections, a.buildOutputDisplaySection("Subcommand Output"))
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}
