package mint

import (
	"strings"
	"testing"
)

// besideScreen is a top pane and n-1 panes laid out side by side.
func besideScreen(n int) *Screen {
	s := paneScreen(n)
	s.TopTitle = "Left"
	for _, p := range s.Panes {
		p.Beside = true
	}
	return s
}

func gridOf(s *Screen, h, w int) []string {
	return frameGrid(s.renderFrame(h, w), h, w)
}

func rowWith(grid []string, text string) int {
	for i, line := range grid {
		if strings.Contains(line, text) {
			return i + 1
		}
	}
	return 0
}

func colOf(grid []string, row int, text string) int {
	line := []rune(grid[row-1])
	for c := 0; c+len([]rune(text)) <= len(line); c++ {
		if string(line[c:c+len([]rune(text))]) == text {
			return c + 1
		}
	}
	return 0
}

func TestBesideSpansShareTheWidth(t *testing.T) {
	s := besideScreen(3)
	s.viewWidth = 100
	gs := s.groups()
	if len(gs) != 1 || len(gs[0]) != 3 {
		t.Fatalf("groups %+v", gs)
	}
	sum, x := 0, 1
	for _, c := range gs[0] {
		if c.x0 != x {
			t.Errorf("pane %d starts at %d, want %d", c.index, c.x0, x)
		}
		if c.w < splitMinPaneWidth {
			t.Errorf("pane %d is %d wide", c.index, c.w)
		}
		x += c.w
		sum += c.w
	}
	if sum != 100 {
		t.Errorf("widths sum to %d, want 100", sum)
	}
	// Width shares.
	s = besideScreen(2)
	s.viewWidth = 100
	s.TopWidth, s.Panes[0].Width = 1, 2
	gs = s.groups()
	if len(gs) != 1 || len(gs[0]) != 2 || gs[0][0].w >= gs[0][1].w || gs[0][0].w+gs[0][1].w != 100 {
		t.Errorf("shared 1:2 gave %+v", gs)
	}
	// A share too small for the minimum is raised to it.
	s.TopWidth, s.Panes[0].Width = 1, 20
	gs = s.groups()
	if gs[0][0].w < splitMinPaneWidth || gs[0][0].w+gs[0][1].w != 100 {
		t.Errorf("tiny share gave %+v", gs)
	}
}

func TestBesideStacksWhenNarrow(t *testing.T) {
	s := besideScreen(2)
	s.viewWidth = 2*splitMinPaneWidth - 1
	gs := s.groups()
	if len(gs) != 2 || len(gs[0]) != 1 || len(gs[1]) != 1 || gs[1][0].w != s.viewWidth {
		t.Fatalf("narrow groups %+v", gs)
	}
	s.viewWidth = 2 * splitMinPaneWidth
	if gs := s.groups(); len(gs) != 1 || len(gs[0]) != 2 {
		t.Fatalf("exactly enough width: %+v", gs)
	}
	// Before the screen is measured nothing is split.
	s.viewWidth = 0
	if gs := s.groups(); len(gs) != 2 {
		t.Fatalf("unmeasured groups %+v", gs)
	}
}

func TestBesideNeverStartsAGroup(t *testing.T) {
	s := paneScreen(3)
	s.Panes[1].Beside = true // Beta beside Alpha; Alpha itself is stacked
	s.viewWidth = 100
	gs := s.groups()
	if len(gs) != 2 || len(gs[0]) != 1 || len(gs[1]) != 2 || gs[1][0].index != 1 || gs[1][1].index != 2 {
		t.Fatalf("groups %+v", gs)
	}
	// The first pane has nothing before it to sit beside.
	s2 := paneScreen(2)
	s2.TopTitle = "T"
	s2.Panes[0].Beside = false
	s2.viewWidth = 100
	if len(s2.groups()) != 2 {
		t.Fatal("stacked panes were grouped")
	}
}

func TestBesideDrawsOnTheSameRows(t *testing.T) {
	s := besideScreen(2)
	grid := gridOf(s, 30, 100)
	left, right := rowWith(grid, "Left"), rowWith(grid, "Alpha")
	if left == 0 || right == 0 {
		t.Fatalf("titles missing: left row %d, right row %d\n%s", left, right, strings.Join(grid, "\n"))
	}
	if right != left {
		t.Fatalf("Alpha on row %d, Left on row %d\n%s", right, left, strings.Join(grid, "\n"))
	}
	row := rowWith(grid, "top-row")
	if r := rowWith(grid, "alpha-row"); r != row {
		t.Fatalf("alpha-row on row %d, top-row on row %d", r, row)
	}
	x0, _ := s.paneSpan(1)
	if c := colOf(grid, row, "alpha-row"); c <= x0 {
		t.Fatalf("alpha-row at column %d, its pane starts at %d", c, x0)
	}
	if c := colOf(grid, row, "top-row"); c >= x0 {
		t.Fatalf("top-row at column %d, beyond the left pane (%d)", c, x0)
	}
	// One band, shared rows.
	if len(s.bands) != 1 || len(s.bands[0].cols) != 2 {
		t.Fatalf("bands %+v", s.bands)
	}
	// The rule between the panes runs down the band.
	b := s.bands[0]
	for r := b.top; r < b.top+b.rows; r++ {
		if !strings.ContainsRune(grid[r-1], '│') || colOf(grid, r, "│") == 0 {
			t.Fatalf("no rule between the panes on row %d: %q", r, grid[r-1])
		}
	}
}

