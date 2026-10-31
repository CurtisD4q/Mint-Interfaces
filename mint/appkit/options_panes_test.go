package appkit

import (
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// Option sections go in two panes: the ordinary options first, then the
// advanced ones under "More Options".
func TestOptionsSplitIntoTwoPanes(t *testing.T) {
	spec := ToolSpec{Name: "ffmpeg", Bin: "echo", ArchetypeID: "convert", Flags: []FlagSpec{
		{Flag: "--i", Kind: FlagBool, Label: "Input", Section: "Options"},
		{Flag: "--y", Kind: FlagBool, Label: "Overwrite", Section: "Options"},
		{Flag: "--z", Kind: FlagBool, Label: "Hidden", Section: "Options", Advanced: true},
		{Flag: "--q", Kind: FlagBool, Label: "Quality", Section: "Quality", Advanced: true},
	}}
	a := newTestApp(t, spec)
	panes := a.optionsPanes(a.flagSections())
	got := paneTitles(&mint.Screen{Panes: panes})
	if len(got) != 2 || got[0] != "Options" || got[1] != "More Options" {
		t.Fatalf("pane titles %v, want [Options More Options]", got)
	}
	// The ordinary section keeps the ordinary flags, and the advanced one
	// starts the second pane.
	if n := len(panes[0].Sections); n != 1 || panes[0].Sections[0].Name != "Options" {
		t.Errorf("first pane: %d sections", n)
	}
	if n := len(panes[1].Sections); n != 1 || panes[1].Sections[0].Name != "Quality (Advanced)" {
		t.Errorf("second pane: %d sections", n)
	}
}

// With nothing advanced there is nothing to split: one pane, as before.
func TestOptionsStayOnePaneWithoutAdvanced(t *testing.T) {
	spec := ToolSpec{Name: "x", Bin: "echo", ArchetypeID: "convert", Flags: []FlagSpec{
		{Flag: "--a", Kind: FlagBool, Label: "A", Section: "Options"},
	}}
	a := newTestApp(t, spec)
	got := paneTitles(&mint.Screen{Panes: a.optionsPanes(a.flagSections())})
	if len(got) != 1 || got[0] != "Options" {
		t.Fatalf("got %v, want one Options pane", got)
	}
}

// Every section that is only advanced options belongs to the second half.
func TestSplitOptionSections(t *testing.T) {
	first, second := splitOptionSections([]mint.Section{
		{Name: "Options"}, {Name: "Output"}, {Name: "Advanced (Advanced)"}, {Name: "More (Advanced)"},
	})
	if len(first) != 2 || first[1].Name != "Output" {
		t.Errorf("first half %v", namesOf(first))
	}
	if len(second) != 2 || second[0].Name != "Advanced (Advanced)" {
		t.Errorf("second half %v", namesOf(second))
	}
	// Nothing advanced: no split at all.
	if f, s := splitOptionSections([]mint.Section{{Name: "Options"}}); len(f) != 1 || s != nil {
		t.Errorf("no-advanced split %v %v", f, s)
	}
}

func namesOf(secs []mint.Section) []string {
	var out []string
	for _, s := range secs {
		out = append(out, s.Name)
	}
	return out
}
