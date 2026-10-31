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

const toolHelpMaxBytes = 2 << 20

// helpEnvKeep is the environment a program keeps when run for its help.
// Everything else (tokens, passwords, proxies) is dropped.
var helpEnvKeep = []string{"PATH", "MANPATH", "MANSECT", "TZ"}

var helpCrashSigns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^Traceback \(most recent call last\):`),
	regexp.MustCompile(`error while loading shared libraries`),
	regexp.MustCompile(`(?m)^Exception in thread "`),
	regexp.MustCompile(`(?m)^(?:node:internal/|Error: Cannot find module )`),
	regexp.MustCompile(`(?m)^.*: (?:cannot load such file|\(LoadError\))`),
}

var helpOptionLine = regexp.MustCompile(`(?m)^\s{0,12}-{1,2}[A-Za-z0-9]`)

// helpCapture runs a program for its help the way mint-gen does: never
// through a shell, in a new session with no terminal and stdin closed, in
// an empty folder that is also HOME, with a cleaned environment, a time
// limit and an output cap, and never as root.
type helpCapture struct {
	dir  string
	cred *syscall.Credential
}

func newHelpCapture() (*helpCapture, func(), error) {
	dir, err := os.MkdirTemp("", "mint-help-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	h := &helpCapture{dir: dir}
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
	env := []string{
		"HOME=" + h.dir, "TMPDIR=" + h.dir,
		"LC_ALL=C", "LANG=C", "MANWIDTH=100", "COLUMNS=100",
		"TERM=dumb", "NO_COLOR=1", "PAGER=cat", "MANPAGER=cat",
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

func (h *helpCapture) run(program string, args ...string) (string, error) {
	cmd := exec.Command(program, args...)
	cmd.Dir = h.dir
	cmd.Env = h.env()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: h.cred}
	cmd.WaitDelay = 500 * time.Millisecond
	stdout, stderr := &helpCap{max: toolHelpMaxBytes}, &helpCap{max: toolHelpMaxBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return "", err
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

type helpCap struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *helpCap) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
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

// captureToolHelp reads the tool's man page, or its --help, falling back
// to -h when --help prints nothing or crashes. how is "man", "--help" or "-h".
func captureToolHelp(bin string) (text, how string) {
	name := filepath.Base(strings.TrimSpace(bin))
	h, cleanup, err := newHelpCapture()
	if err != nil {
		return "", ""
	}
	defer cleanup()

	if !strings.HasPrefix(name, "-") {
		if page, _, _ := h.runSeparate("man", "-P", "cat", name); helpLooksLikeMan(page) {
			return page, "man"
		}
	}
	path := bin
	if !strings.ContainsRune(bin, '/') {
		if p, err := exec.LookPath(bin); err == nil {
			path = p
		}
	}
	help, _ := h.run(path, "--help")
	if strings.TrimSpace(help) == "" || helpCrashed(help) {
		alt, _ := h.run(path, "-h")
		if strings.TrimSpace(alt) != "" && !helpCrashed(alt) {
			return alt, "-h"
		}
		if strings.TrimSpace(help) != "" && !helpCrashed(help) {
			return help, "--help"
		}
		return "", ""
	}
	return help, "--help"
}

// runSeparate is run, but standard error is dropped. man prints the page
// on standard output and warnings on standard error.
func (h *helpCapture) runSeparate(program string, args ...string) (string, string, error) {
	cmd := exec.Command(program, args...)
	cmd.Dir = h.dir
	cmd.Env = h.env()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: h.cred}
	cmd.WaitDelay = 500 * time.Millisecond
	stdout, stderr := &helpCap{max: toolHelpMaxBytes}, &helpCap{max: toolHelpMaxBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return "", "", err
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
	return stdout.String(), stderr.String(), err
}

// HelpScreen opens the tool's man page or --help in the log viewer.
func (a *ToolApp) HelpScreen() *mint.Screen {
	text, how := captureToolHelp(a.Spec.BinaryName())
	title := a.Spec.Name + " — help"
	if how != "" {
		title = a.Spec.Name + " — " + how
	}
	if strings.TrimSpace(text) == "" {
		text = "No manual page or help text was found for " + a.Spec.Name + "."
	}
	raw := strings.Split(strings.TrimRight(text, "\n"), "\n")
	lines := make([]mint.LogLine, len(raw))
	for i, l := range raw {
		lines[i] = mint.LogLine{Text: mint.CleanLine(l)}
	}
	return mint.ViewLog(title, func() []mint.LogLine { return lines })
}
