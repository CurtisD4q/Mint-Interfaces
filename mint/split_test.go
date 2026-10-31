package mint

import (
	"fmt"
	"strings"
	"testing"
)

func paneScreen(n int) *Screen {
	s := &Screen{
		Title: "Top",
		Sections: []Section{{
			Name: "Top",
			Rows: []Row{
				Action(ActionOpts{Label: "top-row"}),
				Action(ActionOpts{Label: "top-next"}),
			},
		}},
	}
	titles := []string{"Alpha", "Beta", "Gamma"}
	labels := []string{"alpha-row", "beta-row", "gamma-row"}
	for i := 0; i < n-1; i++ {
		title, label := titles[i], labels[i]
		s.Panes = append(s.Panes, &Pane{
			Title: title,
			Sections: []Section{{
				Name: title,
				Rows: []Row{Action(ActionOpts{Label: label})},
			}},
		})
	}
	return s
}

func stripFrame(frame string) string {
	return sgrSeq.ReplaceAllString(frame, "")
}

func TestSplitDrawsTwoThreeAndFourPanes(t *testing.T) {
	for _, n := range []int{2, 3, 4} {
		s := paneScreen(n)
		frame := stripFrame(s.renderFrame(30, 80))
		if !strings.Contains(frame, "top-row") {
			t.Fatalf("%d panes: top row missing\n%s", n, frame)
		}
		for i := 0; i < n-1; i++ {
			title := []string{"Alpha", "Beta", "Gamma"}[i]
			label := []string{"alpha-row", "beta-row", "gamma-row"}[i]
			if !strings.Contains(frame, title) {
				t.Fatalf("%d panes: divider %q missing\n%s", n, title, frame)
			}
			if !strings.Contains(frame, label) {
				t.Fatalf("%d panes: %q missing\n%s", n, label, frame)
			}
		}
		if !strings.Contains(frame, "Tab/S-Tab") || !strings.Contains(frame, "| hide") {
			t.Fatalf("%d panes: split keys missing from footer\n%s", n, frame)
		}
		bands := s.layoutBands(s.viewTop, s.viewAvail)
		if len(bands) != n {
			t.Fatalf("%d panes: %d bands", n, len(bands))
		}
		for _, b := range bands {
			if b.rows < 3 {
				t.Fatalf("%d panes: band %d has %d rows", n, b.index, b.rows)
			}
			if b.index > 0 && b.divRow == 0 {
				t.Fatalf("%d panes: extra pane %d has no divider", n, b.index)
			}
		}
	}
}

func TestSplitFocusCycles(t *testing.T) {
	s := paneScreen(4)
	s.renderFrame(30, 80)
	if s.focus != 0 {
		t.Fatalf("focus %d", s.focus)
	}
	s.pressKey(&Key{Name: "\t"}, 30, 80)
	s.pressKey(&Key{Name: "\t"}, 30, 80)
	if s.focus != 2 {
		t.Fatalf("after two tabs focus %d", s.focus)
	}
	s.pressKey(&Key{Name: "btab", Special: true}, 30, 80)
	if s.focus != 1 {
		t.Fatalf("shift-tab focus %d", s.focus)
	}
	s.focus = 3
	s.pressKey(&Key{Name: "\t"}, 30, 80)
	if s.focus != 0 {
		t.Fatalf("wrap focus %d", s.focus)
	}
	s.pressKey(&Key{Name: "btab", Special: true}, 30, 80)
	if s.focus != 3 {
		t.Fatalf("backward wrap focus %d", s.focus)
	}
}

func TestSplitResizeClamps(t *testing.T) {
	s := paneScreen(2)
	s.renderFrame(30, 80)
	before := s.layoutBands(s.viewTop, s.viewAvail)
	if len(before) != 2 || before[0].rows < 3 || before[1].rows < 3 {
		t.Fatalf("start %+v", before)
	}
	for i := 0; i < 40; i++ {
		s.pressKey(&Key{Name: "]"}, 30, 80)
	}
	grown := s.layoutBands(s.viewTop, s.viewAvail)
	if grown[0].rows <= before[0].rows {
		t.Fatalf("] did not grow the top pane: before %d after %d", before[0].rows, grown[0].rows)
	}
	for _, b := range grown {
		if b.rows < 3 {
			t.Fatalf("pane %d shrank below 3: %+v", b.index, grown)
		}
	}
	for i := 0; i < 80; i++ {
		s.pressKey(&Key{Name: "["}, 30, 80)
	}
	shrunk := s.layoutBands(s.viewTop, s.viewAvail)
	for _, b := range shrunk {
		if b.rows < 3 {
			t.Fatalf("pane %d below 3 after [: %+v", b.index, shrunk)
		}
	}
	if shrunk[0].rows != 3 {
		t.Fatalf("focused pane should stop at 3 rows, got %+v", shrunk)
	}
}

