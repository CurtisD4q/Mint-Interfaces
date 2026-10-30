package mint

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

func wideTable() Row {
	return Table(TableOpts{
		Headers: []string{"PID", "USER", "COMMAND", "DESCRIPTION"},
		Rows: [][]string{
			{"1", "root", "/sbin/init", "a very long description that will not fit"},
			{"42", "curtis", "bash", "another long description here"},
		},
	})
}

// With the viewer off, a wide table is fitted to the width by shrinking its
// widest column, exactly as before.
func TestTableViewerOffFitsTheWidth(t *testing.T) {
	rows := expandTable(wideTable(), 60)
	for _, r := range rows {
		if got := util.VLen(r.label); got > 60 {
			t.Errorf("row is %d cells at width 60: %q", got, r.label)
		}
	}
}

// With the viewer on, every column keeps its natural width and the whole
// table scrolls sideways instead. Fitting would shrink the widest column;
// the viewer keeps it, so scrolling to the end still reaches the last one.
func TestTableViewerOnKeepsNaturalWidth(t *testing.T) {
	tv := wideTable()
	tv.viewer = true
	var tail string
	for _, skip := range []int{0, 8, 20, 35} {
		tv.hscroll = skip
		for _, r := range expandTable(tv, 30) {
			if r.wtype == "tablerow" {
				tail += r.label + " "
				break
			}
		}
	}
	if !strings.Contains(tail, "that will not fit") {
		t.Errorf("scrolling should reach the end of the description; saw %q", tail)
	}

	// The fitted table has no such column left: its description is cut.
	fitted := wideTable()
	for _, r := range expandTable(fitted, 30) {
		if r.wtype == "tablerow" {
			if strings.Contains(r.label, "long description that will not fit") {
				t.Errorf("a fitted table should not keep the full description: %q", r.label)
			}
			break
		}
	}
}

// Scrolling sideways shows a later slice of the same columns.
func TestTableViewerScrollsSideways(t *testing.T) {
	tv := wideTable()
	tv.viewer = true
	first := expandTable(tv, 30)
	tv.hscroll = 8
	second := expandTable(tv, 30)

	var a, b string
	for _, r := range first {
		if r.wtype == "tablerow" {
			a = r.label
			break
		}
	}
	for _, r := range second {
		if r.wtype == "tablerow" {
			b = r.label
			break
		}
	}
	if a == b {
		t.Fatal("scrolling sideways did not change the row")
	}
	if util.VLen(b) > 30 {
		t.Errorf("scrolled row is %d cells wide, window is 30", util.VLen(b))
	}
	if len(b) == 0 {
		t.Error("scrolled row is empty")
	}
}

// The header stays put when the body scrolls vertically: it is marked so
// the screen can pin it.
func TestTableViewerPinsTheHeader(t *testing.T) {
	tv := wideTable()
	tv.viewer = true
	rows := expandTable(tv, 60)
	var header, rule int
	for _, r := range rows {
		if r.pin {
			if r.style == StyleHeading {
				header++
			} else {
				rule++
			}
		}
	}
	if header != 1 || rule != 1 {
		t.Errorf("pinned rows: %d headers, %d rules; want 1 and 1", header, rule)
	}
}

// A column can be made wider or narrower, and the change sticks across
// redraws because it is stored on the row the caller holds.
func TestTableViewerResizesColumns(t *testing.T) {
	tv := wideTable()
	tv.viewer = true
	before := expandTable(tv, 80)
	tv.colWidths = map[int]int{2: 30}
	after := expandTable(tv, 80)

	var a, b string
	for _, r := range before {
		if r.wtype == "tablerow" {
			a = r.label
			break
		}
	}
	for _, r := range after {
		if r.wtype == "tablerow" {
			b = r.label
			break
		}
	}
	if util.VLen(b) <= util.VLen(a) {
		t.Errorf("widening column 2 did not widen the row: %d -> %d", util.VLen(a), util.VLen(b))
	}
}

// ← → scroll a viewer table and leave every other row alone.
func TestTableViewerKeyScroll(t *testing.T) {
	tv := wideTable()
	tv.viewer = true
	scrollTable(&tv, tableScrollStep, 0)
	if tv.hscroll != tableScrollStep {
		t.Errorf("hscroll = %d, want %d", tv.hscroll, tableScrollStep)
	}
	scrollTable(&tv, -100, 0)
	if tv.hscroll != 0 {
		t.Errorf("hscroll = %d, want 0: scrolling left stops at the first column", tv.hscroll)
	}
	// A table that is not a viewer is never scrolled by the keys, because
	// screen.go only calls this for a viewer row.
}

