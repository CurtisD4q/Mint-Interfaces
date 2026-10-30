// Package util holds dependency-free helpers: safe command running, files,
// a small JSON wrapper, and the UTF-8 width helpers the UI uses.
//
// Derived from Terminal Settings Manager (Curtis, Apache-2.0):
// https://github.com/CurtisD4q/Terminal-Settings-Manager
//
// No settings logic lives here.
package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// asciiSpace is ASCII whitespace: space, \t, \n, \v, \f, \r.
const asciiSpace = " \t\n\v\f\r"

// Trim removes leading and trailing ASCII whitespace.
func Trim(s string) string { return strings.Trim(s, asciiSpace) }

// TrimLeft and TrimRight are the one-sided versions of Trim.
func TrimLeft(s string) string  { return strings.TrimLeft(s, asciiSpace) }
func TrimRight(s string) string { return strings.TrimRight(s, asciiSpace) }

// IsSpace reports whether a single character is ASCII whitespace.
func IsSpace(ch string) bool { return len(ch) == 1 && strings.IndexByte(asciiSpace, ch[0]) >= 0 }

// ShQuote POSIX single-quotes s. The one shell-escaping helper in this tree.
func ShQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func capture(cmd *exec.Cmd) (string, error) {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil // discarded, like 2>/dev/null
	err := cmd.Run()
	if err == nil {
		return out.String(), nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), fmt.Errorf("exit %d", ee.ExitCode())
	}
	return out.String(), errors.New("cannot spawn")
}

// Run runs a program with each argument passed as data, never parsed by a
// shell. It returns stdout and an error of the form "exit N" on failure.
func Run(program string, args ...string) (string, error) {
	return capture(exec.Command(program, args...))
}

// ErrTimeout is returned by RunTimeout when the program was stopped for
// running too long.
var ErrTimeout = errors.New("timed out")

// MaxCapture is how much stdout RunTimeout keeps; the rest is discarded.
const MaxCapture = 16 << 20

// RunTimeout is Run with a time limit. The program runs in its own session,
// with no terminal and stdin closed, so it can never wait for input or draw
// over the interface; when the limit passes, it and every process it started
// are killed and the output so far is returned with ErrTimeout.
func RunTimeout(timeout time.Duration, program string, args ...string) (string, error) {
	cmd := exec.Command(program, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.WaitDelay = time.Second
	out := &limitedBuffer{max: MaxCapture}
	cmd.Stdout = out
	if err := cmd.Start(); err != nil {
		return "", errors.New("cannot spawn")
	}
	pid := cmd.Process.Pid
	liveMu.Lock()
	live[pid] = true
	liveMu.Unlock()
	defer func() {
		liveMu.Lock()
		delete(live, pid)
		liveMu.Unlock()
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}
	select {
	case err := <-done:
		if err == nil || errors.Is(err, exec.ErrWaitDelay) {
			return out.String(), nil
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), fmt.Errorf("exit %d", ee.ExitCode())
		}
		return out.String(), err
	case <-timer:
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return out.String(), ErrTimeout
	}
}

var (
	liveMu sync.Mutex
	live   = map[int]bool{} // process groups started by RunTimeout and still running
)

// StopAll kills every command RunTimeout started that is still running,
// with everything it started. Call it when the interface closes, as with
// runner.StopAll, so no command outlives the app.
func StopAll() {
	liveMu.Lock()
	defer liveMu.Unlock()
	for pid := range live {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// RunOK reports whether the program exited 0.
func RunOK(program string, args ...string) bool {
	_, err := Run(program, args...)
	return err == nil
}

// RunValue returns trimmed stdout, or ok=false on failure or empty output.
func RunValue(program string, args ...string) (string, bool) {
	out, err := Run(program, args...)
	return value(out, err)
}

// RunLines returns the non-empty trimmed lines of stdout.
func RunLines(program string, args ...string) []string {
	out, err := Run(program, args...)
	return lines(out, err)
}

func value(out string, err error) (string, bool) {
	if err != nil {
		return "", false
	}
	t := Trim(out)
	return t, t != ""
}

func lines(out string, err error) []string {
	res := []string{}
	if err != nil {
		return res
	}
	for _, line := range strings.Split(out, "\n") {
		if t := Trim(line); t != "" {
			res = append(res, t)
		}
	}
	return res
}

// ReadFile returns the whole file as a string.
func ReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("cannot open")
	}
	return string(b), nil
}

// WriteFile replaces the file's contents.
func WriteFile(path, data string) error {
	return os.WriteFile(path, []byte(data), 0o644)
}

