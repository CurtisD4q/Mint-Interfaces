package mint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

func TestBlendIsThirtyFivePercentTowardBackground(t *testing.T) {
	if inactiveBlend != 0.35 {
		t.Fatalf("blend ratio %v, want 0.35", inactiveBlend)
	}
	// 200, 100 and 0 moved 35% toward 0, 0 and 50.
	got := blendRGB([3]int{200, 100, 0}, [3]int{0, 0, 50}, 0.35)
	if got != [3]int{130, 65, 18} {
		t.Fatalf("blend = %v, want [130 65 18]", got)
	}
	// Nord accent toward nord0.
	nord := blendRGB([3]int{136, 192, 208}, [3]int{46, 52, 64}, 0.35)
	if nord != [3]int{105, 143, 158} {
		t.Fatalf("nord blend = %v, want [105 143 158]", nord)
	}
}

func TestSGRRGBReadsTruecolorIndexAndANSI(t *testing.T) {
	fg, ok := sgrRGB("\x1b[38;2;136;192;208m")
	if !ok || fg != [3]int{136, 192, 208} {
		t.Fatalf("truecolor %v ok=%v", fg, ok)
	}
	fg, ok = sgrRGB("\x1b[38;5;141m")
	if !ok || fg != [3]int{175, 135, 255} {
		t.Fatalf("index 141 %v ok=%v", fg, ok)
	}
	fg, ok = sgrRGB("\x1b[95m")
	if !ok || fg != [3]int{255, 0, 255} {
		t.Fatalf("ansi 95 %v ok=%v", fg, ok)
	}
	if _, ok := sgrRGB("\x1b[1m"); ok {
		t.Fatal("bold parsed as a colour")
	}
}

func TestInactiveHeadingColourByDepth(t *testing.T) {
	// Truecolor: the accent blended toward the theme background, never faint.
	useHeadingPalette(t, 16777216, "nord")
	col, ok := inactiveHeadingColour()
	if !ok {
		t.Fatal("truecolor produced no dim colour")
	}
	want := truecolorFG(blendRGB([3]int{136, 192, 208}, [3]int{46, 52, 64}, 0.35))
	if col != want {
		t.Fatalf("truecolor dim %q, want %q", col, want)
	}
	if strings.Contains(col, "\x1b[2m") || strings.Contains(headingStyle(false), "\x1b[2m") {
		t.Fatalf("dim heading relies on faint: %q", headingStyle(false))
	}
	if headingStyle(true) != C.Accent+C.Bold {
		t.Fatalf("focused heading changed: %q", headingStyle(true))
	}
	if C.Background != [3]int{46, 52, 64} {
		t.Fatalf("nord background %v", C.Background)
	}

	// 256 and 16: the theme's muted colour, not a truecolor blend.
	for _, depth := range []int{256, 16} {
		useHeadingPalette(t, depth, "default")
		col, ok = inactiveHeadingColour()
		if !ok || col != C.Muted {
			t.Fatalf("depth %d dim %q, want muted %q", depth, col, C.Muted)
		}
		if strings.Contains(col, "38;2;") || strings.Contains(col, "\x1b[2m") {
			t.Fatalf("depth %d dim is not the muted colour: %q", depth, col)
		}
		if C.Muted == C.Accent {
			t.Fatalf("depth %d muted and accent collapsed to %q", depth, C.Muted)
		}
	}

	// NO_COLOR, and a terminal with colour disabled, leave the heading alone
	// even when a colour palette is still loaded.
	useHeadingPalette(t, 16777216, "nord")
	t.Setenv("NO_COLOR", "1")
	if headingsDim() {
		t.Fatal("NO_COLOR still dims")
	}
	if headingStyle(false) != C.Accent+C.Bold {
		t.Fatalf("NO_COLOR heading %q, want the ordinary accent", headingStyle(false))
	}
	t.Setenv("NO_COLOR", "")
	rt.profile.ColorDepth = 0
	if headingsDim() || headingStyle(false) != headingStyle(true) {
		t.Fatal("colour depth 0 changed the heading")
	}
}

