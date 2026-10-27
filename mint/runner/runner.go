// Package runner executes external commands in the background for a Mint
// screen, capturing output line-by-line, parsing progress, and stopping
// the whole process group cleanly on request or timeout.
package runner

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// State is the lifecycle stage of a Job.
type State int

const (
	Idle State = iota
	Running
	Done
	Failed
	Stopped
	TimedOut
)

func (s State) String() string {
	switch s {
	case Running:
		return "running"
	case Done:
		return "done"
	case Failed:
		return "failed"
	case Stopped:
		return "stopped"
	case TimedOut:
		return "timed out"
	default:
		return "idle"
	}
}

// DefaultMaxLines is the default ring-buffer capacity for a Job when
// Cmd.MaxLines is 0. Raised to 50,000 lines so tools with large text output
// retain full context in memory without unbounded growth.
const DefaultMaxLines = 50000

// Cmd describes a command to run in the background.
type Cmd struct {
	Name       string                   // program to execute; it runs directly, never through a shell
	Bin        string                   // alias for Name
	Args       []string                 // arguments passed to Name/Bin
	Dir        string                   // working directory ("" = inherit)
	Env        []string                 // extra KEY=VALUE entries added to the environment
	Stdin      string                   // written to stdin, then stdin is closed
	Input      bool                     // keep stdin open for SendLine
	Timeout    time.Duration            // 0 = no timeout
	MaxLines   int                      // 0 = DefaultMaxLines (50,000)
	StopSignal syscall.Signal           // signal sent by Stop (0 = SIGTERM)
	RawStdout  int                      // keep up to this many bytes of stdout exactly as written, for parsing machine-readable output (0 = off); see Job.RawStdout
	OKExits    map[int]string           // exit codes besides 0 that are results, not failures, with what they mean (diff: 1 → "files differ")
	Label      string                   // short name for status badges ("" = command name)
	Progress   func(string) (int, bool) // optional parser returning (0..100, true) from a line
	OnLine     func(mint.LogLine)       // optional callback invoked on every line
	Redact     func(string) string      // optional: rewrites each line before it is stored or shown (hides secrets)
	OnExit     func(*Job)               // optional callback invoked when the job finishes
}

// Job is a running or finished background command. All methods are safe to
// call from any goroutine.
type Job struct {
	c       Cmd
	mu      sync.Mutex
	state   State
	code    int
	err     error
	pct     int
	lines   []mint.LogLine
	dropped int
	lastErr string
	dirty   bool
	started time.Time
	ended   time.Time
	pid     int
	stdin   io.WriteCloser
	done    chan struct{}
	raw     *capBuffer // stdout as written, when Cmd.RawStdout > 0

	// lastRead is when output was last read (UnixNano). After the program
	// exits, its remaining output is read for as long as it keeps arriving.
	lastRead atomic.Int64
}

// drainIdle is how long the runner keeps reading after the program exits
// with nothing new arriving, before deciding the pipes are held open by a
// background child and closing them.
const drainIdle = 2 * time.Second

var (
	activeMu sync.Mutex
	active   = map[*Job]struct{}{}
)

// StopAll terminates every job started in this process that is still running.
// Call it after app.Run() returns so background commands never outlive the UI.
func StopAll() {
	activeMu.Lock()
	jobs := make([]*Job, 0, len(active))
	for j := range active {
		jobs = append(jobs, j)
	}
	activeMu.Unlock()
	for _, j := range jobs {
		j.Stop()
	}
	for _, j := range jobs {
		select {
		case <-j.done:
		case <-time.After(3 * time.Second):
		}
	}
}

