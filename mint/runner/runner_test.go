package runner

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// shArgs runs script with sh -c. Tests use a shell to make test programs;
// the runner itself never does.
func shArgs(script string) []string { return []string{"-c", script} }

func texts(j *Job) []string {
	var out []string
	for _, l := range j.Lines() {
		out = append(out, l.Text)
	}
	return out
}

func waitOrFail(t *testing.T, j *Job, limit time.Duration) {
	t.Helper()
	select {
	case <-j.Done():
	case <-time.After(limit):
		t.Fatalf("job still running after %v", limit)
	}
}

func TestDoneAndOutput(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: []string{"-c", "echo one; echo two"}})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Done || j.ExitCode() != 0 || j.Percent() != 100 {
		t.Fatalf("state %v code %d pct %d", j.State(), j.ExitCode(), j.Percent())
	}
	if got := strings.Join(texts(j), "|"); got != "one|two" {
		t.Fatalf("lines %q", got)
	}
	if j.Output() != "one\ntwo\n" {
		t.Fatalf("output %q", j.Output())
	}
	if b := j.Badge(); !strings.HasPrefix(b.Text, "sh done in 0:0") {
		t.Fatalf("badge %q", b.Text)
	}
}

func TestExitCode(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs("echo oops >&2; exit 3"), Label: "thing"})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Failed || j.ExitCode() != 3 {
		t.Fatalf("state %v code %d", j.State(), j.ExitCode())
	}
	l := j.Lines()
	if len(l) != 1 || l[0].Text != "oops" || !l[0].Err {
		t.Fatalf("stderr line %+v", l)
	}
	if b := j.Badge().Text; b != "thing failed (exit 3)" {
		t.Fatalf("badge %q", b)
	}
}

func TestMissingProgram(t *testing.T) {
	j := Start(Cmd{Name: "definitely-not-a-real-program-xyz"})
	waitOrFail(t, j, time.Second)
	if j.State() != Failed || j.Err() == nil {
		t.Fatalf("state %v err %v", j.State(), j.Err())
	}
	if l := j.Lines(); len(l) != 1 || !strings.HasPrefix(l[0].Text, "could not start") {
		t.Fatalf("lines %+v", l)
	}
	if b := j.Badge().Text; !strings.HasSuffix(b, "could not start") {
		t.Fatalf("badge %q", b)
	}
}

func TestCarriageReturnRedraw(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs(`printf 'start\n10%%\r50%%\r100%%\ndone\r\n'; printf 'a\rb'`), Progress: Percent})
	waitOrFail(t, j, 5*time.Second)
	if got := strings.Join(texts(j), "|"); got != "start|100%|done|b" {
		t.Fatalf("lines %q", got)
	}
	if j.Percent() != 100 {
		t.Fatalf("percent %d", j.Percent())
	}
}