func TestHeadingFramesFollowColourDepth(t *testing.T) {
	const h, w = 28, 80
	for _, depth := range []int{256, 16} {
		useHeadingPalette(t, depth, "default")
		s := stackedHeadingScreen()
		frame := s.renderFrame(h, w)
		assertHeadingStyle(t, frame, "Options", true, C.Accent+C.Bold)
		assertHeadingStyle(t, frame, "Output", false, C.Muted+C.Bold)
		if strings.Contains(styleImmediatelyBefore(frame, "┄ Output"), "38;2;") ||
			strings.Contains(styleImmediatelyBefore(frame, "┄ Output"), "\x1b[2m") {
			t.Fatalf("depth %d heading is not muted: %q", depth, styleImmediatelyBefore(frame, "┄ Output"))
		}
		assertImmediateStyle(t, frame, "err-row", C.Err, C.Muted)
		assertImmediateStyle(t, frame, "out-line", C.Dim, C.Muted)
		assertImmediateStyle(t, frame, "tbl-cell", C.Text, C.Muted)
		out, ok := bandByTitle(s, "Output")
		if !ok {
			t.Fatal("no output pane")
		}
		if got := styleAt(frame, out.divRow-2, 2); got != C.Muted {
			t.Fatalf("depth %d rule %q, want muted %q", depth, got, C.Muted)
		}
	}

	useHeadingPalette(t, 16777216, "nord")
	t.Setenv("NO_COLOR", "1")
	plain := stackedHeadingScreen().renderFrame(h, w)
	opt := styleImmediatelyBefore(plain, "▸ Options")
	outStyle := styleImmediatelyBefore(plain, "┄ Output")
	if opt != outStyle || opt != C.Accent+C.Bold {
		t.Fatalf("NO_COLOR headings differ: focused %q unfocused %q", opt, outStyle)
	}
	dim := truecolorFG(blendRGB([3]int{136, 192, 208}, [3]int{46, 52, 64}, inactiveBlend))
	if strings.Contains(outStyle, dim) {
		t.Fatalf("NO_COLOR heading was blended: %q", outStyle)
	}

	// Colour disabled: the mono palette, no new colour on either heading.
	t.Setenv("NO_COLOR", "")
	rt.profile.ColorDepth = 0
	C = monoPalette()
	mono := stackedHeadingScreen().renderFrame(h, w)
	opt = styleImmediatelyBefore(mono, "▸ Options")
	outStyle = styleImmediatelyBefore(mono, "┄ Output")
	if opt != outStyle || strings.Contains(opt, "38;") {
		t.Fatalf("mono headings %q and %q", opt, outStyle)
	}
}

func TestStackedHeadingsDimOnlyWhenUnfocused(t *testing.T) {
	useHeadingPalette(t, 16777216, "nord")
	s := stackedHeadingScreen()
	const h, w = 32, 80

	frame := s.renderFrame(h, w)
	dim := headingStyle(false)
	bright := headingStyle(true)
	assertHeadingStyle(t, frame, "Options", true, bright)
	assertHeadingStyle(t, frame, "Output", false, dim)
	assertStackedChrome(t, s, frame, h, w, false)

	s.focus = 1
	frame = s.renderFrame(h, w)
	assertHeadingStyle(t, frame, "Options", false, dim)
	assertHeadingStyle(t, frame, "Output", true, bright)
	assertStackedChrome(t, s, frame, h, w, true)
}

