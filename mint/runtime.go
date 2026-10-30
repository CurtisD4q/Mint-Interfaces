package mint

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// ── Multi-Terminal Capability Cache (~/.config/mint-interfaces/termcaps.json) ──

type termProfile struct {
	Signature       string `json:"signature"`
	Term            string `json:"term"`
	TermProgram     string `json:"term_program,omitempty"`
	Wrapper         string `json:"wrapper,omitempty"`
	Version         string `json:"version,omitempty"`
	Level           int    `json:"level"`
	ColorDepth      int    `json:"color_depth"` // 0=mono, 16, 256, 16777216
	ASCIIOnly       bool   `json:"ascii_only"`
	AltScreen       bool   `json:"alt_screen"`
	SGRMouse        bool   `json:"sgr_mouse"`
	BracketedPaste  bool   `json:"bracketed_paste"`
	SyncOutput      bool   `json:"sync_output"`
	GraphemeCluster bool   `json:"grapheme_cluster"`
	KittyKeyboard   bool   `json:"kitty_keyboard"`
	OSC52           bool   `json:"osc52"`
	OSC8            bool   `json:"osc8"`
	FocusEvents     bool   `json:"focus_events"`
	TmuxPassthrough bool   `json:"tmux_passthrough"`
	ProbedAt        string `json:"probed_at"`
}

type termCacheFile struct {
	UpdatedAt string                 `json:"updated_at"`
	ActiveKey string                 `json:"active_key"`
	Default   termProfile            `json:"default"`
	Terminals map[string]termProfile `json:"terminals"`
}

func termCapsPath() string {
	if p := os.Getenv("MINT_TERMCAPS"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mint-interfaces", "termcaps.json")
}

func currentTermSignature() string {
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" {
		term = "dumb"
	}
	prog := strings.TrimSpace(os.Getenv("TERM_PROGRAM"))
	if prog == "" {
		switch {
		case os.Getenv("KITTY_WINDOW_ID") != "":
			prog = "kitty"
		case os.Getenv("WEZTERM_PANE") != "":
			prog = "WezTerm"
		case os.Getenv("ALACRITTY_WINDOW_ID") != "" || os.Getenv("ALACRITTY_LOG") != "":
			prog = "Alacritty"
		case os.Getenv("GHOSTTY_RESOURCES_DIR") != "":
			prog = "ghostty"
		case os.Getenv("KONSOLE_VERSION") != "":
			prog = "Konsole"
		case os.Getenv("VTE_VERSION") != "":
			prog = "VTE"
		case os.Getenv("WT_SESSION") != "":
			prog = "WindowsTerminal"
		}
	}
	wrap := detectWrapper()
	ct := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	return term + "|" + prog + "|" + wrap + "|" + ct
}

func hasTrueColorEnv() bool {
	ct := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	return ct == "truecolor" || ct == "24bit"
}

func detectWrapper() string {
	var parts []string
	if os.Getenv("TMUX") != "" {
		parts = append(parts, "tmux")
	}
	if os.Getenv("STY") != "" {
		parts = append(parts, "screen")
	}
	if os.Getenv("NVIM") != "" || os.Getenv("NVIM_LISTEN_ADDRESS") != "" {
		parts = append(parts, "nvim")
	}
	if os.Getenv("INSIDE_EMACS") != "" || os.Getenv("EMACS") != "" {
		parts = append(parts, "emacs")
	}
	if os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != "" {
		parts = append(parts, "ssh")
	}
	return strings.Join(parts, "+")
}