func TestBesideFallsBackOnANarrowTerminal(t *testing.T) {
	s := besideScreen(2)
	grid := gridOf(s, 30, 56)
	left, right := rowWith(grid, "Left"), rowWith(grid, "Alpha")
	if left == 0 || right == 0 || left == right {
		t.Fatalf("expected stacked titles, got rows %d and %d\n%s", left, right, strings.Join(grid, "\n"))
	}
	if len(s.bands) != 2 {
		t.Fatalf("%d bands, want 2 stacked", len(s.bands))
	}
}

func TestBesideKeepsEachPaneInItsWindow(t *testing.T) {
	long := strings.Repeat("Ω", 200)
	s := besideScreen(2)
	s.Sections = []Section{{Name: "Left", Rows: []Row{Note(long), Action(ActionOpts{Label: long})}}}
	s.Panes[0].Sections = []Section{{Name: "Alpha", Rows: []Row{Note(strings.Repeat("Ψ", 200)), Action(ActionOpts{Label: strings.Repeat("Ψ", 200)})}}}
	grid := gridOf(s, 30, 100)
	x0, w := s.paneSpan(1)
	for r, line := range grid {
		cells := []rune(line)
		for c, ch := range cells {
			col := c + 1
			if ch == 'Ω' && col >= x0 {
				t.Fatalf("left pane text spilled to column %d of row %d: %q", col, r+1, line)
			}
			if ch == 'Ψ' && (col < x0 || col >= x0+w) {
				t.Fatalf("right pane text at column %d of row %d is outside its window [%d,%d)", col, r+1, x0, x0+w)
			}
		}
	}
}

func TestBesideMouseFocusesAndUsesLocalColumns(t *testing.T) {
	on := false
	s := besideScreen(2)
	s.Sections = []Section{{Name: "Left", Rows: []Row{Action(ActionOpts{Label: "top-row"})}}}
	s.Panes[0].Sections = []Section{{Name: "Alpha", Rows: []Row{
		Checkbox(CheckboxOpts{ID: "c", Label: "tick", Get: func() bool { return on }, Set: func(v bool) { on = v }}),
	}}}
	s.renderFrame(30, 100)
	x0, _ := s.paneSpan(1)
	grid := gridOf(s, 30, 100)
	row := rowWith(grid, "tick")
	// A click in the left half focuses the left pane.
	s.pressKey(&Key{Mouse: &Mouse{Row: row, Col: 6, Click: true}}, 30, 100)
	if s.focus != 0 {
		t.Fatalf("left click focused %d", s.focus)
	}
	// A click on the mark column of the right pane's checkbox: columns 5
	// and 6 of the pane itself, so x0-1+5 on the screen.
	s.pressKey(&Key{Mouse: &Mouse{Row: row, Col: x0 - 1 + 5, Click: true}}, 30, 100)
	if s.focus != 1 {
		t.Fatalf("right click focused %d", s.focus)
	}
	if !on {
		t.Fatal("a click on the checkbox mark in the right pane did not tick it")
	}
}

func TestBesideTabCyclesEveryPane(t *testing.T) {
	s := besideScreen(3)
	s.renderFrame(30, 100)
	seen := []int{s.focus}
	for i := 0; i < 3; i++ {
		s.pressKey(&Key{Name: "\t"}, 30, 100)
		seen = append(seen, s.focus)
	}
	if got := seen; got[0] != 0 || got[1] != 1 || got[2] != 2 || got[3] != 0 {
		t.Fatalf("tab order %v", got)
	}
}

func TestBesideShortTerminalKeepsTheFocusedGroupOpen(t *testing.T) {
	s := besideScreen(2)
	s.Panes = append(s.Panes, &Pane{Title: "Below", Sections: []Section{{Name: "Below", Rows: manyNotes("below ", 40)}}})
	s.renderFrame(14, 100)
	// Focus on the second pane of the split band keeps that band open.
	s.focus = 1
	s.renderFrame(14, 100)
	for _, b := range s.bands {
		if b.index == 0 && (b.rows == 0 || b.collapsed) {
			t.Fatalf("the focused split band collapsed: %+v", b)
		}
	}
}

