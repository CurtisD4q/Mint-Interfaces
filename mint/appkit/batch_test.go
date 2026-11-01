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

func TestColourArgsFollowBatchArgs(t *testing.T) {
	app := &ToolApp{Spec: ToolSpec{Name: "zz", BatchArgs: []string{"-b"}, ColourArgs: []string{"--pipe", "false"}}, Values: map[string]string{}}
	args, _ := app.BuildArgs()
	if strings.Join(args, " ") != "-b --pipe false" {
		t.Errorf("args %q", args)
	}
}

// fastfetch, asked for its colours, keeps them on the output lines.
func TestFastfetchKeepsColours(t *testing.T) {
	if _, err := exec.LookPath("fastfetch"); err != nil {
		t.Skip("no fastfetch")
	}
	app := &ToolApp{Spec: ToolSpec{Name: "fastfetch", ArchetypeID: "inspect", ColourArgs: []string{"--pipe", "false"}}, Values: map[string]string{}}
	job, err := app.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	coloured := 0
	for _, l := range app.ActiveLogLines() {
		if len(l.Colours) > 0 {
			coloured++
		}
		if strings.ContainsRune(l.Text, '\x1b') {
			t.Fatalf("escape left in text: %q", l.Text)
		}
	}
	if coloured < 5 {
		t.Errorf("only %d coloured lines", coloured)
	}
}
