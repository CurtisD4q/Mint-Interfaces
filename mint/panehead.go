package mint

import (
	"os"
	"strconv"
	"strings"
)

// Inactive pane headings.
//
// Every pane heading is drawn in the theme's accent, so a screen of several
// panes gives no colour cue for which one has focus. A pane that does not
// have focus has its heading, and the rule just above that heading, drawn
// in a dimmer colour. The focused pane is left as it was.
//
// On a truecolor terminal the dimmer colour is the heading colour blended
// 35% toward the theme background. SGR 2 (faint) is not used: terminals
// such as Emacs term and ansi-term ignore it, which would leave the
// heading as bright as the focused one. On a 256- or 16-colour terminal
// the theme's muted colour is used instead. With NO_COLOR, or no colour
// at all, headings stay exactly as they are.
//
// Only that heading and its rule change. Options, output, tables, text,
// section headings and error rows are drawn by the same code as before.

// inactiveBlend is how far an unfocused heading moves from its own colour
// toward the theme background.
const inactiveBlend = 0.35

// headingsDim reports whether unfocused pane headings are recoloured.
func headingsDim() bool {
	if os.Getenv("NO_COLOR") != "" || rt.profile.ColorDepth == 0 {
		return false
	}
	// A monochrome palette has no colour to blend and no muted colour.
	if C.Accent == "" && C.Muted == "" && C.Text == "" {
		return false
	}
	return true
}

// inactiveChrome is the foreground of an unfocused pane heading, without
// bold. ok is false when the heading should be drawn as it always was.
func inactiveChrome(focused bool) (string, bool) {
	if focused || !headingsDim() {
		return "", false
	}
	return inactiveHeadingColour()
}

// inactiveHeadingColour is the dim foreground for an unfocused heading.
func inactiveHeadingColour() (string, bool) {
	if rt.profile.ColorDepth >= 16777216 {
		if fg, ok := sgrRGB(C.Accent); ok {
			return truecolorFG(blendRGB(fg, C.Background, inactiveBlend)), true
		}
	}
	if C.Muted == "" {
		return "", false
	}
	return C.Muted, true
}

// headingStyle is how a pane heading is drawn. A focused heading is the
// accent, bold, as before. An unfocused one is the dim colour, still bold.
func headingStyle(focused bool) string {
	if col, ok := inactiveChrome(focused); ok {
		return col + C.Bold
	}
	return C.Accent + C.Bold
}

// paneRuleStyle is how the rule above a pane heading is drawn. A focused
// pane keeps the ordinary accent rule. An unfocused pane uses the dim
// heading colour and not SGR 2, so the rule fades on terminals that ignore
// faint.
func paneRuleStyle(focused bool) string {
	if col, ok := inactiveChrome(focused); ok {
		return col
	}
	return ruleStyle()
}

// blendRGB moves each channel of fg the fraction t of the way to bg.
func blendRGB(fg, bg [3]int, t float64) [3]int {
	var out [3]int
	for i := 0; i < 3; i++ {
		v := float64(fg[i])*(1-t) + float64(bg[i])*t
		n := int(v + 0.5)
		if n < 0 {
			n = 0
		}
		if n > 255 {
			n = 255
		}
		out[i] = n
	}
	return out
}

func truecolorFG(c [3]int) string {
	return "\x1b[38;2;" + strconv.Itoa(c[0]) + ";" + strconv.Itoa(c[1]) + ";" + strconv.Itoa(c[2]) + "m"
}

// sgrRGB reads a foreground colour out of an SGR sequence: 24-bit, a
// 256-colour index, or one of the 16 ANSI colours. Anything else is not
// a colour we can blend.
func sgrRGB(seq string) ([3]int, bool) {
	if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
		return [3]int{}, false
	}
	parts := strings.Split(seq[2:len(seq)-1], ";")
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			continue
		}
		switch {
		case n == 38 || n == 48:
			if i+1 >= len(parts) {
				return [3]int{}, false
			}
			mode, err := strconv.Atoi(parts[i+1])
			if err != nil {
				return [3]int{}, false
			}
			if n != 38 {
				return [3]int{}, false
			}
			switch mode {
			case 5:
				if i+2 >= len(parts) {
					return [3]int{}, false
				}
				idx, err := strconv.Atoi(parts[i+2])
				if err != nil {
					return [3]int{}, false
				}
				return colourFromIndex(idx)
			case 2:
				if i+4 >= len(parts) {
					return [3]int{}, false
				}
				r, e1 := strconv.Atoi(parts[i+2])
				g, e2 := strconv.Atoi(parts[i+3])
				b, e3 := strconv.Atoi(parts[i+4])
				if e1 != nil || e2 != nil || e3 != nil {
					return [3]int{}, false
				}
				return [3]int{clampByte(r), clampByte(g), clampByte(b)}, true
			default:
				return [3]int{}, false
			}
		case n >= 30 && n <= 37:
			return standard16[n-30], true
		case n >= 90 && n <= 97:
			return standard16[n-90+8], true
		}
	}
	return [3]int{}, false
}

// colourFromIndex is the xterm RGB for a 256-colour index.
func colourFromIndex(n int) ([3]int, bool) {
	if n < 0 || n > 255 {
		return [3]int{}, false
	}
	if n < 16 {
		return standard16[n], true
	}
	if n < 232 {
		n -= 16
		steps := [6]int{0, 95, 135, 175, 215, 255}
		return [3]int{steps[n/36], steps[(n/6)%6], steps[n%6]}, true
	}
	v := 8 + (n-232)*10
	return [3]int{v, v, v}, true
}

func clampByte(n int) int {
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}
