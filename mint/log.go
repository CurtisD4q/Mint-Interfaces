package mint

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// LogLine is one line of output from a command. Err is true when the line
// came from standard error.
type LogLine struct {
	Text      string
	Err       bool
	Overwrite bool
}

var ansiEsc = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-Z\\-_])`)

// CleanLine strips terminal escape sequences and control characters, expands
// tabs to 8-column stops, and resolves backspace overstrikes (\b / 0x08, as
// produced by man pages like N\bNA\bAM\bME\bE or _\bX).
func CleanLine(raw string) string {
	s := ansiEsc.ReplaceAllString(raw, "")
	var out []rune
	col := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '\b':
			if len(out) > 0 && col > 0 {
				out = out[:len(out)-1]
				col--
			}
		case r == '\t':
			spaces := 8 - (col % 8)
			for j := 0; j < spaces; j++ {
				out = append(out, ' ')
			}
			col += spaces
		case r == '\r' || r == '\n':
			// dropped
		case r < 32 || r == 0x7f:
			// drop other control characters
		case r == utf8.RuneError && size == 1:
			// drop invalid UTF-8 byte
		default:
			out = append(out, r)
			col++
		}
	}
	return string(out)
}

// sliceCols returns a horizontal window of s starting at cell offset and
// spanning at most width cells. When offset > 0 and s has content to the left,
// the first cell becomes "‹". When content remains to the right, the last cell
// becomes "…".
func sliceCols(s string, offset, width int) string {
	if width <= 0 || s == "" {
		return ""
	}
	if offset <= 0 {
		return util.Truncate(s, width)
	}
	chars := util.Chars(s)
	n := len(chars)
	if offset >= n {
		if width >= 1 {
			return "‹"
		}
		return ""
	}
	if width == 1 {
		return "‹"
	}
	rem := chars[offset:]
	if len(rem) <= width-1 {
		return "‹" + strings.Join(rem, "")
	}
	if width == 2 {
		return "‹…"
	}
	return "‹" + strings.Join(rem[:width-2], "") + "…"
}

// LogOpts configures an inline Log box inside a Section.
type LogOpts struct {
	ID     string
	Label  string
	Height int
	Lines  func() []LogLine
}

// Log creates an inline log box showing the last Height lines returned by Lines.
func Log(opts LogOpts) Row {
	h := opts.Height
	if h <= 0 {
		h = 6
	}
	return Row{
		kind:      "row",
		wtype:     "log",
		id:        opts.ID,
		label:     opts.Label,
		logHeight: h,
		logLines:  opts.Lines,
	}
}

func expandLog(r Row) []Row {
	h := r.logHeight
	if h <= 0 {
		h = 6
	}
	var lines []LogLine
	if r.logLines != nil {
		func() {
			defer func() { _ = recover() }()
			lines = r.logLines()
		}()
	}
	var out []Row
	if r.label != "" {
		lbl := r.label
		if len(lines) > 0 {
			lbl = fmt.Sprintf("%s (%d lines)", r.label, len(lines))
		}
		out = append(out, Label(StyleCaption, lbl))
	}
	start := 0
	if len(lines) > h {
		start = len(lines) - h
	}
	tail := lines[start:]
	for i := 0; i < h; i++ {
		if i < len(tail) {
			out = append(out, Row{
				kind:    "row",
				wtype:   "logline",
				logLine: LogLine{Text: CleanLine(tail[i].Text), Err: tail[i].Err},
			})
		} else if i == 0 && len(tail) == 0 {
			out = append(out, Row{
				kind:     "row",
				wtype:    "logline",
				logEmpty: true,
				logLine:  LogLine{Text: "(no output)"},
			})
		} else {
			out = append(out, Row{
				kind:  "row",
				wtype: "logline",
			})
		}
	}
	return out
}

func (c *canvas) logLine(r, w int, it *Row) {
	room := max(4, w-8)
	prefix := C.Dim + "│ " + Reset
	txt := util.Truncate(it.logLine.Text, room)
	switch {
	case it.logEmpty:
		c.put(r, 3, prefix+C.Dim+txt+Reset)
	case it.logLine.Err:
		c.put(r, 3, prefix+C.Amber+txt+Reset)
	default:
		c.put(r, 3, prefix+C.Text+txt+Reset)
	}
}

// ViewLogOpts configures the full-screen ViewLog viewer.
type ViewLogOpts struct {
	Title      string
	Breadcrumb string
	Lines      func() []LogLine
	Poll       func() bool
	Status     func() []Badge
	SaveDir    string
	SaveName   string
	Follow     bool
}

// SaveLogLines writes log lines to path, creating parent directories if needed.
func SaveLogLines(path string, lines []LogLine) error {
	path = util.ExpandUser(strings.TrimSpace(path))
	if path == "" {
		return fmt.Errorf("empty file path")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := util.Mkdir(dir); err != nil {
			return err
		}
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(CleanLine(l.Text))
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ViewLog returns a full-screen log viewer Screen with vertical scrolling,
// horizontal panning (←/→/0), case-insensitive search (/, n, N), and
// save-to-file (s).
func ViewLog(title string, lines func() []LogLine) *Screen {
	return ViewLogWith(ViewLogOpts{
		Title: title,
		Lines: lines,
	})
}

// ViewLogWith returns a full-screen log viewer Screen configured by opts.
func ViewLogWith(opts ViewLogOpts) *Screen {
	title := opts.Title
	if title == "" {
		title = "Output"
	}
	scr := &Screen{
		Title:      title,
		Breadcrumb: opts.Breadcrumb,
		Status:     opts.Status,
	}
	scr.customRun = func() screenResult {
		s := scr
		return runViewLog(s, opts)
	}
	return scr
}

func runViewLog(screen *Screen, opts ViewLogOpts) screenResult {
	scroll := 0
	hscroll := 0
	follow := opts.Follow
	query := ""
	var matches []int
	matchIdx := -1
	lastCount := -1
	var sb *scrollInfo
	footSig := ""

	getLines := func() []LogLine {
		if opts.Lines == nil {
			return nil
		}
		var out []LogLine
		func() {
			defer func() { _ = recover() }()
			out = opts.Lines()
		}()
		return out
	}

	recomputeMatches := func(lines []LogLine) {
		matches = matches[:0]
		if query == "" {
			matchIdx = -1
			return
		}
		for i, l := range lines {
			if containsFold(CleanLine(l.Text), query) {
				matches = append(matches, i)
			}
		}
		if len(matches) == 0 {
			matchIdx = -1
		} else if matchIdx < 0 || matchIdx >= len(matches) {
			matchIdx = 0
		}
	}

	jumpToMatch := func(avail int) {
		if len(matches) == 0 || matchIdx < 0 || matchIdx >= len(matches) {
			return
		}
		target := matches[matchIdx]
		follow = false
		if target < scroll || target >= scroll+avail {
			scroll = max(0, target-avail/3)
		}
	}

	statusFn := func() []Badge {
		if screen.Status != nil {
			return callStatus(screen.Status)
		}
		return callStatus(screen.appStatus)
	}

	const viewHints = " ↑↓ scroll   ←→ pan   / find   n/N next   s save   b close "

	draw := func(h, w int) (int, []LogLine) {
		lines := getLines()
		total := len(lines)
		if total != lastCount {
			recomputeMatches(lines)
			lastCount = total
		}
		cv := begin(h, w)
		if w < 48 || h < 12 {
			cv.frame(h, w)
			cv.put(2, 3, C.Amber+"Terminal too small"+Reset)
			cv.put(3, 3, C.Muted+fmt.Sprintf("need 48x12, have %dx%d", w, h)+Reset)
			cv.paint()
			return 1, lines
		}
		cv.frame(h, w)

		contentTop := 4
		contentBottom := h - 2
		avail := max(1, contentBottom-contentTop+1)
		maxScroll := max(0, total-avail)
		if follow {
			scroll = maxScroll
		}
		scroll = clampInt(scroll, 0, maxScroll)

		// Build right-side breadcrumb info (lines, col offset, search matches).
		var crumbs []string
		if total == 0 {
			crumbs = append(crumbs, "0 lines")
		} else {
			end := min(total, scroll+avail)
			crumbs = append(crumbs, fmt.Sprintf("%d–%d of %d", scroll+1, end, total))
		}
		if hscroll > 0 {
			crumbs = append(crumbs, fmt.Sprintf("col +%d", hscroll))
		}
		if query != "" {
			if len(matches) == 0 {
				crumbs = append(crumbs, fmt.Sprintf("/%s (0)", query))
			} else {
				crumbs = append(crumbs, fmt.Sprintf("/%s (%d/%d)", query, matchIdx+1, len(matches)))
			}
		}
		crumb := strings.Join(crumbs, " · ")
		cv.chromeTitle(w, screen.Title, crumb)

		lineWidth := max(4, w-6)
		qLower := strings.ToLower(query)
		activeLine := -1
		if len(matches) > 0 && matchIdx >= 0 && matchIdx < len(matches) {
			activeLine = matches[matchIdx]
		}

		if total == 0 {
			cv.put(contentTop, 3, C.Dim+"(no output)"+Reset)
		} else {
			for row := 0; row < avail; row++ {
				idx := scroll + row
				if idx >= total {
					break
				}
				y := contentTop + row
				raw := CleanLine(lines[idx].Text)
				shown := sliceCols(raw, hscroll, lineWidth)
				isMatch := qLower != "" && containsFold(raw, query)
				switch {
				case idx == activeLine:
					cv.selBar(y, w)
					cv.put(y, 3, C.SelBg+C.Accent+C.Bold+shown+Reset)
				case isMatch:
					cv.put(y, 3, C.Bright+C.Bold+shown+Reset)
				case lines[idx].Err:
					cv.put(y, 3, C.Amber+shown+Reset)
				default:
					cv.put(y, 3, C.Text+shown+Reset)
				}
			}
		}

		sb = nil
		if total > avail {
			sbx := w - 1
			thumb := max(1, floorf(float64(avail*avail)/float64(total)+0.5))
			maxtop := avail - thumb
			tpos := 0
			if maxScroll > 0 {
				tpos = floorf(float64(scroll)/float64(maxScroll)*float64(maxtop) + 0.5)
			}
			sb = &scrollInfo{col: sbx, top: contentTop, rows: avail, maxoff: maxScroll, vis: avail}
			cv.scrollbar(sbx, contentTop, avail, thumb, tpos)
		}

		badges := statusFn()
		cv.footer(h, w, viewHints, badges)
		footSig = footerSig(badges)
		cv.paint()
		return avail, lines
	}

	h, w := TermSize()
	avail, lines := draw(h, w)
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
			if opts.Poll != nil {
				changed := false
				protect(func() { changed = opts.Poll() })
				if changed {
					dirty = true
				}
			}
			if cur := getLines(); len(cur) != len(lines) {
				dirty = true
			}
			if footerSig(statusFn()) != footSig {
				dirty = true
			}
		case k.Mouse != nil && (k.Mouse.WheelUp || k.Mouse.WheelDown):
			follow = false
			if k.Mouse.WheelUp {
				scroll = max(0, scroll-3)
			} else {
				scroll = min(maxScroll, scroll+3)
			}
			dirty = true
		case k.Mouse != nil && k.Mouse.Click:
			m := k.Mouse
			if sb != nil && m.Col == sb.col && m.Row >= sb.top && m.Row <= sb.top+sb.rows-1 {
				follow = false
				frac := float64(m.Row-sb.top) / float64(max(1, sb.rows-1))
				scroll = clampInt(int(math.Floor(frac*float64(sb.maxoff)+0.5)), 0, sb.maxoff)
				dirty = true
			}
		case k.Name == "up":
			follow = false
			scroll = max(0, scroll-1)
			dirty = true
		case k.Name == "down":
			follow = false
			scroll = min(maxScroll, scroll+1)
			dirty = true
		case k.Name == "pgup":
			follow = false
			scroll = max(0, scroll-max(1, avail-1))
			dirty = true
		case k.Name == "pgdn" || k.Name == " ":
			follow = false
			scroll = min(maxScroll, scroll+max(1, avail-1))
			dirty = true
		case k.Name == "home" || k.Name == "g":
			follow = false
			scroll = 0
			dirty = true
		case k.Name == "end" || k.Name == "G":
			scroll = maxScroll
			follow = opts.Follow
			dirty = true
		case k.Name == "right":
			hscroll += 8
			dirty = true
		case k.Name == "left":
			hscroll = max(0, hscroll-8)
			dirty = true
		case k.Name == "0":
			hscroll = 0
			dirty = true
		case k.Name == "/":
			if q, ok := Prompt("Search output", "Case-insensitive substring (empty clears search)", query); ok {
				query = strings.TrimSpace(q)
				recomputeMatches(lines)
				if query != "" {
					if len(matches) == 0 {
						ToastWarn("No matches for " + query)
					} else {
						matchIdx = 0
						jumpToMatch(avail)
					}
				}
			}
			dirty = true
		case k.Name == "n":
			if len(matches) > 0 {
				matchIdx = (matchIdx + 1) % len(matches)
				jumpToMatch(avail)
				dirty = true
			} else if query != "" {
				ToastWarn("No matches for " + query)
				dirty = true
			}
		case k.Name == "N":
			if len(matches) > 0 {
				matchIdx = (matchIdx - 1 + len(matches)) % len(matches)
				jumpToMatch(avail)
				dirty = true
			} else if query != "" {
				ToastWarn("No matches for " + query)
				dirty = true
			}
		case k.Name == "s":
			defDir := opts.SaveDir
			if defDir == "" {
				defDir = "~"
			}
			defName := opts.SaveName
			if defName == "" {
				defName = fmt.Sprintf("mint-output-%s.txt", time.Now().Format("20060102-150405"))
			}
			initial := filepath.Join(defDir, defName)
			if dest, ok := Prompt("Save output to file", "Directory will be created if needed", initial); ok && strings.TrimSpace(dest) != "" {
				if err := SaveLogLines(dest, lines); err != nil {
					ToastError("Save failed: " + err.Error())
				} else {
					Toast("Saved " + util.ExpandUser(dest))
				}
			}
			dirty = true
		case k.Name == "b" || k.Name == "esc":
			return screenResult{op: "back"}
		case k.Name == "q" || k.Name == "\x03":
			return screenResult{op: "quit"}
		}
		if dirty {
			avail, lines = draw(h, w)
		}
	}
}
