package appkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// toolHelpTimeout bounds reading a manual or --help. A program cannot make
// the screen wait, and the run is killed when the limit passes.
var toolHelpTimeout = 5 * time.Second

// helpEnvKeep is the environment a program keeps when run for its manual.
// Everything else (tokens, passwords, proxies) is dropped. MANWIDTH is set
// by the capture, not copied from the caller.
var helpEnvKeep = []string{"PATH", "MANPATH", "MANSECT", "TZ"}

var helpCrashSigns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^Traceback \(most recent call last\):`),
	regexp.MustCompile(`error while loading shared libraries`),
	regexp.MustCompile(`(?m)^Exception in thread "`),
	regexp.MustCompile(`(?m)^(?:node:internal/|Error: Cannot find module )`),
	regexp.MustCompile(`(?m)^.*: (?:cannot load such file|\(LoadError\))`),
}

var helpOptionLine = regexp.MustCompile(`(?m)^\s{0,12}-{1,2}[A-Za-z0-9]`)

// helpCapture runs a program for its manual the way mint-gen does: never
// through a shell, in a new session with no terminal and stdin closed, in
// an empty folder that is also HOME, with a cleaned environment, a time
// limit, and never as root. The manual itself is not truncated: the time
// limit is what stops a program that writes forever, and every byte it
// produced before then is kept.
type helpCapture struct {
	dir   string
	width int
	cred  *syscall.Credential
}

func newHelpCapture(width int) (*helpCapture, func(), error) {
	dir, err := os.MkdirTemp("", "mint-help-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if width <= 0 {
		_, width = mint.TermSize()
	}
	if width <= 0 {
		width = 80
	}
	h := &helpCapture{dir: dir, width: width}
	if os.Geteuid() == 0 {
		uid, gid := uint32(65534), uint32(65534)
		if u, err := strconv.ParseUint(os.Getenv("SUDO_UID"), 10, 32); err == nil && u != 0 {
			uid = uint32(u)
			gid = uid
			if g, err := strconv.ParseUint(os.Getenv("SUDO_GID"), 10, 32); err == nil {
				gid = uint32(g)
			}
		}
		if err := os.Chown(dir, int(uid), int(gid)); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		h.cred = &syscall.Credential{Uid: uid, Gid: gid}
	}
	return h, cleanup, nil
}

func (h *helpCapture) env() []string {
	width := h.width
	if width <= 0 {
		width = 80
	}
	env := []string{
		"HOME=" + h.dir, "TMPDIR=" + h.dir,
		"LC_ALL=C", "LANG=C",
		"MANWIDTH=" + strconv.Itoa(width),
		"COLUMNS=" + strconv.Itoa(width),
		"TERM=dumb", "NO_COLOR=1", "PAGER=cat", "MANPAGER=cat",
		"GROFF_NO_SGR=1",
	}
	havePath := false
	for _, k := range helpEnvKeep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
			havePath = havePath || k == "PATH"
		}
	}
	if !havePath {
		env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin")
	}
	return env
}

// command builds a sandboxed command. Stdin is left nil, which is the null
// device: not a terminal, and the first read is EOF.
func (h *helpCapture) command(program string, args ...string) *exec.Cmd {
	cmd := exec.Command(program, args...)
	cmd.Dir = h.dir
	cmd.Env = h.env()
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: h.cred}
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd
}