// ReadLine returns the first line of a file, trimmed.
func ReadLine(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("cannot open")
	}
	if len(b) == 0 {
		return "", errors.New("empty")
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return Trim(line), nil
}

// PathExists reports whether anything exists at path.
func PathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Mkdir creates path and its parents.
func Mkdir(path string) error {
	if PathExists(path) {
		return nil
	}
	return os.MkdirAll(path, 0o755)
}

// Spawn starts argv in the background in its own process group, with stdout
// and stderr going to outfile. The leader pid is written to outfile+".pid".
// It returns immediately.
func Spawn(argv []string, outfile string) bool {
	if len(argv) == 0 {
		return false
	}
	f, err := os.Create(outfile)
	if err != nil {
		return false
	}
	defer f.Close()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = f, f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return false
	}
	_ = os.WriteFile(outfile+".pid", []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644)
	go cmd.Wait() // reap; the child keeps running independently
	return true
}

// ReadPID reads a pid file written by Spawn.
func ReadPID(pidfile string) (int, bool) {
	line, err := ReadLine(pidfile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(line)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// PIDAlive reports whether /proc/<pid> exists.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); err == nil {
		return true
	}
	return syscall.Kill(pid, 0) == nil
}

// Kill sends SIGTERM to the process group, then to the pid.
func Kill(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

var (
	haveMu    sync.Mutex
	haveCache = map[string]bool{}
)

// Have reports whether a program is on PATH (cached).
func Have(bin string) bool {
	haveMu.Lock()
	defer haveMu.Unlock()
	if v, ok := haveCache[bin]; ok {
		return v
	}
	_, err := exec.LookPath(bin)
	haveCache[bin] = err == nil
	return haveCache[bin]
}

// ForgetHave clears the Have cache.
func ForgetHave() {
	haveMu.Lock()
	haveCache = map[string]bool{}
	haveMu.Unlock()
}

// DetectTools fills the Have cache for several programs at once.
func DetectTools(names []string) {
	haveMu.Lock()
	defer haveMu.Unlock()
	for _, n := range names {
		_, err := exec.LookPath(n)
		haveCache[n] = err == nil
	}
}

// ── JSON ─────────────────────────────────────────────────────────────────────

// JSONDecode parses s into maps, slices, strings, float64s and bools.
func JSONDecode(s string) (any, error) {
	if s == "" {
		return nil, errors.New("empty input")
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errors.New("malformed json")
	}
	return v, nil
}

// JSONEncode encodes v compactly with sorted map keys and no HTML escaping.
func JSONEncode(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// ── width ────────────────────────────────────────────────────────────────────

// RuneWidth returns the terminal column width (0, 1, or 2) of a Unicode
// code point following East Asian Width / WCWidth rules, as the active Width
// measures it.
func RuneWidth(r rune) int { return activeWidth().RuneWidth(r) }

// RuneWidth returns the column width of r under w.
func (w Width) RuneWidth(r rune) int {
	if r < 32 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if r == 0x200b || r == 0x200c || r == 0x200d || r == 0xfeff {
		return 0
	}
	if (r >= 0xfe00 && r <= 0xfe0f) || (r >= 0xe0100 && r <= 0xe01ef) {
		return 0
	}
	if (r >= 0x0300 && r <= 0x036f) || (r >= 0x1ab0 && r <= 0x1aff) ||
		(r >= 0x1dc0 && r <= 0x1dff) || (r >= 0x20d0 && r <= 0x20ff) ||
		(r >= 0xfe20 && r <= 0xfe2f) {
		return 0
	}
	if w.AmbiguousWide && isAmbiguous(r) {
		return 2
	}
	if (r >= 0x1100 && r <= 0x115f) ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0x303e) ||
		(r >= 0x3040 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1f64f) ||
		(r >= 0x1f680 && r <= 0x1f6ff) ||
		(r >= 0x1f900 && r <= 0x1f9ff) ||
		(r >= 0x1fa70 && r <= 0x1faff) ||
		(r >= 0x20000 && r <= 0x2fffd) ||
		(r >= 0x30000 && r <= 0x3fffd) {
		return 2
	}
	return 1
}

// VLen counts visual terminal columns, accounting for 0-width combining marks
// and 2-column CJK/emoji runes while matching 1-cell per UTF-8 character on
// standard Latin/box-drawing strings, as the active Width measures it.
func VLen(s string) int { return activeWidth().VLen(s) }

// VLen counts the columns of s under w.
func (w Width) VLen(s string) int {
	n := 0
	i := 0
	for i < len(s) {
		b := s[i]
		if b < 0x80 {
			n++
			i++
			continue
		}
		if b < 0xC2 || b > 0xF4 {
			i++
			continue
		}
		r, size := decodeValidUTF8(s[i:])
		if size == 0 {
			i++
			continue
		}
		n += w.RuneWidth(r)
		i += size
	}
	return n
}

func decodeValidUTF8(s string) (rune, int) {
	b := s[0]
	need := 1
	switch {
	case b >= 0xF0:
		need = 4
	case b >= 0xE0:
		need = 3
	case b >= 0xC2:
		need = 2
	default:
		return 0, 0
	}
	if len(s) < need {
		return rune(b), 1
	}
	var r rune
	switch need {
	case 2:
		if s[1] < 0x80 || s[1] > 0xBF {
			return rune(b), 1
		}
		r = rune(b&0x1F)<<6 | rune(s[1]&0x3F)
	case 3:
		if s[1] < 0x80 || s[1] > 0xBF || s[2] < 0x80 || s[2] > 0xBF {
			return rune(b), 1
		}
		r = rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F)
	case 4:
		if s[1] < 0x80 || s[1] > 0xBF || s[2] < 0x80 || s[2] > 0xBF || s[3] < 0x80 || s[3] > 0xBF {
			return rune(b), 1
		}
		r = rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F)
	}
	return r, need
}

