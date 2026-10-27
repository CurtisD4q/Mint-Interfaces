package appkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
	"github.com/CurtisD4q/Mint-Interfaces/mint/transcript"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// MixinTranscript runs an interactive program (psql, sqlite3, gdb…) with
// a command box and a transcript of each command and what it printed.
const MixinTranscript MixinName = "WithTranscript"

// TranscriptSpec configures WithTranscript.
type TranscriptSpec struct {
	// Adapter is the program Mint knows how to drive: one of
	// transcript.Adapters ("psql", "sqlite3", "gdb", "redis-cli", "ftp",
	// or "lines" for any other that reads one command per line).
	Adapter string `json:"adapter"`
}

var transcriptDefaults = map[string]string{
	"psql": "psql", "sqlite3": "sqlite3", "gdb": "gdb", "redis-cli": "redis-cli",
	"ftp": "ftp", "tnftp": "ftp",
}

// KnownTranscriptSpec returns WithTranscript's settings for a program Mint
// knows. mint-gen doesn't attach it by itself: psql and sqlite3 are as
// often run for one query (the Query archetype), so the archetype decides.
func KnownTranscriptSpec(bin string) (TranscriptSpec, bool) {
	ad, ok := transcriptDefaults[toolBase(bin)]
	return TranscriptSpec{Adapter: ad}, ok
}

func validateTranscript(s *ToolSpec) error {
	if _, ok := transcript.Adapters[s.Transcript.Adapter]; !ok {
		names := make([]string, 0, len(transcript.Adapters))
		for n := range transcript.Adapters {
			names = append(names, n)
		}
		sort.Strings(names)
		return fmt.Errorf("unknown adapter %q (known: %s)", s.Transcript.Adapter, strings.Join(names, ", "))
	}
	return nil
}

func init() {
	registerMixin(MixinInfo{
		Name: MixinTranscript, Order: 106,
		Summary:  "command box and transcript for an interactive program (Session: psql, sqlite3, gdb, redis-cli)",
		Fields:   "Transcript *TranscriptSpec",
		Enabled:  func(s *ToolSpec) bool { return s.Transcript != nil },
		Validate: validateTranscript,
	})
}

func (a *ToolApp) transcriptAdapter() transcript.Adapter {
	return transcript.Adapters[a.Spec.Transcript.Adapter]
}

// startSession starts the program with the user's options, stopping any
// session already open.
func (a *ToolApp) startSession() (*transcript.Session, error) {
	if labels := a.expiredSecrets(); len(labels) > 0 {
		return nil, fmt.Errorf("%s expired after a period of inactivity; enter %s again", strings.Join(labels, ", "), pronoun(len(labels)))
	}
	if box := a.staleMaskedText(); box != "" {
		return nil, fmt.Errorf("%s has a hidden value (%s) that was not saved; type it again", box, secretMask)
	}
	args, _, stdin := a.buildArgs(false)
	if len(stdin) > 0 {
		// In a session standard input is the command box: a secret written
		// there would run as a command and appear in the transcript.
		return nil, errors.New("in a session, " + a.Spec.Name + "'s input is the command box, so it can't be given a secret that way")
	}
	_ = a.SaveSettings()
	a.mu.Lock()
	old := a.session
	a.mu.Unlock()
	if old != nil {
		old.Close()
	}
	s := transcript.StartCmd(a.transcriptAdapter(), runner.Cmd{
		Name:   a.Spec.BinaryName(),
		Args:   args,
		Env:    a.secretEnv(false),
		Redact: a.newRunMasker().Redact,
	})
	a.mu.Lock()
	a.session = s
	a.activeJob = s.Job()
	a.mu.Unlock()
	return s, nil
}

// Session returns the open session, or nil.
func (a *ToolApp) Session() *transcript.Session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session
}

// commandsPath is where typed commands are kept between runs (KeepHistory).
func (a *ToolApp) commandsPath() string { return filepath.Join(a.DataDir, "commands.json") }

const maxCommands = 500

func (a *ToolApp) loadCommands() []string {
	if !a.Spec.KeepHistory || a.DataDir == "" {
		return nil
	}
	b, err := os.ReadFile(a.commandsPath())
	if err != nil {
		return nil
	}
	var cmds []string
	if json.Unmarshal(b, &cmds) != nil {
		return nil
	}
	return cmds
}

func (a *ToolApp) saveCommands(cmds []string) {
	if !a.Spec.KeepHistory || a.DataDir == "" {
		return
	}
	_ = util.Mkdir(a.DataDir)
	b, err := json.Marshal(cmds)
	if err == nil && os.WriteFile(a.commandsPath(), b, 0o600) == nil {
		_ = os.Chmod(a.commandsPath(), 0o600) // an older file may have had wider permissions
		a.giveBack(a.commandsPath())
	}
}