func TestProgressWhileRunning(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs(`printf '37%%\r'; sleep 2`), Label: "printf", Progress: Percent})
	defer j.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for j.Percent() != 37 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if j.Percent() != 37 || !j.Running() {
		t.Fatalf("percent %d running %v", j.Percent(), j.Running())
	}
	if b := j.Badge().Text; !strings.HasPrefix(b, "printf 37% · 0:0") {
		t.Fatalf("badge %q", b)
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestStopKillsChildren(t *testing.T) {
	// The shell starts two background sleeps and prints their pids.
	j := Start(Cmd{Name: "sh", Args: shArgs(`sleep 60 & echo $!; sleep 60 & echo $!; wait`), Label: "sleep"})
	var pids []int
	deadline := time.Now().Add(3 * time.Second)
	for len(pids) < 2 && time.Now().Before(deadline) {
		pids = pids[:0]
		for _, s := range texts(j) {
			if p, err := strconv.Atoi(s); err == nil {
				pids = append(pids, p)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(pids) < 2 {
		t.Fatalf("children not started: %v", texts(j))
	}
	j.Stop()
	waitOrFail(t, j, 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	for _, p := range pids {
		if alive(p) {
			// a zombie still answers kill(0); check /proc state
			if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", p)); err == nil && !strings.Contains(string(b), ") Z ") {
				t.Fatalf("child %d still running after Stop", p)
			}
		}
	}
	if j.State() != Stopped || j.Badge().Text != "sleep stopped" {
		t.Fatalf("state %v badge %q", j.State(), j.Badge().Text)
	}
}

func TestStopEscalatesToKill(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs(`trap '' TERM; echo ready; while :; do sleep 0.1; done`)})
	for len(j.Lines()) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	j.Stop()
	waitOrFail(t, j, 6*time.Second)
	if d := time.Since(start); d < 2*time.Second {
		t.Fatalf("ended after %v; TERM should have been ignored", d)
	}
	if j.State() != Stopped {
		t.Fatalf("state %v", j.State())
	}
}

func TestTimeout(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs("sleep 30"), Label: "sleep", Timeout: 300 * time.Millisecond})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != TimedOut || j.Badge().Text != "sleep timed out" {
		t.Fatalf("state %v badge %q", j.State(), j.Badge().Text)
	}
}

func TestInput(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs(`read a; read b; echo "got $a and $b"`), Input: true})
	if err := j.SendLine("hello"); err != nil {
		t.Fatal(err)
	}
	if err := j.SendLine("world"); err != nil {
		t.Fatal(err)
	}
	waitOrFail(t, j, 5*time.Second)
	if got := strings.Join(texts(j), "|"); got != "got hello and world" {
		t.Fatalf("lines %q", got)
	}
	if err := Start(Cmd{Name: "sh", Args: shArgs("true")}).SendLine("x"); err == nil {
		t.Fatal("SendLine without Input should fail")
	}
}

func TestNoTerminalAccess(t *testing.T) {
	// A job must not be able to open the controlling terminal.
	j := Start(Cmd{Name: "sh", Args: shArgs(`if (: </dev/tty) 2>/dev/null; then echo tty; else echo notty; fi`)})
	waitOrFail(t, j, 5*time.Second)
	if got := strings.Join(texts(j), "|"); got != "notty" {
		t.Fatalf("got %q", got)
	}
}

func TestLineCap(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs("seq 1 20000"), MaxLines: 100})
	waitOrFail(t, j, 10*time.Second)
	l := texts(j)
	if len(l) != 100 || l[0] != "19901" || l[99] != "20000" {
		t.Fatalf("len %d first %q last %q", len(l), l[0], l[len(l)-1])
	}
	if tail := j.Tail(3); len(tail) != 3 || tail[2].Text != "20000" {
		t.Fatalf("tail %+v", tail)
	}
}

func TestCleanOutput(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs(`printf '\033[1;31mred\033[0m\tx\n\033]0;title\007ok\n'`)})
	waitOrFail(t, j, 5*time.Second)
	if got := strings.Join(texts(j), "|"); got != "red     x|ok" {
		t.Fatalf("lines %q", got)
	}
}

func TestBackgroundChildHoldingOutput(t *testing.T) {
	// The program exits at once but leaves a child holding stdout open.
	j := Start(Cmd{Name: "sh", Args: shArgs(`sleep 30 & echo $!`)})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Done {
		t.Fatalf("state %v", j.State())
	}
	if p, err := strconv.Atoi(texts(j)[0]); err == nil {
		_ = syscall.Kill(p, syscall.SIGKILL)
	}
}

func TestChanged(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs("echo a")})
	waitOrFail(t, j, 5*time.Second)
	if !j.Changed() {
		t.Fatal("first Changed should be true")
	}
	if j.Changed() {
		t.Fatal("second Changed should be false")
	}
}

func TestStopAll(t *testing.T) {
	a := Start(Cmd{Name: "sh", Args: shArgs("sleep 30")})
	b := Start(Cmd{Name: "sh", Args: shArgs("sleep 30")})
	StopAll()
	if a.State() != Stopped || b.State() != Stopped {
		t.Fatalf("states %v %v", a.State(), b.State())
	}
}

