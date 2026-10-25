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

// VLen counts characters the way the original does: every byte that is not
// a UTF-8 continuation byte (0x80–0xBF) starts a new cell.
func VLen(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if b := s[i]; b < 0x80 || b > 0xBF {
			n++
		}
	}
	return n
}

// Chars splits s into characters exactly as the Lua pattern
// "[%z\1-\127\194-\244][\128-\191]*" does: a lead byte (0x00–0x7F or
// 0xC2–0xF4) plus any continuation bytes. Other bytes are skipped.
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
			out = append(out, s[i:j])
			i = j
		} else {
			i++
		}
	}
	return out
}

// Truncate shortens s to n cells, ending in "…" when it had to cut.
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
	count := 0
	for _, ch := range Chars(s) {
		count++
		if count >= n {
			break
		}
		b.WriteString(ch)
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