// Chars splits s into characters (grapheme clusters: lead UTF-8 rune plus any
// attached 0-width combining marks or ZWJ sequences). Invalid continuation
// bytes are skipped.
func Chars(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		b := s[i]
		if b <= 0x7F || (b >= 0xC2 && b <= 0xF4) {
			j := i + 1
			for j < len(s) && s[j] >= 0x80 && s[j] <= 0xBF {
				j++
			}
			// Also attach any immediately following 0-width combining mark or variation selector.
			for j < len(s) && s[j] >= 0xC2 && s[j] <= 0xF4 {
				nr, nw := decodeValidUTF8(s[j:])
				if nw > 1 && activeWidth().RuneWidth(nr) == 0 {
					j += nw
					if nr == 0x200d && j < len(s) && s[j] >= 0xC2 && s[j] <= 0xF4 {
						_, nw2 := decodeValidUTF8(s[j:])
						if nw2 > 1 {
							j += nw2
						}
					}
					continue
				}
				break
			}
			out = append(out, s[i:j])
			i = j
		} else {
			i++
		}
	}
	return out
}

// Truncate shortens s to n visual cells, ending in "…" when it had to cut.
func Truncate(s string, n int) string { return activeWidth().Truncate(s, n) }

// Truncate cuts s to at most n columns under w, ending it with an ellipsis
// when anything was cut.
func (w Width) Truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	if w.VLen(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	budget := n - 1
	for _, ch := range Chars(s) {
		cw := w.VLen(ch)
		if cw < 1 {
			cw = 1
		}
		if used+cw > budget {
			break
		}
		b.WriteString(ch)
		used += cw
	}
	return b.String() + "…"
}

