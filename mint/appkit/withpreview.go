package appkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/fileplan"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

// MixinPreview shows what a run would change before it runs.
const MixinPreview MixinName = "WithPreview"

// Preview adapters (PreviewSpec.Adapter).
const (
	PreviewCoreutils = "coreutils" // Mint works it out itself (rm, cp, mv)
	PreviewRsync     = "rsync"     // rsync --dry-run --itemize-changes
	PreviewGitClean  = "git-clean" // git clean --dry-run
)

// PreviewSpec configures WithPreview.
type PreviewSpec struct {
	Adapter string `json:"adapter"`
	// DryRunArgs are added, after any subcommand words, to run the tool's
	// own dry run (rsync, git clean). Mint's own planner needs none.
	DryRunArgs []string `json:"dry_run_args,omitempty"`
}

var previewDefaults = map[string]PreviewSpec{
	"rm":    {Adapter: PreviewCoreutils},
	"cp":    {Adapter: PreviewCoreutils},
	"mv":    {Adapter: PreviewCoreutils},
	"rsync": {Adapter: PreviewRsync, DryRunArgs: []string{"--dry-run", "--itemize-changes"}},
}

// KnownPreviewSpec returns WithPreview's settings for a tool Mint knows.
func KnownPreviewSpec(bin string) (PreviewSpec, bool) {
	p, ok := previewDefaults[toolBase(bin)]
	p.DryRunArgs = append([]string(nil), p.DryRunArgs...)
	return p, ok
}

func validatePreview(s *ToolSpec) error {
	p := s.Preview
	switch p.Adapter {
	case PreviewCoreutils:
		switch toolBase(s.BinaryName()) {
		case "rm", "cp", "mv":
		default:
			return fmt.Errorf("Mint's own planner knows rm, cp and mv, not %s", s.BinaryName())
		}
	case PreviewRsync, PreviewGitClean:
		if len(p.DryRunArgs) == 0 {
			return errors.New("dry_run_args are needed to run the tool's own dry run")
		}
	case "":
		return errors.New("adapter is required")
	default:
		return fmt.Errorf("unknown adapter %q", p.Adapter)
	}
	for _, a := range p.DryRunArgs {
		if !strings.HasPrefix(a, "-") {
			return fmt.Errorf("dry_run_args must be options, got %q", a)
		}
	}
	return nil
}

func init() {
	registerMixin(MixinInfo{
		Name: MixinPreview, Order: 103,
		Summary:  "show what a run would change before it runs (Edit files: rm, cp, mv, rsync)",
		Fields:   "Preview *PreviewSpec",
		Enabled:  func(s *ToolSpec) bool { return s.Preview != nil },
		Validate: validatePreview,
	})
}

// dryRunTimeout bounds a tool's own dry run.
var dryRunTimeout = 2 * time.Minute

// MakePlan works out what running the tool now would change.
func (a *ToolApp) MakePlan() (fileplan.Plan, error) {
	args, _ := a.BuildArgs()
	wd, _ := os.Getwd()
	switch a.Spec.Preview.Adapter {
	case PreviewCoreutils:
		home, _ := os.UserHomeDir()
		if inv := sudoInvoker(); inv != nil {
			home = inv.home // under sudo, "home" means the user's, not root's
		}
		return fileplan.For(a.Spec.BinaryName(), args, fileplan.Env{Dir: wd, Home: home})
	case PreviewRsync, PreviewGitClean:
		at := a.argPrefixLen()
		dry := make([]string, 0, len(args)+len(a.Spec.Preview.DryRunArgs))
		dry = append(dry, args[:at]...)
		dry = append(dry, a.Spec.Preview.DryRunArgs...)
		dry = append(dry, args[at:]...)
		job := runner.Start(runner.Cmd{Name: a.Spec.BinaryName(), Args: dry, Env: a.secretEnv(false),
			Timeout: dryRunTimeout, Redact: a.newRunMasker().Redact})
		job.Wait()
		if job.State() != runner.Done {
			msg := job.LastError()
			if msg == "" {
				msg = job.State().String()
			}
			return fileplan.Plan{}, fmt.Errorf("the dry run failed: %s", msg)
		}
		var lines []string
		for _, l := range job.Lines() {
			if !l.Err {
				lines = append(lines, l.Text)
			}
		}
		if a.Spec.Preview.Adapter == PreviewRsync {
			return fileplan.ParseRsyncItemize(lines), nil
		}
		return fileplan.ParseGitClean(lines), nil
	}
	return fileplan.Plan{}, fmt.Errorf("unknown adapter %q", a.Spec.Preview.Adapter)
}

