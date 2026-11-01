package appkit

import (
	"os/exec"
	"strings"
	"testing"
)

// Batch words start every run and every refresh, before the options.
func TestBatchArgsStartEveryRun(t *testing.T) {
	app := &ToolApp{Spec: ToolSpec{Name: "zz", ArchetypeID: "watch", ReadOnly: true, BatchArgs: []string{"-b", "-n", "1"},
		Flags: []FlagSpec{{Flag: "-d", Kind: FlagText, Label: "Delay"}}}, Values: map[string]string{"-d": "2"}}
	args, _ := app.BuildArgs()
	if strings.Join(args, " ") != "-b -n 1 -d 2" {
		t.Errorf("run args %q", args)
	}
	st, ok := app.statusArgs()
	if !ok || strings.Join(st, " ") != "-b -n 1 -d 2" {
		t.Errorf("refresh args %q (%v)", st, ok)
	}
}

// A live view of top in batch mode reads its process table.
func TestBatchTopRefreshes(t *testing.T) {
	if _, err := exec.LookPath("top"); err != nil {
		t.Skip("no top")
	}
	app := &ToolApp{Spec: ToolSpec{Name: "top", ArchetypeID: "watch", ReadOnly: true, BatchArgs: []string{"-b", "-n", "1"}}, Values: map[string]string{}}
	if err := app.RefreshState(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(app.SnapshotLines, "\n"), "PID") {
		t.Errorf("no process table in %d lines", len(app.SnapshotLines))
	}
}