func TestSplitHideAndShow(t *testing.T) {
	s := paneScreen(3)
	s.renderFrame(30, 80)
	s.pressKey(&Key{Name: "|"}, 30, 80)
	if !s.topHidden {
		t.Fatal("top pane was not hidden")
	}
	frame := stripFrame(s.renderFrame(30, 80))
	if strings.Contains(frame, "top-row") {
		t.Fatalf("hidden top still drew its row\n%s", frame)
	}
	if !strings.Contains(frame, "alpha-row") || !strings.Contains(frame, "beta-row") {
		t.Fatalf("other panes disappeared\n%s", frame)
	}
	s.pressKey(&Key{Name: "|"}, 30, 80)
	if s.topHidden {
		t.Fatal("| did not show the focused pane again")
	}
	// Hide two panes; the last one stays.
	s.pressKey(&Key{Name: "|"}, 30, 80) // hide top again
	s.pressKey(&Key{Name: "\t"}, 30, 80)
	s.pressKey(&Key{Name: "|"}, 30, 80) // hide alpha
	s.pressKey(&Key{Name: "\t"}, 30, 80)
	if s.focus != 2 {
		t.Fatalf("focus %d, want the last pane", s.focus)
	}
	s.pressKey(&Key{Name: "|"}, 30, 80)
	if s.paneHidden(2) {
		t.Fatal("hid the last visible pane")
	}
	frame = stripFrame(s.renderFrame(30, 80))
	if !strings.Contains(frame, "beta-row") {
		t.Fatalf("last pane lost its rows\n%s", frame)
	}
}

func TestSplitShortTerminalCollapsesUnfocused(t *testing.T) {
	s := paneScreen(4)
	const h, w = 12, 80
	frame := stripFrame(s.renderFrame(h, w))
	if strings.Contains(frame, "Terminal too small") {
		t.Fatalf("12 rows should still draw\n%s", frame)
	}
	if !strings.Contains(frame, "top-row") {
		t.Fatalf("focused top missing\n%s", frame)
	}
	for _, label := range []string{"alpha-row", "beta-row", "gamma-row"} {
		if strings.Contains(frame, label) {
			t.Fatalf("unfocused %s was not collapsed\n%s", label, frame)
		}
	}
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(frame, title) {
			t.Fatalf("collapsed pane lost its divider %s\n%s", title, frame)
		}
	}
	s.pressKey(&Key{Name: "\t"}, h, w)
	frame = stripFrame(s.renderFrame(h, w))
	if !strings.Contains(frame, "alpha-row") {
		t.Fatalf("tab did not open Alpha\n%s", frame)
	}
	if strings.Contains(frame, "top-row") {
		t.Fatalf("previous pane stayed open\n%s", frame)
	}
}

func TestSplitScrollsIndependently(t *testing.T) {
	rows := make([]Row, 0, 25)
	for i := 0; i < 25; i++ {
		rows = append(rows, Action(ActionOpts{Label: "top-item"}))
	}
	other := make([]Row, 0, 25)
	for i := 0; i < 25; i++ {
		other = append(other, Action(ActionOpts{Label: "low-item"}))
	}
	s := &Screen{
		Title:    "Top",
		Sections: []Section{{Name: "Top", Rows: rows}},
		Panes: []*Pane{{
			Title:    "Lower",
			Sections: []Section{{Name: "Lower", Rows: other}},
		}},
	}
	s.renderFrame(20, 80)
	top, low := s.uiAt(0), s.uiAt(1)
	topRow, lowRow := top.row, low.row
	topScroll, lowScroll := top.scroll, low.scroll
	for i := 0; i < 8; i++ {
		s.pressKey(&Key{Name: "down"}, 20, 80)
	}
	if top.row == topRow && top.scroll == topScroll {
		t.Fatal("top pane did not scroll")
	}
	if low.row != lowRow || low.scroll != lowScroll {
		t.Fatalf("lower pane moved with the top: row %d->%d scroll %d->%d", lowRow, low.row, lowScroll, low.scroll)
	}
	s.pressKey(&Key{Name: "\t"}, 20, 80)
	lowRow, lowScroll = low.row, low.scroll
	topRow, topScroll = top.row, top.scroll
	for i := 0; i < 5; i++ {
		s.pressKey(&Key{Name: "down"}, 20, 80)
	}
	if low.row == lowRow && low.scroll == lowScroll {
		t.Fatal("lower pane did not scroll on its own")
	}
	if top.row != topRow || top.scroll != topScroll {
		t.Fatalf("top pane moved while the lower had focus: row %d->%d scroll %d->%d", topRow, top.row, topScroll, top.scroll)
	}
}

