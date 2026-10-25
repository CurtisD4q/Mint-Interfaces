package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type termProfile struct {
	Signature       string `json:"signature"`
	Term            string `json:"term"`
	TermProgram     string `json:"term_program,omitempty"`
	Wrapper         string `json:"wrapper,omitempty"`
	Version         string `json:"version,omitempty"`
	ProbedAt        string `json:"probed_at"`
	Level           int    `json:"level"`
	ColorDepth      int    `json:"color_depth"`
	ASCIIOnly       bool   `json:"ascii_only,omitempty"`
	AltScreen       bool   `json:"alt_screen"`
	SGRMouse        bool   `json:"sgr_mouse"`
	BracketedPaste  bool   `json:"bracketed_paste"`
	SyncOutput      bool   `json:"sync_output"`
	GraphemeCluster bool   `json:"grapheme_cluster"`
	KittyKeyboard   bool   `json:"kitty_keyboard"`
	FocusEvents     bool   `json:"focus_events"`
	OSC52           bool   `json:"osc52"`
	OSC8            bool   `json:"osc8"`
	TmuxPassthrough bool   `json:"tmux_passthrough,omitempty"`
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
	if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
		return filepath.Join(cfg, "mint-interfaces", "termcaps.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mint-interfaces", "termcaps.json")
}

func currentTermSignature() (sig, term, prog, wrapper string) {
	term = os.Getenv("TERM")
	if term == "" {
		term = "xterm-256color"
	}
	prog = os.Getenv("TERM_PROGRAM")
	switch {
	case os.Getenv("TMUX") != "":
		wrapper = "tmux"
	case os.Getenv("NVIM") != "" || os.Getenv("NVIM_LISTEN_ADDRESS") != "":
		wrapper = "nvim"
	case os.Getenv("INSIDE_EMACS") != "":
		wrapper = "emacs"
	case os.Getenv("STY") != "":
		wrapper = "screen"
	case os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != "":
		wrapper = "ssh"
	}
	ct := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	sig = term + "|" + prog + "|" + wrapper + "|" + ct
	return sig, term, prog, wrapper
}

func ensureTermProfile(forceProbe bool) (termProfile, string, bool) {
	sig, term, prog, wrapper := currentTermSignature()
	cachePath := termCapsPath()
	var cache termCacheFile
	if b, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	if cache.Terminals == nil {
		cache.Terminals = make(map[string]termProfile)
	}
	if !forceProbe {
		if prof, ok := cache.Terminals[sig]; ok && prof.Level >= 1 {
			return prof, cachePath, true
		}
	}

	prof := probeCurrentTerminal(sig, term, prog, wrapper)
	cache.UpdatedAt = prof.ProbedAt
	cache.ActiveKey = sig
	cache.Default = prof
	cache.Terminals[sig] = prof
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
		if b, err := json.MarshalIndent(cache, "", "  "); err == nil {
			_ = os.WriteFile(cachePath, b, 0o644)
		}
	}
	return prof, cachePath, false
}