// Start spawns c in its own session (no controlling terminal) and begins
// reading its stdout and stderr in the background.
func Start(c Cmd) *Job {
	if c.MaxLines <= 0 {
		c.MaxLines = DefaultMaxLines
	}
	if c.Name == "" && c.Bin != "" {
		c.Name = c.Bin
	}
	if c.Label == "" {
		if c.Name != "" {
			c.Label = filepath.Base(c.Name)
		} else {
			c.Label = "job"
		}
	}
	j := &Job{
		c:       c,
		state:   Running,
		pct:     -1,
		dirty:   true,
		started: time.Now(),
		done:    make(chan struct{}),
	}

	cmd := exec.Command(c.Name, c.Args...)
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}
	if len(c.Env) > 0 {
		cmd.Env = append(cmd.Environ(), c.Env...)
	}
	// Setsid detaches from the controlling terminal so the child cannot read
	// keystrokes or scribble over the Mint screen, and creates a new process
	// group led by cmd.Process.Pid so Stop can kill the whole tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	var stdinPipe io.WriteCloser
	var err3 error
	if c.Input {
		stdinPipe, err3 = cmd.StdinPipe()
	} else if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	if err := firstErr(err1, err2, err3); err != nil {
		j.failStart(err)
		return j
	}
	if err := cmd.Start(); err != nil {
		j.failStart(err)
		return j
	}

	j.mu.Lock()
	j.pid = cmd.Process.Pid
	j.stdin = stdinPipe
	j.mu.Unlock()

	activeMu.Lock()
	active[j] = struct{}{}
	activeMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	var out io.Reader = stdout
	if c.RawStdout > 0 {
		j.raw = &capBuffer{limit: c.RawStdout}
		out = io.TeeReader(stdout, j.raw)
	}
	go func() {
		defer wg.Done()
		j.readPipe(out, false)
	}()
	go func() {
		defer wg.Done()
		j.readPipe(stderr, true)
	}()

	var timer *time.Timer
	if c.Timeout > 0 {
		timer = time.AfterFunc(c.Timeout, func() {
			j.stopWith(TimedOut)
		})
	}

	go func() {
		// Wait for the main process to exit without closing stdout/stderr yet,
		// so buffered pipe output (like `seq 1 20000`) drains completely, while
		// still timing out if a background grandchild holds stdout open.
		pstate, perr := cmd.Process.Wait()
		if timer != nil {
			timer.Stop()
		}
		drainDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(drainDone)
		}()
		// Read everything the program wrote before it exited, however long
		// that takes. Only when nothing has arrived for drainIdle (a
		// background child is holding the pipes open) are they closed.
		j.touch()
		tick := time.NewTicker(50 * time.Millisecond)
	drain:
		for {
			select {
			case <-drainDone:
				break drain
			case <-tick.C:
				if time.Since(time.Unix(0, j.lastRead.Load())) > drainIdle {
					_ = stdout.Close()
					_ = stderr.Close()
					<-drainDone
					break drain
				}
			}
		}
		tick.Stop()
		_ = cmd.Wait()

		activeMu.Lock()
		delete(active, j)
		activeMu.Unlock()

		j.mu.Lock()
		if stdinPipe != nil {
			_ = stdinPipe.Close()
			j.stdin = nil
		}
		j.ended = time.Now()
		j.dirty = true
		if j.state == Running {
			exitCode := 0
			if pstate != nil {
				exitCode = pstate.ExitCode()
			} else if perr != nil {
				exitCode = -1
			}
			_, okExit := j.c.OKExits[exitCode]
			if exitCode == 0 && perr == nil || okExit && exitCode > 0 {
				j.state = Done
				j.code = exitCode
				j.pct = 100
			} else {
				j.state = Failed
				j.err = perr
				j.code = exitCode
				if j.lastErr == "" {
					j.lastErr = fmt.Sprintf("exit %d", j.code)
				}
			}
		}
		onExit := j.c.OnExit
		j.mu.Unlock()

		if onExit != nil {
			onExit(j)
		}
		close(j.done)
	}()

	return j
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func (j *Job) failStart(err error) {
	msg := "could not start " + j.c.Label + ": " + err.Error()
	j.state = Failed
	j.code = -1
	j.err = err
	j.lastErr = msg
	j.ended = time.Now()
	j.lines = []mint.LogLine{{Text: msg, Err: true}}
	close(j.done)
	if j.c.OnExit != nil {
		go j.c.OnExit(j)
	}
}