func TestSplitPinnedHeaderInTopPane(t *testing.T) {
	body := make([][]string, 16)
	for i := range body {
		body[i] = []string{"id-row", "name", "detail"}
	}
	s := &Screen{
		Title: "Processes",
		Sections: []Section{{
			Name: "Processes",
			Rows: []Row{Table(TableOpts{
				Headers: []string{"ID", "NAME", "DETAIL"},
				Rows:    body,
				Viewer:  true,
			})},
		}},
		Panes: []*Pane{{
			Title:    "Details",
			Sections: []Section{{Rows: []Row{Note("detail-pane")}}},
		}},
	}
	s.renderFrame(24, 90)
	top := s.uiAt(0)
	// Move onto the table body and past the first rows.
	for i := 0; i < 12; i++ {
		s.pressKey(&Key{Name: "down"}, 24, 90)
	}
	if top.scroll < 1 && top.row < 4 {
		t.Fatalf("table did not scroll: row %d scroll %d", top.row, top.scroll)
	}
	frame := stripFrame(s.renderFrame(24, 90))
	if !strings.Contains(frame, "ID") || !strings.Contains(frame, "NAME") {
		t.Fatalf("pinned header missing\n%s", frame)
	}
	if !strings.Contains(frame, "detail-pane") {
		t.Fatalf("lower pane missing\n%s", frame)
	}
	// The header is pinned inside the top band, above the body that remains.
	bands := s.bands
	if len(bands) == 0 || bands[0].index != 0 || bands[0].rows < 3 {
		t.Fatalf("top band %+v", bands)
	}
	items := top.items
	lines := layoutContent(items, top.scroll, bands[0].top, bands[0].rows)
	if len(lines) < 2 {
		t.Fatalf("laid out %d lines", len(lines))
	}
	if !items[lines[0].index].pin {
		t.Fatalf("first line in the top pane is not pinned: %q", items[lines[0].index].label)
	}
}

