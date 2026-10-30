package mint

import (
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