// ExpandUser replaces a leading "~" with $HOME.
func ExpandUser(path string) string {
	if path == "~" {
		if h := os.Getenv("HOME"); h != "" {
			return h
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		return os.Getenv("HOME") + path[1:]
	}
	return path
}

// Width is how the terminal measures characters. The zero value measures
// East Asian Ambiguous runes (─ ╭ ● … ↑) as one column, which is right in a
// Western locale.
type Width struct {
	// AmbiguousWide measures East Asian Ambiguous runes as two columns, as
	// a terminal in a Japanese, Chinese or Korean locale draws them.
	AmbiguousWide bool
}

var (
	widthMu sync.RWMutex
	width   Width
)

// activeWidth is the Width in force. It is read on every glyph, so it takes
// a read lock; SetWidth runs once, before any screen exists.
func activeWidth() Width {
	widthMu.RLock()
	defer widthMu.RUnlock()
	return width
}

// SetWidth sets the Width the package-level VLen, Truncate and RuneWidth use.
// Call it before a screen starts: nothing re-measures text already laid out.
func SetWidth(w Width) {
	widthMu.Lock()
	defer widthMu.Unlock()
	width = w
}

// isAmbiguous reports whether r is East Asian Ambiguous (Unicode's EAW
// property A): one column in a Western terminal, two in a CJK one. It covers
// the ranges Mint draws from; the rest of the property is not measured.
func isAmbiguous(r rune) bool {
	switch {
	case r >= 0x00a1 && r <= 0x00a1, // ¡
		r >= 0x00a4 && r <= 0x00a4, // ¤
		r >= 0x00a7 && r <= 0x00a8, // § ¨
		r >= 0x00aa && r <= 0x00aa, // ª
		r >= 0x00ad && r <= 0x00ae, // soft hyphen ®
		r >= 0x00b0 && r <= 0x00b4, // ° ± ² ³ ´
		r >= 0x00b6 && r <= 0x00ba, // ¶ · ¸ ¹ º
		r >= 0x00bc && r <= 0x00bf, // ¼ ½ ¾ ¿
		r >= 0x00c6 && r <= 0x00c6, // Æ
		r >= 0x00d0 && r <= 0x00d0, // Ð
		r >= 0x00d7 && r <= 0x00d8, // × Ø
		r >= 0x00de && r <= 0x00e1, // Þ ß à á
		r >= 0x00e6 && r <= 0x00e6, // æ
		r >= 0x00e8 && r <= 0x00ea, // è é ê
		r >= 0x00ec && r <= 0x00ed, // ì í
		r >= 0x00f0 && r <= 0x00f0, // ð
		r >= 0x00f2 && r <= 0x00f3, // ò ó
		r >= 0x00f7 && r <= 0x00fa, // ÷ ø ù ú
		r >= 0x00fc && r <= 0x00fc, // ü
		r >= 0x00fe && r <= 0x00fe: // þ
		return true
	case r >= 0x0391 && r <= 0x03a9, // Greek capitals
		r >= 0x03b1 && r <= 0x03c9, // Greek small
		r >= 0x0401 && r <= 0x0401, // Cyrillic
		r >= 0x0410 && r <= 0x044f,
		r >= 0x0451 && r <= 0x0451:
		return true
	case r >= 0x2010 && r <= 0x2010, // hyphen
		r >= 0x2013 && r <= 0x2016, // – — ― ‖
		r >= 0x2018 && r <= 0x2019, // ‘ ’
		r >= 0x201c && r <= 0x201d, // “ ”
		r >= 0x2020 && r <= 0x2022, // † ‡ •
		r >= 0x2024 && r <= 0x2027, // ․ ‥ … ‧
		r >= 0x2030 && r <= 0x2030, // ‰
		r >= 0x2032 && r <= 0x2033, // ′ ″
		r >= 0x2035 && r <= 0x2035, // ‵
		r >= 0x203b && r <= 0x203b, // ※
		r >= 0x203e && r <= 0x203e, // ‾
		r >= 0x2074 && r <= 0x2074, // ⁴
		r >= 0x207f && r <= 0x207f, // ⁿ
		r >= 0x2081 && r <= 0x2084, // ₁ ₂ ₃ ₄
		r >= 0x20ac && r <= 0x20ac, // €
		r >= 0x2103 && r <= 0x2103, // ℃
		r >= 0x2105 && r <= 0x2105, // ℅
		r >= 0x2109 && r <= 0x2109, // ℉
		r >= 0x2113 && r <= 0x2113, // ℓ
		r >= 0x2116 && r <= 0x2116, // №
		r >= 0x2121 && r <= 0x2122, // ℡ ™
		r >= 0x2126 && r <= 0x2126, // Ω
		r >= 0x212b && r <= 0x212b, // Å
		r >= 0x2153 && r <= 0x2154, // ⅓ ⅔
		r >= 0x215b && r <= 0x215e, // ⅛ … ⅞
		r >= 0x2160 && r <= 0x216b, // Roman numerals
		r >= 0x2170 && r <= 0x2179,
		r >= 0x2189 && r <= 0x2189, // ↉
		r >= 0x2190 && r <= 0x2199, // arrows ← ↑ → ↓ …
		r >= 0x21b8 && r <= 0x21b9,
		r >= 0x21d2 && r <= 0x21d2, // ⇒
		r >= 0x21d4 && r <= 0x21d4, // ⇔
		r >= 0x21e7 && r <= 0x21e7, // ⇧
		r >= 0x2200 && r <= 0x2200, // ∀
		r >= 0x2202 && r <= 0x2203, // ∂ ∃
		r >= 0x2207 && r <= 0x2208, // ∇ ∈
		r >= 0x220b && r <= 0x220b, // ∋
		r >= 0x220f && r <= 0x220f, // ∏
		r >= 0x2211 && r <= 0x2211, // ∑
		r >= 0x2215 && r <= 0x2215, // ∕
		r >= 0x221a && r <= 0x221a, // √
		r >= 0x221d && r <= 0x2220, // ∝ ∞ ∟ ∠
		r >= 0x2223 && r <= 0x2223, // ∣
		r >= 0x2225 && r <= 0x2225, // ∥
		r >= 0x2227 && r <= 0x222c, // ∧ ∨ ∩ ∪ ∫ ∬
		r >= 0x222e && r <= 0x222e, // ∮
		r >= 0x2234 && r <= 0x2237, // ∴ ∵ ∶ ∷
		r >= 0x223c && r <= 0x223d, // ∼ ∽
		r >= 0x2248 && r <= 0x2248, // ≈
		r >= 0x224c && r <= 0x224c, // ≌
		r >= 0x2252 && r <= 0x2252, // ≒
		r >= 0x2260 && r <= 0x2261, // ≠ ≡
		r >= 0x2264 && r <= 0x2267, // ≤ ≥ ≦ ≧
		r >= 0x226a && r <= 0x226b, // ≪ ≫
		r >= 0x226e && r <= 0x226f, // ≮ ≯
		r >= 0x2282 && r <= 0x2283, // ⊂ ⊃
		r >= 0x2286 && r <= 0x2287, // ⊆ ⊇
		r >= 0x2295 && r <= 0x2295, // ⊕
		r >= 0x2299 && r <= 0x2299, // ⊙
		r >= 0x22a5 && r <= 0x22a5, // ⊥
		r >= 0x22bf && r <= 0x22bf, // ⊿
		r >= 0x2312 && r <= 0x2312, // ⌒
		r >= 0x2460 && r <= 0x24e9, // circled numbers and letters
		r >= 0x24eb && r <= 0x254b, // boxed and block elements
		r >= 0x2550 && r <= 0x2573, // box drawing (double, mixed)
		r >= 0x2580 && r <= 0x258f, // block elements
		r >= 0x2592 && r <= 0x2595, // ▒ ▓ ▔ ▕
		r >= 0x25b8 && r <= 0x25bb, // ▸ ▹ ► ▻
		r >= 0x25e2 && r <= 0x25e5, // ◢ ◣ ◤ ◥
		r >= 0x25a0 && r <= 0x25a1, // ■ □
		r >= 0x25a3 && r <= 0x25a9, // ▣ … ▩
		r >= 0x25b2 && r <= 0x25b3, // ▲ △
		r >= 0x25b6 && r <= 0x25b7, // ▶ ▷
		r >= 0x25bc && r <= 0x25bd, // ▼ ▽
		r >= 0x25c0 && r <= 0x25c1, // ◀ ◁
		r >= 0x25c6 && r <= 0x25c8, // ◆ ◇ ◈
		r >= 0x25cb && r <= 0x25cb, // ○
		r >= 0x25ce && r <= 0x25d1, // ◎ ● ◐ ◑
		r >= 0x25ef && r <= 0x25ef, // ◯
		r >= 0x2605 && r <= 0x2606, // ★ ☆
		r >= 0x2609 && r <= 0x2609, // ☉
		r >= 0x260e && r <= 0x260f, // ☎ ☏
		r >= 0x2614 && r <= 0x2615, // ☔ ☕
		r >= 0x261c && r <= 0x261c, // ☜
		r >= 0x261e && r <= 0x261e, // ☞
		r >= 0x2640 && r <= 0x2640, // ♀
		r >= 0x2642 && r <= 0x2642, // ♂
		r >= 0x2660 && r <= 0x2661, // ♠ ♡
		r >= 0x2663 && r <= 0x2665, // ♣ ♤ ♥
		r >= 0x2667 && r <= 0x266a, // ♧ ♨ ♩ ♪
		r >= 0x266c && r <= 0x266d, // ♬ ♭
		r >= 0x266f && r <= 0x266f, // ♯
		r >= 0x269e && r <= 0x269f, // ⚞ ⚟
		r >= 0x26be && r <= 0x26bf, // ⚾ ⚿
		r >= 0x26c4 && r <= 0x26cd, // ⛄ … ⛍
		r >= 0x26cf && r <= 0x26e1, // ⛏ … ⛡
		r >= 0x26e3 && r <= 0x26e3, // ⛣
		r >= 0x26e8 && r <= 0x26ff, // ⛨ … ⛿
		r >= 0x273d && r <= 0x273d, // ✽
		r >= 0x2757 && r <= 0x2757, // ❗
		r >= 0x2776 && r <= 0x277f, // ❶ … ❿
		r >= 0x2b55 && r <= 0x2b59, // ⭕ …
		r >= 0x3248 && r <= 0x324f, // ㉈ …
		r >= 0xe000 && r <= 0xf8ff, // private use
		r >= 0xfe00 && r <= 0xfe0f, // variation selectors
		r >= 0xfffd && r <= 0xfffd: // replacement character
		return true
	}
	return false
}