func TestNoPanesRegression(t *testing.T) {
	s := &Screen{
		Title: "Tools",
		Sections: []Section{{
			Name: "List",
			Rows: []Row{
				Note("alpha"),
				Action(ActionOpts{Label: "beta"}),
				Note("gamma"),
			},
		}},
	}
	const h, w = 24, 80
	first := s.renderFrame(h, w)
	second := s.renderFrame(h, w)
	if first != second {
		t.Fatal("drawing a screen twice changed the frame")
	}
	plain := stripFrame(first)
	for _, want := range []string{"Tools", "List", "alpha", "beta", "gamma", "move"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("frame missing %q\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Tab/S-Tab") || strings.Contains(plain, "| hide") {
		t.Fatalf("split keys leaked onto a screen with no panes\n%s", plain)
	}
	// The same rows land on the lines the single-column layout has always used.
	st := s.uiAt(0)
	items := st.flatten(s.Sections, w, s.SectionSpacing)
	lines := layoutContent(items, 0, s.viewTop, s.viewAvail)
	grid := frameGrid(first, h, w)
	for _, ln := range lines {
		it := items[ln.index]
		if it.label == "" || it.wtype == "spacer" {
			continue
		}
		if ln.row < 1 || ln.row > h {
			t.Fatalf("row %d out of range", ln.row)
		}
		if !strings.Contains(grid[ln.row-1], it.label) {
			t.Fatalf("label %q not on line %d\n%s", it.label, ln.row, strings.Join(grid, "\n"))
		}
	}
	// A second screen with the same sections produces the same frame.
	again := (&Screen{
		Title: "Tools",
		Sections: []Section{{
			Name: "List",
			Rows: []Row{
				Note("alpha"),
				Action(ActionOpts{Label: "beta"}),
				Note("gamma"),
			},
		}},
	}).renderFrame(h, w)
	if again != first {
		t.Fatal("two screens with no panes drew different frames")
	}
}

func TestSplitDetailFollowsTheCursor(t *testing.T) {
	var got string
	s := &Screen{
		Title: "Processes",
		Sections: []Section{{
			Name: "Processes",
			Rows: []Row{Table(TableOpts{
				Headers: []string{"PID", "CMD"},
				Rows:    [][]string{{"1", "init"}, {"2", "bash"}},
			})},
		}},
	}
	s.Panes = []*Pane{{
		Title: "Details",
		Refresh: func(p *Pane) {
			_, cells, _, ok := s.SelectedTableRow()
			if !ok || len(cells) == 0 {
				got = ""
				p.Sections = []Section{{Rows: []Row{Note("none")}}}
				return
			}
			got = cells[0]
			p.Sections = []Section{{Rows: []Row{Note("pid " + cells[0])}}}
		},
	}}
	frame := stripFrame(s.renderFrame(24, 80))
	if got != "1" || !strings.Contains(frame, "pid 1") {
		t.Fatalf("first row: got %q\n%s", got, frame)
	}
	// The header and rule are not selectable; one Down reaches the next row.
	s.pressKey(&Key{Name: "down"}, 24, 80)
	frame = stripFrame(s.renderFrame(24, 80))
	if got != "2" || !strings.Contains(frame, "pid 2") {
		t.Fatalf("second row: got %q\n%s", got, frame)
	}
}

func TestSplitMouseFocusesThePane(t *testing.T) {
	s := paneScreen(3)
	s.renderFrame(30, 80)
	var alpha int
	for _, b := range s.bands {
		if b.index == 1 && b.rows > 0 {
			alpha = b.top
		}
	}
	if alpha == 0 {
		t.Fatalf("no alpha band %+v", s.bands)
	}
	s.pressKey(&Key{Mouse: &Mouse{Row: alpha, Col: 4, Click: true}}, 30, 80)
	if s.focus != 1 {
		t.Fatalf("click focused %d, want 1", s.focus)
	}
}

// frameGrid replays a captured frame into a cell grid so tests can see
// which line a label was drawn on.
func frameGrid(frame string, h, w int) []string {
	grid := make([][]rune, h)
	for i := range grid {
		row := make([]rune, w)
		for j := range row {
			row[j] = ' '
		}
		grid[i] = row
	}
	r, c := 1, 1
	i := 0
	for i < len(frame) {
		if frame[i] == 0x1b && i+1 < len(frame) && frame[i+1] == '[' {
			j := i + 2
			for j < len(frame) {
				b := frame[j]
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					j++
					break
				}
				j++
			}
			seq := frame[i+2 : j]
			if len(seq) == 0 {
				i = j
				continue
			}
			cmd := seq[len(seq)-1]
			args := seq[:len(seq)-1]
			switch cmd {
			case 'H', 'f':
				r, c = 1, 1
				if args != "" {
					parts := strings.Split(args, ";")
					if len(parts) > 0 && parts[0] != "" {
						r = atoi(parts[0])
					}
					if len(parts) > 1 && parts[1] != "" {
						c = atoi(parts[1])
					}
				}
			case 'J':
				if args == "2" || args == "" {
					for y := range grid {
						for x := range grid[y] {
							grid[y][x] = ' '
						}
					}
				}
			}
			i = j
			continue
		}
		ch, size := rune(frame[i]), 1
		if frame[i] >= 0x80 {
			ch, size = decodeRune(frame[i:])
		}
		if ch == '\n' {
			r++
			c = 1
		} else if r >= 1 && r <= h && c >= 1 && c <= w {
			grid[r-1][c-1] = ch
			c++
		} else if c >= 1 {
			c++
		}
		i += size
	}
	out := make([]string, h)
	for i := range grid {
		out[i] = strings.TrimRight(string(grid[i]), " ")
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func decodeRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 1
}

func manyNotes(prefix string, n int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Note(prefix)
	}
	if n > 0 {
		rows[0] = Note(prefix + "-first")
		rows[n-1] = Note(prefix + "-last")
	}
	return rows
}

