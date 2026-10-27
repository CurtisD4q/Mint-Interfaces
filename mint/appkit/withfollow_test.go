package appkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/follow"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

func followApp(t *testing.T, bin string, spec FollowSpec) *ToolApp {
	t.Helper()
	return newTestApp(t, ToolSpec{Name: filepath.Base(bin), Bin: bin, ReadOnly: true, Follow: &spec})
}

func TestFollowArgs(t *testing.T) {
	f, _ := KnownFollowSpec("journalctl")
	a := newTestApp(t, ToolSpec{Name: "journalctl", Follow: &f, Flags: []FlagSpec{{Flag: "-u", Kind: FlagText}}})
	a.Values["-u"] = "ssh"
	args, _ := a.BuildArgs()
	if got := strings.Join(args, " "); got != "--output=json --follow -u ssh" {
		t.Fatalf("args = %q", got)
	}
	if a.followFormat() != follow.JournalJSON {
		t.Fatal("format should be journal-json")
	}
	// The user's own output format wins, and lines are then read as text.
	a.ExtraArgs = "-o cat -f"
	args, _ = a.BuildArgs()
	if got := strings.Join(args, " "); got != "-u ssh -o cat -f" {
		t.Fatalf("args with -o cat -f = %q", got)
	}
	if a.followFormat() != follow.Text {
		t.Fatal("-o cat should be read as text")
	}
	a.ExtraArgs = "-o json"
	if a.followFormat() != follow.JournalJSON {
		t.Fatal("-o json is still json")
	}

	tf, _ := KnownFollowSpec("tail")
	ta := newTestApp(t, ToolSpec{Name: "tail", Follow: &tf})
	ta.Target = "/var/log/syslog"
	if args, _ := ta.BuildArgs(); strings.Join(args, " ") != "-F /var/log/syslog" {
		t.Fatalf("tail args %q", args)
	}
	ta.ExtraArgs = "-f"
	if args, _ := ta.BuildArgs(); strings.Join(args, " ") != "-f /var/log/syslog" {
		t.Fatalf("tail -f args %q", args)
	}
}

func TestValidateFollow(t *testing.T) {
	for _, f := range []FollowSpec{
		{Format: "xml"},
		{Format: FollowJournalJSON},
		{Restart: "always"},
		{Levels: []FollowLevel{{Level: "loud", Regex: "x"}}},
		{Levels: []FollowLevel{{Level: "error", Regex: "("}}},
		{FollowArgs: []string{"file"}},
	} {
		f := f
		if err := (ToolSpec{Name: "x", Follow: &f}).Validate(); err == nil {
			t.Errorf("accepted %+v", f)
		}
	}
	for _, bin := range []string{"tail", "journalctl", "dmesg"} {
		f, _ := KnownFollowSpec(bin)
		if err := (ToolSpec{Name: bin, Follow: &f}).Validate(); err != nil {
			t.Errorf("%s: %v", bin, err)
		}
	}
}

func pollUntil(t *testing.T, s *followSource, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			var text []string
			for i := 0; i < s.Len(); i++ {
				text = append(text, s.Line(i).Text())
			}
			t.Fatalf("timed out; view has:\n%s", strings.Join(text, "\n"))
		}
		s.Poll()
		time.Sleep(20 * time.Millisecond)
	}
}

func viewText(s *followSource) string {
	var out []string
	for i := 0; i < s.Len(); i++ {
		out = append(out, s.Line(i).Text())
	}
	return strings.Join(out, "\n")
}

// The real tail -F: lines appended to a file show up, and stopping it is
// marked, not restarted.
func TestFollowRealTail(t *testing.T) {
	dir := t.TempDir()
	logf := filepath.Join(dir, "app.log")
	os.WriteFile(logf, []byte("first\n"), 0o644)
	f, _ := KnownFollowSpec("tail")
	a := followApp(t, "tail", f)
	a.Target = logf
	scr := a.FollowScreen() // read-only and AutoStart: starts tail
	if scr == nil || a.ActiveJob() == nil || !a.ActiveJob().Running() {
		t.Fatal("tail not started on open")
	}
	defer a.ActiveJob().Stop()
	src := a.newFollowSource()
	pollUntil(t, src, func() bool { return strings.Contains(viewText(src), "first") })

	fh, _ := os.OpenFile(logf, os.O_APPEND|os.O_WRONLY, 0)
	fh.WriteString("ERROR: disk full\nok again\n")
	fh.Close()
	pollUntil(t, src, func() bool { return strings.Contains(viewText(src), "ok again") })
	if src.buf.At(1).Level != follow.Error {
		t.Fatalf("level of %q = %v", src.buf.At(1).Text, src.buf.At(1).Level)
	}
	src.buf.SetMinLevel(follow.Error)
	if src.Len() != 1 {
		t.Fatalf("error-only view has %d lines", src.Len())
	}

	a.ActiveJob().Stop()
	a.ActiveJob().Wait()
	pollUntil(t, src, func() bool { return strings.Contains(viewText(src), "ended: stopped") })
	src.Poll()
	if a.ActiveJob().Running() {
		t.Fatal("restarted after the user stopped it")
	}
}

// AutoStart is ignored for a tool that isn't read-only.
func TestFollowNoAutoStartUnlessReadOnly(t *testing.T) {
	f := FollowSpec{AutoStart: true}
	a := newTestApp(t, ToolSpec{Name: "true", Follow: &f})
	a.FollowScreen()
	if a.ActiveJob() != nil {
		t.Fatal("started a tool that isn't read-only")
	}
}

// Restart on exit, waiting longer each time.
func TestFollowRestart(t *testing.T) {
	bin, _ := fakeTool(t, "flaky", "echo up; exit 3")
	a := followApp(t, bin, FollowSpec{Restart: "on-exit", AutoStart: true})
	clock := time.Now()
	src := a.newFollowSource()
	src.now = func() time.Time { return clock }
	a.FollowScreen()
	first := a.ActiveJob()
	first.Wait()
	src.Poll() // notes the exit
	if !strings.Contains(viewText(src), "ended (exit 3)") {
		t.Fatalf("exit not marked:\n%s", viewText(src))
	}
	src.Poll()
	if a.ActiveJob() != first {
		t.Fatal("restarted before the wait")
	}
	clock = clock.Add(1100 * time.Millisecond)
	src.Poll()
	second := a.ActiveJob()
	if second == first {
		t.Fatal("not restarted after the wait")
	}
	if src.backoff != 2*time.Second {
		t.Fatalf("backoff %v", src.backoff)
	}
	second.Wait()
	src.Poll()
	if !strings.Contains(viewText(src), "started again") {
		t.Fatalf("restart not marked:\n%s", viewText(src))
	}
	_ = runner.Done
}

func TestFollowLineStyling(t *testing.T) {
	f, _ := KnownFollowSpec("journalctl")
	a := followApp(t, "journalctl", f)
	src := a.newFollowSource()
	src.buf.Add(mintLine(`{"__REALTIME_TIMESTAMP":"1761566400000000","PRIORITY":"3","_SYSTEMD_UNIT":"x‮.service","MESSAGE":"boom\u001b[2J"}`))
	got := src.Line(0).Text()
	if strings.ContainsRune(got, 0x1b) || strings.ContainsRune(got, 0x202E) || !strings.Contains(got, "<U+202E>.service: boom") {
		t.Fatalf("line %q", got)
	}
}

func mintLine(s string) mint.LogLine { return mint.LogLine{Text: s} }