// parseTerminfoColors reads the compiled terminfo binary in /usr/share/terminfo
// (or $TERMINFO) to determine max_colors without spawning infocmp or tput.
func parseTerminfoColors(termName string) (maxColors int, found bool) {
	if termName == "" || termName == "dumb" {
		return 0, false
	}
	first := string(termName[0])
	hexFirst := fmt.Sprintf("%02x", termName[0])
	var dirs []string
	if custom := os.Getenv("TERMINFO"); custom != "" {
		dirs = append(dirs, custom)
	}
	if home := os.Getenv("HOME"); home != "" {
		dirs = append(dirs, filepath.Join(home, ".terminfo"))
	}
	dirs = append(dirs, "/etc/terminfo", "/lib/terminfo", "/usr/share/terminfo")

	for _, d := range dirs {
		for _, sub := range []string{first, hexFirst} {
			p := filepath.Join(d, sub, termName)
			data, err := os.ReadFile(p)
			if err != nil || len(data) < 12 {
				continue
			}
			magic := int(binary.LittleEndian.Uint16(data[0:2]))
			numSize := 2
			if magic == 0x021e { // 32-bit numbers
				numSize = 4
			} else if magic != 0x011a { // 16-bit standard ncurses magic
				continue
			}
			nameSize := int(binary.LittleEndian.Uint16(data[2:4]))
			boolCount := int(binary.LittleEndian.Uint16(data[4:6]))
			numCount := int(binary.LittleEndian.Uint16(data[6:8]))
			offset := 12 + nameSize + boolCount
			if offset%2 != 0 {
				offset++
			}
			// max_colors is numeric capability index 13 in standard terminfo.
			const maxColorsIdx = 13
			if numCount > maxColorsIdx && offset+(maxColorsIdx+1)*numSize <= len(data) {
				pos := offset + maxColorsIdx*numSize
				var val int
				if numSize == 4 {
					val = int(int32(binary.LittleEndian.Uint32(data[pos : pos+4])))
				} else {
					val = int(int16(binary.LittleEndian.Uint16(data[pos : pos+2])))
				}
				if val > 0 {
					return val, true
				}
			}
			return 8, true
		}
	}
	return 0, false
}

// probeTerminalFeatures performs independent feature-by-feature detection:
// 1. Environment & locale inspection ($TERM, $COLORTERM, $NO_COLOR, $LANG, wrappers)
// 2. Local binary /usr/share/terminfo parsing
// 3. Optional non-blocking wire query (DECRQM + XTVERSION + DA1) when tty is interactive
func probeTerminalFeatures(interactiveWireProbe bool) termProfile {
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" {
		term = "dumb"
	}
	prog := strings.TrimSpace(os.Getenv("TERM_PROGRAM"))
	wrap := detectWrapper()
	sig := currentTermSignature()

	p := termProfile{
		Signature: sig,
		Term:      term,
		Wrapper:   wrap,
		ProbedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if prog != "" {
		p.TermProgram = prog
	}

	// Check locale for UTF-8 vs ASCII fallback.
	if !utf8Locale(os.Getenv("LC_ALL"), os.Getenv("LC_CTYPE"), os.Getenv("LANG")) {
		p.ASCIIOnly = true
	}
	if term == "dumb" || term == "vt100" {
		p.ASCIIOnly = true
	}
	// A Japanese, Chinese or Korean locale draws East Asian Ambiguous runes
	// two columns wide, which is what the frame and the symbols are.
	util.SetWidth(util.Width{AmbiguousWide: cjkLocale(os.Getenv("LC_ALL"), os.Getenv("LC_CTYPE"), os.Getenv("LANG"))})

	// Determine baseline colour depth from terminfo + environment.
	depth := 16
	if tc, ok := parseTerminfoColors(term); ok {
		if tc >= 256 {
			depth = 256
		} else if tc <= 1 {
			depth = 0
		}
	}
	lowerTerm := strings.ToLower(term)
	modernTerm := strings.Contains(lowerTerm, "direct") ||
		strings.Contains(lowerTerm, "kitty") ||
		strings.Contains(lowerTerm, "alacritty") ||
		strings.Contains(lowerTerm, "wezterm") ||
		strings.Contains(lowerTerm, "ghostty") ||
		strings.Contains(lowerTerm, "foot") ||
		os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("WEZTERM_PANE") != "" ||
		os.Getenv("ALACRITTY_WINDOW_ID") != "" ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" ||
		os.Getenv("VTE_VERSION") != "" ||
		os.Getenv("KONSOLE_VERSION") != "" ||
		os.Getenv("WT_SESSION") != ""
	if strings.Contains(lowerTerm, "256color") || modernTerm {
		if depth < 256 {
			depth = 256
		}
	}
	// Gate 24-bit TrueColor strictly on $COLORTERM ∈ {"truecolor", "24bit"}.
	if hasTrueColorEnv() {
		depth = 16777216
	}
	if os.Getenv("NO_COLOR") != "" || term == "dumb" || strings.HasSuffix(lowerTerm, "-mono") || strings.HasSuffix(lowerTerm, "-m") {
		depth = 0
	}
	p.ColorDepth = depth

	if term != "dumb" && term != "vt100" {
		p.AltScreen = true
		p.SGRMouse = lowerTerm != "linux"
		p.BracketedPaste = lowerTerm != "linux"
	}

	// Heuristic defaults for modern capabilities before optional wire verification.
	if depth == 16777216 || modernTerm {
		p.SyncOutput = true
		p.OSC52 = true
		p.OSC8 = true
		p.FocusEvents = true
	}
	if strings.Contains(lowerTerm, "kitty") || strings.Contains(lowerTerm, "wezterm") || strings.Contains(lowerTerm, "ghostty") || strings.Contains(lowerTerm, "foot") {
		p.GraphemeCluster = true
		p.KittyKeyboard = true
	}
	if strings.Contains(wrap, "tmux") {
		p.TmuxPassthrough = true
		p.FocusEvents = true
	}

	// Non-blocking wire probe (DECRQM ?1006, ?2004, ?2026, ?2027, XTVERSION, DA1)
	// when running interactively on a real TTY. Each capability is tested independently.
	if interactiveWireProbe && isTerminal() && term != "dumb" {
		wireReply := queryTerminalWire(45 * time.Millisecond)
		if wireReply != "" {
			applyWireReply(&p, wireReply)
		}
	}

	p.Level = computeRuntimeLevel(p)
	return p
}

