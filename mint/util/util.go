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

// luaSpace is Lua's %s class: space, \t, \n, \v, \f, \r.
const luaSpace = " \t\n\v\f\r"

// Trim removes leading and trailing whitespace (Lua %s).
func Trim(s string) string { return strings.Trim(s, luaSpace) }

// TrimLeft and TrimRight are the one-sided versions of Trim.
func TrimLeft(s string) string  { return strings.TrimLeft(s, luaSpace) }
func TrimRight(s string) string { return strings.TrimRight(s, luaSpace) }

// IsSpace reports whether a single character is Lua %s whitespace.
func IsSpace(ch string) bool { return len(ch) == 1 && strings.IndexByte(luaSpace, ch[0]) >= 0 }

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

// Shell runs a raw shell command. Quote interpolated values with ShQuote.
func Shell(command string) (string, error) {
	return capture(exec.Command("sh", "-c", command))
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

// ShellValue is RunValue for a shell command.
func ShellValue(command string) (string, bool) {
	out, err := Shell(command)
	return value(out, err)
}

// ShellLines is RunLines for a shell command.
func ShellLines(command string) []string {
	out, err := Shell(command)
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
// code point following East Asian Width / WCWidth rules.
func RuneWidth(r rune) int {
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
// standard Latin/box-drawing strings.
func VLen(s string) int {
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
		r, w := decodeValidUTF8(s[i:])
		if w == 0 {
			i++
			continue
		}
		n += RuneWidth(r)
		i += w
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
// bytes are skipped just as in the original Lua pattern.
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
				if nw > 1 && RuneWidth(nr) == 0 {
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
func Truncate(s string, n int) string {
	if n < 1 {
		return ""
	}
	if VLen(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	budget := n - 1
	for _, ch := range Chars(s) {
		w := VLen(ch)
		if w < 1 {
			w = 1
		}
		if used+w > budget {
			break
		}
		b.WriteString(ch)
		used += w
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