// MaxLineBytes is the longest line kept from a program's output. Text past
// it, up to the next line break, is dropped, so a stream with no newlines
// (a large blob sent to "nc -l", a binary file) can't grow memory without
// bound. A variable so tests can shorten it.
var MaxLineBytes = 16 * 1024

// LineTruncatedMark ends a line that was cut at MaxLineBytes.
const LineTruncatedMark = " … [line too long: the rest is not shown]"

func (j *Job) readPipe(r io.Reader, isErr bool) {
	br := bufio.NewReaderSize(r, 64*1024)
	var cur []byte
	lastWasCR := false
	skipping := false // past MaxLineBytes: drop bytes until the next line break
	n := 0

	emit := func(replaceable bool) {
		j.appendLine(displayLine(cur), isErr, replaceable)
		cur = cur[:0]
	}

	for {
		b, err := br.ReadByte()
		if n++; n&1023 == 0 {
			j.touch()
		}
		if err != nil {
			if len(cur) > 0 {
				emit(false)
			}
			return
		}
		switch b {
		case '\r':
			if skipping {
				skipping = false
				lastWasCR = false
				continue
			}
			emit(true)
			lastWasCR = true
		case '\n':
			if skipping {
				skipping = false
				lastWasCR = false
				continue
			}
			if lastWasCR && len(cur) == 0 {
				// \r\n pair: the line was already emitted on \r; mark it final.
				j.finalizeLast(isErr)
				lastWasCR = false
				continue
			}
			emit(false)
			lastWasCR = false
		default:
			if skipping {
				continue
			}
			cur = append(cur, b)
			if len(cur) >= MaxLineBytes {
				text := displayLine(trimPartialRune(cur)) + LineTruncatedMark
				j.appendLine(text, isErr, false)
				cur = cur[:0]
				skipping = true
				lastWasCR = false
			}
		}
	}
}

// displayLine turns raw output bytes into text that is safe to show. Lines
// that look binary (a NUL byte, or mostly invalid UTF-8) are summarised
// rather than cleaned, since cleaning would leave meaningless fragments.
func displayLine(raw []byte) string {
	if looksBinary(raw) {
		return binarySummary(raw)
	}
	return mint.CleanLine(string(raw))
}

// looksBinary reports whether raw is binary data rather than text: it has
// a NUL byte, or more than a quarter of it is not valid UTF-8.
func looksBinary(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	bad := 0
	for i := 0; i < len(raw); {
		if raw[i] == 0 {
			return true
		}
		r, size := utf8.DecodeRune(raw[i:])
		if r == utf8.RuneError && size == 1 {
			bad++
		}
		i += size
	}
	return bad*4 > len(raw)
}

