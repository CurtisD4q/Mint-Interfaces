package mint

import (
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

// asciiLower lowercases A–Z only, as Lua's string.lower does.
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

// ChooseItem offers a filterable list. Typing filters by label.
func ChooseItem(title string, items []Choice, subtitle string) (Choice, bool) {
	if title == "" {
		title = "Select"
	}
	filter := ""
	sel, top := 1, 1
	hit := map[int]int{}
	lastIdx, lastTime := 0, time.Time{}

	filtered := func() []Choice {
		if filter == "" {
			return items
		}
		needle := asciiLower(filter)
		var out []Choice
		for _, it := range items {
			if strings.Contains(asciiLower(it.Label), needle) {
				out = append(out, it)
			}
		}
		return out
	}

	page := 1
	draw := func() {
		h, w := TermSize()
		view := filtered()
		if sel > len(view) {
			sel = max(1, len(view))
		}
		if sel < 1 {
			sel = 1
		}
		const listTop = 5
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
		cv.chromeTitle(w, title, "")
		if subtitle != "" {
			cv.put(3, 3, C.Muted+Truncate(subtitle, w-6)+Reset)
		}
		if filter != "" {
			cv.put(4, 3, C.Bright+"filter: "+filter+Reset)
		} else {
			cv.put(4, 3, C.Dim+"type to filter"+Reset)
		}
		hit = map[int]int{}
		if len(view) == 0 {
			cv.put(listTop, 3, C.Muted+"no matches"+Reset)
		} else {
			for i := 0; i < listH; i++ {
				idx := top + i
				if idx > len(view) {
					break
				}
				row := listTop + i
				label := Truncate(view[idx-1].Label, w-8)
				if idx == sel {
					cv.selBar(row, w)
					cv.put(row, 3, C.SelBg+C.Accent+C.Bold+"▸ "+C.Bright+label+Reset)
				} else {
					cv.put(row, 3, C.Text+"  "+label+Reset)
				}
				hit[row] = idx
			}
		}
		cv.chromeFooter(h, w, " ↑↓ move   Enter select   Esc cancel ")
		cv.paint()
	}

	draw()
	sz := watchSize()
	for {
		view := filtered()
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
			return Choice{}, false
		case n == "up":
			if sel > 1 {
				sel--
				draw()
			}
		case n == "down":
			if sel < len(view) {
				sel++
				draw()
			}
		case n == "home" || n == "end" || n == "pgup" || n == "pgdn":
			switch n {
			case "home":
				sel = 1
			case "end":
				sel = len(view)
			case "pgup":
				sel = max(1, sel-page)
			case "pgdn":
				sel = min(len(view), sel+page)
			}
			draw()
		case k.Mouse != nil && (k.Mouse.WheelUp || k.Mouse.WheelDown):
			if k.Mouse.WheelUp && sel > 1 {
				sel--
				draw()
			} else if k.Mouse.WheelDown && sel < len(view) {
				sel++
				draw()
			}
		case k.Mouse != nil && k.Mouse.Click:
			if idx, ok := hit[k.Mouse.Row]; ok {
				now := time.Now()
				if idx == sel && lastIdx == idx && now.Sub(lastTime) <= 450*time.Millisecond {
					if idx <= len(view) {
						return view[idx-1], true
					}
					return Choice{}, false
				}
				sel = idx
				lastIdx, lastTime = idx, now
				draw()
			}
		case n == "\r" || n == "\n":
			if sel >= 1 && sel <= len(view) {
				return view[sel-1], true
			}
		case n == "\x7f" || n == "\b":
			if filter != "" {
				filter = dropLastChar(filter)
				sel, top = 1, 1
				draw()
			}
		case k.IsText():
			filter += n
			sel, top = 1, 1
			draw()
		}
	}
}

type editOpts struct {
	title, subtitle, initial, placeholder string
	mask                                  bool
}

func editLoop(o editOpts) (string, bool) {
	chars := util.Chars(o.initial)
	reveal := false
	value := func() string { return strings.Join(chars, "") }
	draw := func() {
		h, w := TermSize()
		cv := begin(h, w)
		cv.frame(h, w)
		cv.chromeTitle(w, o.title, "")
		if o.subtitle != "" {
			cv.put(4, 3, C.Muted+Truncate(o.subtitle, w-6)+Reset)
		}
		shown := value()
		if o.mask && !reveal {
			shown = strings.Repeat("*", VLen(shown))
		}
		if shown == "" {
			cv.put(6, 3, C.Dim+o.placeholder+Reset)
			cv.put(6, 3, C.Accent+"_"+Reset)
		} else {
			vis := Truncate(shown, w-8)
			cv.put(6, 3, C.Bright+vis+Reset)
			cv.put(6, 3+VLen(vis), C.Accent+"_"+Reset)
		}
		hint := " Enter confirm   Esc cancel   Backspace delete "
		if o.mask {
			word := "reveal"
			if reveal {
				word = "hide"
			}
			hint = " Enter confirm   Esc cancel   Tab " + word + " "
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
		case n == "\x7f" || n == "\b":
			if len(chars) > 0 {
				chars = chars[:len(chars)-1]
				draw()
			}
		case o.mask && n == "\t":
			reveal = !reveal
			draw()
		case k.IsText():
			chars = append(chars, n)
			draw()
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
	typed := ""
	draw := func() {
		h, w := TermSize()
		cv := begin(h, w)
		cv.frame(h, w)
		cv.chromeTitle(w, title, "")
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
			cv.put(row, 3, C.Muted+" "+typed+"_ "+Reset)
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
			if typed == phrase {
				return true
			}
		case n == "\x7f" || n == "\b":
			typed = dropLastChar(typed)
			draw()
		case k.IsText():
			typed += n
			draw()
		}
	}
}
