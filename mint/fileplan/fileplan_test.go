package fileplan

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// tree builds files under root from "path=content" (content "/" makes a
// folder, "->target" a symlink), with mtimes spaced so -u is predictable.
func tree(t *testing.T, root string, spec ...string) {
	t.Helper()
	base := time.Now().Add(-time.Hour)
	for i, s := range spec {
		p, content, _ := strings.Cut(s, "=")
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		switch {
		case content == "/":
			os.MkdirAll(full, 0o755)
		case strings.HasPrefix(content, "->"):
			os.Symlink(content[2:], full)
			continue
		default:
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		mt := base.Add(time.Duration(i) * time.Minute)
		os.Chtimes(full, mt, mt)
	}
}

// snapshot is every path under root (relative), with "/" for folders,
// "->x" for symlinks and file contents otherwise.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			out[rel] = "->" + l
		case fi.IsDir():
			out[rel] = "/"
		default:
			b, _ := os.ReadFile(p)
			out[rel] = string(b)
		}
		return nil
	})
	return out
}

// predict applies a plan's effects to a snapshot.
func predict(t *testing.T, before map[string]string, root string, p Plan) map[string]string {
	t.Helper()
	after := map[string]string{}
	for k, v := range before {
		after[k] = v
	}
	rel := func(s string) string {
		r, err := filepath.Rel(root, s)
		if err != nil || strings.HasPrefix(r, "..") {
			t.Fatalf("effect outside the test tree: %s", s)
		}
		return r
	}
	contentOf := func(note string) (string, bool) {
		for _, pre := range []string{"replaced by a copy of ", "copy of ", "replaced by "} {
			if strings.HasPrefix(note, pre) {
				return before[rel(strings.TrimPrefix(note, pre))], true
			}
		}
		return "", false
	}
	// Moves first take their sources away, then everything lands.
	for _, e := range p.Effects {
		switch e.Kind {
		case Delete:
			delete(after, rel(e.Path))
		case Move:
			src := rel(e.Path)
			for k, v := range before {
				if k == src || strings.HasPrefix(k, src+"/") {
					delete(after, k)
					after[rel(e.Dest)+strings.TrimPrefix(k, src)] = v
				}
			}
		case Create:
			if e.IsDir {
				after[rel(e.Path)] = "/"
			} else if c, ok := contentOf(e.Note); ok {
				after[rel(e.Path)] = c
			} else {
				// a file inside a copied folder: same relative content
				after[rel(e.Path)] = "?"
			}
		case Overwrite:
			if c, ok := contentOf(e.Note); ok {
				after[rel(e.Path)] = c
			}
		}
	}
	return after
}

type scenario struct {
	name  string
	files []string
	tool  string
	args  []string
	fails bool // the real tool exits non-zero
	// exitVaries: the exit status differs between coreutils versions
	// (mv -n fails since 9.2 when it skips a file); only the files are
	// compared.
	exitVaries bool
	// unsure: real paths under this prefix may or may not appear (the
	// tool's result there depends on directory read order).
	unsure string
}