// assertStackedChrome checks the rule above the output pane, the screen's
// own title rule, and that nothing inside either pane picked up the dim colour.
func assertStackedChrome(t *testing.T, s *Screen, frame string, h, w int, outputFocused bool) {
	t.Helper()
	dim := inactiveColour(t)
	if styleAt(frame, 3, 2) != ruleStyle() {
		t.Fatalf("title rule changed: %q", styleAt(frame, 3, 2))
	}
	out, ok := bandByTitle(s, "Output")
	if !ok || out.divRow < 3 {
		t.Fatalf("output band %+v", s.bands)
	}
	grid := frameGrid(frame, h, w)
	if !gapIsRuleThenBlank(grid, out.divRow) {
		t.Fatalf("gap above Output is not a rule then a blank\n%s", strings.Join(grid, "\n"))
	}
	rule := styleAt(frame, out.divRow-2, 2)
	wantRule := dim
	if outputFocused {
		wantRule = ruleStyle()
	}
	if rule != wantRule {
		t.Fatalf("output rule %q, want %q", rule, wantRule)
	}
	if strings.Contains(rule, "\x1b[2m") && !outputFocused {
		t.Fatalf("unfocused rule uses faint: %q", rule)
	}

	// Section headings stay at full accent in both panes. The dim colour
	// belongs to the pane heading only.
	assertImmediateStyle(t, frame, "┄ Fields", C.Accent+C.Bold, dim)
	assertImmediateStyle(t, frame, "┄ Log", C.Accent+C.Bold, dim)

	// Contents keep their own colours. Notes stay faint-only, as before.
	// The error row stays red. The table cell and the plain action stay text.
	assertImmediateStyle(t, frame, "note-row", C.Dim, dim)
	assertImmediateStyle(t, frame, "out-line", C.Dim, dim)
	assertImmediateStyle(t, frame, "err-row", C.Err, dim)
	assertImmediateStyle(t, frame, "tbl-cell", C.Text, dim)
	if outputFocused {
		assertImmediateStyle(t, frame, "opt-next", C.Text, dim)
	} else {
		assertImmediateStyle(t, frame, "out-act", C.Text, dim)
	}
}

func TestBesideHeadingsAndNarrowFallback(t *testing.T) {
	useHeadingPalette(t, 16777216, "dracula")
	s := besideHeadingScreen()
	dim := headingStyle(false)
	bright := headingStyle(true)

	wide := s.renderFrame(24, 100)
	left := rowWith(frameGrid(wide, 24, 100), "Commands")
	right := rowWith(frameGrid(wide, 24, 100), "Form")
	if left == 0 || left != right {
		t.Fatalf("wide headings not side by side: commands row %d, form row %d", left, right)
	}
	assertHeadingStyle(t, wide, "Commands", true, bright)
	assertHeadingStyle(t, wide, "Form", false, dim)
	assertImmediateStyle(t, wide, "form-note", C.Dim, inactiveColour(t))
	assertImmediateStyle(t, wide, "┄ List", C.Accent+C.Bold, inactiveColour(t))
	assertImmediateStyle(t, wide, "┄ Commit", C.Accent+C.Bold, inactiveColour(t))

	// Too narrow for two panes: they stack, and the dimming still follows focus.
	narrowScreen := besideHeadingScreen()
	narrow := narrowScreen.renderFrame(36, 56)
	grid := frameGrid(narrow, 36, 56)
	lr, rr := rowWith(grid, "Commands"), rowWith(grid, "Form")
	if lr == 0 || rr == 0 || lr == rr {
		t.Fatalf("narrow terminal did not stack headings: rows %d and %d\n%s", lr, rr, strings.Join(grid, "\n"))
	}
	if len(narrowScreen.bands) < 2 {
		t.Fatalf("narrow bands %+v", narrowScreen.bands)
	}
	assertHeadingStyle(t, narrow, "Commands", true, bright)
	assertHeadingStyle(t, narrow, "Form", false, dim)
	assertImmediateStyle(t, narrow, "form-note", C.Dim, inactiveColour(t))

	narrowScreen.focus = 1
	narrow = narrowScreen.renderFrame(36, 56)
	assertHeadingStyle(t, narrow, "Commands", false, dim)
	assertHeadingStyle(t, narrow, "Form", true, bright)
}

