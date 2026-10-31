// Package transcript runs an interactive program (psql, sqlite3, gdb,
// redis-cli, ftp…) over pipes and splits its output into one entry per
// command, so an interface can show a transcript and a command box.
//
// Programs get no terminal, so they print no prompt Mint could wait for.
// After each command Mint sends a marker command that prints a random
// token; output up to the token belongs to the command. The token is new
// for every command, so output can't end an entry early by printing an
// older one, and a line must end with the token (not merely mention it).
package transcript

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

// Entry is one command and what it printed.
type Entry struct {
	Command string // "" for output that came on its own (a banner, a notice)
	Output  []mint.LogLine
	Start   time.Time
	End     time.Time // zero while running
	// Interrupted is set when the user cancelled it.
	Interrupted bool

	// hidden entries are commands Mint sends for itself; after reads
	// their output when they finish.
	hidden bool
	after  func(e *Entry) (keep bool)
}

// Running reports whether the command hasn't finished.
func (e *Entry) Running() bool { return e.End.IsZero() }

// HasErrors reports whether anything came on standard error.
func (e *Entry) HasErrors() bool {
	for _, l := range e.Output {
		if l.Err {
			return true
		}
	}
	return false
}

// Timing for programs without markers, and for the stderr side.
var (
	// IdleEnd: without markers, output that has been quiet this long has ended.
	IdleEnd = 1500 * time.Millisecond
	// StderrGrace: after the stdout marker, how long to wait for the
	// stderr marker (or, without one, for late error lines).
	StderrGrace = 400 * time.Millisecond
)

// ErrBusy is returned by Send while a command is still running.
var ErrBusy = errors.New("the last command is still running")

// ErrCannotCancel is returned by Interrupt for programs that can't cancel
// a command and carry on (Adapter.Cancel is CancelNone).
var ErrCannotCancel = errors.New("this program can't cancel a command without ending; Stop ends the session")

// Session is an interactive program and its transcript. Methods are safe
// to call from any goroutine.
type Session struct {
	a    Adapter
	job  *runner.Job
	bin  string
	args []string // the user's options, without BaseArgs
	env  []string

	mu      sync.Mutex
	entries []*Entry
	cur     *Entry // the running command, or nil
	outMark string // full lines that end cur, on stdout and stderr
	errMark string
	outCmd  string // the marker command text, which an echoing program repeats
	errCmd  string
	gotOut  time.Time
	gotErr  bool
	lastOut time.Time
	next    int // absolute line number to read next
	now     func() time.Time

	// For CancelBackend: which server process runs this session's
	// queries, and when it started (so a reused PID is never cancelled).
	backendPID   int
	backendStart string
	needIdentity bool
	safe         bool // SafeArgs are in use
	// queued is a command the user sent while one of Mint's own was
	// running; it goes as soon as that one ends.
	queued *Entry
}

// Start runs bin with the adapter's base options, then args.
func Start(a Adapter, bin string, args []string, env []string) *Session {
	return StartCmd(a, runner.Cmd{Name: bin, Args: args, Env: env})
}

// StartCmd is Start with more of the job's settings (Redact, Dir…).
// c.Args are the user's options; the adapter's go before them, and Input
// and Label are set here.
func StartCmd(a Adapter, c runner.Cmd) *Session {
	bin, args := c.Name, c.Args
	if bin == "" {
		bin = c.Bin
	}
	all := append(append([]string(nil), a.BaseArgs...), args...)
	safe := len(a.SafeArgs) > 0 && safeSupported(bin, a)
	if safe {
		all = append(append([]string(nil), a.SafeArgs...), all...)
	}
	s := &Session{a: a, now: time.Now, bin: bin, args: append([]string(nil), args...), env: c.Env, safe: safe}
	c.Name, c.Bin, c.Args, c.Input, c.Label = bin, "", all, true, a.Name
	c.Stdin = ""
	s.job = runner.Start(c)
	if a.Cancel == CancelBackend {
		s.mu.Lock()
		_ = s.sendIdentityLocked()
		s.mu.Unlock()
	}
	return s
}

// safeSupported asks the program whether it accepts SafeArgs, by running
// SafeProbe (a version query) briefly.
func safeSupported(bin string, a Adapter) bool {
	if len(a.SafeProbe) == 0 {
		return true
	}
	j := runner.Start(runner.Cmd{Name: bin, Args: a.SafeProbe, Timeout: 3 * time.Second})
	j.Wait()
	return j.State() == runner.Done
}

// Job is the program's job (for status and stopping).
func (s *Session) Job() *runner.Job { return s.job }

func token() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("transcript: no randomness: " + err.Error())
	}
	return "__MINT_" + hex.EncodeToString(b) + "__"
}

// Send runs one whole command. It fails while another is running.
func (s *Session) Send(cmd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.a.Cancel == CancelBackend && reconnects.MatchString(cmd) {
		s.needIdentity = true // a new connection: a new server process
	}
	e := &Entry{Command: cmd, Start: s.now()}
	if s.cur != nil && s.cur.hidden && s.queued == nil && s.job.Running() {
		s.queued = e
		s.entries = append(s.entries, e)
		return nil
	}
	return s.sendLocked(e)
}