// decrqmSupported checks whether a DECRQM response `\x1b[?<mode>;<Pm>$y`
// reports the mode as supported (Pm == 1 set, Pm == 2 reset, Pm == 3 permanently set).
func decrqmSupported(reply string, mode int) (supported bool, present bool) {
	prefix := fmt.Sprintf("\x1b[?%d;", mode)
	idx := strings.Index(reply, prefix)
	if idx < 0 {
		return false, false
	}
	rest := reply[idx+len(prefix):]
	semi := strings.Index(rest, "$y")
	if semi < 0 || semi > 4 {
		return false, false
	}
	pm, err := strconv.Atoi(rest[:semi])
	if err != nil {
		return false, false
	}
	return pm == 1 || pm == 2 || pm == 3, true
}

func applyWireReply(p *termProfile, reply string) {
	if sup, ok := decrqmSupported(reply, 1006); ok {
		p.SGRMouse = sup
	}
	if sup, ok := decrqmSupported(reply, 2004); ok {
		p.BracketedPaste = sup
	}
	if sup, ok := decrqmSupported(reply, 2026); ok {
		p.SyncOutput = sup
	}
	if sup, ok := decrqmSupported(reply, 2027); ok {
		p.GraphemeCluster = sup
	}
	if sup, ok := decrqmSupported(reply, 1004); ok {
		p.FocusEvents = sup
	}
	if strings.Contains(reply, "\x1b[?") && strings.Contains(reply, "u") {
		p.KittyKeyboard = true
	}
	// Parse XTVERSION: \x1bP>|...version...\x1b\
	if start := strings.Index(reply, "\x1bP>|"); start >= 0 {
		rest := reply[start+4:]
		if end := strings.Index(rest, "\x1b\\"); end >= 0 {
			ver := strings.TrimSpace(rest[:end])
			if ver != "" {
				p.Version = ver
				if hasTrueColorEnv() {
					p.ColorDepth = 16777216
				} else if p.ColorDepth < 256 {
					p.ColorDepth = 256
				}
				p.OSC52 = true
				p.OSC8 = true
			}
		}
	}
}

func queryTerminalWire(timeout time.Duration) string {
	orig, err := tcget(0)
	if err != nil {
		return ""
	}
	raw := *orig
	makeRaw(&raw)
	raw.Cc[syscall.VMIN] = 0
	raw.Cc[syscall.VTIME] = 0
	if tcset(0, &raw) != nil {
		return ""
	}
	defer func() { _ = tcset(0, orig) }()

	// Query independent features in a single batch terminated by DA1 (\x1b[c):
	// DECRQM for ?1006 (SGR mouse), ?2004 (Bracketed Paste), ?2026 (Sync Output),
	// ?2027 (Grapheme Clustering), ?1004 (Focus), XTVERSION (\x1b[>0q), and DA1 (\x1b[c).
	query := "\x1b[?1006$p\x1b[?2004$p\x1b[?2026$p\x1b[?2027$p\x1b[?1004$p\x1b[>0q\x1b[c"
	_, _ = os.Stdout.WriteString(query)

	deadline := time.Now().Add(timeout)
	var buf strings.Builder
	tmp := make([]byte, 256)
	for time.Now().Before(deadline) {
		n, _ := syscall.Read(0, tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			// DA1 response ends with 'c' after '\x1b[?'
			s := buf.String()
			if lastEsc := strings.LastIndex(s, "\x1b[?"); lastEsc >= 0 {
				tail := s[lastEsc:]
				if strings.HasSuffix(tail, "c") && !strings.Contains(tail, "$y") {
					break
				}
			}
		} else {
			time.Sleep(2 * time.Millisecond)
		}
	}
	return buf.String()
}

