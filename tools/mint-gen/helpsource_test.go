package main

import (
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeHelpTool writes an executable shell script that every user can run
// (the help runner drops root privileges, so tests running as root need
// that), and returns its path.
func fakeHelpTool(t *testing.T, name, body string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mint-gen-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func runForHelp(t *testing.T, program string, args ...string) (string, error) {
	t.Helper()
	r, cleanup, err := newHelpRunner()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	return r.run(program, args...)
}

// The program gets a cleaned environment, and an empty folder as its
// working folder and HOME, which is removed afterwards.
func TestHelpRunnerIsolation(t *testing.T) {
	t.Setenv("MY_SECRET_TOKEN", "hunter22")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "abc")
	tool := fakeHelpTool(t, "envtool", `echo "PWD=$(pwd)"; env; touch "$HOME/created-on-first-run"; echo "HOMEFILES=$(ls -A "$HOME")"`)
	out, err := runForHelp(t, tool, "--help")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, leak := range []string{"MY_SECRET_TOKEN", "hunter22", "AWS_SECRET_ACCESS_KEY"} {
		if strings.Contains(out, leak) {
			t.Fatalf("environment leaked %q:\n%s", leak, out)
		}
	}
	var home, pwd string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "HOME=") {
			home = strings.TrimPrefix(l, "HOME=")
		}
		if strings.HasPrefix(l, "PWD=") {
			pwd = strings.TrimPrefix(l, "PWD=")
		}
	}
	realHome, _ := os.UserHomeDir()
	if home == "" || home == realHome || pwd != home || !strings.Contains(home, "mint-gen-help-") {
		t.Fatalf("HOME=%q PWD=%q", home, pwd)
	}
	if !strings.Contains(out, "HOMEFILES=created-on-first-run") {
		t.Fatalf("the program could not use its own HOME:\n%s", out)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("the temporary HOME was not removed")
	}
	if !strings.Contains(out, "LC_ALL=C") || !strings.Contains(out, "PATH=") {
		t.Fatalf("expected locale and PATH:\n%s", out)
	}
}

// Never as root: as the user behind sudo, else nobody.
func TestHelpRunnerDropsRoot(t *testing.T) {
	tool := fakeHelpTool(t, "idtool", `id -u; id -G`)
	oldEuid, oldGetenv := helpEuid, helpGetenv
	defer func() { helpEuid, helpGetenv = oldEuid, oldGetenv }()

	if os.Geteuid() != 0 {
		// Not root: programs run as this user, unchanged.
		out, _ := runForHelp(t, tool)
		if strings.Fields(out)[0] != strconv.Itoa(os.Getuid()) {
			t.Fatalf("ran as %q", out)
		}
		t.Skip("dropping root needs root; the rest is checked when tests run as root")
	}
	helpGetenv = func(string) string { return "" }
	out, err := runForHelp(t, tool)
	if err != nil || strings.Fields(out)[0] != "65534" {
		t.Fatalf("as root without sudo, ran as %q (%v); want nobody", out, err)
	}
	if f := strings.Fields(out); len(f) > 2 {
		t.Fatalf("kept supplementary groups: %q", out)
	}
	helpGetenv = func(k string) string { return map[string]string{"SUDO_UID": "1234", "SUDO_GID": "1234"}[k] }
	out, err = runForHelp(t, tool)
	if err != nil || strings.Fields(out)[0] != "1234" {
		t.Fatalf("under sudo, ran as %q (%v); want 1234", out, err)
	}
}

// A program that never finishes is stopped, with everything it started.
func TestHelpRunnerTimeoutKillsChildren(t *testing.T) {
	pidFile := filepath.Join(openDir(t), "child.pid")
	tool := fakeHelpTool(t, "hang", `sleep 300 & echo $! > `+pidFile+`; sleep 300`)
	start := time.Now()
	_, err := runForHelp(t, tool, "--help")
	if err == nil || time.Since(start) > helpTimeout+2*time.Second {
		t.Fatalf("not stopped in time: %v after %v", err, time.Since(start))
	}
	b, _ := os.ReadFile(pidFile)
	pid := strings.TrimSpace(string(b))
	if pid == "" {
		t.Fatal("child did not start")
	}
	time.Sleep(200 * time.Millisecond)
	if st, err := os.ReadFile("/proc/" + pid + "/stat"); err == nil && !strings.Contains(string(st), ") Z ") {
		t.Fatalf("child %s still running", pid)
	}
}

// Output beyond the cap is dropped.
func TestHelpRunnerCapsOutput(t *testing.T) {
	tool := fakeHelpTool(t, "chatty", `head -c 6000000 /dev/zero | tr '\0' 'x'`)
	out, _ := runForHelp(t, tool, "--help")
	if len(out) > helpMaxBytes || len(out) < helpMaxBytes/2 {
		t.Fatalf("captured %d bytes, cap %d", len(out), helpMaxBytes)
	}
}

