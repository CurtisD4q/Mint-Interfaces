package transcript

import (
	"strings"
	"testing"
)

func TestSplitScript(t *testing.T) {
	script := "-- set up\r\n" +
		"create table t(a);\n" +
		"\n" +
		"insert into t\n" +
		"  values (1);\n" +
		"\\d t\n" +
		"select 'a;\n" +
		"b';\n" +
		"\\! rm -rf ~\n" +
		"select 1; -- trailing\n"
	got, err := SplitScript(Adapters["psql"], script)
	if err != nil {
		t.Fatal(err)
	}
	want := []ScriptCommand{
		{Line: 2, Text: "create table t(a);"},
		{Line: 4, Text: "insert into t\n  values (1);"},
		{Line: 6, Text: `\d t`},
		{Line: 7, Text: "select 'a;\nb';"},
		{Line: 9, Text: `\! rm -rf ~`, Escape: "runs a shell command"},
		{Line: 10, Text: "select 1; -- trailing"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d commands: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSplitScriptProblems(t *testing.T) {
	if _, err := SplitScript(Adapters["psql"], "select 1;\nselect 2\n"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("open statement: %v", err)
	}
	if _, err := SplitScript(Adapters["sqlite3"], "select 1;\x00"); err == nil {
		t.Fatal("NUL accepted")
	}
	if _, err := SplitScript(Adapters["gdb"], strings.Repeat("x", MaxScript+1)); err == nil {
		t.Fatal("oversized script accepted")
	}
	// An escape on any line of a statement counts.
	got, err := SplitScript(Adapters["sqlite3"], "select\n load_extension('x');\n")
	if err != nil || len(got) != 1 || got[0].Escape == "" {
		t.Fatalf("escape inside statement: %+v %v", got, err)
	}
	// Line-based programs: one command per line, comments skipped.
	got, err = SplitScript(Adapters["gdb"], "# start\nbreak main\n\nrun\nshell id\n")
	if err != nil || len(got) != 3 || got[0].Text != "break main" || got[2].Escape == "" || got[2].Line != 5 {
		t.Fatalf("gdb: %+v %v", got, err)
	}
}

func TestMaskSecrets(t *testing.T) {
	for _, c := range []struct{ adapter, in, want string }{
		{"psql", "ALTER USER bob PASSWORD 'hunter2';", "ALTER USER bob PASSWORD •••;"},
		{"psql", "create role x login password 'it''s secret' valid until 'infinity';", "create role x login password ••• valid until 'infinity';"},
		{"psql", "alter role x password $p$a'b$p$;", "alter role x password •••;"},
		{"psql", "select password from users;", ""},
		{"sqlite3", "-- mysql style\ncreate user a identified by 'pw';", "-- mysql style\ncreate user a identified by •••;"},
		{"redis-cli", "AUTH s3cret", "AUTH •••"},
		{"redis-cli", "auth default s3cret", "auth default •••"},
		{"redis-cli", "CONFIG SET requirepass hunter22", "CONFIG SET requirepass •••"},
		{"redis-cli", "ACL SETUSER alice on >pass1 ~* +@all", "ACL SETUSER alice on >••• ~* +@all"},
		{"redis-cli", "GET auth", ""},
		{"ftp", "user anna hunter2", "user anna •••"},
		{"gdb", "print password", ""},
	} {
		got, found := MaskSecrets(Adapters[c.adapter], c.in)
		if c.want == "" {
			if found || got != c.in {
				t.Errorf("%s %q: masked to %q", c.adapter, c.in, got)
			}
			continue
		}
		if !found || got != c.want {
			t.Errorf("%s %q: %q %v, want %q", c.adapter, c.in, got, found, c.want)
		}
	}
}

func TestRefuse(t *testing.T) {
	for _, c := range []struct {
		adapter, line string
		refused       bool
	}{
		{"psql", `\password bob`, true},
		{"psql", `  \password`, true},
		{"psql", `\prompt 'Name? ' name`, true},
		{"psql", `select '\password';`, false},
		{"ftp", "user anna", true},
		{"ftp", "user anna hunter2", false},
		{"sqlite3", ".tables", false},
	} {
		a := Adapters[c.adapter]
		got := a.Refuse != nil && a.Refuse(c.line) != ""
		if got != c.refused {
			t.Errorf("%s %q: refused %v", c.adapter, c.line, got)
		}
	}
	cmds, err := SplitScript(Adapters["psql"], "select 1;\n\\password\n")
	if err != nil || len(cmds) != 2 || cmds[1].Refuse == "" {
		t.Fatalf("script refuse: %+v %v", cmds, err)
	}
}
