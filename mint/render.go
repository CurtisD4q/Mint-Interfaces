package mint

import (
	"os"
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// C is the active palette. It starts as the theme selected in
// ~/.config/mint-interfaces/theme.conf (the default purple if none),
// quantised to the terminal's probed colour depth when necessary.
var C = rt.quantizePalette(loadPalette())

// loadPalette picks the theme from ~/.config/mint-interfaces/theme.conf,
// then applies any colours given in the MINT_PALETTE environment variable
// (see theme.FromSpec), which is how launchers such as Emacs pass their
// own colours in.
func loadPalette() (p theme.Palette) {
	defer func() {
		if recover() != nil || !p.Complete() {
			p = theme.Load("default")
		}
	}()
	p = theme.Load("")
	if spec := os.Getenv("MINT_PALETTE"); spec != "" {
		if q, err := theme.FromSpec(p, spec); err == nil {
			p = q
		}
	}
	return p
}

// SetPalette applies a palette immediately. It is ignored, and false is
// returned, unless every role is set.
func SetPalette(p theme.Palette) bool {
	if !p.Complete() {
		return false
	}
	C = p
	return true
}

// VLen and Truncate are the width helpers the layout uses.
var (
	VLen     = util.VLen
	Truncate = util.Truncate
)

// Default footers.
var (
	HomeHints   = " ↑↓ move   ⏎ open   q quit "
	ScreenHints = " ↑↓ move   ←→/Space change   b home   q quit "
	EditHints   = " type   Enter save   Esc cancel "
)

// canvas collects one frame: a clear followed by positioned strings,
// written to the terminal in a single write.
type canvas struct {
	b strings.Builder
	w int
}

func begin(h, w int) *canvas {
	c := &canvas{w: w}
	c.b.WriteString(esc + "2J")
	return c
}

func (c *canvas) put(r, col int, s string) {
	if r < 1 || col < 1 || col > c.w {
		return
	}
	c.b.WriteString(move(r, col))
	c.b.WriteString(s)
}

// paint writes the frame and parks the (hidden) cursor at the top left.
// Terminals embedded in editors scroll their window to keep the cursor in
// view; leaving it at the bottom right made Neovim's :terminal stay
// scrolled sideways after the window shrank.
func (c *canvas) paint() { out(rt.wrapFrame(c.b.String() + esc + "H")) }

func (c *canvas) frame(h, w int) {
	if w < 4 || h < 4 {
		return
	}
	horiz := strings.Repeat("─", w-2)
	c.put(1, 1, C.Accent+"╭"+horiz+"╮"+Reset)
	for row := 2; row <= h-1; row++ {
		c.put(row, 1, C.Accent+"│"+Reset)
		c.put(row, w, C.Accent+"│"+Reset)
	}
	c.put(h, 1, C.Accent+"╰"+horiz+"╯"+Reset)
}

func (c *canvas) chromeTitle(w int, title, crumb string) {
	room := w - 6 - VLen(crumb)
	if room < 4 {
		room = 4
	}
	c.put(2, 3, C.Accent+C.Bold+Truncate(title, room)+Reset)
	if crumb != "" && w-2-VLen(crumb) > 3 {
		c.put(2, w-2-VLen(crumb), C.Muted+crumb+Reset)
	}
}

func (c *canvas) chromeFooter(h, w int, keys string) {
	width := w - 2
	if width < 0 {
		width = 0
	}
	c.put(h-1, 2, C.Dim+strings.Repeat(" ", width)+Reset)
	col := w - 1 - VLen(keys)
	if col < 2 {
		col = 2
	}
	c.put(h-1, col, C.Muted+keys+Reset)
}

func (c *canvas) scrollbar(col, top, rows, thumb, tpos int) {
	for k := 0; k < rows; k++ {
		glyph, colour := "│", C.Dim
		if k >= tpos && k < tpos+thumb {
			glyph, colour = "█", C.Accent
		}
		c.put(top+k, col, colour+glyph+Reset)
	}
}

func (c *canvas) selBar(r, w int) {
	n := w - 2
	if n < 0 {
		n = 0
	}
	c.put(r, 2, C.SelBg+strings.Repeat(" ", n)+Reset)
}

// sizeWatch notices terminal resizes on idle ticks.
type sizeWatch struct{ h, w int }

func watchSize() *sizeWatch {
	h, w := TermSize()
	return &sizeWatch{h, w}
}

// changed reports whether the size differs from the last call.
func (s *sizeWatch) changed() bool {
	sigResized := rt.consumeResize()
	h, w := TermSize()
	if sigResized || h != s.h || w != s.w {
		s.h, s.w = h, w
		return true
	}
	return false
}

var sgrSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainLen(s string) int { return VLen(sgrSeq.ReplaceAllString(s, "")) }

// wrapHead splits the first line of s at a space within width.
// It returns the line, the remaining text, and whether a space was used.
func wrapHead(s string, width int) (string, string) {
	chars := util.Chars(s)
	n := len(chars)
	if n > width {
		n = width
	}
	head := chars[:n]
	lastSpace := 0
	for i, ch := range head {
		if util.IsSpace(ch) {
			lastSpace = i + 1
		}
	}
	cut, skip := width, width
	if lastSpace > 1 {
		cut, skip = lastSpace-1, lastSpace
	}
	if cut > len(head) {
		cut = len(head)
	}
	if skip > len(chars) {
		skip = len(chars)
	}
	return util.TrimRight(strings.Join(head[:cut], "")), util.TrimLeft(strings.Join(chars[skip:], ""))
}

// wrap2 fits s into at most two lines of width cells; the second is
// truncated with "…" if it still does not fit.
func wrap2(s string, width int) [2]string {
	if s == "" {
		return [2]string{"", ""}
	}
	if VLen(s) <= width {
		return [2]string{s, ""}
	}
	l1, l2 := wrapHead(s, width)
	if VLen(l2) > width {
		l2 = Truncate(l2, width)
	}
	return [2]string{l1, l2}
}

// wrapLines word-wraps every paragraph of text to width.
func wrapLines(text string, width int) []string {
	var lines []string
	if text == "" {
		return lines
	}
	for _, para := range strings.Split(text, "\n") {
		if para == "" {
			lines = append(lines, "")
			continue
		}
		rest := para
		for rest != "" {
			if VLen(rest) <= width {
				lines = append(lines, rest)
				break
			}
			var line string
			line, rest = wrapHead(rest, width)
			lines = append(lines, line)
		}
	}
	return lines
}