func (h *helpCapture) wait(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	pgid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(toolHelpTimeout)
	defer timer.Stop()
	var err error
	select {
	case err = <-done:
	case <-timer.C:
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
		err = errors.New("timed out")
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	return err
}

func (h *helpCapture) run(program string, args ...string) (string, error) {
	cmd := h.command(program, args...)
	stdout, stderr := &helpCap{}, &helpCap{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := h.wait(cmd)
	out, errOut := stdout.String(), stderr.String()
	switch {
	case errOut == "":
		return out, err
	case out == "":
		return errOut, err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + errOut, err
}

// runSeparate is run, but standard error is dropped. man prints the page
// on standard output and warnings on standard error.
func (h *helpCapture) runSeparate(program string, args ...string) (string, string, error) {
	cmd := h.command(program, args...)
	stdout, stderr := &helpCap{}, &helpCap{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := h.wait(cmd)
	return stdout.String(), stderr.String(), err
}

// runStdin is runSeparate with the given text on standard input. col reads
// the formatted manual there. The target program is never started this way.
func (h *helpCapture) runStdin(input, program string, args ...string) (string, error) {
	cmd := h.command(program, args...)
	cmd.Stdin = strings.NewReader(input)
	stdout := &helpCap{}
	cmd.Stdout, cmd.Stderr = stdout, &helpCap{}
	err := h.wait(cmd)
	return stdout.String(), err
}

type helpCap struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *helpCap) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *helpCap) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func helpLooksLikeMan(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(mint.CleanLine(l)) == "NAME" {
			return true
		}
	}
	return false
}

func helpCrashed(text string) bool {
	if strings.TrimSpace(text) == "" || helpOptionLine.MatchString(text) {
		return false
	}
	for _, re := range helpCrashSigns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// helpHit is one captured manual.
type helpHit struct {
	text, how string
}

// manualPage is the man page to read: the tool, or tool-sub when a
// toolkit subcommand is selected (man docker-run).
func manualPage(bin string, prefix []string) string {
	name := filepath.Base(strings.TrimSpace(bin))
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") {
		return ""
	}
	if len(prefix) == 0 {
		return name
	}
	for _, p := range prefix {
		if p == "" || strings.HasPrefix(p, "-") || strings.ContainsAny(p, " \t/") {
			return ""
		}
	}
	return name + "-" + strings.Join(prefix, "-")
}

// manPage reads one manual as plain text. man is paged with cat so it
// does not open a terminal, then col -b strips nroff overstrikes. The
// formatted page is kept when col is missing or rejects it.
func (h *helpCapture) manPage(page string) (string, bool) {
	if page == "" || strings.HasPrefix(page, "-") {
		return "", false
	}
	out, _, _ := h.runSeparate("man", "-P", "cat", page)
	if !helpLooksLikeMan(out) {
		return "", false
	}
	if plain, err := h.runStdin(out, "col", "-b"); err == nil && helpLooksLikeMan(plain) {
		return plain, true
	}
	return out, true
}

// captureToolHelp reads the tool's man page, or its --help, falling back
// to -h when --help prints nothing or crashes. how is "man", "--help" or "-h".
// The words passed to the tool stay --help and -h: those are how the tool
// is asked for its text, not the name of this screen.
func captureToolHelp(bin string) (text, how string) {
	return captureToolHelpArgs(bin, nil, 0)
}

// captureToolHelpArgs is captureToolHelp for a subcommand. prefix is the
// words before --help (the selected subcommand). width is the terminal
// width MANWIDTH is set to; zero uses the current terminal.
// A subcommand is read as man <tool>-<sub>, then <tool> <sub> --help, then
// <tool> <sub> -h. The tool's own manual is read only when prefix is empty.
func captureToolHelpArgs(bin string, prefix []string, width int) (text, how string) {
	name := strings.TrimSpace(bin)
	if name == "" {
		return "", ""
	}
	h, cleanup, err := newHelpCapture(width)
	if err != nil {
		return "", ""
	}
	defer cleanup()

	if page := manualPage(name, prefix); page != "" {
		if text, ok := h.manPage(page); ok {
			return text, "man"
		}
	}
	path := name
	if !strings.ContainsRune(name, '/') {
		if p, err := exec.LookPath(name); err == nil {
			path = p
		}
	}
	try := func(args []string) (string, string, bool) {
		help, _ := h.run(path, args...)
		switch {
		case strings.TrimSpace(help) == "" || helpCrashed(help):
			return "", "", false
		case len(args) > 0 && args[len(args)-1] == "-h":
			return help, "-h", true
		default:
			return help, "--help", true
		}
	}
	if len(prefix) > 0 {
		base := append([]string{}, prefix...)
		if text, how, ok := try(append(append([]string{}, base...), "--help")); ok {
			return text, how
		}
		if text, how, ok := try(append(append([]string{}, base...), "-h")); ok {
			return text, how
		}
		return "", ""
	}
	if text, how, ok := try([]string{"--help"}); ok {
		return text, how
	}
	if text, how, ok := try([]string{"-h"}); ok {
		return text, how
	}
	return "", ""
}

func (a *ToolApp) cachedHelp(prefix []string, width int) (string, string) {
	if width <= 0 {
		_, width = mint.TermSize()
	}
	key := a.Spec.BinaryName() + "\x00" + strings.Join(prefix, "\x00") + "\x00" + strconv.Itoa(width)
	a.mu.Lock()
	if a.helpCache != nil {
		if hit, ok := a.helpCache[key]; ok {
			a.mu.Unlock()
			return hit.text, hit.how
		}
	}
	a.mu.Unlock()
	text, how := captureToolHelpArgs(a.Spec.BinaryName(), prefix, width)
	a.mu.Lock()
	if a.helpCache == nil {
		a.helpCache = map[string]helpHit{}
	}
	a.helpCache[key] = helpHit{text: text, how: how}
	a.mu.Unlock()
	return text, how
}

// manualLines is the captured text as log lines. The whole text is kept:
// ViewLog scrolls it, including sideways.
func manualLines(text string) []mint.LogLine {
	raw := strings.Split(strings.TrimRight(text, "\n"), "\n")
	lines := make([]mint.LogLine, len(raw))
	for i, l := range raw {
		lines[i] = mint.LogLine{Text: mint.CleanLine(l)}
	}
	return lines
}

// ManualScreen opens the full manual in the log viewer. A toolkit tool
// with a selected subcommand opens that subcommand's manual.
func (a *ToolApp) ManualScreen() *mint.Screen {
	prefix := a.helpPrefix()
	_, cols := mint.TermSize()
	text, _ := a.cachedHelp(prefix, cols)
	subject := a.Spec.Name
	if len(prefix) > 0 {
		subject = a.Spec.Name + " " + strings.Join(prefix, " ")
	}
	title := subject + " — Manual"
	if strings.TrimSpace(text) == "" {
		text = "No manual page or help text was found for " + subject + "."
	}
	lines := manualLines(text)
	return mint.ViewLog(title, func() []mint.LogLine { return lines })
}

// HelpScreen is the previous name of ManualScreen.
func (a *ToolApp) HelpScreen() *mint.Screen { return a.ManualScreen() }
