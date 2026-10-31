package appkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The results pane folds the runs before this one under the output, so a
// reader comparing two runs finds both in the same place.
func TestEarlierRunsAreFoldedUnderTheOutput(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "x"})
	now := time.Now()
	a.history = []RunRecord{
		{ID: "3", Time: now, CmdLine: "x -c", State: "done", LineCount: 1, Lines: []string{"current"}},
		{ID: "2", Time: now.Add(-time.Minute), CmdLine: "x -b", State: "done", LineCount: 2, Lines: []string{"b1", "b2"}},
		{ID: "1", Time: now.Add(-2 * time.Minute), CmdLine: "x -a", State: "done", LineCount: 1, Lines: []string{"a1"}},
	}
	if got := len(a.earlierRunRows()); got != 2 {
		t.Errorf("got %d folded runs, want 2 (the current one is not repeated)", got)
	}

	// None with nothing earlier: the pane is the output alone.
	a.history = a.history[:1]
	if got := len(a.earlierRunRows()); got != 0 {
		t.Errorf("one run gave %d folded rows, want 0", got)
	}
	if rows := (&ToolApp{}).earlierRunRows(); len(rows) != 0 {
		t.Errorf("no history gave %d rows", len(rows))
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