// H and L resize the column the reader is looking at.
func TestTableViewerKeyResize(t *testing.T) {
	tv := wideTable()
	tv.viewer = true
	tv.hscroll = 0 // first column: PID
	resizeTableColumn(&tv, tv.hscroll, 1)
	if got := tv.colWidths[0]; got <= util.VLen("PID") {
		t.Errorf("column 0 width = %d, want wider than %d", got, util.VLen("PID"))
	}
	wide := tv.colWidths[0]
	resizeTableColumn(&tv, tv.hscroll, -1)
	if got := tv.colWidths[0]; got >= wide {
		t.Errorf("column 0 width = %d, want narrower than %d", got, wide)
	}
	// Never narrower than four cells.
	for i := 0; i < 40; i++ {
		resizeTableColumn(&tv, tv.hscroll, -1)
	}
	if got := tv.colWidths[0]; got < 4 {
		t.Errorf("column 0 width = %d, want at least 4", got)
	}
}

// Resizing then redrawing gives wider cells, not just a wider map entry.
func TestTableViewerResizeWidensTheDrawnRow(t *testing.T) {
	tv := wideTable()
	tv.viewer = true
	before := ""
	for _, r := range expandTable(tv, 100) {
		if r.wtype == "tablerow" {
			before = r.label
			break
		}
	}
	tv.colWidths = map[int]int{0: 12}
	after := ""
	for _, r := range expandTable(tv, 100) {
		if r.wtype == "tablerow" {
			after = r.label
			break
		}
	}
	if util.VLen(after) <= util.VLen(before) {
		t.Errorf("widening column 0 did not widen the row: %d -> %d\n%q\n%q",
			util.VLen(before), util.VLen(after), before, after)
	}
	if !strings.HasPrefix(after, "1           ") {
		t.Errorf("column 0 should be padded to 12 cells, got %q", after)
	}
}

// A tall viewer table scrolled well past its first rows still draws the
// header and the rule above the body that is left on screen. Rows that are
// not pinned stay on the line scroll has always given them.
func TestViewerHeaderStaysWhileTheBodyScrolls(t *testing.T) {
	const (
		contentTop = 4
		avail      = 8
		scroll     = 8
	)
	items := tallTable(true)
	lines := layoutContent(items, scroll, contentTop, avail)
	if len(lines) < 3 {
		t.Fatalf("drawn lines: %d", len(lines))
	}
	header, rule := items[lines[0].index], items[lines[1].index]
	if !header.pin || header.style != StyleHeading || !strings.Contains(header.label, "ID") {
		t.Fatalf("first drawn line is not the pinned header: %q pin=%v", header.label, header.pin)
	}
	if !rule.pin || rule.style == StyleHeading || !strings.Contains(rule.label, "─") {
		t.Fatalf("second drawn line is not the pinned rule: %q pin=%v", rule.label, rule.pin)
	}
	if lines[0].row != contentTop || lines[1].row != contentTop+1 || lines[1].row >= lines[2].row {
		t.Fatalf("header and rule are not above the body: %+v", lines[:3])
	}
	var drawn strings.Builder
	for _, ln := range lines[2:] {
		it := items[ln.index]
		if it.pin {
			t.Fatalf("a pinned row was drawn among the body: %q", it.label)
		}
		if ln.row != ln.index-scroll+contentTop {
			t.Fatalf("non-pin row %d drawn on line %d, want %d (%q)",
				ln.index, ln.row, ln.index-scroll+contentTop, it.label)
		}
		drawn.WriteString(it.label)
		drawn.WriteByte('\n')
	}
	body := drawn.String()
	if strings.Contains(body, "id-00") || !strings.Contains(body, "id-") {
		t.Fatalf("visible body should be past the first rows:\n%s", body)
	}
	if !strings.Contains(body, "after the table") {
		t.Fatalf("the row after the table was moved:\n%s", body)
	}

	// Nothing is stuck once the table itself has left the viewport.
	away := layoutContent(items, len(items)-1, contentTop, avail)
	if len(away) != 1 || items[away[0].index].label != "after the table" {
		got := make([]string, len(away))
		for i, ln := range away {
			got[i] = items[ln.index].label
		}
		t.Fatalf("header stuck after the table scrolled away: %q", got)
	}

	// The same scroll with the viewer off, and a screen with no pins, keep
	// every row on the line it had before pins existed. At scroll 0 the
	// viewer header is still on its own line, so it matches too.
	for _, sc := range []int{0, 1, scroll} {
		assertUnpinnedLayout(t, tallTable(false), sc, contentTop, avail)
	}
	assertUnpinnedLayout(t, tallTable(true), 0, contentTop, avail)
	var notes []Row
	for i := 0; i < 30; i++ {
		notes = append(notes, Note(fmt.Sprintf("note %d", i)))
	}
	st := &uiState{collapsed: map[string]bool{}}
	plain := st.flatten([]Section{{Name: "Log", Rows: notes}}, 80, 0)
	for _, sc := range []int{0, 3, 12} {
		assertUnpinnedLayout(t, plain, sc, contentTop, avail)
	}
}