// Names are passed as data, never through a shell.
func TestHelpRunnerNoShell(t *testing.T) {
	marker := filepath.Join(openDir(t), "PWNED")
	tool := fakeHelpTool(t, "tool;touch "+strings.ReplaceAll(marker, "/", "_"), `echo "args: $*"`)
	out, err := runForHelp(t, tool, "--help", "$(touch "+marker+")", "; touch "+marker)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a shell interpreted the arguments")
	}
	if !strings.Contains(out, "$(touch") {
		t.Fatalf("arguments changed on the way: %s", out)
	}
}

// Fuller help is asked for only when the help says it exists.
func TestCollectHelpVariants(t *testing.T) {
	plain := fakeHelpTool(t, "plain", `case "$*" in --help) echo "Usage: plain [options]";; *) echo "UNEXPECTED: $*";; esac`)
	srcs := CollectHelp(plain)
	if len(srcs) == 0 || srcs[len(srcs)-1].Name != "--help" {
		t.Fatalf("sources: %+v", names(srcs))
	}
	for _, s := range srcs {
		if strings.Contains(s.Text, "UNEXPECTED") {
			t.Fatalf("ran with arguments it did not advertise: %s", s.Text)
		}
	}

	full := fakeHelpTool(t, "full", `case "$*" in
--help) echo "Usage: full [options]"; echo "Use \"--help all\" for all options";;
"--help all") echo "  -o, --output <file>  Write to file";;
*) echo "UNEXPECTED: $*";; esac`)
	srcs = CollectHelp(full)
	got := names(srcs)
	if !strings.Contains(got, "--help,--help all") {
		t.Fatalf("sources: %s", got)
	}
	_, help := manAndHelp(srcs)
	if !strings.Contains(help, "--output <file>") || strings.Contains(help, "UNEXPECTED") {
		t.Fatalf("help: %s", help)
	}
}

func names(srcs []HelpSource) string {
	var n []string
	for _, s := range srcs {
		n = append(n, s.Name)
	}
	return strings.Join(n, ",")
}

// openDir is a folder any user can write to, so a test can tell whether the
// program (running as nobody when tests run as root) did something there.
func openDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mint-gen-open-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	os.Chmod(dir, 0o777)
	return dir
}

// A program that exits but leaves something running in the background:
// what it left is stopped too.
func TestHelpRunnerStopsLeftovers(t *testing.T) {
	pidFile := filepath.Join(openDir(t), "child.pid")
	tool := fakeHelpTool(t, "leaver", `sleep 300 </dev/null >/dev/null 2>&1 & echo $! > `+pidFile+`; echo "Usage: leaver"`)
	out, err := runForHelp(t, tool, "--help")
	if err != nil || !strings.Contains(out, "Usage") {
		t.Fatalf("%v %q", err, out)
	}
	b, _ := os.ReadFile(pidFile)
	pid := strings.TrimSpace(string(b))
	time.Sleep(200 * time.Millisecond)
	if st, err := os.ReadFile("/proc/" + pid + "/stat"); pid == "" || (err == nil && !strings.Contains(string(st), ") Z ")) {
		t.Fatalf("background process %q still running", pid)
	}
}

// Messages from man are not man pages.
func TestLooksLikeManPage(t *testing.T) {
	stub := "This system has been minimized by removing packages and content that are\nnot required on a system that users do not log into.\n\nTo restore this content, including manpages, you can run the 'unminimize'\ncommand."
	if looksLikeManPage(stub) || looksLikeManPage("No manual entry for foo\n") || looksLikeManPage("") {
		t.Fatal("a message was taken for a man page")
	}
	page := "DIG(1)    BIND 9    DIG(1)\n\nN\bNA\bAM\bME\bE\n       dig - DNS lookup utility\n"
	if !looksLikeManPage(page) {
		t.Fatal("a real (overstruck) man page was refused")
	}
}

// A program the unprivileged user cannot run is reported, not silently
// skipped (and never run as root instead).
func TestCollectHelpReportsUnreachableProgram(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root, which drops to nobody to read help")
	}
	dir := t.TempDir() // 0700, owned by root: nobody cannot enter it
	tool := filepath.Join(dir, "rootonly")
	os.WriteFile(tool, []byte("#!/bin/sh\necho 'Usage: rootonly'\n"), 0o755)
	srcs, notes := CollectHelpNotes(tool)
	for _, s := range srcs {
		if s.Name != "man" {
			t.Fatalf("help was read anyway: %+v", s)
		}
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "permission denied") || !strings.Contains(notes[0], "never runs a program as root") {
		t.Fatalf("notes: %q", notes)
	}
	if _, notes := CollectHelpNotes("no-such-program-xyz"); len(notes) == 0 || !strings.Contains(notes[0], "not found") {
		t.Fatalf("missing program: %q", notes)
	}
}