var scenarios = []scenario{
	{"rm files", []string{"a=1", "b=2", "c=3"}, "rm", []string{"--", "a", "b"}, false, false, ""},
	{"rm folder without -r", []string{"d/x=1"}, "rm", []string{"--", "d"}, true, false, ""},
	{"rm -r folder", []string{"d/x=1", "d/e/y=2", "keep=3"}, "rm", []string{"-r", "--", "d"}, false, false, ""},
	{"rm -rf missing", []string{"a=1"}, "rm", []string{"-rf", "--", "nope", "a"}, false, false, ""},
	{"rm missing", []string{"a=1"}, "rm", []string{"--", "nope", "a"}, true, false, ""},
	{"rm symlink to folder", []string{"d/x=1", "l=->d"}, "rm", []string{"-r", "--", "l"}, false, false, ""},
	// A trailing slash on a symlink to a directory names the link, not the
	// directory: coreutils 9.x removes the link and exits 0. The exit status
	// differs between versions, so only the files are compared.
	{"rm -rf link/ removes the link", []string{"target/a=1", "target/s/b=2", "link=->target"}, "rm", []string{"-rf", "--", "link/"}, false, true, ""},
	{"rm -rf link removes the link", []string{"target/a=1", "link=->target"}, "rm", []string{"-rf", "--", "link"}, false, false, ""},
	{"rm -d empty", []string{"e=/"}, "rm", []string{"-d", "--", "e"}, false, false, ""},
	{"rm -d not empty", []string{"e/x=1"}, "rm", []string{"-d", "--", "e"}, true, false, ""},
	{"rm dash name", []string{"-rf=dash", "x=1"}, "rm", []string{"--", "-rf"}, false, false, ""},
	{"rm dot", []string{"a=1"}, "rm", []string{"-rf", "--", "."}, true, false, ""},

	{"cp to new name", []string{"a=1"}, "cp", []string{"--", "a", "b"}, false, false, ""},
	{"cp over file", []string{"a=1", "b=2"}, "cp", []string{"--", "a", "b"}, false, false, ""},
	{"cp -n", []string{"a=1", "b=2"}, "cp", []string{"-n", "--", "a", "b"}, false, false, ""},
	{"cp -u older source", []string{"a=1", "b=2"}, "cp", []string{"-u", "--", "a", "b"}, false, false, ""},
	{"cp -u newer source", []string{"b=2", "a=1"}, "cp", []string{"-u", "--", "a", "b"}, false, false, ""},
	{"cp into folder", []string{"a=1", "d=/"}, "cp", []string{"--", "a", "d"}, false, false, ""},
	{"cp many into folder", []string{"a=1", "b=2", "d/a=old"}, "cp", []string{"--", "a", "b", "d"}, false, false, ""},
	{"cp -t", []string{"a=1", "d=/"}, "cp", []string{"-t", "d", "--", "a"}, false, false, ""},
	{"cp folder without -r", []string{"s/x=1"}, "cp", []string{"--", "s", "t"}, true, false, ""},
	{"cp -r to new", []string{"s/x=1", "s/y/z=2"}, "cp", []string{"-r", "--", "s", "t"}, false, false, ""},
	{"cp -r into existing", []string{"s/x=1", "t/s/x=old", "t/other=o"}, "cp", []string{"-r", "--", "s", "t"}, false, false, ""},
	{"cp -rT merge", []string{"s/x=1", "s/n=new", "t/x=old", "t/keep=k"}, "cp", []string{"-rT", "--", "s", "t"}, false, false, ""},
	{"cp folder into itself", []string{"s/x=1"}, "cp", []string{"-r", "--", "s", "s/in"}, true, false, "s/in/"},
	{"cp file over folder", []string{"a=1", "d/a/x=1"}, "cp", []string{"--", "a", "d"}, true, false, ""},
	{"cp many, not a folder", []string{"a=1", "b=2", "c=3"}, "cp", []string{"--", "a", "b", "c"}, true, false, ""},

	{"mv rename", []string{"a=1"}, "mv", []string{"--", "a", "b"}, false, false, ""},
	{"mv over file", []string{"a=1", "b=2"}, "mv", []string{"--", "a", "b"}, false, false, ""},
	{"mv -n", []string{"a=1", "b=2"}, "mv", []string{"-n", "--", "a", "b"}, false, true, ""},
	{"mv into folder", []string{"a=1", "d=/"}, "mv", []string{"--", "a", "d"}, false, false, ""},
	{"mv folder", []string{"s/x=1", "s/y/z=2"}, "mv", []string{"--", "s", "t"}, false, false, ""},
	{"mv folder onto empty folder", []string{"s/x=1", "t=/"}, "mv", []string{"-T", "--", "s", "t"}, false, false, ""},
	{"mv folder onto full folder", []string{"s/x=1", "t/y=2"}, "mv", []string{"-T", "--", "s", "t"}, true, false, ""},
	{"mv missing", []string{"a=1"}, "mv", []string{"--", "nope", "a", "d"}, true, false, ""},
}

