package mint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

func TestTermProfileCacheAndLevels(t *testing.T) {
	tmp := t.TempDir()
	cacheFile := filepath.Join(tmp, "termcaps.json")
	t.Setenv("MINT_TERMCAPS", cacheFile)
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TERM_PROGRAM", "kitty")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TMUX", "")
	t.Setenv("NO_COLOR", "")

	prof1 := loadOrProbeTermProfile(true)
	if prof1.Level < 3 {
		t.Fatalf("expected Level >= 3 for kitty truecolor, got %d", prof1.Level)
	}
	if !prof1.BracketedPaste || !prof1.SGRMouse || !prof1.SyncOutput {
		t.Fatalf("expected independent capabilities enabled, got %+v", prof1)
	}
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("expected termcaps.json to be written: %v", err)
	}

	// Second call loads from cache
	prof2 := loadOrProbeTermProfile(false)
	if prof2.Signature != prof1.Signature || prof2.Level != prof1.Level {
		t.Fatalf("expected cached profile match, got %+v vs %+v", prof2, prof1)
	}

	// Multi-terminal cache: switch to tmux wrapper and verify second profile entry is added
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	profTmux := loadOrProbeTermProfile(false)
	if profTmux.Level != 5 || !profTmux.TmuxPassthrough {
		t.Fatalf("expected Level 5 with tmux passthrough, got %+v", profTmux)
	}
}

func TestLevel1QuantiserAndASCIIFallback(t *testing.T) {
	t.Setenv("COLORTERM", "")
	var r level5Runtime
	r.profile = termProfile{
		Level:      1,
		ColorDepth: 16,
		ASCIIOnly:  true,
	}
	pal := theme.Load("nord")
	q := r.quantizePalette(pal)
	if strings.Contains(q.Accent, "38;2;") || strings.Contains(q.Accent, "38;5;") {
		t.Fatalf("expected 16-colour SGR at Level 1, got %q", q.Accent)
	}
	got := r.sanitizeGlyphs("╭──╮ ● ☑ ▸")
	if strings.ContainsRune(got, '╭') || strings.ContainsRune(got, '●') {
		t.Fatalf("expected ASCII fallback glyphs, got %q", got)
	}
}

func TestLineBufferCursorAndBracketedPaste(t *testing.T) {
	lb := newLineBuffer("hello world")
	if lb.cursor != 11 {
		t.Fatalf("expected cursor at end (11), got %d", lb.cursor)
	}
	// Move left 5 chars ("world") and insert "mint "
	for i := 0; i < 5; i++ {
		lb.handleKey(named("left"))
	}
	lb.handleKey(pasted("mint\nnew-line"))
	if lb.String() != "hello mint new-lineworld" {
		t.Fatalf("unexpected buffer after paste: %q", lb.String())
	}
	// Ctrl+W deletes previous word
	lb.handleKey(char("\x17"))
	if lb.String() != "hello mint world" {
		t.Fatalf("unexpected buffer after Ctrl+W: %q", lb.String())
	}
	// Home + Delete removes 'h'
	lb.handleKey(named("home"))
	lb.handleKey(named("delete"))
	if lb.String() != "ello mint world" {
		t.Fatalf("unexpected buffer after Home+Delete: %q", lb.String())
	}
}

func TestVirtualList100kAndIncrementalContainsFold(t *testing.T) {
	const N = 100_000
	items := make([]Choice, N)
	descs := make([]string, N)
	for i := 0; i < N; i++ {
		if i == 75432 {
			items[i] = Choice{Label: "SpiderFoot-OSINT-Target.json", Value: "sf"}
		} else {
			items[i] = Choice{Label: "entry-item-file.txt", Value: "other"}
		}
	}
	vl := newVirtualList(items, descs)
	if vl.count() != N {
		t.Fatalf("expected %d items, got %d", N, vl.count())
	}
	vl.filter("spider")
	if vl.count() != 1 {
		t.Fatalf("expected 1 match after 'spider', got %d", vl.count())
	}
	// Incremental narrowing reuses vl.visible
	vl.filter("spiderfoot-osint")
	if vl.count() != 1 || vl.itemAt(0).Value != "sf" {
		t.Fatalf("expected 'sf' item after narrowing, got %+v", vl.itemAt(0))
	}
	// Multi-select toggle
	vl.toggleAt(0)
	if !vl.hasSelected() || len(vl.selectedValues()) != 1 || vl.selectedValues()[0] != "sf" {
		t.Fatalf("expected 'sf' to be selected, got %v", vl.selectedValues())
	}
}

func TestWCWidthAndPlainCleanLine(t *testing.T) {
	if w := util.VLen("日本語"); w != 6 {
		t.Fatalf("expected CJK width 6, got %d", w)
	}
	if w := util.VLen("e\u0301"); w != 1 {
		t.Fatalf("expected combining mark width 1, got %d", w)
	}
	cleaned := CleanLine("\x1b[31mERROR\x1b[0m: N\bNA\bAM\bME\bE")
	if cleaned != "ERROR: NAME" {
		t.Fatalf("expected plain text 'ERROR: NAME', got %q", cleaned)
	}
}