func TestBesideRuleFollowsEachPane(t *testing.T) {
	useHeadingPalette(t, 16777216, "gruvbox")
	s := &Screen{
		Title: "Main",
		Sections: []Section{{Name: "Top", Rows: []Row{
			Action(ActionOpts{Label: "top-row"}),
			Note("top-note"),
		}}},
		Panes: []*Pane{
			{Title: "Left", Sections: []Section{{Name: "Lsec", Rows: []Row{Note("left-line")}}}},
			{Title: "Right", Beside: true, Sections: []Section{{Name: "Rsec", Rows: []Row{Note("right-line")}}}},
		},
	}
	const h, w = 32, 100
	s.focus = 1
	frame := s.renderFrame(h, w)
	if len(s.bands) != 2 || len(s.bands[1].cols) != 2 {
		t.Fatalf("expected a stacked top and a side-by-side band, got %+v", s.bands)
	}
	band := s.bands[1]
	grid := frameGrid(frame, h, w)
	if !gapIsRuleThenBlank(grid, band.divRow) {
		t.Fatalf("no rule above the side-by-side headings\n%s", strings.Join(grid, "\n"))
	}
	x0, _ := s.paneSpan(2)
	ruleRow := band.divRow - 2
	dim := inactiveColour(t)
	if got := styleAt(frame, ruleRow, 2); got != ruleStyle() {
		t.Fatalf("focused half of the rule: %q", got)
	}
	if got := styleAt(frame, ruleRow, x0); got != dim {
		t.Fatalf("unfocused half of the rule at col %d: %q, want %q", x0, got, dim)
	}
	assertHeadingStyle(t, frame, "Left", true, headingStyle(true))
	assertHeadingStyle(t, frame, "Right", false, headingStyle(false))
	assertImmediateStyle(t, frame, "right-line", C.Dim, dim)
	assertImmediateStyle(t, frame, "left-line", C.Dim, dim)

	// The other pane focused: the halves swap, the lines of text do not.
	s.focus = 2
	frame = s.renderFrame(h, w)
	x0, _ = s.paneSpan(2)
	band = s.bands[1]
	if got := styleAt(frame, band.divRow-2, 2); got != dim {
		t.Fatalf("unfocused half after focus moved: %q", got)
	}
	if got := styleAt(frame, band.divRow-2, x0); got != ruleStyle() {
		t.Fatalf("focused half after focus moved: %q", got)
	}
	assertHeadingStyle(t, frame, "Left", false, headingStyle(false))
	assertHeadingStyle(t, frame, "Right", true, headingStyle(true))
}

func TestHeadingDimFollowsTabAndClick(t *testing.T) {
	useHeadingPalette(t, 16777216, "catppuccin")
	const h, w = 32, 80
	s := stackedHeadingScreen()
	s.renderFrame(h, w)
	dim, bright := headingStyle(false), headingStyle(true)

	s.pressKey(&Key{Name: "\t"}, h, w)
	if s.focus != 1 {
		t.Fatalf("tab focused %d", s.focus)
	}
	frame := s.renderFrame(h, w)
	assertHeadingStyle(t, frame, "Options", false, dim)
	assertHeadingStyle(t, frame, "Output", true, bright)
	assertImmediateStyle(t, frame, "err-row", C.Err, inactiveColour(t))

	s.pressKey(&Key{Name: "\t"}, h, w)
	if s.focus != 0 {
		t.Fatalf("second tab focused %d", s.focus)
	}
	frame = s.renderFrame(h, w)
	assertHeadingStyle(t, frame, "Options", true, bright)
	assertHeadingStyle(t, frame, "Output", false, dim)

	// A click on the output pane's heading focuses it and the colours follow.
	out, ok := bandByTitle(s, "Output")
	if !ok || out.divRow == 0 {
		t.Fatalf("output band %+v", s.bands)
	}
	s.pressKey(&Key{Mouse: &Mouse{Row: out.divRow, Col: 6, Click: true}}, h, w)
	if s.focus != 1 {
		t.Fatalf("click on the heading focused %d", s.focus)
	}
	frame = s.renderFrame(h, w)
	assertHeadingStyle(t, frame, "Output", true, bright)
	assertHeadingStyle(t, frame, "Options", false, dim)

	// And a click back on the options pane restores them.
	opt, ok := bandByTitle(s, "Options")
	if !ok {
		t.Fatalf("options band %+v", s.bands)
	}
	row := opt.top
	if row == 0 {
		row = opt.divRow
	}
	s.pressKey(&Key{Mouse: &Mouse{Row: row, Col: 6, Click: true}}, h, w)
	if s.focus != 0 {
		t.Fatalf("click back focused %d", s.focus)
	}
	frame = s.renderFrame(h, w)
	assertHeadingStyle(t, frame, "Options", true, bright)
	assertHeadingStyle(t, frame, "Output", false, dim)

	// Side by side: Tab and a click on the right pane do the same.
	b := besideHeadingScreen()
	const bw = 100
	b.renderFrame(h, bw)
	b.pressKey(&Key{Name: "\t"}, h, bw)
	if b.focus != 1 {
		t.Fatalf("beside tab focused %d", b.focus)
	}
	frame = b.renderFrame(h, bw)
	assertHeadingStyle(t, frame, "Commands", false, dim)
	assertHeadingStyle(t, frame, "Form", true, bright)
	x0, _ := b.paneSpan(0)
	// Click the left heading to move focus back.
	left, ok := bandByTitle(b, "Commands")
	if !ok || left.divRow == 0 {
		t.Fatalf("commands band %+v", b.bands)
	}
	b.pressKey(&Key{Mouse: &Mouse{Row: left.divRow, Col: x0 + 4, Click: true}}, h, bw)
	if b.focus != 0 {
		t.Fatalf("beside click focused %d", b.focus)
	}
	frame = b.renderFrame(h, bw)
	assertHeadingStyle(t, frame, "Commands", true, bright)
	assertHeadingStyle(t, frame, "Form", false, dim)
	assertImmediateStyle(t, frame, "form-note", C.Dim, inactiveColour(t))
}