// Each scenario's plan, applied to the tree, must give what the real
// coreutils tool does to an identical tree.
func TestPlansMatchRealTools(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			bin, err := exec.LookPath(sc.tool)
			if err != nil {
				t.Skipf("%s not installed", sc.tool)
			}
			planDir, runDir := t.TempDir(), t.TempDir()
			tree(t, planDir, sc.files...)
			tree(t, runDir, sc.files...)
			before := snapshot(t, planDir)

			p, err := For(sc.tool, sc.args, Env{Dir: planDir, Home: "/nonexistent-home"})
			if err != nil {
				t.Fatal(err)
			}
			want := predict(t, before, planDir, p)

			cmd := exec.Command(bin, sc.args...)
			cmd.Dir = runDir
			out, runErr := cmd.CombinedOutput()
			got := snapshot(t, runDir)

			if (runErr != nil) != sc.fails && !sc.exitVaries {
				t.Errorf("real %s: err %v (%s), expected failure %v", sc.tool, runErr, out, sc.fails)
			}
			fails := p.Totals().Count[Fail] > 0 || p.Blocked()
			if fails != sc.fails && !sc.exitVaries {
				t.Errorf("plan predicts failure %v, real tool %v: %+v", fails, sc.fails, p)
			}
			// "?" means a copied file whose content the prediction
			// doesn't track; compare only that it exists.
			for k, v := range want {
				if v == "?" {
					if _, ok := got[k]; ok {
						want[k] = got[k]
					}
				}
			}
			if sc.unsure != "" {
				for k := range got {
					if strings.HasPrefix(k, sc.unsure) {
						delete(got, k)
					}
				}
			}
			if !reflect.DeepEqual(want, got) {
				t.Errorf("plan and real %s %v differ\n plan: %v\n real: %v\neffects: %+v", sc.tool, sc.args, sorted(want), sorted(got), p.Effects)
			}
		})
	}
}

func sorted(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func TestDangerousTargets(t *testing.T) {
	home := t.TempDir()
	p, _ := For("rm", []string{"-rf", "--", home}, Env{Dir: "/", Home: home, Budget: Budget{Entries: 10}})
	if !p.Blocked() || !strings.Contains(p.Warnings[0].Text, "your home folder") {
		t.Fatalf("home: %+v", p.Warnings)
	}
	p, _ = For("rm", []string{"-r", "--", "/"}, Env{Dir: "/", Budget: Budget{Entries: 10}})
	if p.Totals().Count[Fail] != 1 || p.Effects[0].Note != "rm refuses to remove / (--preserve-root)" {
		t.Fatalf("/: %+v", p)
	}
	p, _ = For("rm", []string{"-r", "--", "/etc"}, Env{Dir: "/", Budget: Budget{Entries: 10}})
	if !p.Blocked() || !strings.Contains(p.Warnings[0].Text, "top-level system folder") || !p.Incomplete {
		t.Fatalf("/etc: blocked %v incomplete %v %+v", p.Blocked(), p.Incomplete, p.Warnings)
	}
	// rm -rf on a link to home removes the link, with or without a trailing
	// slash: coreutils removes the link itself, so home is never touched.
	link := filepath.Join(t.TempDir(), "h")
	os.Symlink(home, link)
	if p, _ := For("rm", []string{"-rf", "--", link}, Env{Home: home}); p.Blocked() {
		t.Fatalf("removing a link was blocked: %+v", p.Warnings)
	}
	p, _ = For("rm", []string{"-rf", "--", link + "/"}, Env{Home: home})
	if p.Blocked() || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0].Text, "symbolic link") {
		t.Fatalf("link/ to home should remove only the link: %+v", p.Warnings)
	}
	// cp over /etc/passwd-like system file isn't checked (not a folder),
	// but cp onto a top-level folder name is.
	src := filepath.Join(t.TempDir(), "x")
	os.WriteFile(src, []byte("x"), 0o644)
	if p, _ := For("cp", []string{"--", src, "/proc"}, Env{}); p.Totals().Count[Create] != 1 {
		// /proc is a folder, so x goes inside it: not a replacement.
		t.Fatalf("cp into /proc: %+v", p)
	}
}

func TestPatternsAndTilde(t *testing.T) {
	dir := t.TempDir()
	tree(t, dir, "a.log=1", "b.log=2", ".hidden.log=3", "c.txt=4")
	p, _ := For("rm", []string{"--", "*.log"}, Env{Dir: dir})
	if !p.Blocked() || !strings.Contains(p.Warnings[0].Text, "no shell expands it") {
		t.Fatalf("pattern not flagged: %+v", p.Warnings)
	}
	got, changed := ExpandPatterns([]string{"*.log", "c.txt", "*.none"}, dir)
	if !changed || strings.Join(got, " ") != "a.log b.log c.txt *.none" {
		t.Fatalf("expand = %q %v", got, changed)
	}
	if got, _ := ExpandPatterns([]string{".*.log"}, dir); strings.Join(got, " ") != ".hidden.log" {
		t.Fatalf("dot pattern = %q", got)
	}
	if got, changed := ExpandPatterns([]string{"plain"}, dir); changed || got[0] != "plain" {
		t.Fatal("plain word changed")
	}
	p, _ = For("rm", []string{"--", "~/x"}, Env{Dir: dir})
	if !p.Blocked() || !strings.Contains(p.Warnings[0].Text, "starts with ~") {
		t.Fatalf("tilde not flagged: %+v", p.Warnings)
	}
}

