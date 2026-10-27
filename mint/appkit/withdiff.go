package appkit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/diff"
)

// MixinDiff shows the differences a compare tool reports side by side.
const MixinDiff MixinName = "WithDiff"

// Diff output formats (DiffSpec.Format).
const (
	DiffUnified  = "unified"   // diff -u and anything that prints unified diffs
	DiffCmpBytes = "cmp-bytes" // cmp -l
)

// DiffSpec configures WithDiff.
type DiffSpec struct {
	Format string `json:"format"` // DiffUnified or DiffCmpBytes
	// ForceArgs are added so the tool prints a format Mint can read
	// ({"-u"} for diff, {"-l"} for cmp), unless the user has already
	// chosen an output format with one of FormatFlags.
	ForceArgs []string `json:"force_args,omitempty"`
	// FormatFlags are the tool's own output-format options. If any is in
	// use, ForceArgs are left out and Mint shows the output as it is.
	FormatFlags []string `json:"format_flags,omitempty"`
	// ReadableFlags are the format options whose output the view reads
	// ("-u", "-U", "--unified" for diff), besides ForceArgs themselves.
	ReadableFlags []string `json:"readable_flags,omitempty"`
	LeftLabel     string   `json:"left_label,omitempty"` // "" = the file's name
	RightLabel    string   `json:"right_label,omitempty"`
}

// diffDefaults are the settings for compare tools Mint knows.
var diffDefaults = map[string]DiffSpec{
	"diff": {Format: DiffUnified, ForceArgs: []string{"-u"}, ReadableFlags: []string{"-u", "-U", "--unified"},
		FormatFlags: []string{"-u", "-U", "--unified", "-c", "-C", "--context", "-y", "--side-by-side",
			"-e", "--ed", "-n", "--rcs", "-q", "--brief", "--normal", "-D", "--ifdef",
			"--old-line-format", "--new-line-format", "--unchanged-line-format", "--line-format",
			"--old-group-format", "--new-group-format", "--changed-group-format", "--unchanged-group-format"}},
	"cmp": {Format: DiffCmpBytes, ForceArgs: []string{"-l"}, ReadableFlags: []string{"-l", "--verbose"},
		FormatFlags: []string{"-l", "--verbose", "-s", "--quiet", "--silent", "-b", "--print-bytes"}},
}

// KnownDiffSpec returns WithDiff's settings for a compare tool Mint knows.
func KnownDiffSpec(bin string) (DiffSpec, bool) {
	d, ok := diffDefaults[toolBase(bin)]
	if ok {
		d.ForceArgs = append([]string(nil), d.ForceArgs...)
		d.FormatFlags = append([]string(nil), d.FormatFlags...)
		d.ReadableFlags = append([]string(nil), d.ReadableFlags...)
	}
	return d, ok
}

func validateDiff(s *ToolSpec) error {
	switch s.Diff.Format {
	case DiffUnified, DiffCmpBytes:
	case "":
		return errors.New("format is required (unified or cmp-bytes)")
	default:
		return fmt.Errorf("unknown format %q (want unified or cmp-bytes)", s.Diff.Format)
	}
	for _, a := range s.Diff.ForceArgs {
		if !strings.HasPrefix(a, "-") {
			return fmt.Errorf("force_args must be options, got %q", a)
		}
	}
	return nil
}

func init() {
	registerMixin(MixinInfo{
		Name: MixinDiff, Order: 101,
		Summary:  "show differences side by side (Compare: diff, cmp)",
		Fields:   "Diff *DiffSpec",
		Enabled:  func(s *ToolSpec) bool { return s.Diff != nil },
		Validate: validateDiff,
	})
}

// diffArgs adds WithDiff's ForceArgs to args (at index at, after any
// subcommand words) unless the user chose an output format, and reports
// whether the output will be readable by the differences view.
func (a *ToolApp) diffArgs(args []string, at int) (out []string, readable bool) {
	d := a.Spec.Diff
	if d == nil {
		return args, false
	}
	return forceFormat(args, at, d.ForceArgs, d.FormatFlags, d.ReadableFlags)
}

// diffReadable reports whether the last run printed a format the
// differences view can read.
func (a *ToolApp) diffReadable() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.diffOK
}

// DiffLines lays out lines (the tool's output) for the differences view.
func (a *ToolApp) DiffLines(lines []mint.LogLine, width int, mode diff.Mode, showSpace bool) []mint.StyledLine {
	text := make([]string, 0, len(lines))
	for _, l := range lines {
		text = append(text, l.Text)
	}
	d := a.Spec.Diff
	if d.Format == DiffCmpBytes {
		return diff.LayoutCmp(diff.ParseCmp(text), d.LeftLabel, d.RightLabel)
	}
	return diff.Layout(diff.ParseUnified(text), width, diff.Options{
		Mode: mode, LeftLabel: d.LeftLabel, RightLabel: d.RightLabel, ShowSpace: showSpace})
}

// DiffScreen shows the active job's output as differences.
func (a *ToolApp) DiffScreen() *mint.Screen {
	mode := diff.Auto
	showSpace := false
	var hunks, files, added, removed int
	job := a.ActiveJob()
	lines := func() []mint.LogLine {
		if job == nil {
			return nil
		}
		return job.Lines()
	}
	if a.Spec.Diff.Format == DiffUnified && job != nil {
		var text []string
		for _, l := range job.Lines() {
			text = append(text, l.Text)
		}
		for _, f := range diff.ParseUnified(text) {
			files++
			hunks += len(f.Hunks)
			added += f.Added()
			removed += f.Removed()
		}
	}
	modeName := func() string {
		switch mode {
		case diff.SideBySide:
			return "side by side"
		case diff.Unified:
			return "unified"
		}
		return "auto"
	}
	return mint.ViewStyled(mint.ViewStyledOpts{
		Title: a.Spec.Name + " — differences",
		Lines: func(w int) []mint.StyledLine { return a.DiffLines(lines(), w, mode, showSpace) },
		Crumb: func(anchor, anchors int) string {
			if a.Spec.Diff.Format != DiffUnified {
				return ""
			}
			pos := ""
			if anchors > 0 {
				pos = fmt.Sprintf("%d of %d · ", anchor+1, anchors)
			}
			return fmt.Sprintf("%s%d files · %d hunks · +%d −%d · %s", pos, files, hunks, added, removed, modeName())
		},
		Keys: []mint.StyledKey{
			{Key: "v", Hint: "v layout", Do: func() bool { mode = (mode + 1) % 3; return true }},
			{Key: "w", Hint: "w spaces", Do: func() bool { showSpace = !showSpace; return true }},
		},
		Status: a.statusBadges,
	})
}

// diffActionRow is the run screen's "View differences" action.
func (a *ToolApp) diffActionRow() (mint.Row, bool) {
	if a.Spec.Diff == nil {
		return mint.Row{}, false
	}
	return mint.Action(mint.ActionOpts{
		Label: "View differences side by side…",
		Hint:  "diff",
		OnActivate: func(c *mint.Ctx) {
			j := a.ActiveJob()
			switch {
			case j == nil:
				c.ToastWarn("Run " + a.Spec.Name + " first")
			case j.Running():
				c.ToastWarn("Still comparing; try again when it finishes")
			case !a.diffReadable():
				c.ToastWarn("The output format you chose can't be shown side by side; see the log instead")
			default:
				c.Push(a.DiffScreen())
			}
		},
	}), true
}