func contentLines(frame string, h, w, top, avail int) []string {
	grid := frameGrid(frame, h, w)
	if top < 1 {
		top = 1
	}
	end := top - 1 + avail
	if end > len(grid) {
		end = len(grid)
	}
	if top-1 > len(grid) {
		return nil
	}
	return grid[top-1 : end]
}

func contentBlank(line string) bool {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "│╭╮╰╯─ ")
	return strings.TrimSpace(line) == ""
}

func maxInternalBlanks(lines []string) int {
	maxRun, run, seen := 0, 0, false
	for _, ln := range lines {
		if contentBlank(ln) {
			if seen {
				run++
			}
			continue
		}
		if seen && run > maxRun {
			maxRun = run
		}
		seen = true
		run = 0
	}
	return maxRun
}

func blanksBetween(lines []string, from, to string) (int, bool) {
	a, b := -1, -1
	for i, ln := range lines {
		if a < 0 && strings.Contains(ln, from) {
			a = i
			continue
		}
		if a >= 0 && strings.Contains(ln, to) {
			b = i
			break
		}
	}
	if a < 0 || b < 0 {
		return 0, false
	}
	n := 0
	for _, ln := range lines[a+1 : b] {
		if !contentBlank(ln) {
			return -1, false
		}
		n++
	}
	return n, true
}

func blanksAbove(grid []string, row, stop int) int {
	n := 0
	for r := row - 1; r >= stop; r-- {
		if r < 1 || r > len(grid) {
			break
		}
		if !contentBlank(grid[r-1]) {
			break
		}
		n++
	}
	return n
}