// reconnects matches psql's \c and \connect, which open a new connection.
var reconnects = regexp.MustCompile(`(?m)^\s*\\c(onnect)?(\s|$)`)

func (s *Session) sendLocked(e *Entry) error {
	if s.cur != nil {
		return ErrBusy
	}
	if !s.job.Running() {
		return errors.New(s.a.Name + " is not running")
	}
	cmd := e.Command
	if e != s.queued {
		e.Start = s.now()
		s.entries = append(s.entries, e)
	}
	s.cur = e
	s.outMark, s.errMark, s.outCmd, s.errCmd = "", "", "", ""
	s.gotOut, s.gotErr = time.Time{}, false
	s.lastOut = s.now()
	var payload string
	if s.a.Markers != nil {
		t := token()
		payload = sessionPayload(s.a, cmd, t)
		s.outCmd, s.errCmd = s.a.Markers(t)
		s.outMark = t
		s.errMark = ""
		if s.errCmd != "" {
			s.errMark = t
		}
	} else {
		payload = sessionPayload(s.a, cmd, "")
	}
	if err := s.job.SendLine(strings.TrimSuffix(payload, "\n")); err != nil {
		s.cur = nil
		e.End = s.now()
		return err
	}
	return nil
}

// sessionPayload is what the program is sent for one command: the command,
// a semicolon if the command ends inside a "--" comment, and then the
// closing marker. A comment runs to the end of the text, and sqlite3 reads
// its input a chunk at a time, so a marker on the next line would become
// part of the comment and never print. A semicolon closes it, and an empty
// statement between two semicolons is not an error.
func sessionPayload(a Adapter, cmd, token string) string {
	payload := cmd + "\n"
	if a.Complete != nil && EndsInLineComment(cmd, a.Dollar) {
		payload += ";\n"
	}
	if a.Markers != nil && token != "" {
		out, errCmd := a.Markers(token)
		payload += out + "\n"
		if errCmd != "" {
			payload += errCmd + "\n"
		}
	}
	return payload
}

// endsWithMark reports whether line ends with mark, and returns what came
// before it (output that didn't end in a newline). A line that is the
// marker command itself, repeated by a program that echoes its input,
// doesn't count.
func endsWithMark(line, mark, markCmd string) (before string, ok bool) {
	if mark == "" || !strings.HasSuffix(line, mark) {
		return "", false
	}
	if markCmd != "" && strings.HasSuffix(strings.TrimRight(line, " "), strings.TrimSuffix(markCmd, `\n`)) {
		return "", false
	}
	return strings.TrimSuffix(line, mark), true
}

// Poll reads new output into the transcript and reports whether anything
// changed. Call it regularly (a screen's Poll).
func (s *Session) Poll() bool {
	lines, _, total := s.job.Since(s.next)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next = total
	changed := len(lines) > 0
	for _, l := range lines {
		s.take(l)
	}
	if s.cur != nil && (s.finished() || !s.job.Running()) {
		// Finished, or the program ended mid-command.
		s.endLocked()
		changed = true
	}
	if s.cur == nil && s.queued != nil {
		e := s.queued
		if err := s.sendLocked(e); err != nil {
			e.End = s.now()
			e.Output = append(e.Output, mint.LogLine{Text: err.Error(), Err: true})
		}
		s.queued = nil
		changed = true
	}
	if s.cur == nil && s.needIdentity && s.job.Running() {
		_ = s.sendIdentityLocked()
	}
	return changed
}

func (s *Session) endLocked() {
	e := s.cur
	e.End = s.now()
	s.cur = nil
	if e.hidden && e.after != nil && e.after(e) {
		return
	}
	if e.hidden {
		// Mint's own command didn't do what it should: show what came
		// back (a connection error, usually) as output of its own.
		e.hidden, e.Command = false, ""
	}
}

// identityCmd asks psql for its server process and when it started.
// \gset stores them in psql variables, which are removed again.
const identityCmd = `select pg_backend_pid() as mint_backend_pid, backend_start::text as mint_backend_start ` +
	`from pg_stat_activity where pid = pg_backend_pid() \gset` + "\n" +
	`\echo :mint_backend_pid :mint_backend_start` + "\n" +
	`\unset mint_backend_pid` + "\n" + `\unset mint_backend_start`