// planSummary is "delete 1,204 files and 3 folders (3.2 GB) · overwrite 2".
func planSummary(p fileplan.Plan) string {
	var files, dirs int
	for _, e := range p.Effects {
		if e.Kind == fileplan.Delete {
			if e.IsDir {
				dirs++
			} else {
				files++
			}
		}
	}
	t := p.Totals()
	var parts []string
	if files+dirs > 0 {
		s := "delete " + countNoun(files, "file", "files")
		if dirs > 0 {
			s += " and " + countNoun(dirs, "folder", "folders")
		}
		parts = append(parts, s)
	}
	for _, k := range []fileplan.Kind{fileplan.Overwrite, fileplan.Move, fileplan.Create, fileplan.Keep, fileplan.Fail} {
		if n := t.Count[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %s", k, commas(n)))
		}
	}
	s := strings.Join(parts, " · ")
	if s == "" {
		s = "no changes"
	}
	if t.BytesRemoved > 0 {
		s += " (" + humanBytes(t.BytesRemoved) + " removed or replaced)"
	}
	if p.Incomplete {
		s = "at least: " + s
	}
	return s
}

func countNoun(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return commas(n) + " " + many
}

func commas(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// maxShownPerKind bounds the rows per group; the counts stay exact.
const maxShownPerKind = 500

// PreviewScreen shows plan and offers to run. run is called, after the
// plan is checked again, to go on to the usual confirmation and start.
func (a *ToolApp) PreviewScreen(plan fileplan.Plan, run func(c *mint.Ctx, plan fileplan.Plan)) *mint.Screen {
	s := &mint.Screen{Icon: "◩", Title: a.Spec.Name + " — what will change", Status: a.statusBadges}
	s.Refresh = func(scr *mint.Screen) {
		var head []mint.Row
		head = append(head,
			mint.Value(mint.ValueOpts{Label: "Command", Get: func() string { return a.CommandPreview() }}),
			mint.Label(mint.StyleHeading, "Will "+planSummary(plan)),
		)
		if a.Spec.Preview.Adapter == PreviewCoreutils {
			head = append(head, mint.Label(mint.StyleCaption, "Mint's estimate, from how "+plan.Tool+" works: it is not "+plan.Tool+"'s own output. Files can change before it runs; Mint checks again just before."))
		}
		if len(plan.Unknown) > 0 {
			head = append(head, mint.Label(mint.StyleCaption, "Options this preview doesn't understand: "+mint.CleanLineVisible(strings.Join(plan.Unknown, " "))+". The result may differ."))
		}
		sections := []mint.Section{{Name: "Preview", Rows: head}}

		if len(plan.Warnings) > 0 {
			var rows []mint.Row
			for _, w := range plan.Warnings {
				prefix := "Note: "
				if w.Blocking {
					prefix = "⚠ "
				}
				rows = append(rows, mint.Label(mint.StyleBody, prefix+mint.CleanLineVisible(w.Text)))
			}
			sections = append(sections, mint.Section{Name: "Check before running", Rows: rows})
		}

		var acts []mint.Row
		if words := splitShellWords(a.Target); hasPattern(words) {
			acts = append(acts, mint.Action(mint.ActionOpts{
				Label: "Expand patterns into the files they match",
				Hint:  "expand",
				OnActivate: func(c *mint.Ctx) {
					wd, _ := os.Getwd()
					expanded, changed := fileplan.ExpandPatterns(words, wd)
					if !changed {
						c.ToastWarn("No pattern matches any file")
						return
					}
					a.Target = joinShellWords(expanded)
					p, err := a.MakePlan()
					if err != nil {
						c.ToastError(err.Error())
						return
					}
					plan = p
					c.Toast(fmt.Sprintf("Target is now %d names", len(expanded)))
				},
			}))
		}
		label := "Run " + a.Spec.Name + " now"
		if plan.Blocked() {
			label = "Run " + a.Spec.Name + " anyway (asks you to type to confirm)"
		}
		acts = append(acts, mint.Action(mint.ActionOpts{
			Label: label,
			Hint:  "run",
			OnActivate: func(c *mint.Ctx) {
				// Plan again: the files may have changed while this
				// screen was open.
				var now fileplan.Plan
				var err error
				c.Busy("Checking again…", func() { now, err = a.MakePlan() })
				if err != nil {
					c.ToastError(err.Error())
					return
				}
				if now.Fingerprint() != plan.Fingerprint() {
					plan = now
					c.ToastWarn("The files changed since this preview. Check it again, then run.")
					return
				}
				if plan.Blocked() {
					phrase := strings.TrimSpace(a.Target)
					if phrase == "" {
						phrase = a.Spec.BinaryName()
					}
					if !c.TypeToConfirm("Run despite the warnings?", firstBlocking(plan), phrase) {
						c.ToastWarn("Cancelled")
						return
					}
				}
				run(c, plan)
			},
		}))
		sections = append(sections, mint.Section{Name: "Actions", Rows: acts})

		groups := map[fileplan.Kind][]fileplan.Effect{}
		for _, e := range plan.Effects {
			groups[e.Kind] = append(groups[e.Kind], e)
		}
		wd, _ := os.Getwd()
		for _, k := range []fileplan.Kind{fileplan.Fail, fileplan.Delete, fileplan.Overwrite, fileplan.Move, fileplan.Create, fileplan.Keep} {
			es := groups[k]
			if len(es) == 0 {
				continue
			}
			var rows []mint.Row
			for i, e := range es {
				if i == maxShownPerKind {
					rows = append(rows, mint.Note(fmt.Sprintf("… and %s more", commas(len(es)-i))))
					break
				}
				rows = append(rows, mint.Label(mint.StyleBody, effectLine(e, wd)))
			}
			sections = append(sections, mint.CollapsibleSection(fmt.Sprintf("%s (%s)", kindHeading(k), commas(len(es))), k == fileplan.Keep || k == fileplan.Create && len(es) > 50, rows...))
		}
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}

func kindHeading(k fileplan.Kind) string {
	return map[fileplan.Kind]string{fileplan.Delete: "Deleted", fileplan.Overwrite: "Replaced", fileplan.Create: "Created",
		fileplan.Move: "Moved", fileplan.Keep: "Left alone", fileplan.Fail: "Will fail"}[k]
}

// effectLine is one effect as text, with paths shown relative to wd when
// they are inside it.
func effectLine(e fileplan.Effect, wd string) string {
	rel := func(p string) string {
		if r, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(r, "..") && filepath.IsAbs(p) {
			return r
		}
		return p
	}
	s := rel(e.Path)
	if e.IsDir && !strings.HasSuffix(s, "/") {
		s += "/"
	}
	if e.Kind == fileplan.Move && e.Dest != "" {
		s += " → " + rel(e.Dest)
	}
	if e.Size > 0 {
		s += "  " + humanBytes(e.Size)
	}
	if e.Note != "" && e.Kind != fileplan.Create {
		s += "  — " + e.Note
	}
	return mint.CleanLineVisible(s)
}

func firstBlocking(p fileplan.Plan) string {
	for _, w := range p.Warnings {
		if w.Blocking {
			return mint.CleanLineVisible(w.Text)
		}
	}
	return ""
}

func hasPattern(words []string) bool {
	for _, w := range words {
		if strings.ContainsAny(w, "*?[") {
			return true
		}
	}
	return false
}

// joinShellWords is the inverse of splitShellWords: words with spaces or
// quotes are quoted so they split back the same.
func joinShellWords(words []string) string {
	var out []string
	for _, w := range words {
		if w == "" || strings.ContainsAny(w, " \t\"'\\") {
			w = "'" + strings.ReplaceAll(w, "'", `'"'"'`) + "'"
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}