// utf8Locale reports whether the locale the program is running in can
// render UTF-8, reading LC_ALL, then LC_CTYPE, then LANG: the first one set
// wins, as in the C library. A locale that names no codeset ("C", "POSIX",
// a bare "en_GB") is not taken to be UTF-8, so the ASCII fallback is used.
func utf8Locale(vars ...string) bool {
	for _, v := range vars {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		u := strings.ToUpper(v)
		return strings.Contains(u, "UTF-8") || strings.Contains(u, "UTF8")
	}
	return false
}

// cjkLocale reports whether the locale is one that draws East Asian
// Ambiguous runes two columns wide. It reads LC_ALL, then LC_CTYPE, then
// LANG, as utf8Locale does.
func cjkLocale(vars ...string) bool {
	for _, v := range vars {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		for _, tag := range []string{"JA", "ZH", "KO"} {
			if strings.HasPrefix(strings.ToUpper(v), tag) {
				return true
			}
		}
		return false
	}
	return false
}

func computeRuntimeLevel(p termProfile) int {
	switch {
	case p.TmuxPassthrough || (p.Wrapper != "" && p.SyncOutput):
		return 5
	case p.GraphemeCluster || p.KittyKeyboard || p.OSC52 || p.OSC8:
		return 4
	case p.ColorDepth >= 16777216 || p.SyncOutput:
		return 3
	case p.AltScreen && (p.SGRMouse || p.BracketedPaste):
		return 2
	default:
		return 1
	}
}

// loadOrProbeTermProfile reads ~/.config/mint-interfaces/termcaps.json for the
// current terminal signature. If missing (or forceProbe is true), it probes
// once, updates the multi-terminal cache file, and returns the profile.
func loadOrProbeTermProfile(forceProbe bool) termProfile {
	path := termCapsPath()
	sig := currentTermSignature()

	var cache termCacheFile
	cache.Terminals = map[string]termProfile{}

	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &cache)
		if cache.Terminals == nil {
			cache.Terminals = map[string]termProfile{}
		}
	}

	if !forceProbe {
		if prof, ok := cache.Terminals[sig]; ok && prof.Signature == sig {
			return prof
		}
	}

	prof := probeTerminalFeatures(forceProbe || isTerminal())
	cache.UpdatedAt = prof.ProbedAt
	cache.ActiveKey = sig
	cache.Default = prof
	cache.Terminals[sig] = prof

	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if encoded, err := json.MarshalIndent(cache, "", "  "); err == nil {
		_ = os.WriteFile(path, append(encoded, '\n'), 0o644)
	}
	return prof
}

// ── Unexported Level 1..5 Inheritance Hierarchy ──────────────────────────────

// baseRuntime owns the core screen dimensions, resize signal listener,
// Unicode lineBuffer, and 100k+ entry virtualList engine.
type baseRuntime struct {
	profile   termProfile
	winchOnce sync.Once
	resized   atomic.Bool
	focused   atomic.Bool
}

func (b *baseRuntime) initSignals() {
	b.focused.Store(true)
	b.winchOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		go func() {
			for range ch {
				b.resized.Store(true)
			}
		}()
	})
}

func (b *baseRuntime) consumeResize() bool {
	return b.resized.Swap(false)
}

// level1Runtime embeds baseRuntime and adds 16/256-colour SGR quantisation
// and ASCII glyph fallback for non-UTF-8 / dumb / Linux-console terminals.
type level1Runtime struct {
	baseRuntime
}