// transcriptSource lays out the session's entries for a styled view and
// feeds a script to the program one command at a time.
type transcriptSource struct {
	a     *ToolApp
	ad    transcript.Adapter
	sess  *transcript.Session
	lines []mint.StyledLine
	notes []transcriptNote

	history []string // typed commands, oldest first, without secrets

	script      []transcript.ScriptCommand
	scriptName  string
	scriptNext  int
	scriptEntry int // number of entries when the last script command was sent
	shownEnded  bool
}

// transcriptNote is a line of Mint's own, shown after entry `after`.
type transcriptNote struct {
	after int
	text  string
	role  mint.Role
}

func (a *ToolApp) newTranscriptSource(s *transcript.Session) *transcriptSource {
	src := &transcriptSource{a: a, ad: a.transcriptAdapter(), sess: s, history: a.loadCommands()}
	src.rebuild()
	return src
}

func (t *transcriptSource) Len() int                   { return len(t.lines) }
func (t *transcriptSource) Line(i int) mint.StyledLine { return t.lines[i] }

func (t *transcriptSource) note(text string, role mint.Role) {
	t.notes = append(t.notes, transcriptNote{after: len(t.sess.Entries()), text: text, role: role})
	t.rebuild()
}

// Poll takes in new output, sends the script's next command, and redraws
// when anything changed.
func (t *transcriptSource) Poll() bool {
	changed := t.sess.Poll()
	if t.stepScript() {
		changed = true
	}
	if !t.sess.Job().Running() && !t.shownEnded {
		t.shownEnded = true
		j := t.sess.Job()
		msg := fmt.Sprintf("── %s ended ──", t.a.Spec.Name)
		if code := j.ExitCode(); code > 0 {
			msg = fmt.Sprintf("── %s ended (exit %d) ──", t.a.Spec.Name, code)
		}
		t.notes = append(t.notes, transcriptNote{after: len(t.sess.Entries()), text: msg, role: mint.RoleAccent})
		t.stopScript("")
		changed = true
	}
	if changed {
		t.rebuild()
	}
	return changed
}

func (t *transcriptSource) rebuild() {
	entries := t.sess.Entries()
	var out []mint.StyledLine
	ni := 0
	notesUpTo := func(n int) {
		for ; ni < len(t.notes) && t.notes[ni].after <= n; ni++ {
			out = append(out, mint.StyledLine{Segs: []mint.Seg{{Text: t.notes[ni].text, Role: t.notes[ni].role}}})
		}
	}
	for i, e := range entries {
		notesUpTo(i)
		if e.Command != "" {
			shown, _ := transcript.MaskSecrets(t.ad, e.Command)
			for j, l := range strings.Split(shown, "\n") {
				prefix := "› "
				if j > 0 {
					prefix = "  "
				}
				out = append(out, mint.StyledLine{Anchor: j == 0, Segs: []mint.Seg{
					{Text: prefix, Role: mint.RoleAccent}, {Text: l, Role: mint.RoleAccent}}})
			}
		}
		for _, l := range e.Output {
			role := mint.RoleText
			if l.Err {
				role = mint.RoleError
			}
			out = append(out, mint.StyledLine{Segs: []mint.Seg{{Text: "  " + l.Text, Role: role}}})
		}
		var status []string
		switch {
		case e.Interrupted:
			status = append(status, "cancelled")
		case e.Running() && e.Command != "":
			status = append(status, "running…")
		}
		if !e.Running() && e.Command != "" {
			if d := e.End.Sub(e.Start); d >= time.Second {
				status = append(status, fmt.Sprintf("took %.1f s", d.Seconds()))
			}
		}
		if len(status) > 0 {
			out = append(out, mint.StyledLine{Segs: []mint.Seg{{Text: "  " + strings.Join(status, " · "), Role: mint.RoleMuted}}})
		}
		if e.Command != "" || i == 0 {
			out = append(out, mint.StyledLine{})
		}
	}
	notesUpTo(len(entries))
	if len(out) == 0 {
		out = append(out, mint.StyledLine{Segs: []mint.Seg{{Text: "i types a command; s runs a file of commands.", Role: mint.RoleMuted}}})
		if t.sess.Safe() {
			out = append(out, mint.StyledLine{Segs: []mint.Seg{{Text: "Safe mode: commands that reach outside " + t.a.Spec.Name + " are refused.", Role: mint.RoleMuted}}})
		}
	}
	t.lines = out
}

