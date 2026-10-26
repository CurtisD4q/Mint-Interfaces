package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// HelpSource is one piece of documentation mint-gen read about a program.
type HelpSource struct {
	Name string   // "man", "--help", "-h", "--help all", ...
	Args []string // the arguments the program was run with
	Text string
	Why  string `json:",omitempty"` // why it was read, when not obvious ("the help says to run \"dig -h\"")
}

// Limits for reading help. A program cannot make mint-gen wait, fill its
// memory, or keep running after mint-gen has read what it needs.
const (
	helpTimeout  = 5 * time.Second // Python and Node programs can be slow the first time
	helpMaxBytes = 2 << 20         // per run
)

// Environment variables a program keeps when run for its help. Everything
// else (tokens, passwords, proxies, editor and shell settings) is dropped.
var helpEnvKeep = []string{"PATH", "MANPATH", "MANSECT", "TZ"}

// helpVariants are extra ways of asking for the full help. Each is tried
// only when the program's own help mentions it, so mint-gen never runs a
// program with arguments it did not ask for.
var helpVariants = []struct {
	name string
	re   *regexp.Regexp
	args []string
}{
	{"--help all", regexp.MustCompile(`(?:^|[\s"'\x60(])--help[ ]all\b`), []string{"--help", "all"}},
	{"--help=all", regexp.MustCompile(`(?:^|[\s"'\x60(])--help=all\b`), []string{"--help=all"}},
	{"--help-all", regexp.MustCompile(`(?:^|[\s"'\x60(])--help-all\b`), []string{"--help-all"}},
	{"--long-help", regexp.MustCompile(`(?:^|[\s"'\x60(])--long-help\b`), []string{"--long-help"}},
	// ffmpeg's "-h long -- print more options" and zip's "-h2  show more
	// help": only as an option line that says it gives more help.
	{"-h long", regexp.MustCompile(`(?m)^\s*-h long\b.*\bmore (?:options|help)\b`), []string{"-h", "long"}},
	{"-h2", regexp.MustCompile(`(?m)^\s*-h2\b.*\bmore help\b`), []string{"-h2"}},
}

// helpPointerArgs are the only argument lists mint-gen runs because a
// program's help tells the reader to run them ("Use \"dig -h\" for the
// complete list of options"). A pointer naming anything else (a command,
// a file, "| more") is never followed: "dig help" would look up the
// domain "help", and "ngrep help" would start a capture.
var helpPointerArgs = [][]string{
	{"-h"}, {"-?"}, {"-help"}, {"--help"}, {"--usage"}, {"--long-help"},
	{"--help-all"}, {"--help=all"}, {"--help", "all"}, {"help"}, {"help", "all"},
}

// maxHelpRuns caps how many times a program is run for its help.
const maxHelpRuns = 6

// helpPointer is a command the help tells the reader to run.
type helpPointer struct {
	Args  []string
	Quote string // the command as the help wrote it
}

// findHelpPointers finds quoted commands in a program's help that run the
// program itself with only help arguments: "dig -h", `anacron -h',
// 'ps --help <simple|list|all>' (taken as "ps --help all").
func findHelpPointers(name, text string) []helpPointer {
	if name == "" {
		return nil
	}
	re := regexp.MustCompile(`["'\x60\x{2018}\x{201C}](?:[^\s"'\x60]*/)?` + regexp.QuoteMeta(name) + `((?:[ \t]+[^\s"'\x60\x{2019}\x{201D}]+)+)[ \t]*["'\x60\x{2019}\x{201D}]`)
	var out []helpPointer
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(text, 20) {
		var args []string
		for _, f := range strings.Fields(m[1]) {
			if alt := strings.Trim(f, "<>{}[]()"); alt != f && strings.Contains(alt, "|") {
				f = ""
				for _, a := range strings.Split(alt, "|") {
					if a == "all" {
						f = "all"
					}
				}
			}
			args = append(args, f)
		}
		key := strings.Join(args, " ")
		if seen[key] || !isHelpPointerArgs(args) {
			continue
		}
		seen[key] = true
		q := []rune(strings.Trim(m[0], "\"'\x60\u2018\u2019\u201C\u201D"))
		if len(q) > 80 { // a long path before the name; this is only for display
			q = append(q[:79], '…')
		}
		out = append(out, helpPointer{Args: args, Quote: string(q)})
	}
	return out
}

