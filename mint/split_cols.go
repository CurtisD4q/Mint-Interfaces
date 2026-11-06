package mint

import (
	"strings"
	"unicode/utf8"
)

// Side-by-side panes.
//
// A pane with Beside set is drawn to the right of the pane before it, in
// the same band of rows. The panes in one band form a group: they share the
// band's rows and its place in the stack, and split its width between them
// (by Pane.Width, or equally). Tab still cycles through every pane in
// order, and the stacked panes below and above are laid out as before.
//
// A group falls back to stacked panes when the terminal is too narrow for
// it: every member then becomes a pane of its own, exactly as if Beside had
// not been set. Nothing else about a screen changes.

// splitMinPaneWidth is the narrowest a pane may be drawn beside another.
const splitMinPaneWidth = 30

// colSpan is where one pane of a group sits: its first column and width.
type colSpan struct{ index, x0, w int }

// paneBeside reports whether the pane at index asks to sit beside the one
// before it.
func (s *Screen) paneBeside(index int) bool {
	if index < 1 {
		return false
	}
	p := s.paneAt(index)
	return p != nil && p.Beside
}

func (s *Screen) paneShare(index int) float64 {
	if index < 1 {
		return s.TopWidth
	}
	if p := s.paneAt(index); p != nil {
		return p.Width
	}
	return 0
}

// groups lists the panes band by band. Each group is the panes that share a
// band, in order, with the columns they occupy. A group of one has the full
// width. Before the screen has been measured everything is stacked.
func (s *Screen) groups() [][]colSpan {
	n := s.paneCount()
	width := s.viewWidth
	var raw [][]int
	for i := 0; i < n; i++ {
		if i > 0 && width > 0 && s.paneBeside(i) {
			raw[len(raw)-1] = append(raw[len(raw)-1], i)
			continue
		}
		raw = append(raw, []int{i})
	}
	out := make([][]colSpan, 0, len(raw))
	for _, g := range raw {
		if len(g) == 1 {
			out = append(out, []colSpan{{g[0], 1, width}})
			continue
		}
		if len(g)*splitMinPaneWidth > width {
			// Too narrow: each member goes back to a band of its own.
			for _, i := range g {
				out = append(out, []colSpan{{i, 1, width}})
			}
			continue
		}
		out = append(out, spansFor(s, g, width))
	}
	return out
}

// spansFor shares width between the panes of one group by their Width, or
// equally when none is set. Every pane keeps at least splitMinPaneWidth.
func spansFor(s *Screen, g []int, width int) []colSpan {
	shares := make([]float64, len(g))
	sum := 0.0
	for k, i := range g {
		sh := s.paneShare(i)
		if sh <= 0 {
			sh = 1
		}
		shares[k] = sh
		sum += sh
	}
	ws := make([]int, len(g))
	used := 0
	for k := range g {
		ws[k] = int(float64(width) * shares[k] / sum)
		if ws[k] < splitMinPaneWidth {
			ws[k] = splitMinPaneWidth
		}
		used += ws[k]
	}
	// Settle the difference on the widest pane, so the sum is the width.
	for used != width {
		widest := 0
		for k := range ws {
			if ws[k] > ws[widest] {
				widest = k
			}
		}
		step := width - used
		if ws[widest]+step < splitMinPaneWidth {
			break
		}
		ws[widest] += step
		used += step
	}
	spans := make([]colSpan, len(g))
	x := 1
	for k, i := range g {
		spans[k] = colSpan{i, x, ws[k]}
		x += ws[k]
	}
	return spans
}

// groupOf returns the group (and its leader) a pane belongs to.
func (s *Screen) groupOf(index int) []colSpan {
	for _, g := range s.groups() {
		for _, c := range g {
			if c.index == index {
				return g
			}
		}
	}
	return nil
}

// leaderOf is the first pane of the group index is in; index itself when it
// is not stacked beside anything. A focus past the panes (the dock) is its
// own.
func (s *Screen) leaderOf(index int) int {
	if index >= s.paneCount() {
		return index
	}
	if g := s.groupOf(index); len(g) > 0 {
		return g[0].index
	}
	return index
}

// inSplitGroup reports whether index shares its band with another pane.
func (s *Screen) inSplitGroup(index int) bool { return len(s.groupOf(index)) > 1 }

// paneSpan is where a pane is drawn: first column and width.
func (s *Screen) paneSpan(index int) (x0, w int) {
	for _, c := range s.groupOf(index) {
		if c.index == index {
			return c.x0, c.w
		}
	}
	return 1, s.viewWidth
}

// bandHidden reports whether a whole band is hidden: for a group, all of
// its panes.
func (s *Screen) bandHidden(lead int) bool {
	g := s.groupOf(lead)
	if len(g) < 2 {
		return s.paneHidden(lead)
	}
	for _, c := range g {
		if !s.paneHidden(c.index) {
			return false
		}
	}
	return true
}

// bandIndexAt is the pane under a mouse position: a band's pane, picked by
// column when the band is split.
func (s *Screen) bandIndexAt(row, col int) (int, bool) {
	for _, b := range s.bands {
		in := (b.divRow > 0 && row == b.divRow) || (b.rows > 0 && row >= b.top && row < b.top+b.rows)
		if !in {
			continue
		}
		for _, c := range b.cols {
			if col >= c.x0 && col < c.x0+c.w {
				return c.index, true
			}
		}
		return b.index, true
	}
	return 0, false
}

// view is a window onto a canvas: column 1 of the view is column x0 of the
// parent, and anything drawn past the view's width is cut off. It lets a
// pane be painted as if it had the whole screen to itself.
func (c *canvas) view(x0, w int) *canvas {
	return &canvas{w: w, parent: c, dx: x0 - 1}
}

// clipCells cuts s to n visible cells, keeping its escape sequences, and
// resets the style if it cut anything.
func clipCells(s string, n int) string {
	if n < 1 {
		return ""
	}
	if VLen(s) <= n {
		return s
	}
	var b strings.Builder
	used := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) {
			j := i + 2
			switch s[i+1] {
			case '[': // CSI: parameters, then a final byte 0x40-0x7e
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++
				}
			case ']': // OSC: up to BEL or ESC \
				for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
					j++
				}
				if j < len(s) {
					if s[j] == 0x07 {
						j++
					} else {
						j += 2
					}
				}
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		wd := VLen(s[i : i+size])
		if used+wd > n {
			break
		}
		b.WriteString(s[i : i+size])
		used += wd
		i += size
		_ = r
	}
	return b.String() + Reset
}
