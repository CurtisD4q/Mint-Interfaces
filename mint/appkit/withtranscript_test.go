package appkit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/transcript"
)

func TestTranscriptSpec(t *testing.T) {
	if s, ok := KnownTranscriptSpec("/usr/bin/psql"); !ok || s.Adapter != "psql" {
		t.Fatalf("psql: %+v %v", s, ok)
	}
	if _, ok := KnownTranscriptSpec("ls"); ok {
		t.Fatal("ls has a transcript spec")
	}
	if err := (ToolSpec{Name: "x", Transcript: &TranscriptSpec{Adapter: "nope"}}).Validate(); err == nil || !strings.Contains(err.Error(), "known: ftp, gdb, lines") {
		t.Fatalf("unknown adapter: %v", err)
	}
	// sqlite3's archetype may save output to a file; a session doesn't.
	if a := newTestApp(t, ToolSpec{Name: "sqlite3", Transcript: &TranscriptSpec{Adapter: "sqlite3"}}); a.Spec.OutputsFile {
		t.Fatal("session app saves output to a file")
	}
	spec := ToolSpec{Name: "x", Transcript: &TranscriptSpec{Adapter: "gdb"}}
	if names := spec.EnabledMixinNames(); len(names) != 1 || names[0] != MixinTranscript {
		t.Fatalf("enabled: %v", names)
	}
}

