package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// withToolDirs points the registry at dirs for one test.
func withToolDirs(t *testing.T, dirs ...string) {
	t.Helper()
	old := knownToolDirs
	knownToolDirs = func() []string { return dirs }
	resetKnownTools()
	t.Cleanup(func() { knownToolDirs = old; resetKnownTools() })
}

// privateDir is a folder only this user can change (inside sticky /tmp).
func privateToolDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mint-gen-tools-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	os.Chmod(dir, 0o755)
	return dir
}

func writeToolFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	os.Chmod(p, mode)
	return p
}

func TestBuiltinKnownToolsAreValid(t *testing.T) {
	seen := map[string]bool{}
	for _, kt := range builtinKnownTools {
		if err := validKnownTool(kt); err != nil {
			t.Errorf("built-in entry: %v", err)
		}
		if seen[kt.Name] {
			t.Errorf("%s listed twice", kt.Name)
		}
		seen[kt.Name] = true
	}
}

func TestKnownToolFileIsUsed(t *testing.T) {
	dir := privateToolDir(t)
	writeToolFile(t, dir, "mine.json", `{"tools":[{"name":"mytool","stdout":"log",
		"options":[{"flag":"--save","role":"output-file","ext":"csv","main":true},{"flag":"-i","role":"input-file"}]},
		{"name":"tcpdump","options":[{"flag":"-W","role":"output-file","main":true}]}]}`, 0o644)
	withToolDirs(t, dir)
	if w := KnownToolWarnings(); len(w) > 0 {
		t.Fatalf("warnings: %v", w)
	}
	kt, ok := KnownToolIO("mytool")
	if m, has := kt.mainOutput(); !ok || !has || m.Flag != "--save" || m.Ext != "csv" || kt.Stdout != StdoutLog {
		t.Fatalf("entry not loaded: %+v", kt)
	}
	// A file entry replaces the built-in one.
	if kt, _ := KnownToolIO("tcpdump"); len(kt.Options) != 1 || kt.Options[0].Flag != "-W" {
		t.Fatalf("built-in not replaced: %+v", kt)
	}
	role, _ := ClassifyArg("mytool", appkit.FlagSpec{Flag: "--save", Kind: appkit.FlagText, Label: "Whatever (--save)"})
	if role != appkit.RoleOutputFile {
		t.Fatalf("role %s", role)
	}
}

// Files are read only when nobody else could have written them, and only
// in exactly the expected shape.
func TestKnownToolFileChecks(t *testing.T) {
	good := `{"tools":[{"name":"ok","options":[{"flag":"-o","role":"output-file","main":true}]}]}`
	cases := []struct {
		name, content string
		mode          os.FileMode
		setup         func(dir, path string)
		wantWarn      string
	}{
		{"group-writable.json", good, 0o664, nil, "other users can change it"},
		{"world-writable.json", good, 0o646, nil, "other users can change it"},
		{"unknown-field.json", `{"tools":[{"name":"x","irreversible":false}]}`, 0o644, nil, "unknown field"},
		{"extra-top.json", `{"tools":[],"run":"rm -rf /"}`, 0o644, nil, "unknown field"},
		{"bad-ext.json", `{"tools":[{"name":"x","options":[{"flag":"-o","role":"output-file","ext":"../../evil"}]}]}`, 0o644, nil, "extension"},
		{"bad-flag.json", `{"tools":[{"name":"x","options":[{"flag":"-o; rm -rf ~","role":"output-file"}]}]}`, 0o644, nil, "option"},
		{"bad-name.json", `{"tools":[{"name":"../x"}]}`, 0o644, nil, "tool name"},
		{"bad-role.json", `{"tools":[{"name":"x","options":[{"flag":"-o","role":"anything"}]}]}`, 0o644, nil, "role"},
		{"two-mains.json", `{"tools":[{"name":"x","options":[{"flag":"-o","role":"output-file","main":true},{"flag":"-O","role":"output-file","main":true}]}]}`, 0o644, nil, "more than one main"},
		{"main-not-output.json", `{"tools":[{"name":"x","options":[{"flag":"-i","role":"input-file","main":true}]}]}`, 0o644, nil, "only an output-file"},
		{"two-values.json", good + good, 0o644, nil, "more than one JSON value"},
		{"huge.json", `{"tools":[]}` + strings.Repeat(" ", maxKnownToolBytes), 0o644, nil, "larger than"},
		{"link.json", "", 0o644, func(dir, path string) {
			target := writeToolFile(t, dir, "target.txt", good, 0o644)
			os.Remove(path)
			os.Symlink(target, path)
		}, "link"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := privateToolDir(t)
			path := writeToolFile(t, dir, c.name, c.content, c.mode)
			if c.setup != nil {
				c.setup(dir, path)
			}
			tools, warn := loadKnownToolFiles(dir)
			all := strings.Join(warn, "\n")
			if !strings.Contains(all, c.wantWarn) {
				t.Fatalf("want a warning containing %q, got %q", c.wantWarn, all)
			}
			for _, kt := range tools {
				if kt.Name == "ok" || kt.Name == "x" {
					t.Fatalf("a refused file was used: %+v", kt)
				}
			}
		})
	}
}

func TestKnownToolFolderChecks(t *testing.T) {
	good := `{"tools":[{"name":"ok"}]}`
	// A folder others can write to is ignored as a whole.
	open := privateToolDir(t)
	os.Chmod(open, 0o777)
	writeToolFile(t, open, "a.json", good, 0o644)
	if tools, warn := loadKnownToolFiles(open); len(tools) != 0 || !strings.Contains(strings.Join(warn, " "), "other users can change it") {
		t.Fatalf("open folder used: %v %v", tools, warn)
	}
	// A folder reached through a link is ignored.
	real := privateToolDir(t)
	writeToolFile(t, real, "a.json", good, 0o644)
	link := filepath.Join(privateToolDir(t), "tools.d")
	os.Symlink(real, link)
	if tools, _ := loadKnownToolFiles(link); len(tools) != 0 {
		t.Fatal("folder behind a link used")
	}
	// A file owned by another user is ignored (needs root to set up).
	if os.Geteuid() == 0 {
		dir := privateToolDir(t)
		p := writeToolFile(t, dir, "theirs.json", good, 0o644)
		os.Chown(p, 1234, 1234)
		if tools, warn := loadKnownToolFiles(dir); len(tools) != 0 || !strings.Contains(strings.Join(warn, " "), "another user") {
			t.Fatalf("another user's file used: %v %v", tools, warn)
		}
	}
	// A missing folder is fine and silent.
	if tools, warn := loadKnownToolFiles(filepath.Join(real, "nope")); tools != nil || warn != nil {
		t.Fatal("missing folder reported")
	}
}

func TestSafeTerminalText(t *testing.T) {
	in := "file\x1b]0;evil title\x07name\x1b[2J.json\r\n\u009b31m"
	if got := safeTerminalText(in); strings.ContainsAny(got, "\x1b\x07\r\n\u009b") || !strings.Contains(got, "name") {
		t.Fatalf("%q", got)
	}
}
