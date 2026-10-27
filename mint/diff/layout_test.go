package diff

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

func texts(lines []mint.StyledLine) []string {
	var out []string
	for _, l := range lines {
		out = append(out, l.Text())
	}
	return out
}

var sample = ParseUnified([]string{
	"--- old.txt", "+++ new.txt",
	"@@ -1,4 +1,4 @@ func main",
	" keep", "-colour", "+color", "-gone", " end",
})

func TestLayoutSideBySide(t *testing.T) {
	lines := Layout(sample, 120, Options{})
	got := texts(lines)
	if !strings.Contains(got[0], "old.txt  ↔  new.txt") || !strings.Contains(got[0], "+1 −2") {
		t.Fatalf("file header %q", got[0])
	}
	if !lines[0].Anchor || !lines[1].Anchor || !strings.HasPrefix(got[1], "@@ lines 1–4 → 1–4") || !strings.Contains(got[1], "func main") {
		t.Fatalf("hunk header %q anchor %v", got[1], lines[1].Anchor)
	}
	// Every content row is exactly the view width: columns line up.
	for _, l := range got[2:] {
		if util.VLen(l) != 120 {
			t.Errorf("row %d cells wide, want 120: %q", util.VLen(l), l)
		}
	}
	change := got[3]
	if !strings.Contains(change, "2 - colour") || !strings.Contains(change, "│ 2 + color") {
		t.Fatalf("change row %q", change)
	}
	del := got[4]
	if !strings.Contains(del, "3 - gone") || strings.Contains(del, "+") {
		t.Fatalf("delete row %q", del)
	}
	// The changed character is marked on the left; the rest is not.
	var marked []string
	for _, s := range lines[3].Segs {
		if s.Role == mint.RoleMark {
			marked = append(marked, s.Text)
		}
	}
	if len(marked) != 1 || marked[0] != "u" {
		t.Fatalf("marked runs %q", marked)
	}
}

func TestLayoutAutoNarrowIsUnified(t *testing.T) {
	got := texts(Layout(sample, 80, Options{}))
	want := []string{"1 1   keep", "2   - colour", "  2 + color", "3   - gone", "4 3   end"}
	for i, w := range want {
		if !strings.HasPrefix(got[2+i], w) {
			t.Errorf("line %d = %q, want prefix %q", i, got[2+i], w)
		}
	}
	if s := texts(Layout(sample, 160, Options{Mode: Unified})); strings.Contains(s[3], "│") {
		t.Errorf("forced unified shows columns: %q", s[3])
	}
}

func TestLayoutLongLinesAreCut(t *testing.T) {
	long := strings.Repeat("x", 300)
	files := ParseUnified([]string{"--- a", "+++ b", "@@ -1 +1 @@", "-" + long, "+" + long + "y"})
	for _, w := range []int{100, 121, 140, 201} {
		for _, l := range texts(Layout(files, w, Options{}))[2:] {
			if util.VLen(l) != w {
				t.Errorf("width %d: row is %d cells", w, util.VLen(l))
			}
			if !strings.Contains(l, "…") {
				t.Errorf("width %d: cut row has no …: %q", w, l)
			}
		}
	}
}

// Text from the compared files can't reach the terminal as control codes,
// and bidi tricks are shown, not obeyed.
func TestLayoutCleansHostileText(t *testing.T) {
	files := ParseUnified([]string{"--- a", "+++ b", "@@ -1 +1 @@",
		"-ok", "+ok\x1b]0;pwned\x07 ‮evil\u009b2J"})
	for _, mode := range []Mode{SideBySide, Unified} {
		for _, l := range texts(Layout(files, 140, Options{Mode: mode})) {
			for _, r := range l {
				if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x202E {
					t.Fatalf("mode %d: control %U in %q", mode, r, l)
				}
			}
			if strings.Contains(l, "ok") && strings.Contains(l, "evil") && !strings.Contains(l, "<U+202E>") {
				t.Fatalf("bidi override not shown: %q", l)
			}
		}
	}
}

func TestLayoutHiddenChange(t *testing.T) {
	// A CRLF-only change: cleaned, both sides read "c".
	files := ParseUnified([]string{"--- a", "+++ b", "@@ -1 +1 @@", "-c", "+c"})
	got := texts(Layout(files, 120, Options{}))
	if len(got) != 4 || !strings.Contains(got[3], "differ only in characters not shown") {
		t.Fatalf("hidden change not explained: %q", got)
	}
}

func TestLayoutShowSpace(t *testing.T) {
	files := ParseUnified([]string{"--- a", "+++ b", "@@ -1,2 +1,2 @@", " a b", "-x y", "+x  y"})
	got := texts(Layout(files, 120, Options{ShowSpace: true}))
	if !strings.Contains(got[2], "a b") {
		t.Errorf("unchanged line shows spaces: %q", got[2])
	}
	if !strings.Contains(got[3], "x·y") || !strings.Contains(got[3], "x··y") {
		t.Errorf("changed line hides spaces: %q", got[3])
	}
}

func TestLayoutOtherStatuses(t *testing.T) {
	files := ParseUnified([]string{"Only in A: extra", "Binary files a.bin and b.bin differ", "Files x and y differ"})
	got := texts(Layout(files, 120, Options{}))
	joined := strings.Join(got, "\n")
	for _, want := range []string{"Only in A: extra", "a.bin  ↔  b.bin", "Binary files", "Run without -q"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if got := texts(Layout(nil, 120, Options{})); len(got) != 1 || got[0] != "No differences." {
		t.Errorf("empty = %q", got)
	}
}

func TestLayoutLabels(t *testing.T) {
	got := texts(Layout(sample, 120, Options{LeftLabel: "before", RightLabel: "after"}))
	if !strings.HasPrefix(got[0], "before  ↔  after") {
		t.Fatalf("labels %q", got[0])
	}
}

func TestLayoutCmp(t *testing.T) {
	r := ParseCmp([]string{"cmp: EOF on l after byte 10", " 3 142 102", " 8  12  15"})
	got := texts(LayoutCmp(r, "l", "r"))
	joined := strings.Join(got, "\n")
	for _, want := range []string{"2 bytes differ", "62 'b'", "42 'B'", "0a '.'", "0d '.'", "l ends after byte 10"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if got := texts(LayoutCmp(CmpResult{}, "", "")); got[0] != "No differences." {
		t.Errorf("empty cmp = %q", got)
	}
}
