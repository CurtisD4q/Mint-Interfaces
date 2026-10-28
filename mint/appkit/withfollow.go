package appkit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/follow"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

// MixinFollow opens on a live log that keeps up with new lines.
const MixinFollow MixinName = "WithFollow"

// Follow formats (FollowSpec.Format).
const (
	FollowText        = "text"
	FollowJournalJSON = "journal-json" // journalctl -o json
)

// FollowSpec configures WithFollow.
type FollowSpec struct {
	Format string `json:"format,omitempty"` // FollowText (default) or FollowJournalJSON
	// Flag is a follow switch parsed from the program's own options
	// ("-f" or "--follow") when the tool is not one Mint already knows.
	// followArgs passes it unless that option is already on the command.
	Flag string `json:"flag,omitempty"`
	// FollowArgs make the tool keep running and print new lines ("-F"
	// for tail, "-f" for journalctl), unless one of FollowFlags is in use.
	FollowArgs  []string `json:"follow_args,omitempty"`
	FollowFlags []string `json:"follow_flags,omitempty"`
	// FormatArgs make the tool print Format ("-o json"), unless one of
	// FormatFlags is in use; ReadableFlags are user choices still in it.
	FormatArgs    []string `json:"format_args,omitempty"`
	FormatFlags   []string `json:"format_flags,omitempty"`
	ReadableFlags []string `json:"readable_flags,omitempty"`
	// AutoStart starts following when the screen opens. It applies only
	// to tools marked ReadOnly, like the monitor's polling.
	AutoStart bool `json:"auto_start,omitempty"`
	// Restart starts the tool again when it exits by itself: "never"
	// (default) or "on-exit", waiting longer each time (1 s to 30 s).
	Restart string `json:"restart,omitempty"`
	// Levels mark text lines; empty uses common words (error, warn…).
	Levels []FollowLevel `json:"levels,omitempty"`
}

// FollowLevel gives text lines matching Regex a level (debug, info,
// notice, warning, error).
type FollowLevel struct {
	Level string `json:"level"`
	Regex string `json:"regex"`
}

var followDefaults = map[string]FollowSpec{
	"tail": {Format: FollowText, FollowArgs: []string{"-F"}, FollowFlags: []string{"-f", "-F", "--follow"}, AutoStart: true},
	"journalctl": {Format: FollowJournalJSON, FollowArgs: []string{"--follow"}, FollowFlags: []string{"-f", "--follow"},
		FormatArgs: []string{"--output=json"}, FormatFlags: []string{"-o", "--output"},
		ReadableFlags: []string{"-o json", "--output json", "-ojson", "--output=json"}, AutoStart: true},
	"dmesg": {Format: FollowText, FollowArgs: []string{"--follow"}, FollowFlags: []string{"-w", "--follow", "-W", "--follow-new"}, AutoStart: true},
}

// KnownFollowSpec returns WithFollow's settings for a tool Mint knows.
func KnownFollowSpec(bin string) (FollowSpec, bool) {
	f, ok := followDefaults[toolBase(bin)]
	for _, p := range []*[]string{&f.FollowArgs, &f.FollowFlags, &f.FormatArgs, &f.FormatFlags, &f.ReadableFlags} {
		*p = append([]string(nil), (*p)...)
	}
	return f, ok
}

func validateFollow(s *ToolSpec) error {
	f := s.Follow
	switch f.Format {
	case "", FollowText:
	case FollowJournalJSON:
		if len(f.FormatArgs) == 0 {
			return errors.New("journal-json needs format_args (\"--output=json\")")
		}
	default:
		return fmt.Errorf("unknown format %q", f.Format)
	}
	switch f.Restart {
	case "", "never", "on-exit":
	default:
		return fmt.Errorf("restart must be never or on-exit, got %q", f.Restart)
	}
	if _, err := compileLevels(f.Levels); err != nil {
		return err
	}
	for _, a := range append(append([]string(nil), f.FollowArgs...), f.FormatArgs...) {
		if !strings.HasPrefix(a, "-") {
			return fmt.Errorf("follow_args and format_args must be options, got %q", a)
		}
	}
	if f.Flag != "" && !strings.HasPrefix(f.Flag, "-") {
		return fmt.Errorf("flag must be an option, got %q", f.Flag)
	}
	return nil
}