func TestParsers(t *testing.T) {
	for in, want := range map[string]int{
		"  45%":                            45,
		"1,234,567  12%  1.2MB/s  0:00:03": 12,
		"Downloading 12.5 % done":          12,
		"progress 10% then 80%":            80,
	} {
		if got, ok := Percent(in); !ok || got != want {
			t.Errorf("Percent(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := Percent("no numbers"); ok {
		t.Error("Percent matched nothing")
	}
	if _, ok := Percent("250%"); ok {
		t.Error("Percent accepted 250")
	}
	for in, want := range map[string]int{"12/80 files": 15, "step 3 of 4": 75} {
		if got, ok := Fraction(in); !ok || got != want {
			t.Errorf("Fraction(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := Fraction("9/0"); ok {
		t.Error("Fraction divided by zero")
	}
	if Clock(42*time.Second) != "0:42" || Clock(3723*time.Second) != "1:02:03" {
		t.Error("Clock")
	}
}

func TestStopSignal(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs(`trap 'echo got INT; exit 3' INT; echo ready; while :; do sleep 0.1; done`),
		StopSignal: syscall.SIGINT})
	for len(j.Lines()) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	j.Stop()
	waitOrFail(t, j, 5*time.Second)
	if got := texts(j); len(got) < 2 || got[1] != "got INT" {
		t.Fatalf("handler did not run: %v", got)
	}
}

func TestTailLinesAndSlice(t *testing.T) {
	if DefaultMaxLines != 50000 {
		t.Fatalf("DefaultMaxLines = %d, want 50000", DefaultMaxLines)
	}
	j := Start(Cmd{Name: "sh", Args: shArgs("seq 1 10")})
	waitOrFail(t, j, 5*time.Second)
	tail, total := j.TailLines(3)
	if total != 10 || len(tail) != 3 || tail[0].Text != "8" || tail[2].Text != "10" {
		t.Fatalf("TailLines(3) = %+v, total=%d", tail, total)
	}
	slice, kept := j.Slice(2, 4)
	if kept != 10 || len(slice) != 4 || slice[0].Text != "3" || slice[3].Text != "6" {
		t.Fatalf("Slice(2,4) = %+v, kept=%d", slice, kept)
	}
}

// A slow line handler must not lose the end of the output: the program
// exits with up to a pipe's worth of output still unread.
func TestSlowReaderKeepsAllOutput(t *testing.T) {
	slow := func(line string) (int, bool) {
		for end := time.Now().Add(30 * time.Microsecond); time.Now().Before(end); {
		}
		return Percent(line)
	}
	j := Start(Cmd{Name: "sh", Args: shArgs("seq 1 30000"), Progress: slow})
	waitOrFail(t, j, 30*time.Second)
	if total, last := j.Total(), j.Tail(1)[0].Text; total != 30000 || last != "30000" {
		t.Fatalf("got %d lines, last %q; want 30000 lines ending \"30000\"", total, last)
	}
}

func TestRedactRewritesLines(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs("echo 'pw=hunter22'; echo 'pw=hunter22' >&2; exit 3"),
		Redact: func(s string) string { return strings.ReplaceAll(s, "hunter22", "***") }})
	j.Wait()
	for _, l := range j.Lines() {
		if strings.Contains(l.Text, "hunter22") {
			t.Fatalf("line not redacted: %q", l.Text)
		}
	}
	if strings.Contains(j.LastError(), "hunter22") {
		t.Fatalf("last error not redacted: %q", j.LastError())
	}
}

// newPipeJob is a Job with no process, for feeding readPipe directly.
func newPipeJob() *Job {
	return &Job{c: Cmd{MaxLines: DefaultMaxLines}, done: make(chan struct{})}
}