func bandByTitle(s *Screen, title string) (paneBand, bool) {
	for _, b := range s.bands {
		if s.paneTitle(b.index) == title {
			return b, true
		}
	}
	return paneBand{}, false
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// longSplitScreen is a top pane, one other pane, and Help, each with more
// rows than any of them is given, so the 50/10/40 split is not shrunk.
func longSplitScreen() *Screen {
	return &Screen{
		Title:    "Main",
		Sections: []Section{{Name: "Options", Rows: manyNotes("opt", 40)}},
		Panes: []*Pane{
			{Title: "Targets", Sections: []Section{{Name: "Targets", Rows: manyNotes("tgt", 40)}}},
			{Title: "Help", Sections: []Section{{Name: "Manual", Rows: manyNotes("hlp", 40)}}},
		},
	}
}

func TestPaneSplitAtHeights(t *testing.T) {
	const w = 80
	for _, h := range []int{24, 30, 40, 60} {
		t.Run(fmt.Sprintf("%d", h), func(t *testing.T) {
			s := longSplitScreen()
			frame := s.renderFrame(h, w)
			main, ok := bandByTitle(s, "Main")
			targets, okT := bandByTitle(s, "Targets")
			help, okH := bandByTitle(s, "Help")
			if !ok || !okT || !okH || main.rows < 1 || targets.rows < 1 || help.rows < 1 {
				t.Fatalf("bands %+v", s.bands)
			}
			total := main.rows + targets.rows + help.rows
			if absInt(main.rows-total*50/100) > 1 {
				t.Fatalf("h=%d top %d, want ~50%% of %d", h, main.rows, total)
			}
			if absInt(targets.rows-total*40/100) > 1 {
				t.Fatalf("h=%d targets %d, want ~40%% of %d", h, targets.rows, total)
			}
			helpShare := total * 10 / 100
			if helpShare < paneMinRows {
				helpShare = paneMinRows
			}
			if absInt(help.rows-helpShare) > 1 {
				t.Fatalf("h=%d help %d, want ~%d", h, help.rows, helpShare)
			}
			for _, b := range s.bands {
				if b.rows > 0 && b.rows < paneMinRows {
					t.Fatalf("h=%d pane %d has %d rows", h, b.index, b.rows)
				}
			}
			if slack := bandSlack(s.bands, s.viewTop, s.viewAvail); slack != 0 {
				t.Fatalf("h=%d long panes left %d unused rows", h, slack)
			}
			grid := frameGrid(frame, h, w)
			lines := contentLines(frame, h, w, s.viewTop, s.viewAvail)
			if n := maxInternalBlanks(lines); n > paneGapLines {
				t.Fatalf("h=%d gap of %d blank lines\n%s", h, n, strings.Join(lines, "\n"))
			}
			for _, b := range s.bands {
				if b.index == 0 || b.divRow == 0 {
					continue
				}
				if n := blanksAbove(grid, b.divRow, s.viewTop); n != paneGapLines {
					t.Fatalf("h=%d %s divider has %d blank lines above it\n%s", h, s.paneTitle(b.index), n, strings.Join(lines, "\n"))
				}
			}
		})
	}
}

func TestPaneSplitWithoutHelp(t *testing.T) {
	const w = 80
	for _, h := range []int{24, 30, 40, 60} {
		s := &Screen{
			Title:    "Main",
			Sections: []Section{{Name: "Options", Rows: manyNotes("opt", 40)}},
			Panes: []*Pane{
				{Title: "Details", Sections: []Section{{Name: "Details", Rows: manyNotes("det", 40)}}},
				{Title: "Actions", Sections: []Section{{Name: "Actions", Rows: manyNotes("act", 40)}}},
			},
		}
		frame := s.renderFrame(h, w)
		main, _ := bandByTitle(s, "Main")
		details, _ := bandByTitle(s, "Details")
		actions, _ := bandByTitle(s, "Actions")
		total := main.rows + details.rows + actions.rows
		if absInt(main.rows-total*50/100) > 1 {
			t.Fatalf("h=%d top %d, want ~50%% of %d", h, main.rows, total)
		}
		// The other half is split evenly. The two panes differ by at most one row.
		if absInt(details.rows-actions.rows) > 1 {
			t.Fatalf("h=%d others %d and %d", h, details.rows, actions.rows)
		}
		half := total - main.rows
		if absInt(details.rows-half/2) > 1 || absInt(actions.rows-half/2) > 1 {
			t.Fatalf("h=%d others %d+%d, half is %d", h, details.rows, actions.rows, half)
		}
		lines := contentLines(frame, h, w, s.viewTop, s.viewAvail)
		if n := maxInternalBlanks(lines); n > paneGapLines {
			t.Fatalf("h=%d gap of %d\n%s", h, n, strings.Join(lines, "\n"))
		}
	}
}

func TestShortPaneGivesRowsBack(t *testing.T) {
	const w = 80
	for _, h := range []int{24, 30, 40, 60} {
		s := &Screen{
			Title:    "Main",
			Sections: []Section{{Name: "Options", Rows: manyNotes("opt", 40)}},
			Panes: []*Pane{
				{Title: "Targets", Sections: []Section{{Name: "Targets", Rows: []Row{Note("none")}}}},
				{Title: "Help", Sections: []Section{{Name: "Manual", Rows: manyNotes("hlp", 40)}}},
			},
		}
		frame := s.renderFrame(h, w)
		main, _ := bandByTitle(s, "Main")
		targets, _ := bandByTitle(s, "Targets")
		help, _ := bandByTitle(s, "Help")
		total := main.rows + targets.rows + help.rows
		if targets.rows != paneMinRows {
			t.Fatalf("h=%d short pane kept %d rows", h, targets.rows)
		}
		share := total * 40 / 100
		if share > paneMinRows && targets.rows >= share {
			t.Fatalf("h=%d short pane kept a %d-row share of %d", h, targets.rows, total)
		}
		// The long panes together hold more than the short pane's old share.
		if main.rows+help.rows <= total/2 {
			t.Fatalf("h=%d long panes only have %d of %d", h, main.rows+help.rows, total)
		}
		lines := contentLines(frame, h, w, s.viewTop, s.viewAvail)
		if n := maxInternalBlanks(lines); n > paneGapLines {
			t.Fatalf("h=%d gap of %d\n%s", h, n, strings.Join(lines, "\n"))
		}
		grid := frameGrid(frame, h, w)
		if n := blanksAbove(grid, targets.divRow, s.viewTop); n != paneGapLines {
			t.Fatalf("h=%d gap above targets is %d\n%s", h, n, strings.Join(lines, "\n"))
		}
		if help.divRow > 0 {
			if n := blanksAbove(grid, help.divRow, s.viewTop); n != paneGapLines {
				t.Fatalf("h=%d gap above help is %d\n%s", h, n, strings.Join(lines, "\n"))
			}
		}
	}
}

// TestHelpGapSitsAtTheBottom is the probe screen: a short results pane and
// a short Help pane used to be padded out, so Help sat at the bottom of
// the terminal with a tall blank band above it.
func TestHelpGapSitsAtTheBottom(t *testing.T) {
	const w = 80
	for _, h := range []int{40, 60} {
		s := &Screen{
			Title: "nmap",
			Sections: []Section{
				{Name: "Target", Rows: []Row{Note("host"), Note("ports")}},
				{Name: "Run", Rows: []Row{Note("once"), Note("go")}},
			},
			Panes: []*Pane{
				{Title: "Discovered targets", Sections: []Section{{Name: "Discovered targets", Rows: []Row{Note("none")}}}},
				{Title: "Help", Sections: []Section{{Name: "Manual", Rows: []Row{
					Note("line-a"), Note("line-b"), Note("line-c"), Note("line-d"),
				}}}},
			},
		}
		frame := s.renderFrame(h, w)
		help, ok := bandByTitle(s, "Help")
		targets, okT := bandByTitle(s, "Discovered targets")
		if !ok || !okT || help.top == 0 || targets.divRow == 0 || help.divRow == 0 {
			t.Fatalf("h=%d bands %+v", h, s.bands)
		}
		if targets.rows != paneMinRows {
			t.Fatalf("h=%d discovered targets is %d rows", h, targets.rows)
		}
		grid := frameGrid(frame, h, w)
		lines := contentLines(frame, h, w, s.viewTop, s.viewAvail)
		if n := blanksAbove(grid, help.divRow, s.viewTop); n != paneGapLines {
			t.Fatalf("h=%d %d blank lines above Help\n%s", h, n, strings.Join(lines, "\n"))
		}
		if n := maxInternalBlanks(lines); n > paneGapLines {
			t.Fatalf("h=%d internal gap %d\n%s", h, n, strings.Join(lines, "\n"))
		}
		slack := s.viewTop + s.viewAvail - (help.top + help.rows)
		if slack <= paneGapLines {
			t.Fatalf("h=%d unused rows are not under Help (slack %d)\n%s", h, slack, strings.Join(lines, "\n"))
		}
	}
}

func TestSectionGapIsTwoOnPaneScreens(t *testing.T) {
	const h, w = 40, 80
	s := &Screen{
		Title:          "Main",
		SectionSpacing: 1, // the app default; pane screens still use two
		Sections: []Section{
			{Name: "Target", Rows: []Row{Note("host")}},
			{Name: "Run", Rows: []Row{Note("once")}},
		},
		Panes: []*Pane{{
			Title:    "Help",
			Sections: []Section{{Name: "Manual", Rows: manyNotes("hlp", 30)}},
		}},
	}
	frame := s.renderFrame(h, w)
	lines := contentLines(frame, h, w, s.viewTop, s.viewAvail)
	n, ok := blanksBetween(lines, "host", "Run")
	if !ok || n != paneGapLines {
		t.Fatalf("section gap %d ok=%v\n%s", n, ok, strings.Join(lines, "\n"))
	}
	if gap := maxInternalBlanks(lines); gap > paneGapLines {
		t.Fatalf("gap %d\n%s", gap, strings.Join(lines, "\n"))
	}
}

func TestNoPanesKeepTheirOwnSpacing(t *testing.T) {
	const h, w = 24, 80
	for _, spacing := range []int{0, 1} {
		s := &Screen{
			Title:          "Tools",
			SectionSpacing: spacing,
			Sections: []Section{
				{Name: "List", Rows: []Row{Note("alpha"), Note("beta")}},
				{Name: "More", Rows: []Row{Note("gamma")}},
			},
		}
		first := s.renderFrame(h, w)
		second := s.renderFrame(h, w)
		if first != second {
			t.Fatalf("spacing %d: drawing twice changed the frame", spacing)
		}
		plain := stripFrame(first)
		if strings.Contains(plain, "Tab/S-Tab") || strings.Contains(plain, "| hide") {
			t.Fatalf("spacing %d: split keys on a screen with no panes\n%s", spacing, plain)
		}
		lines := contentLines(first, h, w, s.viewTop, s.viewAvail)
		n, ok := blanksBetween(lines, "beta", "More")
		if !ok || n != spacing {
			t.Fatalf("spacing %d: gap %d ok=%v\n%s", spacing, n, ok, strings.Join(lines, "\n"))
		}
	}
}