// binarySummary shows binary data as its size and its first bytes in hex.
func binarySummary(raw []byte) string {
	const show = 16
	head := raw
	if len(head) > show {
		head = head[:show]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[binary data, %d bytes:", len(raw))
	for _, c := range head {
		fmt.Fprintf(&b, " %02x", c)
	}
	if len(raw) > show {
		b.WriteString(" …")
	}
	b.WriteString("]")
	return b.String()
}

// trimPartialRune drops an incomplete UTF-8 sequence at the end of b, left
// there when a line is cut at a byte limit.
func trimPartialRune(b []byte) []byte {
	for i := 1; i <= utf8.UTFMax && i <= len(b); i++ {
		c := b[len(b)-i]
		if c < 0x80 {
			return b // ASCII: nothing partial
		}
		if utf8.RuneStart(c) {
			if !utf8.FullRune(b[len(b)-i:]) {
				return b[:len(b)-i]
			}
			return b
		}
	}
	return b
}

// touch records that output is still being read.
func (j *Job) touch() { j.lastRead.Store(time.Now().UnixNano()) }

func (j *Job) finalizeLast(isErr bool) {
	j.mu.Lock()
	for i := len(j.lines) - 1; i >= 0; i-- {
		if j.lines[i].Err == isErr {
			j.lines[i].Overwrite = false
			break
		}
	}
	j.mu.Unlock()
}

func (j *Job) appendLine(text string, isErr, replaceable bool) {
	if j.c.Redact != nil {
		text = j.c.Redact(text)
	}
	line := mint.LogLine{Text: text, Err: isErr, Overwrite: replaceable}

	var pct int = -1
	if j.c.Progress != nil {
		if p, ok := j.c.Progress(text); ok && p >= 0 && p <= 100 {
			pct = p
		}
	}

	j.mu.Lock()
	if pct >= 0 {
		j.pct = pct
	}
	if isErr && strings.TrimSpace(text) != "" {
		j.lastErr = strings.TrimSpace(text)
	}
	n := len(j.lines)
	if n > 0 && j.lines[n-1].Err == isErr && j.lines[n-1].Overwrite {
		j.lines[n-1] = line
	} else {
		j.lines = append(j.lines, line)
		if len(j.lines) > j.c.MaxLines {
			// Compact when buffer exceeds MaxLines so len(j.lines) stays == MaxLines
			// and amortized compaction stays O(1).
			if cap(j.lines) > j.c.MaxLines*2 && len(j.lines) >= j.c.MaxLines*2 {
				kept := make([]mint.LogLine, j.c.MaxLines)
				drop := len(j.lines) - j.c.MaxLines
				copy(kept, j.lines[drop:])
				j.lines = kept
				j.dropped += drop
			} else {
				drop := len(j.lines) - j.c.MaxLines
				j.lines = j.lines[drop:]
				j.dropped += drop
			}
		}
	}
	j.dirty = true
	onLine := j.c.OnLine
	j.mu.Unlock()

	if onLine != nil {
		onLine(line)
	}
}

// SendLine writes a line (followed by '\n') to the job's standard input when
// Cmd.Input is true.
func (j *Job) SendLine(line string) error {
	j.mu.Lock()
	w := j.stdin
	j.mu.Unlock()
	if w == nil {
		return errors.New("job stdin is not open (set Cmd.Input: true)")
	}
	_, err := io.WriteString(w, line+"\n")
	return err
}

// Write sends p to the job's standard input as it is (Cmd.Input must be
// true). Unlike SendLine it makes no copy, so a secret held in protected
// memory can be written straight from there.
func (j *Job) Write(p []byte) (int, error) {
	j.mu.Lock()
	w := j.stdin
	j.mu.Unlock()
	if w == nil {
		return 0, errors.New("job stdin is not open (set Cmd.Input: true)")
	}
	return w.Write(p)
}

// Stop terminates the job and its entire process group (SIGTERM or
// Cmd.StopSignal, followed by SIGKILL after 2s if any process ignores it).
func (j *Job) Stop() {
	j.stopWith(Stopped)
}

func (j *Job) stopWith(target State) {
	j.mu.Lock()
	if j.state != Running {
		j.mu.Unlock()
		return
	}
	j.state = target
	j.code = -1
	j.dirty = true
	pid := j.pid
	sig := j.c.StopSignal
	if sig == 0 {
		sig = syscall.SIGTERM
	}
	j.mu.Unlock()

	if pid > 0 {
		_ = syscall.Kill(-pid, sig)
		go func() {
			select {
			case <-j.done:
			case <-time.After(2 * time.Second):
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		}()
	}
}

// Done returns a channel that closes when the job finishes.
func (j *Job) Done() <-chan struct{} {
	return j.done
}

// Wait blocks until the job finishes.
func (j *Job) Wait() {
	<-j.done
}

// CloseInput closes the job's standard input pipe, if Input was enabled.
func (j *Job) CloseInput() {
	j.mu.Lock()
	in := j.stdin
	j.stdin = nil
	j.mu.Unlock()
	if in != nil {
		_ = in.Close()
	}
}

// Running reports whether the command is still executing.
func (j *Job) Running() bool { return j.State() == Running }

// State returns the job's current lifecycle state.
func (j *Job) State() State {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state
}

// ExitCode returns the process exit code (0 on success, -1 if it never
// started or was killed by a signal).
func (j *Job) ExitCode() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.code
}

// Err returns the underlying start or exit error, if any.
func (j *Job) Err() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.err
}

// Percent returns the most recent 0..100 progress value, or 0 if none has
// been seen yet.
func (j *Job) Percent() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.pct < 0 {
		return 0
	}
	return j.pct
}