// A stream with no line breaks must not be held in memory whole.
func TestLongLineIsCut(t *testing.T) {
	old := MaxLineBytes
	MaxLineBytes = 1024
	defer func() { MaxLineBytes = old }()

	j := newPipeJob()
	in := strings.Repeat("a", 10*1024*1024) + "\nnext\n"
	j.readPipe(strings.NewReader(in), false)
	got := texts(j)
	if len(got) != 2 {
		t.Fatalf("got %d lines, want the cut line and the next one", len(got))
	}
	if !strings.HasSuffix(got[0], LineTruncatedMark) || len(got[0]) != 1024+len(LineTruncatedMark) {
		t.Fatalf("first line: %d bytes, ends %q", len(got[0]), got[0][len(got[0])-20:])
	}
	if got[1] != "next" {
		t.Fatalf("line after the long one = %q", got[1])
	}
}

// The long line is shown as soon as it reaches the limit, not when (if
// ever) its newline arrives.
func TestLongLineShownBeforeItEnds(t *testing.T) {
	old := MaxLineBytes
	MaxLineBytes = 64
	defer func() { MaxLineBytes = old }()

	j := newPipeJob()
	j.readPipe(strings.NewReader(strings.Repeat("z", 1000)), false) // EOF, no newline
	if got := texts(j); len(got) != 1 || !strings.HasPrefix(got[0], strings.Repeat("z", 64)) {
		t.Fatalf("lines %q", got)
	}
}

func TestLineCutKeepsWholeRunes(t *testing.T) {
	old := MaxLineBytes
	MaxLineBytes = 10
	defer func() { MaxLineBytes = old }()

	j := newPipeJob()
	j.readPipe(strings.NewReader("aaaaaaaaa€€€\n"), false) // the cut falls inside "€"
	got := texts(j)[0]
	if want := "aaaaaaaaa" + LineTruncatedMark; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBinaryOutputIsSummarised(t *testing.T) {
	j := newPipeJob()
	j.readPipe(strings.NewReader("ELF\x00\x01\x02\x03\nplain text\n\xff\xfe\xfd\xfc\xfbab\n"), false)
	got := texts(j)
	if len(got) != 3 {
		t.Fatalf("lines %q", got)
	}
	if got[0] != "[binary data, 7 bytes: 45 4c 46 00 01 02 03]" {
		t.Fatalf("NUL line = %q", got[0])
	}
	if got[1] != "plain text" {
		t.Fatalf("text line = %q", got[1])
	}
	if !strings.HasPrefix(got[2], "[binary data, 7 bytes:") {
		t.Fatalf("invalid UTF-8 line = %q", got[2])
	}
	// A little invalid UTF-8 in text is still text.
	j = newPipeJob()
	j.readPipe(strings.NewReader("caf\xe9 ordered\n"), false)
	if got := texts(j)[0]; got != "caf ordered" {
		t.Fatalf("mostly-text line = %q", got)
	}
}

// End to end through a real process: 4 MB with no newline.
func TestLongLineFromProgram(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: []string{"-c", "head -c 4194304 /dev/zero | tr '\\000' x; echo; echo done"}})
	waitOrFail(t, j, 20*time.Second)
	got := texts(j)
	if len(got) != 2 || len(got[0]) != MaxLineBytes+len(LineTruncatedMark) || got[1] != "done" {
		t.Fatalf("got %d lines, first %d bytes", len(got), len(got[0]))
	}
}