func TestBudget(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for i := 0; i < 50; i++ {
		files = append(files, fmt.Sprintf("big/f%02d=x", i))
	}
	tree(t, dir, files...)
	p, _ := For("rm", []string{"-r", "--", "big"}, Env{Dir: dir, Budget: Budget{Entries: 10}})
	if !p.Incomplete || len(p.Effects) != 10 {
		t.Fatalf("incomplete %v, %d effects", p.Incomplete, len(p.Effects))
	}
}

func TestParseArgs(t *testing.T) {
	spec := toolOptions["cp"]
	o, err := parseArgs(spec, []string{"-rvt", "dest", "--update=none", "--no-clob", "a", "-x", "--frob", "--", "-n"})
	if err != nil {
		t.Fatal(err)
	}
	if o.set["target-directory"] != "dest" || o.set["update"] != "none" || !o.has("no-clobber") || !o.has("recursive") {
		t.Fatalf("set %v", o.set)
	}
	if strings.Join(o.operands, " ") != "a -n" {
		t.Fatalf("operands %q", o.operands)
	}
	if strings.Join(o.unknown, " ") != "--frob" {
		// -x is cp's --one-file-system
		t.Fatalf("unknown %q", o.unknown)
	}
	if _, err := parseArgs(spec, []string{"-t"}); err == nil {
		t.Fatal("-t without a value accepted")
	}
	if o, _ := parseArgs(spec, []string{"-tdir", "a"}); o.set["target-directory"] != "dir" {
		t.Fatalf("attached value %v", o.set)
	}
	// Ambiguous abbreviation (--no- could be several) is unknown.
	if o, _ := parseArgs(spec, []string{"--no-"}); len(o.unknown) != 1 {
		t.Fatalf("ambiguous prefix accepted: %v", o.set)
	}
	if _, err := For("dd", nil, Env{}); err == nil {
		t.Fatal("planned a tool it doesn't know")
	}
}

func TestCrossFilesystemMove(t *testing.T) {
	// /dev/shm is usually a separate tmpfs.
	if _, err := os.Stat("/dev/shm"); err != nil {
		t.Skip("no /dev/shm")
	}
	src := filepath.Join(t.TempDir(), "f")
	os.WriteFile(src, []byte("x"), 0o644)
	a, _ := devOf(filepath.Dir(src))
	b, _ := devOf("/dev/shm")
	if a == b {
		t.Skip("temp folder and /dev/shm are on one filesystem")
	}
	p, _ := For("mv", []string{"--", src, "/dev/shm/mint-plan-test"}, Env{})
	found := false
	for _, w := range p.Warnings {
		found = found || strings.Contains(w.Text, "different filesystem")
	}
	if !found {
		t.Fatalf("no cross-filesystem note: %+v", p.Warnings)
	}
}

func TestFingerprintChangesWithTree(t *testing.T) {
	dir := t.TempDir()
	tree(t, dir, "d/a=1")
	p1, _ := For("rm", []string{"-r", "--", "d"}, Env{Dir: dir})
	os.WriteFile(filepath.Join(dir, "d", "b"), []byte("2"), 0o644)
	p2, _ := For("rm", []string{"-r", "--", "d"}, Env{Dir: dir})
	if p1.Fingerprint() == p2.Fingerprint() {
		t.Fatal("fingerprint didn't change when a file appeared")
	}
}

// A file reached two ways is counted once.
func TestSamePathCountedOnce(t *testing.T) {
	dir := t.TempDir()
	tree(t, dir, "build/a=1", "build/s/b=2", "link=->build")
	p, _ := For("rm", []string{"-r", "--", "build", "link/", "build"}, Env{Dir: dir})
	// build, build/a, build/s, build/s/b, and the link itself: "link/" is a
	// symbolic link, so rm removes the link, not the folder it points at.
	if c := p.Totals().Count[Delete]; c != 5 {
		t.Fatalf("%d deletes: %+v", c, p.Effects)
	}
}
