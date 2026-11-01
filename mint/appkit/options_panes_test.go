package appkit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// manyAdvanced is enough advanced options (in section "Tuning") for them
// to get their own pane.
func manyAdvanced() []FlagSpec {
	var out []FlagSpec
	for i := 0; i <= fewAdvanced; i++ {
		out = append(out, FlagSpec{Flag: fmt.Sprintf("--t%d", i), Kind: FlagBool, Label: fmt.Sprintf("T%d", i), Section: "Tuning", Advanced: true})
	}
	return out
}

// A tool with many advanced options puts ordinary options in Basic Options
// and everything advanced in one Advanced + Additional pane, including
// Extra arguments.
func TestOptionsSplitWhenManyAdvanced(t *testing.T) {
	spec := ToolSpec{Name: "ffmpeg", Bin: "echo", ArchetypeID: "convert", Flags: append([]FlagSpec{
		{Flag: "--i", Kind: FlagBool, Label: "Input", Section: "Options"},
		{Flag: "--y", Kind: FlagBool, Label: "Overwrite", Section: "Options"},
		{Flag: "--z", Kind: FlagBool, Label: "Hidden", Section: "Options", Advanced: true},
		{Flag: "--q", Kind: FlagBool, Label: "Quality", Section: "Quality", Advanced: true},
	}, manyAdvanced()...)}
	s := newTestApp(t, spec).MainScreen()
	got := orderedPaneTitles(s)
	if len(got) < 3 || got[0] != titleBasic || got[1] != titleAdvanced || got[len(got)-1] != titleOutput {
		t.Fatalf("pane titles %v", got)
	}
	if strings.Count(strings.Join(got, "|"), titleAdvanced) != 1 {
		t.Fatalf("advanced pane repeated in %v", got)
	}
	// Ordinary flags stay in the top pane. The advanced heading and the
	// More Options collapsible, plus Extra arguments, are the one pane.
	if !hasSection(s, "Options") || sectionIn(s.Sections, "Quality (Advanced)") {
		t.Fatalf("basic sections %v", namesOf(s.Sections))
	}
	adv := s.Panes[0]
	if adv.Title != titleAdvanced {
		t.Fatalf("second pane %q", adv.Title)
	}
	if !sectionIn(adv.Sections, "Quality (Advanced)") || !sectionIn(adv.Sections, "Extra arguments") {
		t.Fatalf("advanced sections %v", namesOf(adv.Sections))
	}
	if _, ok := rowByID(s, "extra_args"); !ok {
		t.Fatal("extra arguments left the screen")
	}
}

// With nothing advanced there is no second options pane.
func TestThirteenSkipAdvancedPane(t *testing.T) {
	spec := ToolSpec{Name: "x", Bin: "echo", ArchetypeID: "convert", Flags: []FlagSpec{
		{Flag: "--a", Kind: FlagBool, Label: "A", Section: "Options"},
	}}
	s := newTestApp(t, spec).MainScreen()
	got := orderedPaneTitles(s)
	if len(got) != 2 || got[0] != titleBasic || got[1] != titleOutput {
		t.Fatalf("got %v, want Basic Options then Output", got)
	}
	if !sectionIn(s.Sections, "Options") || !sectionIn(s.Sections, "Extra arguments") && !rowIn(s.Sections, "extra_args") {
		// Extra arguments stay in the input section when the advanced pane is skipped.
		if _, ok := rowByID(s, "extra_args"); !ok {
			t.Fatal("extra arguments missing from Basic Options")
		}
	}
}

// Every archetype splits when there are many advanced options (tshark's
// record, filter), and keeps a few inside Basic Options, so a simple tool
// is one options pane.
func TestAdvancedPaneOnlyWhenManyAdvanced(t *testing.T) {
	few := []FlagSpec{
		{Flag: "--a", Kind: FlagBool, Label: "A", Section: "Options"},
		{Flag: "--z", Kind: FlagBool, Label: "Hidden", Section: "Options", Advanced: true},
		{Flag: "--q", Kind: FlagBool, Label: "Quality", Section: "Quality", Advanced: true},
	}
	for _, id := range []string{"record", "filter", "probe", "watch"} {
		s := newTestApp(t, ToolSpec{Name: "g", Bin: "echo", ArchetypeID: id, Flags: few}).MainScreen()
		got := orderedPaneTitles(s)
		if len(got) != 2 || got[0] != titleBasic || got[1] != titleOutput {
			t.Fatalf("%s with 2 advanced: got %v, want Basic Options then Output", id, got)
		}
		if !sectionIn(s.Sections, "Options") || !sectionIn(s.Sections, "Quality (Advanced)") {
			t.Fatalf("%s: basic sections %v", id, namesOf(s.Sections))
		}

		s = newTestApp(t, ToolSpec{Name: "g", Bin: "echo", ArchetypeID: id, Flags: append(append([]FlagSpec(nil), few...), manyAdvanced()...)}).MainScreen()
		got = orderedPaneTitles(s)
		if len(got) != 3 || got[0] != titleBasic || got[1] != titleAdvanced || got[2] != titleOutput {
			t.Fatalf("%s with many advanced: got %v", id, got)
		}
		if sectionIn(s.Sections, "Quality (Advanced)") || sectionIn(s.Sections, "Tuning (Advanced)") {
			t.Fatalf("%s: advanced left in Basic Options %v", id, namesOf(s.Sections))
		}
	}
}

func TestSplitFlagSections(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "ffmpeg", Bin: "echo", ArchetypeID: "convert", Flags: []FlagSpec{
		{Flag: "--a", Kind: FlagBool, Label: "A", Section: "Options"},
		{Flag: "--b", Kind: FlagBool, Label: "B", Section: "Output"},
		{Flag: "--c", Kind: FlagBool, Label: "C", Section: "Options", Advanced: true},
		{Flag: "--d", Kind: FlagBool, Label: "D", Section: "Advanced", Advanced: true},
		{Flag: "--e", Kind: FlagBool, Label: "E", Section: "More", Advanced: true},
	}})
	basic, advanced := a.splitFlagSections()
	if strings.Join(namesOf(basic), "|") != "Options|Output" {
		t.Errorf("basic %v", namesOf(basic))
	}
	if len(advanced) != 3 || advanced[1].Name != "Advanced (Advanced)" {
		t.Errorf("advanced %v", namesOf(advanced))
	}
	// Nothing advanced: the second list is empty.
	b := newTestApp(t, ToolSpec{Name: "x", Bin: "echo", Flags: []FlagSpec{
		{Flag: "--a", Kind: FlagBool, Label: "A", Section: "Options"},
	}})
	f, s := b.splitFlagSections()
	if len(f) != 1 || len(s) != 0 {
		t.Errorf("no-advanced split %v %v", namesOf(f), namesOf(s))
	}
}

func namesOf(secs []mint.Section) []string {
	var out []string
	for _, s := range secs {
		out = append(out, s.Name)
	}
	return out
}

func sectionIn(secs []mint.Section, name string) bool {
	for _, s := range secs {
		if s.Name == name {
			return true
		}
	}
	return false
}

func rowIn(secs []mint.Section, id string) bool {
	for _, s := range secs {
		for _, r := range s.Rows {
			if rowField(r, "id") == id {
				return true
			}
		}
	}
	return false
}

func orderedPaneTitles(s *mint.Screen) []string {
	var out []string
	if s != nil && s.TopTitle != "" {
		out = append(out, s.TopTitle)
	}
	out = append(out, paneTitles(s)...)
	return out
}