func TestOKExitsAreResults(t *testing.T) {
	ok := map[int]string{0: "no differences", 1: "files differ"}
	j := Start(Cmd{Name: "sh", Args: []string{"-c", "exit 1"}, Label: "diff", OKExits: ok})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Done || j.ExitCode() != 1 || j.ExitMeaning() != "files differ" {
		t.Fatalf("state %v code %d meaning %q", j.State(), j.ExitCode(), j.ExitMeaning())
	}
	if b := j.Badge(); !strings.HasPrefix(b.Text, "diff: files differ") {
		t.Fatalf("badge %q", b.Text)
	}

	j = Start(Cmd{Name: "sh", Args: []string{"-c", "exit 0"}, OKExits: ok})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Done || j.ExitMeaning() != "no differences" {
		t.Fatalf("exit 0: state %v meaning %q", j.State(), j.ExitMeaning())
	}

	// A code that isn't listed is still a failure.
	j = Start(Cmd{Name: "sh", Args: []string{"-c", "echo trouble >&2; exit 2"}, OKExits: ok})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Failed || j.ExitCode() != 2 || j.ExitMeaning() != "" || j.LastError() != "trouble" {
		t.Fatalf("exit 2: state %v code %d meaning %q err %q", j.State(), j.ExitCode(), j.ExitMeaning(), j.LastError())
	}

	// Killed by a signal is never a result, whatever the map says.
	j = Start(Cmd{Name: "sh", Args: []string{"-c", "kill -9 $$"}, OKExits: map[int]string{-1: "x", 137: "x"}})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Failed {
		t.Fatalf("signal: state %v", j.State())
	}

	// Without OKExits, exit 1 fails as before.
	j = Start(Cmd{Name: "sh", Args: []string{"-c", "exit 1"}})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Failed {
		t.Fatalf("no OKExits: state %v", j.State())
	}
}

// Machine-readable output is kept whole, even a single line longer than
// MaxLineBytes, while the displayed line is still cut.
func TestRawStdout(t *testing.T) {
	old := MaxLineBytes
	MaxLineBytes = 100
	defer func() { MaxLineBytes = old }()

	j := Start(Cmd{Name: "sh", Args: shArgs(`printf '{"x":"%0500d"}' 0; echo err >&2`), RawStdout: 1 << 20})
	waitOrFail(t, j, 5*time.Second)
	raw, cut := j.RawStdout()
	if cut || len(raw) != 508 || string(raw[:6]) != `{"x":"` {
		t.Fatalf("raw %d bytes, cut %v: %q", len(raw), cut, raw[:20])
	}
	if strings.Contains(string(raw), "err") {
		t.Fatal("stderr in raw stdout")
	}
	cutShown := false
	for _, l := range j.Lines() { // stderr's line may come first
		cutShown = cutShown || (!l.Err && strings.HasSuffix(l.Text, LineTruncatedMark))
	}
	if !cutShown {
		t.Fatalf("display line not cut: %q", texts(j))
	}

	j = Start(Cmd{Name: "sh", Args: shArgs("seq 1 100000"), RawStdout: 1000})
	waitOrFail(t, j, 5*time.Second)
	if raw, cut := j.RawStdout(); len(raw) != 1000 || !cut {
		t.Fatalf("limit: %d bytes, cut %v", len(raw), cut)
	}
	if j.Total() != 100000 {
		t.Fatalf("display lost lines past the raw limit: %d", j.Total())
	}

	if raw, _ := Start(Cmd{Name: "true"}).RawStdout(); raw != nil {
		t.Fatal("raw kept without RawStdout")
	}
}

func TestSince(t *testing.T) {
	j := Start(Cmd{Name: "sh", Args: shArgs("seq 1 10"), MaxLines: 4})
	waitOrFail(t, j, 5*time.Second)
	lines, first, total := j.Since(0)
	if total != 10 || first < 6 || len(lines) != total-first || lines[len(lines)-1].Text != "10" {
		t.Fatalf("Since(0): %d lines, first %d, total %d", len(lines), first, total)
	}
	if l, f, _ := j.Since(9); f != 9 || len(l) != 1 || l[0].Text != "10" {
		t.Fatalf("Since(9) = %v from %d", l, f)
	}
	if l, f, tot := j.Since(10); l != nil || f != 10 || tot != 10 {
		t.Fatalf("Since(10) = %v %d %d", l, f, tot)
	}
	if l, _, _ := j.Since(99); l != nil {
		t.Fatalf("Since(99) = %v", l)
	}
}