func TestColorTermGateFallbackTo256(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "")
	t.Setenv("NO_COLOR", "")

	prof := probeTerminalFeatures(false)
	if prof.ColorDepth != 256 {
		t.Fatalf("expected ColorDepth 256 when COLORTERM is unset, got %d", prof.ColorDepth)
	}

	pal := theme.Load("nord")
	if strings.Contains(pal.Accent, "38;2;") || strings.Contains(pal.SelBg, "48;2;") {
		t.Fatalf("expected 24-bit sequences to fall back to 256-colour when COLORTERM is unset, got Accent=%q SelBg=%q", pal.Accent, pal.SelBg)
	}
	if !strings.Contains(pal.Accent, "38;5;") || !strings.Contains(pal.SelBg, "48;5;") {
		t.Fatalf("expected 256-colour SGR sequences, got Accent=%q SelBg=%q", pal.Accent, pal.SelBg)
	}
}

// A locale that cannot render UTF-8 gets the ASCII fallback whatever the
// terminal calls itself: the glyphs Mint draws are UTF-8 either way.
func TestASCIIFallbackFollowsTheLocale(t *testing.T) {
	for _, tc := range []struct {
		term, lcAll, lcCtype, lang string
		want                       bool
	}{
		{"xterm-256color", "", "", "en_GB.UTF-8", false},
		{"xterm-256color", "", "", "C.UTF-8", false},
		{"xterm-256color", "", "", "en_US.utf8", false},
		{"xterm-256color", "", "", "en_GB.ISO-8859-15", true},
		{"xterm-256color", "", "", "POSIX", true},
		{"xterm-256color", "", "", "C", true},
		{"xterm-256color", "", "", "", true},
		// LC_ALL beats LC_CTYPE beats LANG, as in the C library.
		{"xterm-256color", "C", "", "en_GB.UTF-8", true},
		{"xterm-256color", "", "C", "en_GB.UTF-8", true},
		{"xterm-256color", "", "en_GB.ISO-8859-15", "C.UTF-8", true},
		// The terminal itself can force it.
		{"dumb", "", "", "en_GB.UTF-8", true},
		{"vt100", "", "", "en_GB.UTF-8", true},
	} {
		t.Setenv("TERM", tc.term)
		t.Setenv("LC_ALL", tc.lcAll)
		t.Setenv("LC_CTYPE", tc.lcCtype)
		t.Setenv("LANG", tc.lang)
		t.Setenv("MINT_TERMCAPS", filepath.Join(t.TempDir(), "termcaps.json"))
		p := probeTerminalFeatures(false)
		if p.ASCIIOnly != tc.want {
			t.Errorf("TERM=%s LC_ALL=%q LC_CTYPE=%q LANG=%q: ASCIIOnly = %v, want %v",
				tc.term, tc.lcAll, tc.lcCtype, tc.lang, p.ASCIIOnly, tc.want)
		}
	}
}

// A glyph repeated to fill a span must fill columns, not count cells: in a
// CJK locale the frame edge is two wide and half as many are needed.
func TestRepeatToFillsColumns(t *testing.T) {
	wide := termProfile{AmbiguousWide: true}
	narrow := termProfile{}
	for _, tc := range []struct {
		p     termProfile
		glyph string
		n     int
		want  string
	}{
		{narrow, "─", 8, "────────"},
		{wide, "─", 8, "────"},
		{wide, "━", 13, "━━━━━━"},
		{narrow, " ", 5, "     "},
		{wide, " ", 5, "     "}, // spaces are never ambiguous
		{narrow, "─", 0, ""},
		{wide, "─", 1, ""}, // no room for even one wide glyph
	} {
		if got := tc.p.repeatTo(tc.glyph, tc.n); got != tc.want {
			t.Errorf("repeatTo(%q, %d) with AmbiguousWide=%v = %q, want %q",
				tc.glyph, tc.n, tc.p.AmbiguousWide, got, tc.want)
		}
	}
}

// The top border must not overrun the frame in either locale.
func TestFrameBorderFits(t *testing.T) {
	for _, wide := range []bool{false, true} {
		rt.profile.AmbiguousWide = wide
		const w = 40
		border := "╭" + rt.profile.repeatTo("─", w-2) + "╮"
		if got := VLen(border); got > w {
			t.Errorf("AmbiguousWide=%v: border is %d cells, frame is %d", wide, got, w)
		}
	}
	rt.profile.AmbiguousWide = false
}

