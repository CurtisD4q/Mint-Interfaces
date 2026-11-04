package appkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// startIn makes dir the folder the app is started in, for this test.
func startIn(t *testing.T, dir string) {
	t.Helper()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
}

// A tool that writes files of its own runs in the results folder; one
// that works on the folder it is in keeps running where the app started.
func TestToolRunsInResultsFolder(t *testing.T) {
	bin, err := exec.LookPath("pwd")
	if err != nil {
		t.Skip("pwd not installed")
	}
	start := t.TempDir()
	startIn(t, start)
	for _, tc := range []struct {
		arch string
		want func(a *ToolApp) string
	}{
		{"probe", func(a *ToolApp) string { return a.ResultsDir }},
		{"transfer", func(a *ToolApp) string { return a.ResultsDir }},
		{"search", func(*ToolApp) string { return start }},
		{"build", func(*ToolApp) string { return start }},
		{"", func(*ToolApp) string { return start }},
	} {
		a := newTestApp(t, ToolSpec{Name: "pwd", Bin: bin, ArchetypeID: tc.arch})
		job, err := a.StartJob()
		if err != nil {
			t.Fatal(err)
		}
		job.Wait()
		got, _ := filepath.EvalSymlinks(strings.TrimSpace(job.Output()))
		want, _ := filepath.EvalSymlinks(tc.want(a))
		if got != want {
			t.Errorf("%q ran in %q, want %q", tc.arch, got, want)
		}
	}
}

// Paths you type keep pointing where you meant: inputs and anything that
// is in the start folder become absolute; output names stay relative (the
// results folder); other values are left alone.
func TestRelativePathsKeepTheirMeaning(t *testing.T) {
	start := t.TempDir()
	for _, f := range []string{"in.pcap", "80", "list.txt"} {
		if err := os.WriteFile(filepath.Join(start, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	startIn(t, start)
	a := newTestApp(t, ToolSpec{Name: "probe-tool", Bin: "/bin/true", ArchetypeID: "probe",
		Flags: []FlagSpec{
			{Flag: "-p", Kind: FlagText, Label: "Ports"},
			{Flag: "-iL", Kind: FlagText, Label: "Input list", Role: RoleInputFile},
			{Flag: "-oN", Kind: FlagText, Label: "Normal output", Role: RoleOutputFile},
		}})
	a.Values["-p"] = "80"
	a.Values["-iL"] = "hosts.txt"
	a.Values["-oN"] = "scan.txt"
	a.Target = "in.pcap example.org ./new ../up"
	a.ExtraArgs = "-r in.pcap --list=list.txt -w out.pcap"
	args, _ := a.BuildArgs()
	line := " " + strings.Join(args, " ") + " "
	for _, want := range []string{
		" -p 80 ",
		" -iL " + filepath.Join(start, "hosts.txt") + " ",
		" -oN scan.txt ",
		" " + filepath.Join(start, "in.pcap") + " example.org " + filepath.Join(start, "new") + " " + filepath.Join(filepath.Dir(start), "up") + " ",
		" -r " + filepath.Join(start, "in.pcap") + " ",
		" --list=" + filepath.Join(start, "list.txt") + " ",
		" -w out.pcap ",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("args %q lack %q", line, want)
		}
	}

	// A tool that runs where the app started gets the words as typed.
	b := newTestApp(t, ToolSpec{Name: "rg", Bin: "/bin/true", ArchetypeID: "search"})
	b.Target = "in.pcap ./new"
	if args, _ := b.BuildArgs(); strings.Join(args, " ") != "in.pcap ./new" {
		t.Errorf("search args %q", args)
	}
}

// The viewer's save key offers the results folder, and sensitive output
// is never readable by others, even for a moment.
func TestViewerSaveIsPrivate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mint.SaveLogLinesPrivate(p, []mint.LogLine{{Text: "secret"}}); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("existing file is %v", fi.Mode().Perm())
	}
	p2 := filepath.Join(dir, "sub", "new.txt")
	if err := mint.SaveLogLinesPrivate(p2, nil); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p2); fi.Mode().Perm() != 0o600 {
		t.Fatalf("new file is %v", fi.Mode().Perm())
	}
	if b, _ := os.ReadFile(p); string(b) != "secret\n" {
		t.Fatalf("content %q", b)
	}
}

// Under sudo, results in the app's own folder go back to the user, but
// the app's folder and its own files (the program sudo runs) do not.
func TestGiveBackInAppFolder(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "nmap")
	a := &ToolApp{Spec: ToolSpec{Name: "nmap", DefaultResultsDir: appDir},
		owner: &invoker{uid: 1000, gid: 1000, home: filepath.Join(root, "home", "u")}}
	var got []string
	old := chown
	chown = func(p string, uid, gid int) error { got = append(got, p); return nil }
	defer func() { chown = old }()

	a.giveBack(filepath.Join(appDir, "scans", "scan.xml"), appDir,
		filepath.Join(appDir, "nmap-mint"), filepath.Join(appDir, "spec.go"),
		filepath.Join(root, "apps", "other", "x"), "/etc/passwd")
	want := []string{filepath.Join(appDir, "scans", "scan.xml"), filepath.Join(appDir, "scans")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("chowned %q, want %q", got, want)
	}

	// A spec whose folder is not named after the tool gets nothing.
	got = nil
	a.Spec.DefaultResultsDir = filepath.Join(root, "apps")
	a.giveBack(filepath.Join(root, "apps", "x"))
	if len(got) != 0 {
		t.Fatalf("chowned %q", got)
	}
}