func tallTable(viewer bool) []Row {
	rows := make([][]string, 12)
	for i := range rows {
		rows[i] = []string{fmt.Sprintf("id-%02d", i), "name", "detail"}
	}
	tv := Table(TableOpts{
		Headers: []string{"ID", "NAME", "DETAIL"},
		Rows:    rows,
		Viewer:  viewer,
	})
	st := &uiState{collapsed: map[string]bool{}}
	return st.flatten([]Section{{
		Name: "Processes",
		Rows: []Row{tv, Note("after the table")},
	}}, 100, 0)
}

// assertUnpinnedLayout checks that every item still lands on the line the
// screen used before pinned rows: index - scroll + contentTop.
func assertUnpinnedLayout(t *testing.T, items []Row, scroll, top, avail int) {
	t.Helper()
	lines := layoutContent(items, scroll, top, avail)
	bottom := top + avail - 1
	var want []contentLine
	for i := range items {
		r := i - scroll + top
		if r >= top && r <= bottom {
			want = append(want, contentLine{index: i, row: r})
		}
	}
	if len(lines) != len(want) {
		t.Fatalf("scroll %d: got %d lines, want %d\n got %+v\nwant %+v", scroll, len(lines), len(want), lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("scroll %d line %d: got %+v, want %+v", scroll, i, lines[i], want[i])
		}
	}
}

// The footer names ←→ and H/L only while the focused row is a viewer table.
func TestViewerFooterHints(t *testing.T) {
	saved := ScreenHints
	t.Cleanup(func() { ScreenHints = saved })
	base := " ↑↓ move   ←→/Space change   b home   q quit "
	ScreenHints = base
	custom := " b back "
	focused := viewerRow(t, true)
	plain := viewerRow(t, false)

	got := footerHints(false, "", focused)
	if got != strings.TrimRight(base, " ")+"   ←→ scroll sideways   H/L narrow or widen " {
		t.Fatalf("viewer hints: %q", got)
	}
	if !strings.Contains(got, "←→ scroll sideways") || !strings.Contains(got, "H/L narrow or widen") {
		t.Fatalf("missing viewer keys: %q", got)
	}
	if footerHints(false, "", plain) != base {
		t.Fatalf("non-viewer hints changed: %q", footerHints(false, "", plain))
	}
	if footerHints(false, "", nil) != base {
		t.Fatalf("no row changed hints: %q", footerHints(false, "", nil))
	}
	if footerHints(false, custom, plain) != custom {
		t.Fatalf("custom hints changed for a fitted table: %q", footerHints(false, custom, plain))
	}
	with := footerHints(false, custom, focused)
	if !strings.HasPrefix(with, strings.TrimRight(custom, " ")) || !strings.Contains(with, "H/L narrow or widen") {
		t.Fatalf("custom viewer hints: %q", with)
	}
	if footerHints(true, "", focused) != EditHints {
		t.Fatalf("editing shows %q", footerHints(true, "", focused))
	}
}

func viewerRow(t *testing.T, viewer bool) *Row {
	t.Helper()
	tv := wideTable()
	tv.viewer = viewer
	rows := expandTable(tv, 80)
	for i := range rows {
		if rows[i].wtype == "tablerow" {
			return &rows[i]
		}
	}
	t.Fatal("no table row")
	return nil
}

// tree(1) output is one indented column: its │, ├── and └── are the tree's
// drawing, not column separators, so it must not be split into columns and
// lose its left edge.
func TestParseTreeOutput(t *testing.T) {
	out := `.├── bot-home
│   ├── file1
│   └── file2
├── docs
│   └── a.md
└── README`
	headers, rows := ParseTable(strings.Split(out, "\n"))
	if len(headers) != 0 {
		t.Errorf("tree output should have no header row, got %q", headers)
	}
	if len(rows) != 6 {
		t.Fatalf("got %d rows, want 6: %v", len(rows), rows)
	}
	for i, want := range []string{
		".├── bot-home",
		"│   ├── file1",
		"│   └── file2",
		"├── docs",
		"│   └── a.md",
		"└── README",
	} {
		if len(rows[i]) != 1 || rows[i][0] != want {
			t.Errorf("row %d = %q, want %q", i, rows[i], want)
		}
	}
}

// Ordinary aligned and pipe tables must still parse into columns.
func TestParseTableStillSplitsColumns(t *testing.T) {
	for _, tc := range []struct {
		lines   []string
		headers []string
	}{
		{[]string{"NAME   READY   STATUS", "a      1/1     Running"}, []string{"NAME", "READY", "STATUS"}},
		{[]string{"PORT     STATE  SERVICE", "22/tcp   open   ssh"}, []string{"PORT", "STATE", "SERVICE"}},
		{[]string{"| a | b |", "|---|---|", "| 1 | 2 |"}, []string{"a", "b"}},
	} {
		h, rows := ParseTable(tc.lines)
		if len(h) != len(tc.headers) || len(rows) != 1 {
			t.Errorf("%q: headers=%q rows=%v", tc.lines[0], h, rows)
			continue
		}
		for i := range tc.headers {
			if h[i] != tc.headers[i] {
				t.Errorf("%q: header %d = %q, want %q", tc.lines[0], i, h[i], tc.headers[i])
			}
		}
	}
}
