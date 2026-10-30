package util

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunTimeoutFinishes(t *testing.T) {
	out, err := RunTimeout(5*time.Second, "sh", "-c", "echo hi; exit 3")
	if strings.TrimSpace(out) != "hi" || err == nil || err.Error() != "exit 3" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestRunTimeoutKillsTheWholeGroup(t *testing.T) {
	start := time.Now()
	out, err := RunTimeout(300*time.Millisecond, "sh", "-c", "sleep 30 & echo $!; tail -f /dev/null")
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 3*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(out))
	time.Sleep(100 * time.Millisecond)
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		if b, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); !strings.Contains(string(b), ") Z ") {
			t.Fatalf("background child %d survived the timeout", pid)
		}
	}
}

func TestRunTimeoutNeverReadsTheKeyboard(t *testing.T) {
	out, err := RunTimeout(2*time.Second, "sh", "-c", "read x; echo got:$x")
	if err != nil || strings.TrimSpace(out) != "got:" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestStopAllEndsRunningCommands(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		_, err := RunTimeout(time.Minute, "sleep", "30")
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	StopAll()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("StopAll did not end the command")
	}
}

// Ambiguous runes are one column in a Western locale and two in a CJK one.
func TestWidthAmbiguous(t *testing.T) {
	old := activeWidth()
	t.Cleanup(func() { SetWidth(old) })

	for _, g := range []string{"╭", "─", "│", "●", "…", "↑", "█"} {
		SetWidth(Width{})
		if got := VLen(g); got != 1 {
			t.Errorf("VLen(%q) = %d, want 1 with AmbiguousWide off", g, got)
		}
		SetWidth(Width{AmbiguousWide: true})
		if got := VLen(g); got != 2 {
			t.Errorf("VLen(%q) = %d, want 2 with AmbiguousWide on", g, got)
		}
	}
	// A narrow rune is unaffected either way.
	for _, wide := range []bool{false, true} {
		SetWidth(Width{AmbiguousWide: wide})
		if got := VLen("ab"); got != 2 {
			t.Errorf("VLen(\"ab\") = %d, want 2 (AmbiguousWide %v)", got, wide)
		}
		if got := VLen("日"); got != 2 {
			t.Errorf("VLen(日) = %d, want 2 (AmbiguousWide %v)", got, wide)
		}
	}
}

// The frame arithmetic has to agree with what the terminal draws: a
// ten-cell top border is ten cells when ambiguous runes are narrow, and
// twenty when they are wide.
func TestWidthFrameArithmetic(t *testing.T) {
	old := activeWidth()
	t.Cleanup(func() { SetWidth(old) })

	SetWidth(Width{})
	if got := VLen("╭────────╮"); got != 10 {
		t.Errorf("frame VLen = %d, want 10", got)
	}
	SetWidth(Width{AmbiguousWide: true})
	if got := VLen("╭────────╮"); got != 20 {
		t.Errorf("frame VLen = %d, want 20", got)
	}
	// Truncating measures the same way, and must not split a wide rune.
	if got := VLen(Truncate("╭────────╮", 10)); got > 10 {
		t.Errorf("Truncate to 10 gave %d cells", got)
	}
}

// SetWidth must be safe to read while it is written: it runs once at
// startup, but VLen is read from every frame.
func TestWidthIsRaceFree(t *testing.T) {
	old := activeWidth()
	t.Cleanup(func() { SetWidth(old) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			SetWidth(Width{AmbiguousWide: i%2 == 0})
		}
	}()
	for i := 0; i < 200; i++ {
		_ = VLen("╭─╮")
		_ = Truncate("╭─╮", 2)
	}
	<-done
}