// monoPalette is the palette for a terminal with no colour. Emphasis that
// would have been colour becomes bold or reverse. Text and Muted stay empty
// on purpose: they are the roles such a terminal shows plainly, and empty is
// how a role is left unstyled. Complete() is false for it by design, so
// loadPalette's fallback never mistakes it for a loaded theme.
func monoPalette() theme.Palette {
	return theme.Palette{
		Accent: "",
		Bright: "\x1b[1m",
		Text:   "",
		Muted:  "",
		On:     "\x1b[1m",
		Amber:  "\x1b[1m",
		SelBg:  "\x1b[7m",
		Bold:   "\x1b[1m",
		Dim:    "",
		Err:    "\x1b[1m",
	}
}

func (l1 *level1Runtime) quantizePalette(p theme.Palette) theme.Palette {
	if l1.profile.ColorDepth == 0 {
		return monoPalette()
	}
	if l1.profile.ColorDepth > 16 {
		return p
	}
	quant := func(seq string, isBg bool) string {
		if idx, ok := parse256SGR(seq); ok {
			return ansi16From256(idx, isBg)
		}
		return seq
	}
	return theme.Palette{
		Accent: quant(p.Accent, false),
		Bright: quant(p.Bright, false),
		Text:   quant(p.Text, false),
		Muted:  quant(p.Muted, false),
		On:     quant(p.On, false),
		Amber:  quant(p.Amber, false),
		SelBg:  quant(p.SelBg, true),
		Bold:   p.Bold,
		Dim:    p.Dim,
		Err:    quant(p.Err, false),
	}
}

var asciiReplacer = strings.NewReplacer(
	"╭", "+", "╮", "+", "╰", "+", "╯", "+",
	"┌", "+", "┐", "+", "└", "+", "┘", "+",
	"─", "-", "━", "=", "│", "|", "┃", "|",
	"▸", ">", "▾", "v", "‹", "<", "›", ">",
	"●", "*", "○", "o", "◉", "(*)", "☐", "[ ]", "☑", "[x]",
	"█", "#", "░", ".", "┃", "|", "◈", "*", "…", "~",
	"✓", "+", "✕", "x",
)

func (l1 *level1Runtime) sanitizeGlyphs(s string) string {
	if !l1.profile.ASCIIOnly {
		return s
	}
	return asciiReplacer.Replace(s)
}

// level2Runtime embeds level1Runtime and adds alternate screen, SGR mouse
// (?1000h/?1006h), and Bracketed Paste (?2004h) sequences.
type level2Runtime struct {
	level1Runtime
}

func (l2 *level2Runtime) enterSeq() string {
	var b strings.Builder
	if l2.profile.AltScreen {
		b.WriteString(esc + "?1049h")
	}
	b.WriteString(esc + "?25l")
	if l2.profile.SGRMouse {
		b.WriteString(esc + "?1000h" + esc + "?1006h")
	}
	if l2.profile.BracketedPaste {
		b.WriteString(esc + "?2004h")
	}
	b.WriteString(esc + "2J" + esc + "H")
	return b.String()
}

func (l2 *level2Runtime) leaveSeq() string {
	var b strings.Builder
	if l2.profile.BracketedPaste {
		b.WriteString(esc + "?2004l")
	}
	if l2.profile.SGRMouse {
		b.WriteString(esc + "?1006l" + esc + "?1000l")
	}
	b.WriteString(esc + "?25h")
	if l2.profile.AltScreen {
		b.WriteString(esc + "?1049l")
	}
	b.WriteString(Reset)
	return b.String()
}

// level3Runtime embeds level2Runtime and adds ?2026 Synchronized Output
// frame-tearing elimination and 24-bit TrueColor pass-through.
type level3Runtime struct {
	level2Runtime
}

func (l3 *level3Runtime) wrapFrame(frame string) string {
	frame = l3.sanitizeGlyphs(frame)
	if !l3.profile.SyncOutput {
		return frame
	}
	return esc + "?2026h" + frame + esc + "?2026l"
}

// level4Runtime embeds level3Runtime and adds Mode 2027 Unicode grapheme
// clustering, Kitty CSI-u keyboard disambiguation, and OSC 52 clipboard copy.
type level4Runtime struct {
	level3Runtime
}

func (l4 *level4Runtime) enterSeq() string {
	s := l4.level2Runtime.enterSeq()
	if l4.profile.GraphemeCluster {
		s += esc + "?2027h"
	}
	return s
}

func (l4 *level4Runtime) leaveSeq() string {
	var prefix string
	if l4.profile.GraphemeCluster {
		prefix += esc + "?2027l"
	}
	return prefix + l4.level2Runtime.leaveSeq()
}

