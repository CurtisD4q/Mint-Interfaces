package mint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanLineBackspaceOverstrike(t *testing.T) {
	// Man-page bold: N\bNA\bAM\bME\bE -> NAME
	got := CleanLine("N\bNA\bAM\bME\bE")
	if got != "NAME" {
		t.Fatalf("CleanLine bold overstrike = %q, want %q", got, "NAME")
	}
	// Man-page underline: _\bf\bfo\boo\bo -> foo
	got2 := CleanLine("_\bf_\bo_\bo")
	if got2 != "foo" {
		t.Fatalf("CleanLine underline overstrike = %q, want %q", got2, "foo")
	}
	// Leading backspace at col 0 is harmlessly ignored.
	got3 := CleanLine("\b\bhello\tworld")
	if got3 != "hello   world" {
		t.Fatalf("CleanLine tabs + leading backspace = %q, want %q", got3, "hello   world")
	}
}

func TestCleanLinePreservesBoxDrawingAndBraille(t *testing.T) {
	box := "┌──┬──┐ █▓▒░ ⠁⠂⠃⠄"
	if got := CleanLine(box); got != box {
		t.Fatalf("CleanLine altered Unicode box/braille: got %q, want %q", got, box)
	}
}

func TestSliceColsHorizontalScroll(t *testing.T) {
	line := "0123456789abcdefghij"
	// At offset 0, width 10 -> 9 chars + ellipsis
	s0 := sliceCols(line, 0, 10)
	if VLen(s0) != 10 || !strings.HasSuffix(s0, "…") || strings.HasPrefix(s0, "‹") {
		t.Fatalf("sliceCols(offset=0,width=10) = %q (vlen=%d)", s0, VLen(s0))
	}
	// At offset 8, width 10 -> leading ‹ + middle chars + trailing …
	s8 := sliceCols(line, 8, 10)
	if VLen(s8) != 10 || !strings.HasPrefix(s8, "‹") || !strings.HasSuffix(s8, "…") {
		t.Fatalf("sliceCols(offset=8,width=10) = %q (vlen=%d)", s8, VLen(s8))
	}
	// Past end of line -> just leading ‹
	sEnd := sliceCols("short", 16, 10)
	if sEnd != "‹" {
		t.Fatalf("sliceCols past end = %q, want %q", sEnd, "‹")
	}
}

func TestSaveLogLines(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "sub", "output.txt")
	lines := []LogLine{
		{Text: "first line"},
		{Text: "second line", Err: true},
	}
	if err := SaveLogLines(outPath, lines); err != nil {
		t.Fatalf("SaveLogLines error: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "first line\nsecond line\n" {
		t.Fatalf("file content = %q", string(data))
	}
}
