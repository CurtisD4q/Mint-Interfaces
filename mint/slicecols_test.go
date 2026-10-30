package mint

import "testing"

// ViewLog's windowing used to count characters once panned, so wide text
// overflowed the pane. It now measures cells, through util.SliceCols.
func TestViewLogPaneIsWidthCorrect(t *testing.T) {
	for _, s := range []string{
		"日本語のテキストです",
		"mixed 日本 text",
		"e\u0301 combining",
		"👨\u200d👩\u200d👧 emoji",
	} {
		for _, off := range []int{0, 1, 2, 5} {
			for _, w := range []int{2, 4, 10} {
				if got := VLen(sliceCols(s, off, w)); got > w {
					t.Errorf("sliceCols(%q, %d, %d) is %d cells, want at most %d", s, off, w, got, w)
				}
			}
		}
	}
}

// Output from a program keeps its padding as plain spaces: tree(1) indents
// with U+00A0, and Mint measures and splits by cells and spaces, so the two
// must not be different characters downstream.
func TestCleanLineNormalisesNBSP(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"\u2502\u00a0\u00a0 \u251c\u2500\u2500 cyme", "\u2502   \u251c\u2500\u2500 cyme"},
		{"a\u00a0b", "a b"},
		{"\u00a0", " "},
		{"\u00a0\u00a0\u00a0", "   "},
		{"plain", "plain"},
		{"", ""},
	} {
		if got := CleanLine(tc.in); got != tc.want {
			t.Errorf("CleanLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// The width is unchanged: an NBSP was already one cell.
	if got, want := VLen(CleanLine("a\u00a0b")), VLen("a\u00a0b"); got != want {
		t.Errorf("width changed: %d -> %d", want, got)
	}
	// The visible variant normalises too, so a diff of two lines that differ
	// only by NBSP against space shows no false difference.
	if got := CleanLineVisible("a\u00a0b"); got != "a b" {
		t.Errorf("CleanLineVisible(%q) = %q, want %q", "a\u00a0b", got, "a b")
	}
}

// Real tree(1) output, with its NBSP padding, parses as whole lines.
func TestTreeOutputWithNBSP(t *testing.T) {
	out := []string{
		".",
		"\u251c\u2500\u2500 apps",
		"\u2502\u00a0\u00a0 \u251c\u2500\u2500 cyme",
		"\u2502\u00a0\u00a0 \u2502\u00a0\u00a0 \u251c\u2500\u2500 cyme-mint",
		"\u2502\u00a0\u00a0 \u2502\u00a0\u00a0 \u251c\u2500\u2500 cyme-mint.el",
	}
	headers, rows := ParseTable(out)
	if len(headers) != 0 {
		t.Errorf("tree output should have no header row, got %q", headers)
	}
	if len(rows) != len(out) {
		t.Fatalf("got %d rows, want %d: %v", len(rows), len(out), rows)
	}
	for i, r := range rows {
		if len(r) != 1 {
			t.Errorf("row %d has %d cells, want 1: %q", i, len(r), r)
			continue
		}
		if want := CleanLine(out[i]); r[0] != want {
			t.Errorf("row %d = %q, want %q", i, r[0], want)
		}
	}
}
