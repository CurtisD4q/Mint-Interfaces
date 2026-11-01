package main

import "testing"

// A close call between Inspect and List goes to List when the program
// has options for a table's headings or columns, and stays close.
func TestListInspectTieBreak(t *testing.T) {
	scores := []ArchetypeScore{{ID: "inspect", Score: 50}, {ID: "list", Score: 45}, {ID: "ask", Score: 10}}
	p := programText{OptionText: "  -H, --heading     print a line of column headings\n"}
	got := applyTieBreaks(p, append([]ArchetypeScore(nil), scores...))
	if got[0].ID != "list" || got[0].Score-got[1].Score >= clearLead {
		t.Fatalf("got %v", got[:2])
	}
	// No table options: unchanged.
	if got := applyTieBreaks(programText{OptionText: "  -v, --verbose\n"}, append([]ArchetypeScore(nil), scores...)); got[0].ID != "inspect" {
		t.Fatalf("changed without table options: %v", got[:2])
	}
	// A clear lead is not a close call.
	clear := []ArchetypeScore{{ID: "inspect", Score: 50 + clearLead}, {ID: "list", Score: 45}}
	if got := applyTieBreaks(p, clear); got[0].ID != "inspect" {
		t.Fatal("a clear choice was overturned")
	}
}