func probeCurrentTerminal(sig, term, prog, wrapper string) termProfile {
	p := termProfile{
		Signature:   sig,
		Term:        term,
		TermProgram: prog,
		Wrapper:     wrapper,
		ProbedAt:    time.Now().UTC().Format(time.RFC3339),
		ColorDepth:  16,
	}
	tLower := strings.ToLower(term)
	pLower := strings.ToLower(prog)

	if term == "dumb" || os.Getenv("NO_COLOR") != "" {
		p.ColorDepth = 2
		if term == "dumb" {
			p.ASCIIOnly = true
		}
		p.Level = 1
		return p
	}
	if term == "linux" || strings.HasPrefix(tLower, "vt10") || strings.HasPrefix(tLower, "vt22") {
		p.ColorDepth = 16
		p.ASCIIOnly = strings.HasPrefix(tLower, "vt1")
		p.AltScreen = term == "linux"
		p.Level = 1
		return p
	}

	if nColors, hasCup := readTerminfoCaps(term); nColors > 0 || hasCup {
		if nColors >= 256 {
			p.ColorDepth = 256
		}
		if hasCup {
			p.AltScreen = true
		}
	}
	if strings.Contains(tLower, "256color") && p.ColorDepth < 256 {
		p.ColorDepth = 256
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if ct == "truecolor" || ct == "24bit" {
		p.ColorDepth = 16777216
	}

	modern := strings.Contains(tLower, "kitty") ||
		strings.Contains(tLower, "ghostty") ||
		strings.Contains(tLower, "wezterm") ||
		strings.Contains(tLower, "alacritty") ||
		strings.Contains(tLower, "foot") ||
		strings.Contains(tLower, "Rio") ||
		strings.Contains(pLower, "kitty") ||
		strings.Contains(pLower, "ghostty") ||
		strings.Contains(pLower, "wezterm") ||
		strings.Contains(pLower, "iterm") ||
		strings.Contains(pLower, "vscode") ||
		os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("WEZTERM_PANE") != "" ||
		os.Getenv("ALACRITTY_WINDOW_ID") != "" ||
		os.Getenv("VTE_VERSION") != "" ||
		os.Getenv("WT_SESSION") != ""

	if modern {
		if ct == "truecolor" || ct == "24bit" {
			p.ColorDepth = 16777216
		} else if p.ColorDepth < 256 {
			p.ColorDepth = 256
		}
		p.AltScreen = true
		p.SGRMouse = true
		p.BracketedPaste = true
		p.SyncOutput = true
		p.FocusEvents = true
		p.OSC52 = true
		p.OSC8 = true
	} else if tLower != "dumb" {
		if p.ColorDepth < 256 {
			p.ColorDepth = 256
		}
		p.AltScreen = true
		p.SGRMouse = true
		p.BracketedPaste = true
	}

	if strings.Contains(tLower, "kitty") || strings.Contains(tLower, "ghostty") ||
		strings.Contains(tLower, "wezterm") || strings.Contains(tLower, "foot") ||
		os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("WEZTERM_PANE") != "" {
		p.GraphemeCluster = true
		p.KittyKeyboard = true
	}

	if raw := queryTTYWire(45 * time.Millisecond); raw != "" {
		if decrqmOK(raw, 1006) {
			p.SGRMouse = true
		}
		if decrqmOK(raw, 2004) {
			p.BracketedPaste = true
		}
		if decrqmOK(raw, 2026) {
			p.SyncOutput = true
		}
		if decrqmOK(raw, 2027) {
			p.GraphemeCluster = true
		}
		if decrqmOK(raw, 1004) {
			p.FocusEvents = true
		}
		if idx := strings.Index(raw, "\x1bP>|"); idx >= 0 {
			rest := raw[idx+4:]
			if end := strings.Index(rest, "\x1b\\"); end >= 0 {
				ver := strings.TrimSpace(rest[:end])
				if ver != "" {
					p.Version = ver
					if ct == "truecolor" || ct == "24bit" {
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

	if wrapper == "tmux" {
		p.TmuxPassthrough = true
		p.KittyKeyboard = false
	}

	switch {
	case p.TmuxPassthrough || (p.Wrapper != "" && p.SyncOutput):
		p.Level = 5
	case p.GraphemeCluster || p.KittyKeyboard || p.OSC52 || p.OSC8:
		p.Level = 4
	case p.ColorDepth >= 16777216 || p.SyncOutput:
		p.Level = 3
	case p.AltScreen && (p.SGRMouse || p.BracketedPaste):
		p.Level = 2
	default:
		p.Level = 1
	}
	return p
}

func decrqmOK(raw string, mode int) bool {
	prefix := "\x1b[?" + strconv.Itoa(mode) + ";"
	idx := strings.Index(raw, prefix)
	if idx < 0 {
		return false
	}
	rest := raw[idx+len(prefix):]
	semi := strings.Index(rest, "$y")
	if semi <= 0 {
		return false
	}
	code, err := strconv.Atoi(rest[:semi])
	if err != nil {
		return false
	}
	return code == 1 || code == 2
}

func queryTTYWire(timeout time.Duration) string {
	var orig syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, 0, syscall.TCGETS, uintptr(unsafe.Pointer(&orig))); e != 0 {
		return ""
	}
	raw := orig
	raw.Iflag &^= syscall.ICRNL | syscall.IXON
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG
	raw.Cc[syscall.VMIN] = 0
	raw.Cc[syscall.VTIME] = 0
	const tcsetsw = 0x5403
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, 0, tcsetsw, uintptr(unsafe.Pointer(&raw))); e != 0 {
		return ""
	}
	defer func() {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, 0, tcsetsw, uintptr(unsafe.Pointer(&orig)))
	}()

	_, _ = os.Stdout.WriteString("\x1b[?1006$p\x1b[?2004$p\x1b[?2026$p\x1b[?2027$p\x1b[?1004$p\x1b[>0q\x1b[c")
	deadline := time.Now().Add(timeout)
	var buf strings.Builder
	tmp := make([]byte, 256)
	for time.Now().Before(deadline) {
		n, _ := syscall.Read(0, tmp)
		if n > 0 {
			buf.Write(tmp[:n])
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

func readTerminfoCaps(term string) (colors int, hasCup bool) {
	if term == "" {
		return 0, false
	}
	first := string(term[0])
	hexFirst := fmt.Sprintf("%02x", term[0])
	var dirs []string
	if ti := os.Getenv("TERMINFO"); ti != "" {
		dirs = append(dirs, ti)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".terminfo"))
	}
	dirs = append(dirs, "/etc/terminfo", "/lib/terminfo", "/usr/share/terminfo")
	for _, d := range dirs {
		for _, sub := range []string{first, hexFirst} {
			p := filepath.Join(d, sub, term)
			b, err := os.ReadFile(p)
			if err != nil || len(b) < 12 {
				continue
			}
			magic := int(binary.LittleEndian.Uint16(b[0:2]))
			numWidth := 2
			if magic == 01036 {
				numWidth = 4
			} else if magic != 0432 {
				continue
			}
			nameSize := int(binary.LittleEndian.Uint16(b[2:4]))
			boolCount := int(binary.LittleEndian.Uint16(b[4:6]))
			numCount := int(binary.LittleEndian.Uint16(b[6:8]))
			offset := 12 + nameSize + boolCount
			if offset%2 != 0 {
				offset++
			}
			const maxColorsIdx = 13
			if numCount > maxColorsIdx && offset+(maxColorsIdx+1)*numWidth <= len(b) {
				pos := offset + maxColorsIdx*numWidth
				if numWidth == 4 {
					v := int32(binary.LittleEndian.Uint32(b[pos : pos+4]))
					if v > 0 {
						colors = int(v)
					}
				} else {
					v := int16(binary.LittleEndian.Uint16(b[pos : pos+2]))
					if v > 0 {
						colors = int(v)
					}
				}
			}
			hasCup = bytes.Contains(b, []byte("\x1b["))
			return colors, hasCup
		}
	}
	return 0, false
}

func formatTermSummary(p termProfile, cached bool) string {
	var caps []string
	switch {
	case p.ColorDepth >= 16777216:
		caps = append(caps, "TrueColor")
	case p.ColorDepth >= 256:
		caps = append(caps, "256-color")
	default:
		caps = append(caps, fmt.Sprintf("%d-color", p.ColorDepth))
	}
	if p.SGRMouse {
		caps = append(caps, "SGR-mouse")
	}
	if p.BracketedPaste {
		caps = append(caps, "bracketed-paste")
	}
	if p.SyncOutput {
		caps = append(caps, "sync-output(?2026)")
	}
	if p.GraphemeCluster {
		caps = append(caps, "graphemes(?2027)")
	}
	if p.KittyKeyboard {
		caps = append(caps, "CSI-u")
	}
	if p.OSC52 {
		caps = append(caps, "OSC52")
	}
	if p.Wrapper != "" {
		caps = append(caps, "wrapper="+p.Wrapper)
	}
	src := "probed & cached"
	if cached {
		src = "cached"
	}
	return fmt.Sprintf("Level %d (%s: %s) [%s]", p.Level, p.Term, strings.Join(caps, ", "), src)
}