// A server start time as PostgreSQL prints it: 2026-10-27 16:30:00.123456+00.
var startTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(\.\d+)?([+-]\d{2}(:\d{2})?)?$`)

func (s *Session) sendIdentityLocked() error {
	s.needIdentity = false
	s.backendPID, s.backendStart = 0, ""
	return s.sendLocked(&Entry{Command: identityCmd, hidden: true, after: func(e *Entry) bool {
		if len(e.Output) != 1 || e.Output[0].Err {
			return false
		}
		pid, start, _ := strings.Cut(e.Output[0].Text, " ")
		n, err := strconv.Atoi(pid)
		if err != nil || n <= 0 || !startTime.MatchString(start) {
			return false
		}
		s.backendPID, s.backendStart = n, start
		return true
	}})
}

func (s *Session) take(l mint.LogLine) {
	if s.cur == nil {
		// Output nobody asked for: keep it in an entry of its own.
		n := len(s.entries)
		if n == 0 || s.entries[n-1].Command != "" {
			s.entries = append(s.entries, &Entry{Start: s.now(), End: s.now()})
			n++
		}
		s.entries[n-1].Output = append(s.entries[n-1].Output, l)
		return
	}
	mark, cmd := s.outMark, s.outCmd
	if l.Err {
		mark, cmd = s.errMark, s.errCmd
	}
	if before, ok := endsWithMark(l.Text, mark, cmd); ok {
		if before != "" {
			s.cur.Output = append(s.cur.Output, mint.LogLine{Text: before, Err: l.Err})
		}
		if l.Err {
			s.gotErr = true
		} else {
			s.gotOut = s.now()
		}
		return
	}
	s.cur.Output = append(s.cur.Output, l)
	s.lastOut = s.now()
}

func (s *Session) finished() bool {
	now := s.now()
	switch {
	case s.outMark == "":
		return now.Sub(s.lastOut) >= IdleEnd
	case s.gotOut.IsZero():
		return false
	case s.errMark != "":
		return s.gotErr || now.Sub(s.gotOut) >= StderrGrace
	default:
		return now.Sub(s.gotOut) >= StderrGrace
	}
}

// Safe reports whether the program runs in its safe mode (sqlite3 -safe),
// in which it refuses the commands Adapter.Escape finds, and ends the
// session when it gets one: an interface should not send them.
func (s *Session) Safe() bool { return s.safe }

// CanInterrupt reports whether Interrupt can cancel a command and leave
// the program running.
func (s *Session) CanInterrupt() bool { return s.a.Cancel != CancelNone }

// Busy reports whether a command is running.
func (s *Session) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queued != nil || s.cur != nil && !s.cur.hidden
}

// CancelTimeout bounds the separate connection CancelBackend opens.
var CancelTimeout = 5 * time.Second

// Interrupt cancels the running command and leaves the program running,
// the way the adapter says (Adapter.Cancel). It returns ErrCannotCancel
// for programs that would quit.
func (s *Session) Interrupt() error {
	s.mu.Lock()
	if q := s.queued; q != nil {
		// Not sent yet: it never will be.
		q.End, q.Interrupted, s.queued = s.now(), true, nil
		s.mu.Unlock()
		return nil
	}
	cur, pid, start := s.cur, s.backendPID, s.backendStart
	s.mu.Unlock()
	if cur == nil || cur.hidden {
		return nil
	}
	switch s.a.Cancel {
	case CancelSignal:
		s.job.Interrupt()
	case CancelBackend:
		if pid == 0 {
			return errors.New("can't cancel: Mint doesn't know which server process runs this session")
		}
		if err := s.cancelBackend(pid, start); err != nil {
			return err
		}
	default:
		return ErrCannotCancel
	}
	s.mu.Lock()
	cur.Interrupted = true
	s.mu.Unlock()
	return nil
}

// cancelBackend asks the server, over a second connection made with the
// same options, to cancel the query of process pid, but only if that
// process started at start: after a reconnection the number may belong
// to someone else's session. pid and start were checked when read, so
// they can't change the query.
func (s *Session) cancelBackend(pid int, start string) error {
	sql := "select pg_cancel_backend(pid) from pg_stat_activity where pid = " + strconv.Itoa(pid) +
		" and backend_start::text = '" + start + "'"
	args := append(append([]string{"-X", "-q", "-A", "-t"}, s.args...), "-c", sql)
	j := runner.Start(runner.Cmd{Name: s.bin, Args: args, Env: s.env, Timeout: CancelTimeout, Label: "cancel"})
	j.Wait()
	out := strings.TrimSpace(j.Output())
	switch {
	case j.State() != runner.Done:
		return errors.New("can't cancel: " + j.LastError())
	case out == "t":
		return nil
	case out == "":
		return errors.New("can't cancel: this session's connection has changed")
	default:
		return errors.New("the server refused to cancel the query")
	}
}

// Close ends the program.
func (s *Session) Close() {
	s.job.CloseInput()
	s.job.Stop()
}

// Entries returns a copy of the transcript.
func (s *Session) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if e.hidden {
			continue
		}
		c := *e
		c.after = nil
		c.Output = append([]mint.LogLine(nil), e.Output...)
		out = append(out, c)
	}
	return out
}

// Wait polls until the running command finishes or timeout passes, and
// reports whether it finished. For scripts and tests.
func (s *Session) Wait(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		s.Poll()
		if !s.Busy() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