// Submit sends a command typed by the user. A command the program would
// answer from its input, or (in safe mode) one that would end the session,
// isn't sent.
func (t *transcriptSource) Submit(cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	for _, l := range strings.Split(cmd, "\n") {
		if t.ad.Refuse != nil {
			if why := t.ad.Refuse(l); why != "" {
				return errors.New("not sent: " + strings.TrimSpace(l) + " " + why)
			}
		}
		if t.sess.Safe() {
			if why := t.ad.Escape(l); why != "" {
				return fmt.Errorf("not sent: %s %s; in safe mode it would end the session", strings.TrimSpace(l), why)
			}
		}
	}
	if t.scriptRunning() {
		return errors.New("a script is running; wait for it, or cancel it (c)")
	}
	if err := t.sess.Send(cmd); err != nil {
		if errors.Is(err, transcript.ErrBusy) {
			return errors.New("the last command is still running; wait, or cancel it (c)")
		}
		return err
	}
	t.remember(cmd)
	t.rebuild()
	return nil
}

// remember adds cmd to the command history, unless it holds a password.
func (t *transcriptSource) remember(cmd string) {
	if _, secret := transcript.MaskSecrets(t.ad, cmd); secret {
		return
	}
	if found := FindFreeTextSecrets(cmd, nil); len(found) > 0 {
		return
	}
	if n := len(t.history); n > 0 && t.history[n-1] == cmd {
		return
	}
	t.history = append(t.history, cmd)
	if len(t.history) > maxCommands {
		t.history = t.history[len(t.history)-maxCommands:]
	}
	t.a.saveCommands(t.history)
}

// Cancel cancels the running command, or the script.
func (t *transcriptSource) Cancel() error {
	if t.scriptRunning() {
		t.stopScript("cancelled")
	}
	if !t.sess.Busy() {
		return nil
	}
	return t.sess.Interrupt()
}