// Elapsed returns how long the job has been running (or ran for).
func (j *Job) Elapsed() time.Duration {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.ended.IsZero() {
		return time.Since(j.started)
	}
	return j.ended.Sub(j.started)
}

// LastError returns the last non-empty stderr line (or start error), suitable
// for passing straight to mint.ToastError.
func (j *Job) LastError() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.lastErr
}

// Lines returns a copy of the captured output lines, ready for mint.Log or
// mint.ViewLog.
func (j *Job) Lines() []mint.LogLine {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]mint.LogLine, len(j.lines))
	copy(out, j.lines)
	return out
}

// Output returns all kept lines joined by '\n' (with a trailing '\n' when
// non-empty).
func (j *Job) Output() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, l := range j.lines {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// Total returns the total number of lines emitted so far, including any
// that were dropped once MaxLines was reached.
func (j *Job) Total() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.dropped + len(j.lines)
}

// Tail returns a copy of the last n kept LogLines.
func (j *Job) Tail(n int) []mint.LogLine {
	tail, _ := j.TailLines(n)
	return tail
}

// TailLines returns a copy of the last n LogLines (or all kept lines when
// n <= 0) together with the total number of lines emitted so far.
func (j *Job) TailLines(n int) (tail []mint.LogLine, total int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	lines := j.lines
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]mint.LogLine, len(lines))
	copy(out, lines)
	return out, j.dropped + len(j.lines)
}

// Since returns the lines from absolute line number from on (counting
// from 0 since the job started, including lines dropped to keep within
// MaxLines), the absolute number of the first line returned (later than
// from if older lines were dropped), and the total so far. It is one
// consistent read, for following output as it arrives.
func (j *Job) Since(from int) (lines []mint.LogLine, first, total int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	total = j.dropped + len(j.lines)
	first = max(from, j.dropped)
	if first >= total {
		return nil, total, total
	}
	lines = make([]mint.LogLine, total-first)
	copy(lines, j.lines[first-j.dropped:])
	return lines, first, total
}

// Slice returns up to count kept LogLines starting at index start (0-based
// within the kept buffer) together with the total number of kept lines.
func (j *Job) Slice(start, count int) (lines []mint.LogLine, kept int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	kept = len(j.lines)
	if start < 0 {
		start = 0
	}
	if start >= kept || count <= 0 {
		return nil, kept
	}
	end := start + count
	if end > kept {
		end = kept
	}
	out := make([]mint.LogLine, end-start)
	copy(out, j.lines[start:end])
	return out, kept
}

// Changed reports whether anything new has happened since the last call to
// Changed, and clears the flag. Wire it directly to Screen.Poll:
//
//	Poll: func(*mint.Screen) bool { return job != nil && job.Changed() }
func (j *Job) Changed() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	d := j.dirty
	j.dirty = false
	return d
}