func isHelpPointerArgs(args []string) bool {
	for _, a := range helpPointerArgs {
		if strings.Join(a, "\x00") == strings.Join(args, "\x00") {
			return true
		}
	}
	return false
}

// helpRunner runs programs for their help text, as safely as a normal
// user can: never through a shell, in a new session with no terminal and
// input closed, in an empty folder that is also HOME (so a program that
// writes files on first run cannot touch the user's), with a cleaned
// environment, a time limit and an output cap, and never as root.
type helpRunner struct {
	dir  string
	cred *syscall.Credential
}

// Seams for tests.
var (
	helpEuid   = os.Geteuid
	helpGetenv = os.Getenv
)

func newHelpRunner() (*helpRunner, func(), error) {
	dir, err := os.MkdirTemp("", "mint-gen-help-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	r := &helpRunner{dir: dir}
	if helpEuid() == 0 {
		// Never run an unknown program as root just to read its help:
		// use the user behind sudo, else nobody.
		uid, gid := uint32(65534), uint32(65534)
		if u, err := strconv.ParseUint(helpGetenv("SUDO_UID"), 10, 32); err == nil && u != 0 {
			uid = uint32(u)
			gid = uid
			if g, err := strconv.ParseUint(helpGetenv("SUDO_GID"), 10, 32); err == nil {
				gid = uint32(g)
			}
		}
		if err := os.Chown(dir, int(uid), int(gid)); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		r.cred = &syscall.Credential{Uid: uid, Gid: gid, Groups: []uint32{}}
	}
	return r, cleanup, nil
}

func (r *helpRunner) env() []string {
	env := []string{
		"HOME=" + r.dir, "TMPDIR=" + r.dir,
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

// run runs program with args and returns what it printed (standard output
// and error together), at most helpMaxBytes.
func (r *helpRunner) run(program string, args ...string) (string, error) {
	cmd := exec.Command(program, args...)
	cmd.Dir = r.dir
	cmd.Env = r.env()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Credential: r.cred}
	cmd.WaitDelay = 500 * time.Millisecond
	out := &capBuffer{max: helpMaxBytes}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return "", err
	}
	pgid := cmd.Process.Pid // Setsid: the program leads its own process group
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(helpTimeout)
	defer timer.Stop()
	var err error
	select {
	case err = <-done:
	case <-timer.C:
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
		err = errors.New("timed out")
	}
	// Anything the program started and left behind goes too.
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	return out.String(), err
}

// capBuffer keeps the first max bytes written to it and discards the rest.
type capBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *capBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil // keep the program running normally; extra output is dropped
}

