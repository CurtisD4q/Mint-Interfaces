package mint

import (
	"os"
	"sync"
)

// Hover and pressed states.
//
// When the terminal reports mouse motion (mode 1003, with SGR reports),
// Mint remembers where the pointer is and whether the left button is held.
// Lists draw the row under the pointer with a hover look, and the row that
// was pressed with a pressed look until the button is let go or the
// pointer leaves it.
//
// Motion never reaches a screen as a key: ReadKey keeps the pointer
// position here and swallows the report. When the pointer moves to another
// row, or the button goes down or up, ReadKey returns nil, the idle tick
// every screen already handles, and the screen redraws if pointerWatch
// says the pointer changed. Moving within one row costs nothing.
//
// Set MINT_HOVER=0 to turn motion reporting off (for a slow SSH link, say);
// clicks and the wheel work as before.

// pointerState is the last pointer position the terminal reported.
type pointerState struct {
	mu       sync.Mutex
	known    bool // a report has arrived, and the pointer has not left the window
	row, col int
	down     bool // the left button is held
	pressRow int  // where it went down
	seq      uint64
}

var ptr pointerState

// hoverWanted reports whether Mint asks the terminal for motion reports.
func hoverWanted() bool {
	return rt.profile.SGRMouse && os.Getenv("MINT_HOVER") != "0"
}

// movePointer records a motion report. It reports whether the row or the
// button changed, which is when a screen needs to redraw.
func movePointer(row, col int, down bool) bool {
	ptr.mu.Lock()
	defer ptr.mu.Unlock()
	changed := !ptr.known || row != ptr.row || down != ptr.down
	ptr.known, ptr.row, ptr.col = true, row, col
	if !down {
		ptr.down = false
	}
	if changed {
		ptr.seq++
	}
	return changed
}

// pressPointer records the left button going down at row, col.
func pressPointer(row, col int) {
	ptr.mu.Lock()
	defer ptr.mu.Unlock()
	ptr.known, ptr.row, ptr.col = true, row, col
	ptr.down, ptr.pressRow = true, row
	ptr.seq++
}

// releasePointer records the button going up. It reports whether anything
// changed.
func releasePointer(row, col int) bool {
	ptr.mu.Lock()
	defer ptr.mu.Unlock()
	changed := ptr.down || !ptr.known || row != ptr.row
	ptr.known, ptr.row, ptr.col, ptr.down = true, row, col, false
	if changed {
		ptr.seq++
	}
	return changed
}

// forgetPointer drops the hover, when the terminal loses focus: the
// pointer is somewhere else, and no report says where.
func forgetPointer() {
	ptr.mu.Lock()
	defer ptr.mu.Unlock()
	if ptr.known || ptr.down {
		ptr.known, ptr.down = false, false
		ptr.seq++
	}
}

// hoverRow is the screen row under the pointer.
func hoverRow() (int, bool) {
	ptr.mu.Lock()
	defer ptr.mu.Unlock()
	return ptr.row, ptr.known && hoverWanted()
}

// pressedRow is the row being pressed: the button went down on it and the
// pointer is still on it.
func pressedRow() (int, bool) {
	ptr.mu.Lock()
	defer ptr.mu.Unlock()
	return ptr.pressRow, ptr.known && ptr.down && ptr.row == ptr.pressRow
}

// hoverAt reports whether row r has the hover look.
func hoverAt(r int) bool {
	row, ok := hoverRow()
	return ok && row == r
}

// pressedAt reports whether row r has the pressed look.
func pressedAt(r int) bool {
	row, ok := pressedRow()
	return ok && row == r
}

// pointerWatch tells a screen when the pointer changed since it last drew.
type pointerWatch struct{ seq uint64 }

func (w *pointerWatch) changed() bool {
	ptr.mu.Lock()
	s := ptr.seq
	ptr.mu.Unlock()
	if s == w.seq {
		return false
	}
	w.seq = s
	return true
}

// Looks for the two states. Both are palette-free, so they read on a
// terminal with no colour too.
const (
	hoverMark = "› "       // in the cursor column of the row under the pointer
	reverse   = esc + "7m" // a pressed row is drawn in reverse video
)

// paintPressed draws row r as pressed: a reverse-video bar with the label.
func (c *canvas) paintPressed(r, w int, label string) {
	if w < 6 {
		return
	}
	text := Truncate("▸ "+label, w-6)
	pad := w - 4 - VLen(text)
	if pad < 0 {
		pad = 0
	}
	c.put(r, 2, reverse+C.Bold+" "+text+spaces(pad)+" "+Reset)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
