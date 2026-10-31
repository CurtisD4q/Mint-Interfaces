package appkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The results pane lists the recorded runs, newest first, so one can be
// reopened without leaving the output. The current run is in the list too:
// it is the one the pane is already showing above.
func TestRecentRunsAreListedUnderTheOutput(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "x"})
	now := time.Now()
	a.history = []RunRecord{
		{ID: "3", Time: now, CmdLine: "x -c", State: "done", LineCount: 1, Lines: []string{"current"}},
		{ID: "2", Time: now.Add(-time.Minute), CmdLine: "x -b", State: "done", LineCount: 2, Lines: []string{"b1", "b2"}},
		{ID: "1", Time: now.Add(-2 * time.Minute), CmdLine: "x -a", State: "done", LineCount: 1, Lines: []string{"a1"}},
	}
	if got := len(a.recentRunRows()); got != 3 {
		t.Errorf("got %d rows, want 3 (every recorded run)", got)
	}
	// No history: nothing to list, and no "no runs" note in the output pane.
	if got := (&ToolApp{}).recentRunRows(); len(got) != 0 {
		t.Errorf("no history gave %d rows, want none", len(got))
	}
}

// A run's lines come from memory when they were kept, from the file it
// wrote when they were not, and say so plainly when neither is there.
func TestRunLines(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "x"})
	dir := t.TempDir()
	file := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(file, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(a.runLines(RunRecord{Lines: []string{"a", "b"}}), "|"); got != "a|b" {
		t.Errorf("kept lines: %q", got)
	}
	if got := strings.Join(a.runLines(RunRecord{OutputFile: file}), "|"); got != "one|two" {
		t.Errorf("lines from the file: %q", got)
	}
	if got := a.runLines(RunRecord{OutputFile: filepath.Join(dir, "gone.txt")}); len(got) != 1 || !strings.Contains(got[0], "no longer there") {
		t.Errorf("missing file: %v", got)
	}
	if got := a.runLines(RunRecord{}); len(got) != 1 || !strings.Contains(got[0], "not kept") {
		t.Errorf("nothing kept: %v", got)
	}
}

// Keeping every run's lines in memory would grow without bound, so the
// newest few keep them and the rest fall back to their file.
func TestKeepLinesIsBounded(t *testing.T) {
	many := make([]string, historyLineLimit+50)
	for i := range many {
		many[i] = "line"
	}
	kept := keepLines(many)
	if len(kept) != historyLineLimit {
		t.Fatalf("kept %d lines, want %d", len(kept), historyLineLimit)
	}
	if !strings.Contains(kept[len(kept)-1], "more lines") {
		t.Errorf("the cut should say how much was dropped: %q", kept[len(kept)-1])
	}
	// A short one is copied, not aliased.
	short := []string{"a"}
	if got := keepLines(short); len(got) != 1 || got[0] != "a" {
		t.Errorf("short: %v", got)
	}
	short[0] = "changed"
	if keepLines(short)[0] != "changed" {
		t.Error("keepLines should copy, not alias")
	}
}

// An older runs.json has no lines in it and must still load.
func TestRunRecordWithoutLinesLoads(t *testing.T) {
	var r RunRecord
	if err := json.Unmarshal([]byte(`{"id":"1","cmd_line":"x -a","state":"done"}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.CmdLine != "x -a" || len(r.Lines) != 0 {
		t.Errorf("loaded %+v", r)
	}
}

// The run screen exists to run the same command again and read what it
// produced, so it carries no input zone: the target and extra arguments
// were settled on the screen that opened it.
func TestRunScreenHasNoInputSection(t *testing.T) {
	// The output section is named "Items" for a list tool, so identify the
	// input zone by what it used to be: the "Filter" heading the input box
	// carried, and the Extra arguments box itself.
	a := newTestApp(t, ToolSpec{Name: "tree", Bin: "echo", ArchetypeID: "list"})
	scr := a.RunScreen()
	for _, sec := range scr.Sections {
		if sec.Name == "Filter" {
			t.Errorf("run screen has an input section %q", sec.Name)
		}
	}
	// The flags are still there: the screen runs the tool.
	found := false
	for _, sec := range scr.Sections {
		if sec.Name == "Run" {
			found = true
		}
	}
	if !found {
		t.Error("run screen lost its Run section")
	}
}