func (b *capBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// resolveProgram finds bin the way a shell would (with mint-gen's own PATH),
// so the runner's reduced PATH never picks a different program.
func resolveProgram(bin string) (string, error) {
	if strings.ContainsRune(bin, '/') {
		return bin, nil
	}
	return exec.LookPath(bin)
}

// CollectHelp reads everything mint-gen can learn about a program: its man
// page, its --help (or -h) output, and the fuller help variants the help
// itself points to (curl's "--help all", "--help-all"…).
func CollectHelp(bin string) []HelpSource {
	srcs, _ := CollectHelpNotes(bin)
	return srcs
}

// CollectHelpNotes is CollectHelp, also returning notes on what could not
// be read and why, for the user to see.
func CollectHelpNotes(bin string) ([]HelpSource, []string) {
	name := toolBaseName(bin)
	var notes []string
	r, cleanup, err := newHelpRunner()
	if err != nil {
		return nil, []string{"could not prepare a safe folder to read the help in: " + err.Error()}
	}
	defer cleanup()

	var out []HelpSource
	if text, _ := r.run("man", "-P", "cat", name); looksLikeManPage(text) {
		out = append(out, HelpSource{Name: "man", Args: []string{"-P", "cat", name}, Text: text})
	}

	path, err := resolveProgram(bin)
	if err != nil {
		return out, append(notes, bin+" was not found on the PATH")
	}
	help, runErr := r.run(path, "--help")
	src := HelpSource{Name: "--help", Args: []string{"--help"}, Text: help}
	if strings.TrimSpace(help) == "" {
		help, runErr = r.run(path, "-h")
		src = HelpSource{Name: "-h", Args: []string{"-h"}, Text: help}
	}
	if strings.TrimSpace(help) == "" {
		if errors.Is(runErr, os.ErrPermission) {
			notes = append(notes, fmt.Sprintf("could not run %s as %s to read its help (permission denied). "+
				"mint-gen never runs a program as root just to read its help: run mint-gen as your normal user, "+
				"or use a copy of the program that user can run.", path, r.user()))
		} else {
			notes = append(notes, path+" printed no help for --help or -h")
		}
		return out, notes
	}
	out = append(out, src)

	// Fuller help the help points to: the variants it mentions, then the
	// commands it says to run. Each argument list is run at most once.
	ran := map[string]bool{"--help": true, src.Name: true}
	runs := 1 + len(ran) // man, --help, and -h when --help printed nothing
	try := func(args []string, why string) {
		key := strings.Join(args, " ")
		if ran[key] || runs >= maxHelpRuns {
			return
		}
		ran[key] = true
		runs++
		if text, _ := r.run(path, args...); strings.TrimSpace(text) != "" && text != help {
			out = append(out, HelpSource{Name: key, Args: args, Text: text, Why: why})
		}
	}
	for _, v := range helpVariants {
		if v.re.MatchString(help) {
			try(v.args, "the help mentions "+v.name)
		}
	}
	for _, p := range findHelpPointers(name, help) {
		try(p.Args, "the help says to run \""+p.Quote+"\"")
	}

	if !hasManPage(out) && helpLineCount(out) < 6 {
		notes = append(notes, fmt.Sprintf("%s has no man page and only a short usage message, so options it does not list there will be missing", name))
	}
	return out, notes
}

func hasManPage(srcs []HelpSource) bool {
	for _, s := range srcs {
		if s.Name == "man" {
			return true
		}
	}
	return false
}

// helpLineCount counts the non-blank lines of help (not man) text.
func helpLineCount(srcs []HelpSource) int {
	n := 0
	for _, s := range srcs {
		if s.Name == "man" {
			continue
		}
		for _, l := range strings.Split(s.Text, "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
	}
	return n
}

// user names who programs run as, for messages.
func (r *helpRunner) user() string {
	if r.cred == nil {
		return "you"
	}
	if r.cred.Uid == 65534 {
		return "nobody"
	}
	return fmt.Sprintf("user %d", r.cred.Uid)
}

func toolBaseName(bin string) string {
	b := strings.TrimSpace(bin)
	if i := strings.LastIndexByte(b, '/'); i >= 0 {
		b = b[i+1:]
	}
	return b
}

// manAndHelp splits collected sources into man text and help text (all
// --help style sources, in order).
func manAndHelp(sources []HelpSource) (man, help string) {
	var h []string
	for _, s := range sources {
		if s.Name == "man" {
			man = s.Text
		} else {
			h = append(h, s.Text)
		}
	}
	return man, strings.Join(h, "\n")
}

// looksLikeManPage reports whether man printed a real manual page (it has
// a NAME section), not a message such as "No manual entry for x" or the
// "This system has been minimized…" stub of minimized Ubuntu images.
func looksLikeManPage(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(mint.CleanLine(l)) == "NAME" {
			return true
		}
	}
	return false
}
