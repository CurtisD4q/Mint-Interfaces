package fileplan

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func effectsText(p Plan) []string {
	var out []string
	for _, e := range p.Effects {
		s := e.Kind.String() + " " + e.Path
		if e.Note != "" {
			s += " (" + e.Note + ")"
		}
		out = append(out, s)
	}
	return out
}

// Real output from rsync 3.2.7: rsync -a -n -i --delete src/ dst/
func TestParseRsyncItemize(t *testing.T) {
	out := []string{
		"*deleting   olddir/y",
		"*deleting   olddir/",
		"*deleting   extra",
		"cL+++++++++ lnk -> new",
		">f.s....... new",
		"cd+++++++++ sub/",
		">f+++++++++ sub/c",
		".d..t...... ./",
		"",
		"sent 172 bytes  received 47 bytes  438.00 bytes/sec",
		"total size is 13  speedup is 0.06 (DRY RUN)",
	}
	got := strings.Join(effectsText(ParseRsyncItemize(out)), "\n")
	want := strings.Join([]string{
		"delete extra",
		"create lnk (a link to new)",
		"overwrite new (changed: size)",
		"delete olddir/",
		"delete olddir/y",
		"create sub/",
		"create sub/c",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestParseRsyncAgainstRealRsync(t *testing.T) {
	bin, err := exec.LookPath("rsync")
	if err != nil {
		t.Skip("rsync not installed")
	}
	dir := t.TempDir()
	tree(t, dir, "src/new=a", "src/sub/c=b", "dst/new=old", "dst/extra=gone", "dst/old/y=x")
	out, err := exec.Command(bin, "-a", "-n", "-i", "--delete", filepath.Join(dir, "src")+"/", filepath.Join(dir, "dst")+"/").Output()
	if err != nil {
		t.Fatal(err)
	}
	p := ParseRsyncItemize(strings.Split(string(out), "\n"))
	c := p.Totals().Count
	if c[Delete] != 3 || c[Create] != 2 || c[Overwrite] != 1 {
		t.Fatalf("counts %v from:\n%s", c, out)
	}
}

func TestParseGitClean(t *testing.T) {
	got := effectsText(ParseGitClean([]string{"Would remove junk", "Would remove d/", "Would skip repository vendor/x/", "Removing nothing"}))
	want := "delete d/|delete junk|keep vendor/x/ (a nested repository (needs -ff))"
	if strings.Join(got, "|") != want {
		t.Fatalf("got %q", got)
	}
	bin, err := exec.LookPath("git")
	if err != nil {
		return
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return string(out)
	}
	run("init", "-q")
	tree(t, dir, "junk=1", "d/f=2")
	p := ParseGitClean(strings.Split(run("clean", "-n", "-d"), "\n"))
	if strings.Join(effectsText(p), "|") != "delete d/|delete junk" {
		t.Fatalf("real git clean: %q", effectsText(p))
	}
}
