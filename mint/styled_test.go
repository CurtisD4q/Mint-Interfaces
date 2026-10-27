package mint

import (
	"strings"
	"testing"
)

func TestRenderStyledCleansAndCuts(t *testing.T) {
	l := StyledLine{Segs: []Seg{
		{Text: "abc", Role: RoleAdded},
		{Text: "\x1b[2J\u009b31m‮x", Role: RoleRemoved},
		{Text: "tail that is long", Role: RoleText},
	}}
	out := renderStyled(l, 20)
	plain := ansiEsc.ReplaceAllString(out, "")
	if VLen(plain) > 20 {
		t.Fatalf("rendered %d cells, want at most 20: %q", VLen(plain), plain)
	}
	if strings.Contains(out, "\x1b[2J") || strings.ContainsRune(out, 0x9b) || strings.ContainsRune(out, 0x202E) {
		t.Fatalf("control text reached the terminal: %q", out)
	}
	// The C1 introducer is dropped; what followed it is plain text.
	if !strings.HasPrefix(plain, "abc31m<U+202E>x") || !strings.HasSuffix(plain, "…") {
		t.Fatalf("plain = %q", plain)
	}
	// The only escape sequences are the palette's, one style and one
	// reset per segment drawn.
	if n := strings.Count(out, Reset); n != 3 {
		t.Fatalf("%d resets in %q", n, out)
	}
}

func TestStyledLineText(t *testing.T) {
	l := StyledLine{Segs: []Seg{{Text: "a​b"}, {Text: "\tc"}}}
	if got := l.Text(); got != "a<U+200B>b        c" {
		t.Fatalf("Text = %q", got)
	}
}

func TestAnchors(t *testing.T) {
	lines := []StyledLine{{Anchor: true}, {}, {}, {Anchor: true}, {}, {Anchor: true}}
	a := anchorsOf(lines)
	if len(a) != 3 || a[0] != 0 || a[1] != 3 || a[2] != 5 {
		t.Fatalf("anchors %v", a)
	}
	for line, want := range map[int]int{0: 0, 2: 0, 3: 1, 4: 1, 9: 2} {
		if got := anchorAt(a, line); got != want {
			t.Errorf("anchorAt(%d) = %d, want %d", line, got, want)
		}
	}
	if anchorAt(nil, 3) != -1 || anchorAt([]int{5}, 2) != -1 {
		t.Error("no anchor above should be -1")
	}
}

func TestEveryRoleHasAStyle(t *testing.T) {
	for r := RoleText; r <= RoleMark; r++ {
		if roleStyle(r) == "" {
			t.Errorf("role %d has no style", r)
		}
	}
}