// level5Runtime embeds level4Runtime and adds multiplexer/editor awareness
// (tmux DCS passthrough and terminal focus-in auto-refresh).
type level5Runtime struct {
	level4Runtime
}

func (l5 *level5Runtime) enterSeq() string {
	s := l5.level4Runtime.enterSeq()
	if l5.profile.FocusEvents {
		s += esc + "?1004h"
	}
	return s
}

func (l5 *level5Runtime) leaveSeq() string {
	var prefix string
	if l5.profile.FocusEvents {
		prefix += esc + "?1004l"
	}
	return prefix + l5.level4Runtime.leaveSeq()
}

var rt = func() *level5Runtime {
	r := &level5Runtime{}
	r.profile = loadOrProbeTermProfile(false)
	r.initSignals()
	return r
}()

// ── 256 -> 16 Colour Quantiser Helpers ──────────────────────────────────────

func parse256SGR(seq string) (idx int, ok bool) {
	if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
		return 0, false
	}
	body := seq[2 : len(seq)-1]
	parts := strings.Split(body, ";")
	if len(parts) != 3 || (parts[0] != "38" && parts[0] != "48") || parts[1] != "5" {
		return 0, false
	}
	n, err := strconv.Atoi(parts[2])
	return n, err == nil
}

func ansi16From256(idx int, isBg bool) string {
	base := 37
	if isBg {
		base = 40
	}
	switch {
	case idx == 237 || idx == 236 || idx == 238:
		if isBg {
			return "\x1b[100m"
		}
		return "\x1b[90m"
	case idx == 114:
		return "\x1b[92m"
	case idx == 214:
		return "\x1b[93m"
	case idx == 203:
		return "\x1b[91m"
	case idx == 141 || idx == 183:
		return "\x1b[95m"
	case idx == 245:
		return "\x1b[90m"
	default:
		return fmt.Sprintf("\x1b[%dm", base)
	}
}

// ── baseRuntime LineBuffer Engine (Unicode + Cursor Movement + Paste) ────────

// lineBuffer manages a single-line Unicode text editor with grapheme-cluster
// cursor movement, horizontal viewport scrolling, word/line deletion, and
// bracketed paste insertion.
type lineBuffer struct {
	clusters []string // grapheme clusters
	cursor   int      // index in [0..len(clusters)]
}

func newLineBuffer(initial string) *lineBuffer {
	c := util.Chars(initial)
	return &lineBuffer{
		clusters: c,
		cursor:   len(c),
	}
}

func (lb *lineBuffer) setString(s string) {
	lb.clusters = util.Chars(s)
	lb.cursor = len(lb.clusters)
}

func (lb *lineBuffer) String() string {
	return strings.Join(lb.clusters, "")
}

func (lb *lineBuffer) insertText(s string) {
	// Strip control newlines/tabs inside single-line inputs while keeping all printable Unicode.
	var clean strings.Builder
	for _, r := range s {
		if r == '\r' || r == '\n' || r == '\t' {
			clean.WriteByte(' ')
		} else if r >= 32 && r != 0x7f {
			clean.WriteRune(r)
		}
	}
	added := util.Chars(clean.String())
	if len(added) == 0 {
		return
	}
	next := make([]string, 0, len(lb.clusters)+len(added))
	next = append(next, lb.clusters[:lb.cursor]...)
	next = append(next, added...)
	next = append(next, lb.clusters[lb.cursor:]...)
	lb.clusters = next
	lb.cursor += len(added)
}