func TestEveryThemeDimsInactiveHeadings(t *testing.T) {
	backgrounds := map[string][3]int{
		"default":    {0, 0, 0},
		"terminal":   {0, 0, 0},
		"nord":       {46, 52, 64},
		"gruvbox":    {40, 40, 40},
		"dracula":    {40, 42, 54},
		"catppuccin": {30, 30, 46},
	}
	for _, name := range theme.Names {
		t.Run(name, func(t *testing.T) {
			if name == "custom" {
				useCustomTheme(t)
			}
			useHeadingPalette(t, 16777216, name)
			if want, ok := backgrounds[name]; ok && C.Background != want {
				t.Fatalf("background %v, want %v", C.Background, want)
			}
			fg, ok := sgrRGB(C.Accent)
			if !ok {
				t.Fatalf("accent %q has no colour to blend", C.Accent)
			}
			s := stackedHeadingScreen()
			frame := s.renderFrame(28, 80)
			assertHeadingStyle(t, frame, "Options", true, C.Accent+C.Bold)
			assertHeadingStyle(t, frame, "Output", false, headingStyle(false))
			dim := inactiveColour(t)
			if dim == C.Accent || strings.Contains(dim, "\x1b[2m") {
				t.Fatalf("dim colour %q is not a blend of %v toward %v", dim, fg, C.Background)
			}
			want := truecolorFG(blendRGB(fg, C.Background, inactiveBlend))
			if dim != want {
				t.Fatalf("dim %q, want %q", dim, want)
			}
			assertImmediateStyle(t, frame, "err-row", C.Err, dim)
			assertImmediateStyle(t, frame, "out-line", C.Dim, dim)
			assertImmediateStyle(t, frame, "tbl-cell", C.Text, dim)

			// 256-colour fallback for this same theme.
			useHeadingPalette(t, 256, name)
			col, ok := inactiveHeadingColour()
			if !ok || col != C.Muted || strings.Contains(col, "38;2;") {
				t.Fatalf("256-colour dim %q, muted %q", col, C.Muted)
			}
		})
	}
}

func stackedHeadingScreen() *Screen {
	return &Screen{
		Title:    "Stacked",
		TopTitle: "Options",
		Sections: []Section{{
			Name: "Fields",
			Rows: []Row{
				Action(ActionOpts{Label: "opt-row"}),
				Action(ActionOpts{Label: "opt-next"}),
				Note("note-row"),
			},
		}},
		Panes: []*Pane{{
			Title: "Output",
			Sections: []Section{{
				Name: "Log",
				Rows: []Row{
					Note("out-line"),
					Action(ActionOpts{Label: "out-act"}),
					Text(TextOpts{ID: "bad", Label: "err-row", Value: "missing", Alert: true}),
					Table(TableOpts{Headers: []string{"Name"}, Rows: [][]string{{"tbl-cell"}}}),
				},
			}},
		}},
	}
}