func TestFindHelpPointers(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"dig", `Use "dig -h" (or "dig -h | more") for complete list of options`, "-h"},
		{"anacron", "type: `anacron -h' for more information", "-h"},
		{"ps", "Try 'ps --help <simple|list|output|threads|misc|all>'", "--help all"},
		{"tool", "Run ‘/usr/bin/tool --help-all’ for everything", "--help-all"},
		{"svn", "Type 'svn help' for usage.", "help"},
		// Never followed: another program, a word that only contains the
		// name, a command or a file after the name, or a pipe.
		{"dig", `Use "xdig -h" or "other -h"`, ""},
		{"rclone", `Use "rclone help flags" to see the global flags.`, ""},
		{"svn", "Type 'svn help <subcommand>' for help on a subcommand.", ""},
		{"docker", "Run 'docker COMMAND --help' for more information.", ""},
		{"dig", `"dig -h | more"`, ""},
		{"ps", "Try 'ps --help <simple|list>'", ""},
		{"tool", `Use "tool -h; rm -rf ~" now`, ""},
	} {
		var got []string
		for _, p := range findHelpPointers(c.name, c.text) {
			got = append(got, strings.Join(p.Args, " "))
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%s %q: got %q, want %q", c.name, c.text, got, c.want)
		}
	}
}

// A program whose --help is only an error pointing to -h is read through
// -h, and nothing else it mentions is run.
func TestCollectHelpFollowsPointers(t *testing.T) {
	tool := fakeHelpTool(t, "pointy", `case "$*" in
--help) echo "pointy: invalid option -- '-'"; echo "Use \"pointy -h\" for the options, or 'pointy help me', or \"pointy -h | more\"";;
-h) echo "  -o, --output <file>  Write to file";;
*) echo "UNEXPECTED: $*";; esac`)
	srcs := CollectHelp(tool)
	if got := names(srcs); got != "--help,-h" {
		t.Fatalf("sources: %s", got)
	}
	if !strings.Contains(srcs[1].Why, `the help says to run "pointy -h"`) {
		t.Fatalf("why: %q", srcs[1].Why)
	}
	_, help := manAndHelp(srcs)
	if !strings.Contains(help, "--output <file>") || strings.Contains(help, "UNEXPECTED") {
		t.Fatalf("help: %s", help)
	}
}

// However much help a program points to, it is run at most maxHelpRuns
// times.
func TestCollectHelpRunCap(t *testing.T) {
	logDir := openDir(t)
	tool := fakeHelpTool(t, "greedy", `echo "$*" >> `+logDir+`/runs
case "$*" in
--help) echo "Usage: greedy --help all --help-all --help=all --long-help"
  for a in -h -? -help --usage help "help all"; do echo "Try 'greedy $a'"; done;;
*) echo "more help for $*";; esac`)
	CollectHelp(tool)
	b, _ := os.ReadFile(filepath.Join(logDir, "runs"))
	runs := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(runs) > maxHelpRuns-1 { // -1: the man run is not the program
		t.Fatalf("%d runs: %q", len(runs), runs)
	}
	if len(runs) < 3 {
		t.Fatalf("pointers not followed: %q", runs)
	}
}

// A program with no man page and only a usage line gets a note.
func TestShortHelpNote(t *testing.T) {
	tool := fakeHelpTool(t, "tiny9", `echo "usage: tiny9 [-v] file"`)
	_, notes := CollectHelpNotes(tool)
	if len(notes) != 1 || !strings.Contains(notes[0], "only a short usage message") {
		t.Fatalf("notes: %q", notes)
	}
}

// --explain lists the help read and the notes, cleaned of control
// characters (a note can quote a program's own help).
func TestExplainShowsHelpSources(t *testing.T) {
	m := IOModel{
		Tool:      "pointy",
		HelpRead:  describeHelpSources([]HelpSource{{Name: "man"}, {Name: "--help"}, {Name: "-h", Why: "the help says to run \"pointy -h\x1b[2J\""}}),
		HelpNotes: []string{"pointy has no man page\x1b]0;title\x07"},
	}
	var b strings.Builder
	printIOModel(&b, m, appkit.ToolSpec{})
	out := b.String()
	if !strings.Contains(out, `Help:     man page, --help, -h (the help says to run "pointy -h`) || !strings.Contains(out, "Note:     pointy has no man page") {
		t.Fatalf("explain:\n%s", out)
	}
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("control characters reached the terminal: %q", out)
	}
}

// A program that crashes when asked for help (a Python traceback) is not
// read as help: -h is tried, and if that crashes too, a note says so.
func TestCrashIsNotHelp(t *testing.T) {
	tool := fakeHelpTool(t, "crashy", `echo "Traceback (most recent call last):"; echo '  File "/usr/bin/crashy", line 61, in <module>'; echo "ModuleNotFoundError: No module named 'crashy'"; exit 1`)
	srcs, notes := CollectHelpNotes(tool)
	if len(srcs) != 0 {
		t.Fatalf("crash read as help: %s", names(srcs))
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "crashed when asked for its help (a Python error)") {
		t.Fatalf("notes: %q", notes)
	}
	// Crashes on --help but -h works: -h is read.
	half := fakeHelpTool(t, "halfcrash", `case "$1" in -h) echo "  -v  be verbose";; *) echo "Traceback (most recent call last):"; exit 1;; esac`)
	if got := names(CollectHelp(half)); got != "-h" {
		t.Fatalf("sources: %s", got)
	}
	// Help that mentions a traceback option is still help.
	if crashReport("Traceback (most recent call last):\n  --tb=style  traceback print mode") != "" {
		t.Fatal("help with options taken as a crash")
	}
}