// Badge returns a single status badge describing the job's current state.
func (j *Job) Badge() mint.Badge {
	j.mu.Lock()
	state, pct, code, label, startErr := j.state, j.pct, j.code, j.c.Label, j.err
	var dur time.Duration
	if j.ended.IsZero() {
		dur = time.Since(j.started)
	} else {
		dur = j.ended.Sub(j.started)
	}
	j.mu.Unlock()

	switch state {
	case Running:
		if pct >= 0 {
			return mint.Badge{Text: fmt.Sprintf("%s %d%% · %s", label, pct, Clock(dur)), Kind: mint.BadgeWarn}
		}
		return mint.Badge{Text: fmt.Sprintf("%s · %s", label, Clock(dur)), Kind: mint.BadgeWarn}
	case Done:
		if meaning := j.ExitMeaning(); meaning != "" {
			return mint.Badge{Text: fmt.Sprintf("%s: %s · %s", label, meaning, Clock(dur)), Kind: mint.BadgeOK}
		}
		return mint.Badge{Text: fmt.Sprintf("%s done in %s", label, Clock(dur)), Kind: mint.BadgeOK}
	case Stopped:
		return mint.Badge{Text: label + " stopped", Kind: mint.BadgeIdle}
	case TimedOut:
		return mint.Badge{Text: label + " timed out", Kind: mint.BadgeError}
	case Failed:
		if code == -1 && startErr != nil {
			return mint.Badge{Text: label + " could not start", Kind: mint.BadgeError}
		}
		return mint.Badge{Text: fmt.Sprintf("%s failed (exit %d)", label, code), Kind: mint.BadgeError}
	default:
		return mint.Badge{Text: label + " idle", Kind: mint.BadgeIdle}
	}
}

// ExitMeaning returns what the job's exit code means when Cmd.OKExits
// describes it ("files differ"), or "" otherwise.
func (j *Job) ExitMeaning() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state != Done {
		return ""
	}
	return j.c.OKExits[j.code]
}

// Badges returns a slice containing the job's current status Badge, ready for
// Screen.Status.
func (j *Job) Badges() []mint.Badge {
	return []mint.Badge{j.Badge()}
}

var (
	pctRe  = regexp.MustCompile(`(\d{1,3})(?:\.\d+)?\s*%`)
	fracRe = regexp.MustCompile(`(\d+)\s*(?:/|\bof\b)\s*(\d+)`)
)

// Percent is a ready-made Cmd.Progress parser that finds the last `NN%`
// (0..100) in a line. Covers curl, rsync, apt, ffmpeg -stats, wget, pv, etc.
func Percent(line string) (int, bool) {
	matches := pctRe.FindAllStringSubmatch(line, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(matches[i][1])
		if err == nil && n >= 0 && n <= 100 {
			return n, true
		}
	}
	return 0, false
}

// Fraction is a ready-made Cmd.Progress parser that matches `done/total` or
// `done of total` (such as `12/80 files` or `step 3 of 4`) and converts it
// to 0..100.
func Fraction(line string) (int, bool) {
	m := fracRe.FindStringSubmatch(line)
	if len(m) < 3 {
		return 0, false
	}
	done, err1 := strconv.Atoi(m[1])
	total, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil || total <= 0 || done < 0 || done > total {
		return 0, false
	}
	return done * 100 / total, true
}

// Clock formats d as M:SS or H:MM:SS.
func Clock(d time.Duration) string {
	secs := max(0, int(d.Seconds()))
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// capBuffer keeps the first limit bytes written to it and notes whether
// more arrived. Writes never fail, so the reader it tees from keeps going.
type capBuffer struct {
	mu        sync.Mutex
	limit     int
	b         []byte
	truncated bool
}

func (c *capBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := c.limit - len(c.b)
	if room < len(p) {
		c.truncated = true
		if room > 0 {
			c.b = append(c.b, p[:room]...)
		}
		return len(p), nil
	}
	c.b = append(c.b, p...)
	return len(p), nil
}

// RawStdout returns a copy of the program's standard output exactly as it
// wrote it, up to Cmd.RawStdout bytes, and whether more was cut off. It is
// for parsing (JSON, SARIF); the bytes are untrusted and must be cleaned
// before anything from them is shown. It is nil unless Cmd.RawStdout > 0.
func (j *Job) RawStdout() (data []byte, truncated bool) {
	if j.raw == nil {
		return nil, false
	}
	j.raw.mu.Lock()
	defer j.raw.mu.Unlock()
	return append([]byte(nil), j.raw.b...), j.raw.truncated
}