func besideHeadingScreen() *Screen {
	return &Screen{
		Title:    "Beside",
		TopTitle: "Commands",
		TopWidth: 1,
		Sections: []Section{{
			Name: "List",
			Rows: []Row{Action(ActionOpts{Label: "cmd-row"})},
		}},
		Panes: []*Pane{{
			Title: "Form", Beside: true, Width: 2,
			Sections: []Section{{
				Name: "Commit",
				Rows: []Row{
					Text(TextOpts{Label: "Message", Value: "hello"}),
					Note("form-note"),
				},
			}},
		}},
	}
}

func useHeadingPalette(t *testing.T, depth int, name string) {
	t.Helper()
	oldC, oldBase, oldDepth := C, basePalette, rt.profile.ColorDepth
	t.Cleanup(func() {
		C, basePalette = oldC, oldBase
		rt.profile.ColorDepth = oldDepth
	})
	if depth >= 16777216 {
		t.Setenv("COLORTERM", "truecolor")
	} else {
		t.Setenv("COLORTERM", "")
	}
	t.Setenv("NO_COLOR", "")
	pal := theme.Load(name)
	basePalette = pal
	rt.profile.ColorDepth = depth
	if depth > 0 && depth <= 16 {
		C = (&level1Runtime{baseRuntime: baseRuntime{profile: termProfile{ColorDepth: depth}}}).quantizePalette(pal)
		return
	}
	C = pal
}

func useCustomTheme(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "theme.conf")
	body := strings.Join([]string{
		"theme = custom",
		"accent = 100 150 200",
		"bright = 210 210 210",
		"text = 220 220 220",
		"muted = 140 140 140",
		"on = 80 180 120",
		"amber = 200 160 60",
		"selbg = 40 40 50",
		"err = 200 60 60",
		"bg = 1 2 3",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := theme.ConfigPath
	theme.ConfigPath = path
	t.Cleanup(func() { theme.ConfigPath = old })
}

func inactiveColour(t *testing.T) string {
	t.Helper()
	col, ok := inactiveHeadingColour()
	if !ok {
		t.Fatal("expected a dim heading colour")
	}
	return col
}

func assertHeadingStyle(t *testing.T, frame, title string, focused bool, want string) {
	t.Helper()
	mark := "┄ " + title
	if focused {
		mark = "▸ " + title
	}
	if !strings.Contains(stripFrame(frame), mark) {
		t.Fatalf("heading %q missing\n%s", mark, stripFrame(frame))
	}
	if got := styleImmediatelyBefore(frame, mark); got != want {
		t.Fatalf("heading %q style %q, want %q", mark, got, want)
	}
}

func assertImmediateStyle(t *testing.T, frame, needle, want, forbid string) {
	t.Helper()
	if !strings.Contains(frame, needle) {
		t.Fatalf("%q missing from the frame", needle)
	}
	got := styleImmediatelyBefore(frame, needle)
	if !strings.Contains(got, want) {
		t.Fatalf("%q style %q, want it to contain %q", needle, got, want)
	}
	if forbid != "" && strings.Contains(got, forbid) {
		t.Fatalf("%q style %q contains the dim colour %q", needle, got, forbid)
	}
}

var (
	trailingSGR = regexp.MustCompile(`(?:\x1b\[[0-9;]*m)+$`)
	leadingSGR  = regexp.MustCompile(`^(?:\x1b\[[0-9;]*m)+`)
)

func styleImmediatelyBefore(frame, needle string) string {
	i := strings.Index(frame, needle)
	if i < 0 {
		return ""
	}
	from := i - 96
	if from < 0 {
		from = 0
	}
	return trailingSGR.FindString(frame[from:i])
}

func styleAt(frame string, row, col int) string {
	key := move(row, col)
	i := strings.Index(frame, key)
	if i < 0 {
		return ""
	}
	rest := frame[i+len(key):]
	if len(rest) > 80 {
		rest = rest[:80]
	}
	return leadingSGR.FindString(rest)
}