func compileLevels(ls []FollowLevel) ([]follow.LevelPattern, error) {
	var out []follow.LevelPattern
	for i, l := range ls {
		lv, ok := follow.ParseLevel(l.Level)
		if !ok {
			return nil, fmt.Errorf("level %d: unknown level %q", i+1, l.Level)
		}
		re, err := regexp.Compile(l.Regex)
		if err != nil {
			return nil, fmt.Errorf("level %d: %w", i+1, err)
		}
		out = append(out, follow.LevelPattern{Level: lv, Re: re})
	}
	return out, nil
}

func init() {
	registerMixin(MixinInfo{
		Name: MixinFollow, Order: 104,
		Summary:  "open on a live log that keeps up (Follow: tail -F, journalctl -f, dmesg -w)",
		Fields:   "Follow *FollowSpec",
		Enabled:  func(s *ToolSpec) bool { return s.Follow != nil },
		Validate: validateFollow,
	})
}

// followArgs adds FollowArgs and FormatArgs where needed.
func (a *ToolApp) followArgs(args []string, at int) []string {
	f := a.Spec.Follow
	if f == nil {
		return args
	}
	if f.Flag != "" {
		args, _ = forceFormat(args, at, []string{f.Flag}, []string{f.Flag}, nil)
	}
	args, _ = forceFormat(args, at, f.FollowArgs, f.FollowFlags, nil)
	args, _ = forceFormat(args, at, f.FormatArgs, f.FormatFlags, f.ReadableFlags)
	return args
}

// followFormat is the format the current arguments make the tool print.
func (a *ToolApp) followFormat() follow.Format {
	f := a.Spec.Follow
	if f == nil || f.Format != FollowJournalJSON {
		return follow.Text
	}
	args, _ := a.BuildArgs()
	if usesAny(args[a.argPrefixLen():], append([]string{strings.Join(f.FormatArgs, " ")}, f.ReadableFlags...)) {
		return follow.JournalJSON
	}
	return follow.Text // the user chose another output format
}

// followSource feeds a ViewStyled from the active job through a buffer,
// restarting the tool when it exits if the spec says so.
type followSource struct {
	a       *ToolApp
	buf     *follow.Buffer
	job     *runner.Job
	next    int // absolute line number to read from next
	exitAt  time.Time
	backoff time.Duration
	noted   bool // the job's end has been marked
	now     func() time.Time
}

func (a *ToolApp) newFollowSource() *followSource {
	var pats []follow.LevelPattern
	if a.Spec.Follow != nil {
		pats, _ = compileLevels(a.Spec.Follow.Levels)
	}
	return &followSource{a: a, buf: &follow.Buffer{Format: a.followFormat(), Patterns: pats}, backoff: time.Second, now: time.Now}
}

func (s *followSource) Len() int { return s.buf.Len() }

func (s *followSource) Line(i int) mint.StyledLine {
	e := s.buf.At(i)
	if e.Mark {
		return mint.StyledLine{Segs: []mint.Seg{{Text: e.Text, Role: mint.RoleAccent}}}
	}
	var segs []mint.Seg
	if !e.Time.IsZero() {
		segs = append(segs, mint.Seg{Text: e.Time.Format("15:04:05") + " ", Role: mint.RoleMuted})
	}
	if e.Unit != "" {
		segs = append(segs, mint.Seg{Text: e.Unit + ": ", Role: mint.RoleMuted})
	}
	role := mint.RoleText
	switch e.Level {
	case follow.Error:
		role = mint.RoleError
	case follow.Warning:
		role = mint.RoleWarn
	case follow.Debug:
		role = mint.RoleMuted
	}
	return mint.StyledLine{Segs: append(segs, mint.Seg{Text: e.Text, Role: role})}
}

