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
	j := Start(Cmd{Shell: "echo oops >&2; exit 3", Label: "thing"})
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
	j := Start(Cmd{Shell: `printf 'start\n10%%\r50%%\r100%%\ndone\r\n'; printf 'a\rb'`, Progress: Percent})
	waitOrFail(t, j, 5*time.Second)
	if got := strings.Join(texts(j), "|"); got != "start|100%|done|b" {
		t.Fatalf("lines %q", got)
	}
	if j.Percent() != 100 {
		t.Fatalf("percent %d", j.Percent())
	}
}

func TestProgressWhileRunning(t *testing.T) {
	j := Start(Cmd{Shell: `printf '37%%\r'; sleep 2`, Progress: Percent})
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
	j := Start(Cmd{Shell: `sleep 60 & echo $!; sleep 60 & echo $!; wait`})
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
	j := Start(Cmd{Shell: `trap '' TERM; echo ready; while :; do sleep 0.1; done`})
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
	j := Start(Cmd{Shell: "sleep 30", Timeout: 300 * time.Millisecond})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != TimedOut || j.Badge().Text != "sleep timed out" {
		t.Fatalf("state %v badge %q", j.State(), j.Badge().Text)
	}
}

func TestInput(t *testing.T) {
	j := Start(Cmd{Shell: `read a; read b; echo "got $a and $b"`, Input: true})
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
	if err := Start(Cmd{Shell: "true"}).SendLine("x"); err == nil {
		t.Fatal("SendLine without Input should fail")
	}
}

func TestNoTerminalAccess(t *testing.T) {
	// A job must not be able to open the controlling terminal.
	j := Start(Cmd{Shell: `if (: </dev/tty) 2>/dev/null; then echo tty; else echo notty; fi`})
	waitOrFail(t, j, 5*time.Second)
	if got := strings.Join(texts(j), "|"); got != "notty" {
		t.Fatalf("got %q", got)
	}
}

func TestLineCap(t *testing.T) {
	j := Start(Cmd{Shell: "seq 1 20000", MaxLines: 100})
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
	j := Start(Cmd{Shell: `printf '\033[1;31mred\033[0m\tx\n\033]0;title\007ok\n'`})
	waitOrFail(t, j, 5*time.Second)
	if got := strings.Join(texts(j), "|"); got != "red     x|ok" {
		t.Fatalf("lines %q", got)
	}
}

func TestBackgroundChildHoldingOutput(t *testing.T) {
	// The program exits at once but leaves a child holding stdout open.
	j := Start(Cmd{Shell: `sleep 30 & echo $!`})
	waitOrFail(t, j, 5*time.Second)
	if j.State() != Done {
		t.Fatalf("state %v", j.State())
	}
	if p, err := strconv.Atoi(texts(j)[0]); err == nil {
		_ = syscall.Kill(p, syscall.SIGKILL)
	}
}

func TestChanged(t *testing.T) {
	j := Start(Cmd{Shell: "echo a"})
	waitOrFail(t, j, 5*time.Second)
	if !j.Changed() {
		t.Fatal("first Changed should be true")
	}
	if j.Changed() {
		t.Fatal("second Changed should be false")
	}
}

func TestStopAll(t *testing.T) {
	a := Start(Cmd{Shell: "sleep 30"})
	b := Start(Cmd{Shell: "sleep 30"})
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
	j := Start(Cmd{Shell: `trap 'echo got INT; exit 3' INT; echo ready; while :; do sleep 0.1; done`,
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
	j := Start(Cmd{Shell: "seq 1 10"})
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
	j := Start(Cmd{Shell: "seq 1 30000", Progress: slow})
	waitOrFail(t, j, 30*time.Second)
	if total, last := j.Total(), j.Tail(1)[0].Text; total != 30000 || last != "30000" {
		t.Fatalf("got %d lines, last %q; want 30000 lines ending \"30000\"", total, last)
	}
}

func TestRedactRewritesLines(t *testing.T) {
	j := Start(Cmd{Shell: "echo 'pw=hunter22'; echo 'pw=hunter22' >&2; exit 3",
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
