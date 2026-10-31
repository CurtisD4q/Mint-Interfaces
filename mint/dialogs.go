package mint

import (
	"fmt"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// Dialogs are full-screen modals drawn in the same frame as the rest of the
// UI. They need the screen to be active (call them from row callbacks, or
// between ScreenEnter and ScreenLeave).

// Message shows body until Enter, Space, Esc, q or a click.
func Message(title, body string) {
	if title == "" {
		title = "Notice"
	}
	draw := func() {
		h, w := TermSize()
		cv := begin(h, w)
		cv.frame(h, w)
		cv.chromeTitle(w, title, "")
		cv.titleRule(w)
		row := 4
		for _, line := range wrapLines(body, max(10, w-6)) {
			if row >= h-3 {
				break
			}
			cv.put(row, 3, C.Text+line+Reset)
			row++
		}
		cv.put(h-3, 3, C.SelBg+C.Bright+" OK "+Reset)
		cv.chromeFooter(h, w, " Enter dismiss ")
		cv.paint()
	}
	draw()
	sz := watchSize()
	for {
		k := readNav(false)
		if k == nil {
			if sz.changed() {
				draw()
			}
			continue
		}
		switch {
		case k.Name == "esc" || k.Name == "\r" || k.Name == "\n" || k.Name == " " || k.Name == "q":
			return
		case k.Mouse != nil && k.Mouse.Click:
			return
		}
	}
}

// Confirm asks Yes/No with No preselected. y/n answer directly.
func Confirm(title, warning string) bool {
	if title == "" {
		title = "Are you sure?"
	}
	sel := 2
	type hitBox struct{ row, col0, col1, sel int }
	var hits []hitBox
	draw := func() {
		h, w := TermSize()
		cv := begin(h, w)
		cv.frame(h, w)
		cv.chromeTitle(w, title, "")
		cv.titleRule(w)
		hits = hits[:0]
		row := 4
		for _, line := range wrapLines(warning, max(10, w-6)) {
			if row >= h-5 {
				break
			}
			if line != "" {
				cv.put(row, 3, C.Text+line+Reset)
			}
			row++
		}
		row++
		yes := C.Muted + " Yes " + Reset
		if sel == 1 {
			yes = C.SelBg + C.Bright + " Yes " + Reset
		}
		no := C.Muted + " No " + Reset
		if sel == 2 {
			no = C.SelBg + C.Bright + " No " + Reset
		}
		cv.put(row, 3, yes+"   "+no)
		hits = append(hits, hitBox{row, 3, 7, 1}, hitBox{row, 11, 14, 2})
		cv.chromeFooter(h, w, " ←→ choose   Enter confirm   Esc cancel   y/n ")
		cv.paint()
	}
	draw()
	sz := watchSize()
	for {
		k := readNav(false)
		if k == nil {
			if sz.changed() {
				draw()
			}
			continue
		}
		switch n := k.Name; {
		case n == "esc" || n == "q" || n == "n" || n == "N" || n == "\x03":
			return false
		case n == "y" || n == "Y":
			return true
		case n == "left":
			sel = 1
			draw()
		case n == "right":
			sel = 2
			draw()
		case n == "\r" || n == "\n" || n == " ":
			return sel == 1
		case k.Mouse != nil && k.Mouse.Click:
			for _, hb := range hits {
				if k.Mouse.Row == hb.row && k.Mouse.Col >= hb.col0 && k.Mouse.Col <= hb.col1 {
					sel = hb.sel
					draw()
					return sel == 1
				}
			}
		}
	}
}

// Choice is a list entry with a label shown and a value returned.
type Choice struct {
	Label string
	Value string
}

// Choose offers a filterable list of strings.
func Choose(title string, items []string, subtitle string) (string, bool) {
	all := make([]Choice, len(items))
	for i, s := range items {
		all[i] = Choice{Label: s, Value: s}
	}
	c, ok := ChooseItem(title, all, subtitle)
	return c.Value, ok
}

// asciiLower lowercases A–Z only, leaving every other byte as it is.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// dropLastChar removes the final character (not just the final byte).
func dropLastChar(s string) string {
	ch := util.Chars(s)
	if len(ch) == 0 {
		return ""
	}
	return strings.Join(ch[:len(ch)-1], "")
}

// ChooseItem offers a filterable list. Powered by baseRuntime's virtualList
// and lineBuffer engines, it scales to 100,000+ entries with zero-alloc
// Unicode case-insensitive incremental narrowing, cursor movement + paste
// in the filter bar, and Space multi-toggle (returning comma-joined values
// in Choice.Value when multiple items are marked).
func ChooseItem(title string, items []Choice, subtitle string) (Choice, bool) {
	if title == "" {
		title = "Select"
	}
	vl := newVirtualList(items, nil)
	filterBuf := newLineBuffer("")
	lastFilter := ""
	sel, top := 1, 1
	hit := map[int]int{}
	lastIdx, lastTime := 0, time.Time{}

	syncFilter := func() {
		q := filterBuf.String()
		if q != lastFilter {
			vl.filter(q)
			lastFilter = q
			sel, top = 1, 1
		}
	}

	page := 1
	draw := func() {
		syncFilter()
		h, w := TermSize()
		total := vl.count()
		if sel > total {
			sel = max(1, total)
		}
		if sel < 1 {
			sel = 1
		}
		// The title rule is row 3. A subtitle moves the filter and the
		// list down one row, and the list height is what remains.
		listTop := 5
		if subtitle != "" {
			listTop = 6
		}
		listH := max(1, h-listTop-2)
		page = listH
		if sel < top {
			top = sel
		}
		if sel > top+listH-1 {
			top = sel - listH + 1
		}
		if top < 1 {
			top = 1
		}
		cv := begin(h, w)
		cv.frame(h, w)
		crumb := ""
		if len(items) >= 50 {
			crumb = fmt.Sprintf("%d/%d", total, len(items))
		}
		cv.chromeTitle(w, title, crumb)
		cv.titleRule(w)
		filterRow := 4
		if subtitle != "" {
			cv.put(4, 3, C.Muted+Truncate(subtitle, w-6)+Reset)
			filterRow = 5
		}
		filter := filterBuf.String()
		if filter != "" {
			cv.put(filterRow, 3, C.Bright+"filter: "+filterBuf.renderViewport(max(8, w-14), "")+Reset)
		} else {
			cv.put(filterRow, 3, C.Dim+"type to filter"+Reset)
		}
		hit = map[int]int{}
		if total == 0 {
			cv.put(listTop, 3, C.Muted+"no matches"+Reset)
		} else {
			for i := 0; i < listH; i++ {
				idx := top + i
				if idx > total {
					break
				}
				row := listTop + i
				it := vl.itemAt(idx - 1)
				valKey := it.Value
				if valKey == "" {
					valKey = it.Label
				}
				mark := ""
				if vl.hasSelected() {
					if vl.selectedSet[valKey] {
						mark = C.On + "☑ " + Reset
					} else {
						mark = C.Muted + "☐ " + Reset
					}
				}
				label := Truncate(it.Label, max(4, w-10))
				if idx == sel {
					cv.selBar(row, w)
					if vl.hasSelected() {
						box := "☐ "
						if vl.selectedSet[valKey] {
							box = "☑ "
						}
						cv.put(row, 3, C.SelBg+C.Accent+C.Bold+"▸ "+box+C.Bright+label+Reset)
					} else {
						cv.put(row, 3, C.SelBg+C.Accent+C.Bold+"▸ "+C.Bright+label+Reset)
					}
				} else {
					cv.put(row, 3, "  "+mark+C.Text+label+Reset)
				}
				hit[row] = idx
			}
		}
		footer := " ↑↓ move   Space mark   Enter select   Esc cancel "
		if vl.hasSelected() {
			footer = fmt.Sprintf(" Space toggle (%d)   Enter confirm   Esc cancel ", len(vl.selectedSet))
		}
		cv.chromeFooter(h, w, footer)
		cv.paint()
	}

	draw()
	sz := watchSize()
	for {
		syncFilter()
		total := vl.count()
		k := readNav(true)
		if k == nil {
			if sz.changed() {
				draw()
			}
			continue
		}
		n := k.Name
		switch {
		case n == "esc" || n == "\x03":
			if filterBuf.String() != "" && n == "esc" {
				filterBuf.setString("")
				draw()
				continue
			}
			return Choice{}, false
		case n == "up":
			if sel > 1 {
				sel--
				draw()
			}
		case n == "down":
			if sel < total {
				sel++
				draw()
			}
		case n == "pgup" || n == "pgdn":
			if n == "pgup" {
				sel = max(1, sel-page)
			} else {
				sel = min(max(1, total), sel+page)
			}
			draw()
		case (n == "home" || n == "end") && filterBuf.String() == "":
			if n == "home" {
				sel = 1
			} else {
				sel = max(1, total)
			}
			draw()
		case n == " " && filterBuf.String() == "":
			if sel >= 1 && sel <= total {
				vl.toggleAt(sel - 1)
				draw()
			}
		case k.Mouse != nil && (k.Mouse.WheelUp || k.Mouse.WheelDown):
			if k.Mouse.WheelUp && sel > 1 {
				sel--
				draw()
			} else if k.Mouse.WheelDown && sel < total {
				sel++
				draw()
			}
		case k.Mouse != nil && k.Mouse.Click:
			if idx, ok := hit[k.Mouse.Row]; ok {
				now := time.Now()
				if idx == sel && lastIdx == idx && now.Sub(lastTime) <= 450*time.Millisecond {
					if idx <= total {
						return vl.itemAt(idx - 1), true
					}
					return Choice{}, false
				}
				sel = idx
				lastIdx, lastTime = idx, now
				draw()
			}
		case n == "\r" || n == "\n":
			if vl.hasSelected() {
				vals := vl.selectedValues()
				joined := strings.Join(vals, ",")
				return Choice{Label: joined, Value: joined}, true
			}
			if sel >= 1 && sel <= total {
				return vl.itemAt(sel - 1), true
			}
		default:
			if filterBuf.handleKey(k) {
				draw()
			}
		}
	}
}

type editOpts struct {
	title, subtitle, initial, placeholder string
	mask                                  bool
	history                               []string // oldest first; ↑ and ↓ step through it
}

// historyNav steps through earlier entries with ↑ and ↓, keeping what was
// being typed as the newest position.
type historyNav struct {
	items []string
	pos   int // len(items) is the line being typed
	draft string
}

func newHistoryNav(items []string) *historyNav { return &historyNav{items: items, pos: len(items)} }

// step moves by d (-1 older, +1 newer) from current, and returns the
// text to show and whether it moved.
func (h *historyNav) step(d int, current string) (string, bool) {
	next := h.pos + d
	if next < 0 || next > len(h.items) {
		return current, false
	}
	if h.pos == len(h.items) {
		h.draft = current
	}
	h.pos = next
	if next == len(h.items) {
		return h.draft, true
	}
	return h.items[next], true
}

func editLoop(o editOpts) (string, bool) {
	buf := newLineBuffer(o.initial)
	hist := newHistoryNav(o.history)
	reveal := false
	value := func() string { return buf.String() }
	draw := func() {
		h, w := TermSize()
		cv := begin(h, w)
		cv.frame(h, w)
		cv.chromeTitle(w, o.title, "")
		cv.titleRule(w)
		if o.subtitle != "" {
			cv.put(4, 3, C.Muted+Truncate(o.subtitle, w-6)+Reset)
		}
		shown := value()
		maskChar := ""
		if o.mask && !reveal {
			maskChar = "*"
		}
		if shown == "" {
			cv.put(6, 3, C.Dim+o.placeholder+Reset)
			cv.put(6, 3, C.Accent+"▏"+Reset)
		} else {
			cv.put(6, 3, C.Bright+buf.renderViewport(max(6, w-8), maskChar)+Reset)
		}
		hint := " ←→ move   Enter confirm   Esc cancel "
		if len(o.history) > 0 {
			hint = " ↑↓ earlier   Enter confirm   Esc cancel "
		}
		if o.mask {
			word := "reveal"
			if reveal {
				word = "hide"
			}
			hint = " ←→ move   Enter confirm   Esc cancel   Tab " + word + " "
		}
		cv.chromeFooter(h, w, hint)
		cv.paint()
	}
	draw()
	sz := watchSize()
	for {
		k := readNav(true)
		if k == nil {
			if sz.changed() {
				draw()
			}
			continue
		}
		n := k.Name
		switch {
		case n == "esc":
			return "", false
		case n == "\r" || n == "\n":
			return value(), true
		case o.mask && n == "\t":
			reveal = !reveal
			draw()
		case len(o.history) > 0 && (n == "up" || n == "down"):
			d := -1
			if n == "down" {
				d = 1
			}
			if text, ok := hist.step(d, value()); ok {
				buf.setString(text)
				draw()
			}
		default:
			if buf.handleKey(k) {
				draw()
			}
		}
	}
}

// Prompt asks for a line of text, starting from initial.
func Prompt(title, subtitle, initial string) (string, bool) {
	if title == "" {
		title = "Enter value"
	}
	return editLoop(editOpts{title: title, subtitle: subtitle, initial: initial, placeholder: "(type a value)"})
}

// PromptHistory is Prompt with earlier entries (oldest first) that ↑ and
// ↓ bring back, as in a shell.
func PromptHistory(title, subtitle, initial string, history []string) (string, bool) {
	if title == "" {
		title = "Enter value"
	}
	return editLoop(editOpts{title: title, subtitle: subtitle, initial: initial, placeholder: "(type a command)", history: history})
}

// Password asks for masked text. Tab reveals it.
func Password(title, subtitle string) (string, bool) {
	if title == "" {
		title = "Password"
	}
	return editLoop(editOpts{title: title, subtitle: subtitle, mask: true, placeholder: "(type password)"})
}

// TypeToConfirm only returns true once phrase has been typed exactly.
func TypeToConfirm(title, warning, phrase string) bool {
	if title == "" {
		title = "Confirm"
	}
	buf := newLineBuffer("")
	draw := func() {
		typed := buf.String()
		h, w := TermSize()
		cv := begin(h, w)
		cv.frame(h, w)
		cv.chromeTitle(w, title, "")
		cv.titleRule(w)
		row := 4
		for _, line := range wrapLines(warning, max(10, w-6)) {
			if row >= h-6 {
				break
			}
			cv.put(row, 3, C.Text+line+Reset)
			row++
		}
		row++
		cv.put(row, 3, C.Dim+"Type "+Reset+C.Bright+phrase+Reset+C.Dim+" to confirm:"+Reset)
		row++
		match := typed == phrase
		if match {
			cv.put(row, 3, C.SelBg+C.Bright+" "+typed+" "+Reset)
			cv.chromeFooter(h, w, " Enter confirm   Esc cancel ")
		} else {
			cv.put(row, 3, C.Muted+" "+buf.renderViewport(max(6, w-10), "")+" "+Reset)
			cv.chromeFooter(h, w, " type the phrase   Esc cancel ")
		}
		cv.paint()
	}
	draw()
	sz := watchSize()
	for {
		k := readNav(true)
		if k == nil {
			if sz.changed() {
				draw()
			}
			continue
		}
		n := k.Name
		switch {
		case n == "esc":
			return false
		case n == "\r" || n == "\n":
			if buf.String() == phrase {
				return true
			}
		default:
			if buf.handleKey(k) {
				draw()
			}
		}
	}
}