// Every terminal Mint can name, from the variable it publishes.
func TestDetectProgram(t *testing.T) {
	clearEnv := func() {
		for _, v := range []string{"TERM_PROGRAM"} {
			t.Setenv(v, "")
		}
		for _, p := range terminalProbes {
			for _, v := range []string{"KITTY_WINDOW_ID", "KITTY_PID", "WEZTERM_PANE", "WEZTERM_EXECUTABLE",
				"ALACRITTY_WINDOW_ID", "ALACRITTY_LOG", "ALACRITTY_SOCKET", "GHOSTTY_RESOURCES_DIR", "GHOSTTY_BIN_DIR",
				"WARP_IS_LOCAL_SHELL_SESSION", "WARP_HONOR_PS1", "WARP_CLIENT_VERSION", "RIO_LOG_LEVEL", "RIO_CONFIG",
				"CONTOUR_PROFILE", "CONTOUR_LOG_FILE", "CONTOUR_LOG_LEVEL", "CMUX_SESSION", "CMUX_PANE",
				"TABBY_CONFIG_DIRECTORY", "HYPER_VERSION", "WAVETERM", "WAVETERM_BLOCKID", "WAVETERM_VERSION",
				"DOMTERM", "TERMINOLOGY", "TERMINOLOGY_VERSION", "COOL_RETRO_TERM", "COOLRETROTERM",
				"BLACKBOX_TERMINAL", "FOOT_VERSION", "DEEPIN_TERMINAL", "PTYXIS_VERSION", "PTYXIS_PROFILE",
				"KONSOLE_VERSION", "XTERM_VERSION", "XTERM_SHELL", "ETERM_VERSION", "RXVT_VERSION",
				"SECURECRT_VER", "PUTTY_SESSION", "PUTTY_SESSION_NAME", "MOSH_IP", "MOSH_PID", "VTE_VERSION"} {
				t.Setenv(v, "")
			}
			_ = p
		}
	}

	for _, tc := range []struct{ variable, want string }{
		{"KITTY_WINDOW_ID", "kitty"},
		{"WEZTERM_PANE", "WezTerm"},
		{"ALACRITTY_WINDOW_ID", "Alacritty"},
		{"GHOSTTY_RESOURCES_DIR", "ghostty"},
		{"WARP_CLIENT_VERSION", "Warp"},
		{"RIO_CONFIG", "Rio"},
		{"CONTOUR_PROFILE", "Contour"},
		{"CMUX_SESSION", "cmux"},
		{"TABBY_CONFIG_DIRECTORY", "Tabby"},
		{"HYPER_VERSION", "Hyper"},
		{"WAVETERM_VERSION", "WaveTerm"},
		{"DOMTERM", "DomTerm"},
		{"TERMINOLOGY", "Terminology"},
		{"COOL_RETRO_TERM", "CoolRetroTerm"},
		{"BLACKBOX_TERMINAL", "BlackBox"},
		{"FOOT_VERSION", "Foot"},
		{"DEEPIN_TERMINAL", "DeepinTerminal"},
		{"PTYXIS_VERSION", "Ptyxis"},
		{"KONSOLE_VERSION", "Konsole"},
		{"XTERM_VERSION", "xterm"},
		{"ETERM_VERSION", "Eterm"},
		{"RXVT_VERSION", "rxvt"},
		{"SECURECRT_VER", "SecureCRT"},
		{"PUTTY_SESSION_NAME", "PuTTY"},
		{"MOSH_IP", "Mosh"},
		{"VTE_VERSION", "VTE"},
	} {
		clearEnv()
		t.Setenv(tc.variable, "1")
		if got := DetectProgram(); got != tc.want {
			t.Errorf("%s set: DetectProgram = %q, want %q", tc.variable, got, tc.want)
		}
	}

	// TERM_PROGRAM wins over any private variable.
	clearEnv()
	t.Setenv("KITTY_WINDOW_ID", "1")
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	if got := DetectProgram(); got != "Apple_Terminal" {
		t.Errorf("TERM_PROGRAM set: DetectProgram = %q, want Apple_Terminal", got)
	}

	// Nothing set: no name, and the caller falls back to $TERM.
	clearEnv()
	if got := DetectProgram(); got != "" {
		t.Errorf("nothing set: DetectProgram = %q, want \"\"", got)
	}
}

// The terminals that publish no private variable are named from $TERM.
func TestProgramFromTerm(t *testing.T) {
	for term, want := range map[string]string{
		"xterm-256color":        "xterm",
		"xterm":                 "xterm",
		"st-256color":           "st",
		"rxvt-unicode-256color": "urxvt",
		"urxvt":                 "urxvt",
		"rxvt":                  "rxvt",
		"alacritty":             "Alacritty",
		"tmux-256color":         "tmux",
		"screen-256color":       "screen",
		"linux":                 "linux",
		"dumb":                  "",
	} {
		if got := programFromTerm(term); got != want {
			t.Errorf("programFromTerm(%q) = %q, want %q", term, got, want)
		}
	}
}