func TestBesideCannotBeHidden(t *testing.T) {
	s := besideScreen(2)
	s.renderFrame(30, 100)
	s.focus = 1
	s.toggleFocused()
	if s.paneHidden(1) {
		t.Fatal("a pane beside another was hidden")
	}
}

func TestBesideResizeDoesNotBreak(t *testing.T) {
	s := besideScreen(2)
	s.Panes = append(s.Panes, &Pane{Title: "Below", Sections: []Section{{Name: "Below", Rows: manyNotes("below ", 40)}}})
	s.renderFrame(40, 100)
	before := s.bands[0].rows
	s.focus = 0
	s.nudgeFocus(1)
	s.renderFrame(40, 100)
	if s.bands[0].rows < before {
		t.Fatalf("growing the split band shrank it: %d -> %d", before, s.bands[0].rows)
	}
}

func TestHoverStaysInsideThePaneWindow(t *testing.T) {
	forgetPointer()
	movePointer(10, 80, false)
	t.Setenv("MINT_HOVER", "")
	rt.profile.SGRMouse = true
	defer forgetPointer()
	paneWin.x0, paneWin.w = 1, 50
	if hoverAt(10) {
		t.Error("the pane to the left lit up for a pointer in the right pane")
	}
	paneWin.x0, paneWin.w = 51, 50
	if !hoverAt(10) {
		t.Error("the pane under the pointer did not light up")
	}
	paneWin.x0, paneWin.w = 0, 0
	if !hoverAt(10) {
		t.Error("a single-column screen lost its hover")
	}
}

func TestClipCells(t *testing.T) {
	if got := clipCells("hello", 10); got != "hello" {
		t.Errorf("short text changed: %q", got)
	}
	got := clipCells("\x1b[31mhello world\x1b[0m", 5)
	if stripFrame(got) != "hello" || !strings.HasPrefix(got, "\x1b[31m") || !strings.HasSuffix(got, Reset) {
		t.Errorf("clipped %q", got)
	}
	if got := clipCells("日本語", 4); stripFrame(got) != "日本" {
		t.Errorf("wide characters: %q", stripFrame(got))
	}
	if got := clipCells("abc", 0); got != "" {
		t.Errorf("zero width: %q", got)
	}
	link := "\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\"
	if got := clipCells(link+"abcdef", 6); !strings.Contains(got, "http://x") || strings.Count(got, "\x1b]8;;") < 2 {
		t.Errorf("link broken by the clip: %q", got)
	}
}

func TestCanvasViewShiftsAndClips(t *testing.T) {
	root := begin(10, 40)
	v := root.view(21, 10)
	v.put(3, 2, "abcdefghijklmnop")
	v.put(3, 11, "x") // past the view
	out := stripFrame(root.b.String())
	if !strings.Contains(out, "abcdefghi") || strings.Contains(out, "abcdefghij") || strings.Contains(out, "x") {
		t.Errorf("view output %q", out)
	}
	if !strings.Contains(root.b.String(), "\x1b[3;22H") {
		t.Errorf("view not shifted to column 22: %q", root.b.String())
	}
}

// Row 1 of one pane and row 1 of the next are different rows: a click on
// each, one after the other, is not a double click.
func TestClicksInTwoPanesAreNotADoubleClick(t *testing.T) {
	on := false
	s := besideScreen(2)
	s.Sections = []Section{{Name: "Left", Rows: []Row{Action(ActionOpts{Label: "top-row"})}}}
	s.Panes[0].Sections = []Section{{Name: "Alpha", Rows: []Row{
		Toggle(ToggleOpts{ID: "t", Label: "flip", Get: func() bool { return on }, Set: func(v bool) { on = v }}),
	}}}
	grid := gridOf(s, 30, 100)
	row := rowWith(grid, "flip")
	x0, _ := s.paneSpan(1)
	s.pressKey(&Key{Mouse: &Mouse{Row: rowWith(grid, "top-row"), Col: 8, Click: true}}, 30, 100)
	s.pressKey(&Key{Mouse: &Mouse{Row: row, Col: x0 + 8, Click: true}}, 30, 100)
	if on {
		t.Fatal("two clicks, one in each pane, toggled the second pane's row")
	}
	// Two clicks on the same row still make a double click.
	s.pressKey(&Key{Mouse: &Mouse{Row: row, Col: x0 + 8, Click: true}}, 30, 100)
	if !on {
		t.Fatal("a real double click no longer toggles")
	}
}
