package mint

import (
	"strings"
	"testing"
)

func TestCleanLineColours(t *testing.T) {
	text, runs := CleanLineColours("\x1b[1m\x1b[31mOS\x1b[m: Ubuntu")
	if text != "OS: Ubuntu" {
		t.Fatalf("text %q", text)
	}
	if len(runs) != 1 || runs[0] != (ColourRun{Start: 0, End: 2, FG: 2, Bold: true}) {
		t.Errorf("runs %+v", runs)
	}
	// The text is always what CleanLine gives.
	for _, raw := range []string{"a\tb\x1b[32mc\td\x1b[0m", "\x1b[2Jx\x1b[31my\x1b]0;title\x07z", "plain"} {
		got, _ := CleanLineColours(raw)
		if want := CleanLine(raw); got != want {
			t.Errorf("%q: text %q, CleanLine %q", raw, got, want)
		}
	}
	if _, runs := CleanLineColours("no colour \x1b[2J here"); runs != nil {
		t.Errorf("only colour codes make runs: %+v", runs)
	}
}

func TestColourMapping(t *testing.T) {
	cases := []struct {
		sgr    string
		fg, bg uint8
	}{
		{"91", 10, 0},          // bright red
		{"44", 0, 5},           // blue background
		{"38;5;196", 10, 0},    // 256-colour red -> bright red
		{"38;2;0;205;0", 3, 0}, // 24-bit green -> green
		{"48;5;9", 0, 10},
	}
	for _, c := range cases {
		_, runs := CleanLineColours("\x1b[" + c.sgr + "mx")
		if len(runs) != 1 || runs[0].FG != c.fg || runs[0].BG != c.bg {
			t.Errorf("%s: %+v, want fg %d bg %d", c.sgr, runs, c.fg, c.bg)
		}
	}
}

// Text can never be made invisible: a background the same as the
// foreground is dropped.
func TestColoursNeverHideText(t *testing.T) {
	_, runs := CleanLineColours("\x1b[31;41mhidden")
	if len(runs) != 1 || runs[0].BG != 0 {
		t.Errorf("same foreground and background: %+v", runs)
	}
}

// What reaches the terminal is Mint's own codes: no byte of a program's
// escape sequences survives, whatever it sent.
func TestRenderColouredWritesOnlyMintCodes(t *testing.T) {
	raw := "\x1b[31mred\x1b]52;c;ZXZpbA==\x07\x1b[?1049h\x1b[38;5;99mviolet\x1b[0m"
	text, runs := CleanLineColours(raw)
	out := renderColoured(text, runs, 40, "")
	for _, bad := range []string{"]52", "?1049", "38;5"} {
		if strings.Contains(out, bad) {
			t.Errorf("program code %q reached the output: %q", bad, out)
		}
	}
	allowed := map[string]bool{}
	for _, r := range runs {
		allowed[sgrFor(r)] = true
	}
	for _, m := range sgrEsc.FindAllString(out, -1) {
		if !allowed[m] && m != Reset {
			t.Errorf("unexpected code %q in %q", m, out)
		}
	}
	if VLen(stripANSI(out)) != VLen(text) {
		t.Errorf("drawn text changed: %q", stripANSI(out))
	}
}

func stripANSI(s string) string { return ansiEsc.ReplaceAllString(s, "") }

func TestCleanedLogLineDropsStaleColours(t *testing.T) {
	l := cleanedLogLine(LogLine{Text: "a\tb", Colours: []ColourRun{{Start: 0, End: 1, FG: 2}}})
	if l.Colours != nil {
		t.Error("colours kept after cleaning changed the text")
	}
	l = cleanedLogLine(LogLine{Text: "ab", Colours: []ColourRun{{Start: 0, End: 1, FG: 2}}})
	if len(l.Colours) != 1 {
		t.Error("colours dropped from clean text")
	}
}

func TestSliceColouredMovesRunsWithTheWindow(t *testing.T) {
	text, runs := CleanLineColours("plain \x1b[31mRED\x1b[0m tail")
	// Whole line fits: runs unchanged.
	out, got := sliceColoured(text, runs, 0, 40)
	if out != text || len(got) != 1 || got[0] != runs[0] {
		t.Fatalf("no slice: %q %+v", out, got)
	}
	// Panned 4 cells: "‹" then "in RED tail"; RED starts after the marker.
	out, got = sliceColoured(text, runs, 4, 40)
	if out != "‹n RED tail" {
		t.Fatalf("out = %q", out)
	}
	if len(got) != 1 || got[0].Start != 3 || got[0].End != 6 || string([]rune(out)[3:6]) != "RED" {
		t.Fatalf("runs = %+v", got)
	}
	// Panned past the colour: no runs left.
	if _, got = sliceColoured(text, runs, 10, 40); len(got) != 0 {
		t.Fatalf("runs past the window = %+v", got)
	}
	// Cut on the right through the colour: run clipped, "…" plain.
	out, got = sliceColoured(text, runs, 0, 9)
	if out != "plain RE…" || len(got) != 1 || got[0].Start != 6 || got[0].End != 8 {
		t.Fatalf("clipped: %q %+v", out, got)
	}
	drawn := renderColoured(out, got, 9, "")
	if strings.Count(drawn, "\x1b[31m") != 1 || !strings.HasSuffix(drawn, "…"+Reset) {
		t.Fatalf("drawn = %q", drawn)
	}
}
