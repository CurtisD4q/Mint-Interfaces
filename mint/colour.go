package mint

// Program colours, kept safely.
//
// Output from a program is untrusted, so CleanLine removes every escape
// sequence. CleanLineColours removes them too, but first reads the ones
// that only set colours (SGR: ESC [ … m) and records them as runs over the
// cleaned text. When the line is drawn, Mint writes its own codes for
// those runs from a fixed set: the 16 standard colours, foreground and
// background, and bold. Nothing the program sent reaches the terminal.
//
// 256-colour and 24-bit colours are mapped to the nearest of the 16, and
// everything else (underline, blink, reverse, conceal, cursor movement,
// titles…) is dropped. A background the same as its foreground is dropped
// too, so coloured text can never be made invisible.

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// ColourRun colours Text[Start:End] (rune positions in the cleaned text).
// FG and BG are 1–16 for the standard colours 0–15, or 0 for none.
type ColourRun struct {
	Start, End int
	FG, BG     uint8
	Bold       bool
}

var sgrEsc = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// colourState is the colour in force while reading a line.
type colourState struct {
	fg, bg uint8
	bold   bool
}

func (s colourState) plain() bool { return s.fg == 0 && s.bg == 0 && !s.bold }

// CleanLineColours cleans raw like CleanLine and returns the colours it
// set as runs over the cleaned text. Runs is nil when the line has none.
func CleanLineColours(raw string) (string, []ColourRun) {
	locs := sgrEsc.FindAllStringSubmatchIndex(raw, -1)
	if len(locs) == 0 {
		return CleanLine(raw), nil
	}
	var b strings.Builder
	var runs []ColourRun
	var st colourState
	col, pos, at := 0, 0, 0 // column for tab stops, rune position in the output, byte offset in raw
	emit := func(piece string) {
		if piece == "" {
			return
		}
		text, next := cleanLineFrom(piece, false, col)
		col = next
		n := utf8.RuneCountInString(text)
		if n == 0 {
			return
		}
		if !st.plain() {
			fg, bg := st.fg, st.bg
			if bg != 0 && bg == fg {
				bg = 0 // never invisible
			}
			r := ColourRun{Start: pos, End: pos + n, FG: fg, BG: bg, Bold: st.bold}
			if k := len(runs) - 1; k >= 0 && runs[k].End == r.Start && runs[k].FG == r.FG && runs[k].BG == r.BG && runs[k].Bold == r.Bold {
				runs[k].End = r.End
			} else {
				runs = append(runs, r)
			}
		}
		b.WriteString(text)
		pos += n
	}
	for _, loc := range locs {
		emit(raw[at:loc[0]])
		st = applySGR(st, raw[loc[2]:loc[3]])
		at = loc[1]
	}
	emit(raw[at:])
	if len(runs) == 0 {
		runs = nil
	}
	return b.String(), runs
}

// applySGR applies one SGR parameter list ("1;31", "38;5;208", "") to st.
func applySGR(st colourState, params string) colourState {
	if params == "" {
		return colourState{}
	}
	ps := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	num := func(i int) (int, bool) {
		if i >= len(ps) {
			return 0, false
		}
		n, err := strconv.Atoi(ps[i])
		return n, err == nil && n >= 0 && n <= 255
	}
	for i := 0; i < len(ps); i++ {
		n, err := strconv.Atoi(ps[i])
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			st = colourState{}
		case n == 1:
			st.bold = true
		case n == 22:
			st.bold = false
		case n >= 30 && n <= 37:
			st.fg = uint8(n-30) + 1
		case n >= 90 && n <= 97:
			st.fg = uint8(n-90) + 9
		case n == 39:
			st.fg = 0
		case n >= 40 && n <= 47:
			st.bg = uint8(n-40) + 1
		case n >= 100 && n <= 107:
			st.bg = uint8(n-100) + 9
		case n == 49:
			st.bg = 0
		case n == 38 || n == 48:
			// 38;5;N or 38;2;R;G;B (and the same for 48).
			var c uint8
			mode, _ := num(i + 1)
			switch mode {
			case 5:
				if v, ok := num(i + 2); ok {
					c = nearest16From256(v) + 1
				}
				i += 2
			case 2:
				r, ok1 := num(i + 2)
				g, ok2 := num(i + 3)
				bl, ok3 := num(i + 4)
				if ok1 && ok2 && ok3 {
					c = nearest16(r, g, bl) + 1
				}
				i += 4
			default:
				i++
			}
			if n == 38 {
				st.fg = c
			} else {
				st.bg = c
			}
		}
	}
	return st
}

// The 16 standard colours, as xterm draws them, for finding the nearest.
var standard16 = [16][3]int{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

// nearest16 is the standard colour nearest to r, g, b (0–15).
func nearest16(r, g, b int) uint8 {
	best, bestD := 0, 1<<30
	for i, c := range standard16 {
		dr, dg, db := r-c[0], g-c[1], b-c[2]
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = i, d
		}
	}
	return uint8(best)
}

// nearest16From256 maps a 256-colour index to the nearest standard colour.
func nearest16From256(n int) uint8 {
	switch {
	case n < 16:
		return uint8(n)
	case n < 232:
		n -= 16
		steps := [6]int{0, 95, 135, 175, 215, 255}
		return nearest16(steps[n/36], steps[(n/6)%6], steps[n%6])
	default:
		v := 8 + (n-232)*10
		return nearest16(v, v, v)
	}
}

// coloursOn reports whether program colours are drawn: not when NO_COLOR
// is set or the theme has no colours.
func coloursOn() bool {
	return os.Getenv("NO_COLOR") == "" && C.Text != ""
}

// sgrFor is Mint's own code for a run's colours.
func sgrFor(r ColourRun) string {
	var codes []string
	if r.Bold {
		codes = append(codes, "1")
	}
	if r.FG > 0 {
		if c := int(r.FG) - 1; c < 8 {
			codes = append(codes, strconv.Itoa(30+c))
		} else {
			codes = append(codes, strconv.Itoa(90+c-8))
		}
	}
	if r.BG > 0 {
		if c := int(r.BG) - 1; c < 8 {
			codes = append(codes, strconv.Itoa(40+c))
		} else {
			codes = append(codes, strconv.Itoa(100+c-8))
		}
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// renderColoured draws text (already cleaned) in at most room cells, with
// its colour runs over the base style.
func renderColoured(text string, runs []ColourRun, room int, base string) string {
	if room <= 0 {
		return ""
	}
	if util.VLen(text) > room {
		text = util.Truncate(text, room)
	}
	var b strings.Builder
	b.WriteString(base)
	ri := 0
	in := false
	pos := 0
	for _, r := range text {
		for ri < len(runs) && runs[ri].End <= pos {
			ri++
		}
		want := ri < len(runs) && runs[ri].Start <= pos
		switch {
		case want && (!in || runs[ri].Start == pos):
			b.WriteString(Reset + base + sgrFor(runs[ri]))
			in = true
		case !want && in:
			b.WriteString(Reset + base)
			in = false
		}
		b.WriteRune(r)
		pos++
	}
	b.WriteString(Reset)
	return b.String()
}

// cleanedLogLine is l with its text cleaned. Its colours are kept only
// when cleaning left the text as it was: they are positions in that text.
func cleanedLogLine(l LogLine) LogLine {
	text := CleanLine(l.Text)
	if text != l.Text {
		l.Colours = nil
	}
	l.Text = text
	return l
}