// LoadScript reads a script file and checks it. It returns the commands
// that reach outside the program, for the user to confirm, or an error
// when the script can't run.
func (t *transcriptSource) LoadScript(path string) (cmds []transcript.ScriptCommand, risky []transcript.ScriptCommand, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil {
		return nil, nil, err
	} else if !st.Mode().IsRegular() {
		return nil, nil, errors.New(filepath.Base(path) + " is not a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, transcript.MaxScript+1))
	if err != nil {
		return nil, nil, err
	}
	cmds, err = transcript.SplitScript(t.ad, string(b))
	if err != nil {
		return nil, nil, err
	}
	if len(cmds) == 0 {
		return nil, nil, errors.New(filepath.Base(path) + " has no commands")
	}
	for _, c := range cmds {
		if c.Refuse != "" {
			return nil, nil, fmt.Errorf("line %d: %s", c.Line, c.Refuse)
		}
		if c.Escape != "" {
			if t.sess.Safe() {
				return nil, nil, fmt.Errorf("line %d %s; %s runs in safe mode, which ends the session on it", c.Line, c.Escape, t.ad.Name)
			}
			risky = append(risky, c)
		}
	}
	return cmds, risky, nil
}

// RunScript sends cmds one at a time, stopping at the first that reports
// an error.
func (t *transcriptSource) RunScript(name string, cmds []transcript.ScriptCommand) error {
	if t.scriptRunning() {
		return errors.New("a script is already running")
	}
	if t.sess.Busy() {
		return errors.New("the last command is still running")
	}
	t.script, t.scriptName, t.scriptNext = cmds, name, 0
	t.note(fmt.Sprintf("── running %s: %d commands ──", name, len(cmds)), mint.RoleAccent)
	t.stepScript()
	return nil
}

func (t *transcriptSource) scriptRunning() bool { return t.script != nil }

func (t *transcriptSource) stopScript(why string) {
	if t.script == nil {
		return
	}
	if why != "" {
		t.notes = append(t.notes, transcriptNote{after: len(t.sess.Entries()), text: "── " + t.scriptName + " " + why + " ──", role: mint.RoleWarn})
	}
	t.script = nil
}

// stepScript sends the next script command once the last has finished.
func (t *transcriptSource) stepScript() bool {
	if t.script == nil || t.sess.Busy() {
		return false
	}
	if t.scriptNext > 0 {
		entries := t.sess.Entries()
		if len(entries) > t.scriptEntry {
			last := entries[t.scriptEntry]
			if last.Interrupted {
				t.stopScript(fmt.Sprintf("stopped at line %d: cancelled", t.script[t.scriptNext-1].Line))
				return true
			}
			if last.HasErrors() {
				t.stopScript(fmt.Sprintf("stopped at line %d: the command reported an error", t.script[t.scriptNext-1].Line))
				return true
			}
		}
	}
	if t.scriptNext >= len(t.script) {
		name := t.scriptName
		t.script = nil
		t.notes = append(t.notes, transcriptNote{after: len(t.sess.Entries()), text: "── " + name + " finished ──", role: mint.RoleAccent})
		return true
	}
	c := t.script[t.scriptNext]
	t.scriptEntry = len(t.sess.Entries())
	if err := t.sess.Send(c.Text); err != nil {
		t.stopScript(fmt.Sprintf("stopped at line %d: %v", c.Line, err))
		return true
	}
	t.scriptNext++
	return true
}

// TranscriptScreen shows the session: each command and what it printed,
// with keys to type a command, run a script, cancel, and start again. It
// starts the program if no session is open.
func (a *ToolApp) TranscriptScreen() *mint.Screen {
	s := a.Session()
	var startErr error
	if s == nil || !s.Job().Running() {
		s, startErr = a.startSession()
	}
	if startErr != nil {
		return mint.ViewStyled(mint.ViewStyledOpts{Title: a.Spec.Name + " — session", Lines: func(int) []mint.StyledLine {
			return []mint.StyledLine{{Segs: []mint.Seg{{Text: "Could not start " + a.Spec.Name + ": " + startErr.Error(), Role: mint.RoleError}}}}
		}})
	}
	src := a.newTranscriptSource(s)
	ad := src.ad
	readCommand := func() (string, bool) {
		var parts []string
		for {
			title, sub, hist := a.Spec.Name+" command", "Enter sends it.", src.history
			if len(parts) > 0 {
				shown, _ := transcript.MaskSecrets(ad, parts[len(parts)-1])
				title, sub, hist = "…continued", "The statement goes on until a ; ends it. So far: "+shown, nil
			}
			line, ok := mint.PromptHistory(title, sub, "", hist)
			if !ok {
				return "", false
			}
			parts = append(parts, line)
			text := strings.Join(parts, "\n")
			if ad.Complete == nil || ad.Complete(text) {
				return text, true
			}
		}
	}
	keys := []mint.StyledKey{
		{Key: "i", Hint: "i command", Do: func() bool {
			cmd, ok := readCommand()
			if !ok {
				return true
			}
			if err := src.Submit(cmd); err != nil {
				mint.ToastError(err.Error())
			}
			return true
		}},
		{Key: "c", Hint: "c cancel", Do: func() bool {
			if !src.scriptRunning() && src.sess.Busy() && !src.sess.CanInterrupt() {
				mint.ToastWarn(ad.Name + " can't cancel a command without ending; x ends the session")
				return true
			}
			var err error
			mint.Busy("Cancelling…", func() { err = src.Cancel() })
			if err != nil {
				mint.ToastError(err.Error())
			}
			return true
		}},
		{Key: "s", Hint: "s script", Do: func() bool {
			path, ok := mint.Prompt("Run a script", "A file of "+ad.Name+" commands, sent one at a time. It stops at the first error.", "")
			path = strings.TrimSpace(path)
			if !ok || path == "" {
				return true
			}
			cmds, risky, err := src.LoadScript(util.ExpandUser(path))
			if err != nil {
				mint.ToastError(err.Error())
				return true
			}
			if len(risky) > 0 {
				var b strings.Builder
				for i, c := range risky {
					if i == 5 {
						fmt.Fprintf(&b, "…and %d more\n", len(risky)-5)
						break
					}
					first, _, _ := strings.Cut(c.Text, "\n")
					fmt.Fprintf(&b, "line %d %s:\n  %s\n", c.Line, c.Escape, mint.CleanLineVisible(first))
				}
				if !mint.Confirm("This script reaches outside "+ad.Name, b.String()+"Run it?") {
					mint.ToastWarn("Not run")
					return true
				}
			}
			if err := src.RunScript(filepath.Base(path), cmds); err != nil {
				mint.ToastError(err.Error())
			}
			return true
		}},
		{Key: "x", Hint: "x end", Do: func() bool {
			src.stopScript("stopped: the session ended")
			src.sess.Close()
			return true
		}},
		{Key: "r", Hint: "r restart", Do: func() bool {
			ns, err := a.startSession()
			if err != nil {
				mint.ToastError(err.Error())
				return true
			}
			*src = *a.newTranscriptSource(ns)
			return true
		}},
	}
	return mint.ViewStyled(mint.ViewStyledOpts{
		Title:  a.Spec.Name + " — session",
		Source: src,
		Follow: true,
		Status: a.statusBadges,
		Crumb: func(int, int) string {
			var parts []string
			n := 0
			for _, e := range src.sess.Entries() {
				if e.Command != "" {
					n++
				}
			}
			parts = append(parts, fmt.Sprintf("%d commands", n))
			if src.scriptRunning() {
				parts = append(parts, fmt.Sprintf("script %d/%d", src.scriptNext, len(src.script)))
			}
			if src.sess.Safe() {
				parts = append(parts, "safe mode")
			}
			return strings.Join(parts, " · ")
		},
		Keys: keys,
	})
}