func (lb *lineBuffer) handleKey(k *Key) (handled bool) {
	if k == nil {
		return false
	}
	if k.paste != "" {
		lb.insertText(k.paste)
		return true
	}
	switch k.Name {
	case "left", "\x02":
		if lb.cursor > 0 {
			lb.cursor--
		}
		return true
	case "right", "\x06":
		if lb.cursor < len(lb.clusters) {
			lb.cursor++
		}
		return true
	case "home", "\x01":
		lb.cursor = 0
		return true
	case "end", "\x05":
		lb.cursor = len(lb.clusters)
		return true
	case "\x7f", "\x08": // Backspace
		if lb.cursor > 0 {
			lb.clusters = append(lb.clusters[:lb.cursor-1], lb.clusters[lb.cursor:]...)
			lb.cursor--
		}
		return true
	case "delete", "\x04": // Delete forward
		if lb.cursor < len(lb.clusters) {
			lb.clusters = append(lb.clusters[:lb.cursor], lb.clusters[lb.cursor+1:]...)
		}
		return true
	case "\x15": // Ctrl+U: clear to start of line
		if lb.cursor > 0 {
			lb.clusters = lb.clusters[lb.cursor:]
			lb.cursor = 0
		}
		return true
	case "\x0b": // Ctrl+K: clear to end of line
		if lb.cursor < len(lb.clusters) {
			lb.clusters = lb.clusters[:lb.cursor]
		}
		return true
	case "\x17": // Ctrl+W: delete previous word
		if lb.cursor > 0 {
			i := lb.cursor
			for i > 0 && strings.TrimSpace(lb.clusters[i-1]) == "" {
				i--
			}
			for i > 0 && strings.TrimSpace(lb.clusters[i-1]) != "" {
				i--
			}
			lb.clusters = append(lb.clusters[:i], lb.clusters[lb.cursor:]...)
			lb.cursor = i
		}
		return true
	}
	if k.IsText() {
		lb.insertText(k.Name)
		return true
	}
	return false
}

// renderViewport renders the line buffer within maxCols terminal cells, keeping
// the cursor visible and drawing a highlighted cursor block at the active position.
func (lb *lineBuffer) renderViewport(maxCols int, mask string) string {
	if maxCols < 2 {
		maxCols = 2
	}
	n := len(lb.clusters)
	display := make([]string, n)
	widths := make([]int, n)
	for i, c := range lb.clusters {
		if mask != "" {
			display[i] = mask
			widths[i] = 1
		} else {
			display[i] = c
			w := VLen(c)
			if w <= 0 {
				w = 1
			}
			widths[i] = w
		}
	}

	// Compute horizontal scroll window [start..end) so lb.cursor is always visible.
	budget := maxCols - 1 // reserve 1 cell when cursor is at end-of-line
	start := 0
	if lb.cursor > 0 {
		cols := 0
		for i := lb.cursor - 1; i >= 0; i-- {
			if cols+widths[i] > budget-1 {
				start = i + 1
				break
			}
			cols += widths[i]
		}
	}

	var b strings.Builder
	used := 0
	for i := start; i < n; i++ {
		if used+widths[i] > maxCols {
			break
		}
		if i == lb.cursor {
			b.WriteString(C.Accent + C.Bold + "▏" + Reset + C.SelBg + C.Bright + display[i])
		} else {
			b.WriteString(display[i])
		}
		used += widths[i]
	}
	if lb.cursor >= n {
		b.WriteString(C.Accent + "▏" + Reset)
	} else {
		b.WriteString(Reset)
	}
	return b.String()
}

// ── baseRuntime VirtualList Engine (100k+ items, zero-alloc containsFold) ────

// containsFold performs zero-allocation Unicode case-insensitive substring matching.
func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	firstRune, firstLen := utf8.DecodeRuneInString(needle)
	restNeedle := needle[firstLen:]
	for i := 0; i < len(haystack); {
		r, w := utf8.DecodeRuneInString(haystack[i:])
		if runesEqualFold(r, firstRune) {
			if hasPrefixFold(haystack[i+w:], restNeedle) {
				return true
			}
		}
		i += w
	}
	return false
}

func hasPrefixFold(s, prefix string) bool {
	for len(prefix) > 0 {
		if len(s) == 0 {
			return false
		}
		sr, sw := utf8.DecodeRuneInString(s)
		pr, pw := utf8.DecodeRuneInString(prefix)
		if !runesEqualFold(sr, pr) {
			return false
		}
		s = s[sw:]
		prefix = prefix[pw:]
	}
	return true
}

func runesEqualFold(a, b rune) bool {
	if a == b {
		return true
	}
	if a < utf8.RuneSelf && b < utf8.RuneSelf {
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		return a == b
	}
	return unicode.SimpleFold(a) == b || unicode.ToLower(a) == unicode.ToLower(b)
}

// virtualList manages a filtered viewport over 100k+ Choices with
// incremental narrowing when the query extends the previous query.
type virtualList struct {
	all         []Choice
	filteredIdx []int
	lastQuery   string
	selectedSet map[string]bool
}

