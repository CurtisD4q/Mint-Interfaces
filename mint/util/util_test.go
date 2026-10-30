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

// SliceCols must never return more than the width asked for, whatever the
// script: ASCII, wide CJK, combining marks, ZWJ emoji, flags, box glyphs.
func TestSliceColsNeverOverflows(t *testing.T) {
	for _, s := range []string{
		"plain ascii line",
		"日本語のテキストです",
		"mixed 日本 text with wide",
		"e\u0301 combining accent",
		"👨\u200d👩\u200d👧 family emoji",
		"🇬🇧 flag",
		"┌──┬──┐ box drawing",
		"a日b語c",
	} {
		for offset := 0; offset <= 20; offset++ {
			for width := 1; width <= 16; width++ {
				out := SliceCols(s, offset, width)
				if got := VLen(out); got > width {
					t.Fatalf("SliceCols(%q, %d, %d) = %q is %d cells, want at most %d",
						s, offset, width, out, got, width)
				}
			}
		}
	}
}

// The markers appear exactly when there is content hidden on that side.
func TestSliceColsMarkers(t *testing.T) {
	s := "abcdefghij"
	if got := SliceCols(s, 0, 4); got != "abc…" {
		t.Errorf("offset 0: %q, want abc…", got)
	}
	if got := SliceCols(s, 3, 4); got != "‹de…" {
		t.Errorf("offset 3: %q, want ‹de…", got)
	}
	// Far enough along that nothing is hidden to the right.
	if got := SliceCols(s, 7, 4); got != "‹hij" {
		t.Errorf("offset 7: %q, want ‹hij", got)
	}
	// A window wider than the text shows it whole, with no marker.
	if got := SliceCols(s, 0, 20); got != s {
		t.Errorf("wide window: %q, want %q", got, s)
	}
}

// A wide rune is never split, and the window still fills as many cells as it
// can without overflowing.
func TestSliceColsWideRunes(t *testing.T) {
	s := "日本語のテキストです" // every rune two cells
	got := SliceCols(s, 0, 10)
	if VLen(got) > 10 {
		t.Errorf("%q is %d cells, want at most 10", got, VLen(got))
	}
	if VLen(got) < 8 {
		t.Errorf("%q is only %d cells; the window should be filled", got, VLen(got))
	}
	// Each cluster in the output is a whole rune: no replacement characters.
	for _, c := range Chars(got) {
		if c == "\uFFFD" {
			t.Errorf("split a rune: %q", got)
		}
	}
}

// The bug this replaces: offset > 0 used to count characters, so wide text
// overflowed the window.
func TestSliceColsOffsetDoesNotOverflow(t *testing.T) {
	for _, offset := range []int{1, 2, 3, 5, 8} {
		got := SliceCols("日本語のテキストです", offset, 10)
		if VLen(got) > 10 {
			t.Errorf("offset %d: %q is %d cells, want at most 10", offset, got, VLen(got))
		}
	}
}
