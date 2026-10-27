package mint

import (
	"fmt"
	"math"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// Role is how a piece of styled text is coloured. Callers pick a role; the
// palette decides the colour. Callers never pass escape sequences, so text
// from a program can't change the terminal.
type Role int

const (
	RoleText    Role = iota // ordinary text
	RoleMuted               // line numbers, separators, context
	RoleAccent              // headings
	RoleAdded               // inserted text
	RoleRemoved             // deleted text
	RoleWarn                // something to notice
	RoleError               // something wrong
	RoleMark                // an emphasised run inside a line (changed characters)
)

// Seg is a run of text in one role. Text is cleaned (CleanLineVisible)
// before it is drawn.
type Seg struct {
	Text string
	Role Role
}

// StyledLine is one line of a styled view. Anchor marks a line that n/p
// jump between (a hunk, a file, a finding).
type StyledLine struct {
	Segs   []Seg
	Anchor bool
}

// Text returns the line's cleaned text without styling.
func (l StyledLine) Text() string {
	var b strings.Builder
	for _, s := range l.Segs {
		b.WriteString(CleanLineVisible(s.Text))
	}
	return b.String()
}

// StyledKey is an extra key a styled view handles.
type StyledKey struct {
	Key  string // as reported by the terminal: "v", "w", "f2"…
	Hint string // shown in the footer: "v view"
	// Do runs when the key is pressed; returning true redraws.
	Do func() bool
}

// ViewStyledOpts configures ViewStyled.
type ViewStyledOpts struct {
	Title string
	// Lines lays out the content for a given width (the space inside the
	// frame). It is called again after a resize or a key that returns true.
	Lines func(width int) []StyledLine
	// Crumb, if set, is shown at the top right ("hunk 3 of 12 · +41 −17").
	// anchor is the index of the anchor at or above the top line, or -1.
	Crumb  func(anchor, anchors int) string
	Keys   []StyledKey
	Status func() []Badge
}

// ViewStyled returns a full-screen, scrollable view of styled lines, with
// n/p (or ]/[) moving between anchors.
func ViewStyled(opts ViewStyledOpts) *Screen {
	scr := &Screen{Title: opts.Title, Status: opts.Status}
	scr.customRun = func() screenResult { return runViewStyled(scr, opts) }
	return scr
}

// roleStyle maps a role to the current palette.
func roleStyle(r Role) string {
	switch r {
	case RoleMuted:
		return C.Muted
	case RoleAccent:
		return C.Accent + C.Bold
	case RoleAdded:
		return C.On
	case RoleRemoved:
		if C.Err != "" {
			return C.Err
		}
		return C.Amber
	case RoleWarn:
		return C.Amber
	case RoleError:
		if C.Err != "" {
			return C.Err + C.Bold
		}
		return C.Amber + C.Bold
	case RoleMark:
		return C.SelBg + C.Bright + C.Bold
	}
	return C.Text
}

// renderStyled draws l in at most width cells, cleaning each segment.
func renderStyled(l StyledLine, width int) string {
	var b strings.Builder
	left := width
	for _, s := range l.Segs {
		if left <= 0 {
			break
		}
		t := CleanLineVisible(s.Text)
		if t == "" {
			continue
		}
		if VLen(t) > left {
			t = util.Truncate(t, left)
		}
		left -= VLen(t)
		b.WriteString(roleStyle(s.Role) + t + Reset)
	}
	return b.String()
}

func anchorsOf(lines []StyledLine) []int {
	var out []int
	for i, l := range lines {
		if l.Anchor {
			out = append(out, i)
		}
	}
	return out
}

// anchorAt returns the index of the last anchor at or before line, or -1.
func anchorAt(anchors []int, line int) int {
	at := -1
	for i, a := range anchors {
		if a <= line {
			at = i
		}
	}
	return at
}

func runViewStyled(screen *Screen, opts ViewStyledOpts) screenResult {
	scroll := 0
	var lines []StyledLine
	var anchors []int
	layoutW := -1
	var sb *scrollInfo
	footSig := ""

	statusFn := func() []Badge {
		if screen.Status != nil {
			return callStatus(screen.Status)
		}
		return callStatus(screen.appStatus)
	}
	hints := " ↑↓ scroll   n/p next/prev "
	for _, k := range opts.Keys {
		hints += "  " + k.Hint + " "
	}
	hints += "  b close "

	layout := func(w int) {
		lines = nil
		if opts.Lines != nil {
			protect(func() { lines = opts.Lines(w) })
		}
		anchors = anchorsOf(lines)
		layoutW = w
	}

	draw := func(h, w int) int {
		cv := begin(h, w)
		if w < 48 || h < 12 {
			cv.frame(h, w)
			cv.put(2, 3, C.Amber+"Terminal too small"+Reset)
			cv.put(3, 3, C.Muted+fmt.Sprintf("need 48x12, have %dx%d", w, h)+Reset)
			cv.paint()
			return 1
		}
		inner := w - 5 // frame, margins and scrollbar
		if inner != layoutW {
			layout(inner)
		}
		cv.frame(h, w)
		top, bottom := 4, h-2
		avail := max(1, bottom-top+1)
		maxScroll := max(0, len(lines)-avail)
		scroll = clampInt(scroll, 0, maxScroll)

		crumb := ""
		if opts.Crumb != nil {
			protect(func() { crumb = opts.Crumb(anchorAt(anchors, scroll), len(anchors)) })
		} else if len(lines) > 0 {
			crumb = fmt.Sprintf("%d–%d of %d", scroll+1, min(len(lines), scroll+avail), len(lines))
		}
		cv.chromeTitle(w, screen.Title, crumb)

		if len(lines) == 0 {
			cv.put(top, 3, C.Dim+"(nothing to show)"+Reset)
		}
		for row := 0; row < avail && scroll+row < len(lines); row++ {
			cv.put(top+row, 3, renderStyled(lines[scroll+row], inner))
		}
		sb = nil
		if len(lines) > avail {
			thumb := max(1, floorf(float64(avail*avail)/float64(len(lines))+0.5))
			tpos := 0
			if maxScroll > 0 {
				tpos = floorf(float64(scroll)/float64(maxScroll)*float64(avail-thumb) + 0.5)
			}
			sb = &scrollInfo{col: w - 1, top: top, rows: avail, maxoff: maxScroll, vis: avail}
			cv.scrollbar(w-1, top, avail, thumb, tpos)
		}
		badges := statusFn()
		cv.footer(h, w, hints, badges)
		footSig = footerSig(badges)
		cv.paint()
		return avail
	}

	h, w := TermSize()
	avail := draw(h, w)
	sc := 0
	for {
		k := readNav(false)
		dirty := false
		maxScroll := max(0, len(lines)-avail)
		switch {
		case k == nil:
			sc++
			if rt.consumeResize() || sc >= 2 {
				sc = 0
				if nh, nw := TermSize(); nh != h || nw != w {
					h, w = nh, nw
					dirty = true
				}
			}
			if footerSig(statusFn()) != footSig {
				dirty = true
			}
		case k.Mouse != nil && (k.Mouse.WheelUp || k.Mouse.WheelDown):
			if k.Mouse.WheelUp {
				scroll = max(0, scroll-3)
			} else {
				scroll = min(maxScroll, scroll+3)
			}
			dirty = true
		case k.Mouse != nil && k.Mouse.Click:
			m := k.Mouse
			if sb != nil && m.Col == sb.col && m.Row >= sb.top && m.Row <= sb.top+sb.rows-1 {
				frac := float64(m.Row-sb.top) / float64(max(1, sb.rows-1))
				scroll = clampInt(int(math.Floor(frac*float64(sb.maxoff)+0.5)), 0, sb.maxoff)
				dirty = true
			}
		case k.Name == "up":
			scroll, dirty = max(0, scroll-1), true
		case k.Name == "down":
			scroll, dirty = min(maxScroll, scroll+1), true
		case k.Name == "pgup":
			scroll, dirty = max(0, scroll-max(1, avail-1)), true
		case k.Name == "pgdn" || k.Name == " ":
			scroll, dirty = min(maxScroll, scroll+max(1, avail-1)), true
		case k.Name == "home" || k.Name == "g":
			scroll, dirty = 0, true
		case k.Name == "end" || k.Name == "G":
			scroll, dirty = maxScroll, true
		case k.Name == "n" || k.Name == "]":
			for _, a := range anchors {
				if a > scroll {
					scroll, dirty = min(a, maxScroll), true
					break
				}
			}
		case k.Name == "p" || k.Name == "[":
			for i := len(anchors) - 1; i >= 0; i-- {
				if anchors[i] < scroll {
					scroll, dirty = anchors[i], true
					break
				}
			}
		case k.Name == "b" || k.Name == "esc":
			return screenResult{op: "back"}
		case k.Name == "q" || k.Name == "\x03":
			return screenResult{op: "quit"}
		default:
			for _, sk := range opts.Keys {
				if k.Name == sk.Key && sk.Do != nil {
					redo := false
					protect(func() { redo = sk.Do() })
					if redo {
						layoutW = -1 // lay out again
						dirty = true
					}
					break
				}
			}
		}
		if dirty {
			avail = draw(h, w)
		}
	}
}