func newVirtualList(items []Choice, preselected []string) *virtualList {
	idx := make([]int, len(items))
	for i := range items {
		idx[i] = i
	}
	sel := map[string]bool{}
	for _, v := range preselected {
		if v != "" {
			sel[v] = true
		}
	}
	return &virtualList{
		all:         items,
		filteredIdx: idx,
		selectedSet: sel,
	}
}

func (vl *virtualList) filter(query string) {
	if query == vl.lastQuery {
		return
	}
	if query == "" {
		idx := make([]int, len(vl.all))
		for i := range vl.all {
			idx[i] = i
		}
		vl.filteredIdx = idx
		vl.lastQuery = ""
		return
	}
	// Incremental narrowing: if the new query extends lastQuery, only scan the
	// already-matched index slice instead of all 100k+ items.
	if vl.lastQuery != "" && strings.HasPrefix(query, vl.lastQuery) {
		next := make([]int, 0, len(vl.filteredIdx))
		for _, i := range vl.filteredIdx {
			it := vl.all[i]
			if containsFold(it.Label, query) || containsFold(it.Value, query) {
				next = append(next, i)
			}
		}
		vl.filteredIdx = next
		vl.lastQuery = query
		return
	}
	next := make([]int, 0, len(vl.all)/2+1)
	for i, it := range vl.all {
		if containsFold(it.Label, query) || containsFold(it.Value, query) {
			next = append(next, i)
		}
	}
	vl.filteredIdx = next
	vl.lastQuery = query
}

func (vl *virtualList) count() int {
	return len(vl.filteredIdx)
}

func (vl *virtualList) itemAt(pos int) Choice {
	if pos < 0 || pos >= len(vl.filteredIdx) {
		return Choice{}
	}
	return vl.all[vl.filteredIdx[pos]]
}

func (vl *virtualList) toggleAt(pos int) {
	if pos < 0 || pos >= len(vl.filteredIdx) {
		return
	}
	it := vl.all[vl.filteredIdx[pos]]
	val := it.Value
	if val == "" {
		val = it.Label
	}
	if vl.selectedSet[val] {
		delete(vl.selectedSet, val)
	} else {
		vl.selectedSet[val] = true
	}
}

func (vl *virtualList) hasSelected() bool {
	return len(vl.selectedSet) > 0
}

func (vl *virtualList) selectedValues() []string {
	var out []string
	for _, it := range vl.all {
		v := it.Value
		if v == "" {
			v = it.Label
		}
		if vl.selectedSet[v] {
			out = append(out, v)
		}
	}
	return out
}

// fastReadDirEntries reads a directory using os.ReadDir (getdents64 d_type on
// Linux without per-file stat() syscalls), sorting directories first and
// scaling cleanly to 100,000+ entries.
func fastReadDirEntries(dir string, showHidden bool, dirsOnly bool) (resolvedDir string, items []Choice, err error) {
	resolvedDir = util.ExpandUser(strings.TrimSpace(dir))
	if resolvedDir == "" {
		resolvedDir = "."
	}
	if abs, errAbs := filepath.Abs(resolvedDir); errAbs == nil {
		resolvedDir = abs
	}
	entries, err := os.ReadDir(resolvedDir)
	if err != nil {
		return resolvedDir, nil, err
	}

	type rawEntry struct {
		name      string
		isDir     bool
		isSymlink bool
	}
	dirs := make([]rawEntry, 0, len(entries)/4+4)
	files := make([]rawEntry, 0, len(entries))

	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		typ := e.Type()
		isDir := typ.IsDir()
		isSym := typ&os.ModeSymlink != 0
		if isDir {
			dirs = append(dirs, rawEntry{name: name, isDir: true})
		} else if !dirsOnly || isSym {
			files = append(files, rawEntry{name: name, isDir: false, isSymlink: isSym})
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	items = make([]Choice, 0, len(dirs)+len(files)+2)
	parent := filepath.Dir(resolvedDir)
	if parent != resolvedDir {
		items = append(items, Choice{
			Label: "📁 .. (parent directory)",
			Value: parent,
		})
	}
	for _, d := range dirs {
		items = append(items, Choice{
			Label: "📁 " + d.name + "/",
			Value: filepath.Join(resolvedDir, d.name),
		})
	}
	for _, f := range files {
		label := "📄 " + f.name
		if f.isSymlink {
			label = "🔗 " + f.name
		}
		items = append(items, Choice{
			Label: label,
			Value: filepath.Join(resolvedDir, f.name),
		})
	}
	return resolvedDir, items, nil
}