// waitSource polls until no command or script is running.
func waitSource(t *testing.T, src *transcriptSource) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		src.Poll()
		if !src.sess.Busy() && !src.scriptRunning() {
			src.Poll()
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("still running")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func shown(src *transcriptSource) string {
	var out []string
	for i := 0; i < src.Len(); i++ {
		out = append(out, src.Line(i).Text())
	}
	return strings.Join(out, "\n")
}

func sqliteApp(t *testing.T) (*ToolApp, *transcriptSource) {
	t.Helper()
	bin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	a := newTestApp(t, ToolSpec{Name: "sqlite3", Bin: bin, KeepHistory: true, Transcript: &TranscriptSpec{Adapter: "sqlite3"}})
	a.Target = filepath.Join(t.TempDir(), "t.db")
	s, err := a.startSession()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return a, a.newTranscriptSource(s)
}

func TestTranscriptSQLite(t *testing.T) {
	a, src := sqliteApp(t)
	for _, cmd := range []string{"create table t(a, b);", "insert into t\nvalues (1, 'x');", "select * from t;", "select * from nope;"} {
		if err := src.Submit(cmd); err != nil {
			t.Fatal(err)
		}
		waitSource(t, src)
	}
	got := shown(src)
	for _, want := range []string{"› create table t(a, b);", "› insert into t\n  values (1, 'x');", "  1|x", "no such table: nope"} {
		if !strings.Contains(got, want) {
			t.Errorf("transcript lacks %q:\n%s", want, got)
		}
	}
	// The error is drawn as one.
	for i := 0; i < src.Len(); i++ {
		if l := src.Line(i); strings.Contains(l.Text(), "no such table") && l.Segs[0].Role != mint.RoleError {
			t.Errorf("error line role %v", l.Segs[0].Role)
		}
	}
	// A password typed in a command is masked, and kept out of history.
	if err := src.Submit("select 'create user a identified by ''pw''' as x; -- identified by 'hunter2'"); err != nil {
		t.Fatal(err)
	}
	waitSource(t, src)
	if strings.Contains(shown(src), "hunter2") {
		t.Errorf("password shown:\n%s", shown(src))
	}
	b, err := os.ReadFile(a.commandsPath())
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(a.commandsPath()); st.Mode().Perm() != 0o600 {
		t.Errorf("commands.json mode %v", st.Mode().Perm())
	}
	var saved []string
	if err := json.Unmarshal(b, &saved); err != nil || len(saved) != 4 || strings.Contains(string(b), "hunter2") {
		t.Fatalf("saved history %q %v", saved, err)
	}
	if got := a.newTranscriptSource(src.sess).history; len(got) != 4 {
		t.Fatalf("history not reloaded: %q", got)
	}
	// sqlite3 can't cancel and carry on.
	src.sess.Send("WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c LIMIT 2000000) SELECT count(*) FROM c;")
	if err := src.Cancel(); err != transcript.ErrCannotCancel {
		t.Fatalf("Cancel = %v", err)
	}
	waitSource(t, src)
}

func TestTranscriptRefuses(t *testing.T) {
	_, src := sqliteApp(t)
	if !src.sess.Safe() {
		t.Skip("this sqlite3 has no -safe")
	}
	if err := src.Submit(".shell id"); err == nil || !strings.Contains(err.Error(), "safe mode") {
		t.Fatalf(".shell: %v", err)
	}
	if !src.sess.Job().Running() {
		t.Fatal("the session ended")
	}
	// psql's \password would read the new password from the command box.
	src.ad = transcript.Adapters["psql"]
	if err := src.Submit(`\password bob`); err == nil || !strings.Contains(err.Error(), "ALTER ROLE") {
		t.Fatalf(`\password: %v`, err)
	}
	if es := src.sess.Entries(); len(es) != 0 {
		t.Fatalf("something was sent: %+v", es)
	}
}

func TestTranscriptScript(t *testing.T) {
	_, src := sqliteApp(t)
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.sql", "-- make it\ncreate table t(a);\ninsert into t values (1);\ninsert into t values (2);\nselect count(*) from t;\n")
	cmds, risky, err := src.LoadScript(good)
	if err != nil || len(cmds) != 4 || len(risky) != 0 {
		t.Fatalf("good: %d %d %v", len(cmds), len(risky), err)
	}
	if err := src.RunScript("good.sql", cmds); err != nil {
		t.Fatal(err)
	}
	waitSource(t, src)
	if got := shown(src); !strings.Contains(got, "  2\n") || !strings.Contains(got, "good.sql finished") {
		t.Fatalf("good script:\n%s", got)
	}

	bad := write("bad.sql", "insert into t values (3);\nselect * from nope;\ninsert into t values (4);\n")
	cmds, _, err = src.LoadScript(bad)
	if err != nil {
		t.Fatal(err)
	}
	src.RunScript("bad.sql", cmds)
	waitSource(t, src)
	got := shown(src)
	if !strings.Contains(got, "bad.sql stopped at line 2") || strings.Contains(got, "values (4)") {
		t.Fatalf("bad script:\n%s", got)
	}

	if _, _, err := src.LoadScript(write("open.sql", "select 1;\nselect 2\n")); err == nil {
		t.Fatal("unfinished statement accepted")
	}
	if _, _, err := src.LoadScript(dir); err == nil {
		t.Fatal("a directory accepted")
	}
	if src.sess.Safe() {
		if _, _, err := src.LoadScript(write("shell.sql", "select 1;\n.shell id\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("escape in safe mode: %v", err)
		}
	}
	src.ad = transcript.Adapters["psql"]
	if _, _, err := src.LoadScript(write("pw.sql", "select 1;\n\\password\n")); err == nil {
		t.Fatal(`\password in a script accepted`)
	}
	// Outside safe mode, escapes are returned for the user to confirm.
	src.ad = transcript.Adapters["gdb"]
	src.sess = transcript.Start(transcript.Adapters["lines"], "cat", nil, nil)
	defer src.sess.Close()
	_, risky, err = src.LoadScript(write("cmds.gdb", "break main\nshell id\nrun\n"))
	if err != nil || len(risky) != 1 || risky[0].Line != 2 {
		t.Fatalf("risky: %+v %v", risky, err)
	}
}

// A secret routed to standard input would be typed into the session.
func TestTranscriptNoStdinSecret(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "x", Bin: "cat", Transcript: &TranscriptSpec{Adapter: "lines"},
		Flags: []FlagSpec{{Flag: "--password-stdin", Label: "Password", Kind: FlagSecret, SecretStdin: true}}})
	if err := a.SetSecret("--password-stdin", "hunter2"); err != nil {
		t.Skipf("no secret store: %v", err)
	}
	if _, _, stdin := a.buildArgs(false); len(stdin) == 0 {
		t.Skip("this secret isn't routed to standard input")
	}
	if _, err := a.startSession(); err == nil || !strings.Contains(err.Error(), "command box") {
		t.Fatalf("startSession = %v", err)
	}
}