// Poll reads new lines, notes the tool ending, and restarts it when asked.
func (s *followSource) Poll() bool {
	changed := false
	job := s.a.ActiveJob()
	if job != s.job {
		if s.job != nil {
			s.buf.AddMark(fmt.Sprintf("── started again at %s ──", s.now().Format("15:04:05")))
		}
		s.job, s.next, s.noted = job, 0, false
		s.buf.Format = s.a.followFormat()
		changed = true
	}
	if job == nil {
		return changed
	}
	lines, first, total := job.Since(s.next)
	if first > s.next && s.next > 0 {
		s.buf.AddMark(fmt.Sprintf("── %d lines came faster than they could be shown, and were skipped ──", first-s.next))
	}
	for _, l := range lines {
		s.buf.Add(l)
	}
	s.next = total
	changed = changed || len(lines) > 0

	if job.Running() {
		return changed
	}
	if !s.noted {
		s.noted = true
		s.exitAt = s.now()
		msg := fmt.Sprintf("── %s ended: %s ──", s.a.Spec.Name, job.State())
		if job.State() == runner.Failed {
			msg = fmt.Sprintf("── %s ended (exit %d) ──", s.a.Spec.Name, job.ExitCode())
		}
		s.buf.AddMark(msg)
		changed = true
		// A run that lasted a while resets the wait.
		if job.Elapsed() > time.Minute {
			s.backoff = time.Second
		}
	}
	// Stopped by the user, or timed out: don't restart.
	if s.a.Spec.Follow != nil && s.a.Spec.Follow.Restart == "on-exit" && (job.State() == runner.Done || job.State() == runner.Failed) &&
		s.now().Sub(s.exitAt) >= s.backoff {
		s.backoff = min(s.backoff*2, 30*time.Second)
		if _, err := s.a.StartJob(); err != nil {
			s.buf.AddMark("── could not start again: " + err.Error() + " ──")
			s.exitAt = s.now()
		}
		changed = true
	}
	return changed
}

// FollowScreen shows the tool's output as it arrives, newest at the
// bottom. It starts the tool if it is read-only and set to start itself.
func (a *ToolApp) FollowScreen() *mint.Screen {
	src := a.newFollowSource()
	if job := a.ActiveJob(); (job == nil || !job.Running()) && a.Spec.Follow.AutoStart && a.Spec.ReadOnly {
		if _, err := a.StartJob(); err != nil {
			src.buf.AddMark("── could not start: " + err.Error() + " ──")
		}
	}
	levelNames := []string{}
	for _, l := range follow.Levels() {
		levelNames = append(levelNames, l.String())
	}
	return mint.ViewStyled(mint.ViewStyledOpts{
		Title:  a.Spec.Name + " — live",
		Source: src,
		Follow: true,
		Status: a.statusBadges,
		Crumb: func(int, int) string {
			text, min := src.buf.Filter()
			var parts []string
			if text != "" {
				parts = append(parts, "filter "+mint.CleanLineVisible(text))
			}
			if min > follow.Debug {
				parts = append(parts, min.String()+" and up")
			}
			parts = append(parts, fmt.Sprintf("%d lines", src.buf.Len()))
			if d := src.buf.Dropped(); d > 0 {
				parts = append(parts, fmt.Sprintf("%d older let go", d))
			}
			return strings.Join(parts, " · ")
		},
		Keys: []mint.StyledKey{
			{Key: "/", Hint: "/ filter", Do: func() bool {
				cur, _ := src.buf.Filter()
				q, ok := mint.Prompt("Filter", "Show lines containing this text. Start with ~ for a regular expression. Empty shows all.", cur)
				if !ok {
					return false
				}
				q = strings.TrimSpace(q)
				regex := strings.HasPrefix(q, "~")
				if err := src.buf.SetFilter(strings.TrimPrefix(q, "~"), regex); err != nil {
					mint.ToastError(err.Error())
				}
				return true
			}},
			{Key: "l", Hint: "l level", Do: func() bool {
				_, cur := src.buf.Filter()
				src.buf.SetMinLevel(follow.Level((int(cur) + 1) % len(levelNames)))
				return true
			}},
			{Key: "r", Hint: "r restart", Do: func() bool {
				if _, err := a.StartJob(); err != nil {
					mint.ToastError(err.Error())
				}
				return true
			}},
			{Key: "x", Hint: "x stop", Do: func() bool {
				if j := a.ActiveJob(); j != nil && j.Running() {
					j.Stop()
				}
				return true
			}},
		},
	})
}
