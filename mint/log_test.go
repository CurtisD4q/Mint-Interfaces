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

func TestCleanLineDropsC1Controls(t *testing.T) {
	for r := rune(0x80); r <= 0x9f; r++ {
		in := "a" + string(r) + "31mb"
		if got := CleanLine(in); got != "a31mb" {
			t.Errorf("CleanLine(%q) = %q, want C1 control dropped", in, got)
		}
	}
	if got := CleanLine("x\u2028y\u2029z"); got != "xyz" {
		t.Errorf("line/paragraph separators kept: %q", got)
	}
}

func TestCleanLineMarksBidiControls(t *testing.T) {
	// Trojan Source: an override that makes code display in a different
	// order from the bytes the compiler reads.
	in := "if access_level != \"user\u202E \u2066// Check if admin\u2069 \u2066\" {"
	got := CleanLine(in)
	for _, r := range got {
		if isBidiControl(r) {
			t.Fatalf("bidi control %U survived: %q", r, got)
		}
	}
	for _, want := range []string{"<U+202E>", "<U+2066>", "<U+2069>"} {
		if !strings.Contains(got, want) {
			t.Errorf("CleanLine(%q) = %q, missing marker %s", in, got, want)
		}
	}
	for r := rune(0x202A); r <= 0x202E; r++ {
		if got := CleanLine(string(r)); got != RuneMarker(r) {
			t.Errorf("CleanLine(%U) = %q", r, got)
		}
	}
	for r := rune(0x2066); r <= 0x2069; r++ {
		if got := CleanLine(string(r)); got != RuneMarker(r) {
			t.Errorf("CleanLine(%U) = %q", r, got)
		}
	}
}

func TestCleanLineKeepsLegitimateFormatCharacters(t *testing.T) {
	keep := []string{
		"שלום\u200E world",         // LRM
		"مرحبا\u200F",              // RLM
		"\u061C123",                // ALM
		"می\u200Cخواهم",            // ZWNJ in Persian
		"👩\u200D💻",                 // ZWJ emoji sequence
		"zero\u200Bwidth\uFEFFbom", // invisible, but CleanLine leaves them
	}
	for _, s := range keep {
		if got := CleanLine(s); got != s {
			t.Errorf("CleanLine(%q) = %q, want unchanged", s, got)
		}
	}
}

func TestCleanLineVisibleMarksInvisibles(t *testing.T) {
	got := CleanLineVisible("abc\u200Bdef\uFEFF\u2060\u00AD")
	want := "abc<U+200B>def<U+FEFF><U+2060><U+00AD>"
	if got != want {
		t.Fatalf("CleanLineVisible = %q, want %q", got, want)
	}
	// The joiners still pass, even in the strict view.
	if s := "👩\u200D💻 می\u200Cخواهم"; CleanLineVisible(s) != s {
		t.Fatalf("CleanLineVisible changed joiners: %q", CleanLineVisible(s))
	}
	// Tab stops count the marker's width.
	if got := CleanLineVisible("\u200B\tx"); got != "<U+200B>        x" {
		t.Fatalf("tab after marker = %q", got)
	}
}

// No input, however hostile, leaves a control character in the output.
func TestCleanLineNeverEmitsControls(t *testing.T) {
	var all []rune
	for r := rune(0); r < 0x3000; r++ {
		all = append(all, r)
	}
	for _, f := range []func(string) string{CleanLine, CleanLineVisible} {
		for _, r := range f(string(all)) {
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) || isBidiControl(r) || r == 0x2028 || r == 0x2029 {
				t.Fatalf("control %U in output", r)
			}
		}
	}
}
