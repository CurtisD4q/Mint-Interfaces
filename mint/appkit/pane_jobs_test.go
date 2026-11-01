package appkit

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// Every archetype keeps many advanced options in their own pane.
func TestEveryArchetypeFollowsItsPaneRule(t *testing.T) {
	if n := len(AllArchetypeDefs()); n != 35 {
		t.Fatalf("%d archetypes, want 35", n)
	}
	var split, rest int
	for _, def := range AllArchetypeDefs() {
		if advancedInOwnPane(string(def.ID)) {
			split++
		} else {
			rest++
		}
		t.Run(string(def.ID), func(t *testing.T) {
			plain := screenFor(t, def, false)
			assertPaneRule(t, def, plain, false)
			with := screenFor(t, def, true)
			assertPaneRule(t, def, with, true)
		})
	}
	if split != 35 || rest != 0 {
		t.Fatalf("%d archetypes split advanced options, %d do not; want all 35", split, rest)
	}
}

// The drawn screen lists Basic Options, then Advanced + Additional, then
// Output, and the run command is under that output.
func TestDrawnOrderPutsTheCommandUnderOutput(t *testing.T) {
	spec := ToolSpec{Name: "nmap", Bin: "echo", ArchetypeID: "probe", Flags: []FlagSpec{
		{Flag: "--a", Kind: FlagBool, Label: "A", Section: "Scan"},
		{Flag: "--z", Kind: FlagBool, Label: "Z", Section: "Scan", Advanced: true},
	}}
	spec.Flags = append(spec.Flags, manyAdvanced()...)
	frame := newTestApp(t, spec).MainScreen().FrameText(40, 100)
	basic := strings.Index(frame, "Basic Options")
	adv := strings.Index(frame, "Advanced + Additional")
	out := strings.Index(frame, "┄ Output")
	if out < 0 {
		out = strings.Index(frame, "▸ Output")
	}
	run := strings.Index(frame, "Probe (nmap)")
	if basic < 0 || adv < basic || out < adv || run < out {
		t.Fatalf("drawn order basic=%d advanced=%d output=%d run=%d", basic, adv, out, run)
	}
}

func screenFor(t *testing.T, def ArchetypeDef, advanced bool) *mint.Screen {
	t.Helper()
	spec := ToolSpec{
		Name: string(def.ID), Bin: "echo", ArchetypeID: string(def.ID),
		Flags: []FlagSpec{
			{Flag: "--plain", Kind: FlagBool, Label: "Plain", Section: "Options"},
		},
	}
	if advanced {
		spec.Flags = append(spec.Flags,
			FlagSpec{Flag: "--hidden", Kind: FlagBool, Label: "Hidden", Section: "Options", Advanced: true},
			FlagSpec{Flag: "--trace", Kind: FlagBool, Label: "Trace", Section: "Debug", Advanced: true},
		)
		spec.Flags = append(spec.Flags, manyAdvanced()...)
	}
	if def.ID == "toolkit" || def.ID == "records" {
		spec.Subcommands = []SubcommandSpec{{Name: "ps", Label: "Ps", Desc: "List"}}
	}
	if def.ID == "audit" {
		spec.Findings = &FindingsSpec{Adapter: FindingsLines}
	}
	return newTestApp(t, spec).MainScreen()
}

func assertPaneRule(t *testing.T, def ArchetypeDef, s *mint.Screen, advanced bool) {
	t.Helper()
	assertNoEmpty(t, s)
	assertDistinctAdjacent(t, s)
	titles := orderedPaneTitles(s)
	if len(titles) < 2 || titles[0] != titleBasic || titles[len(titles)-1] != titleOutput {
		t.Fatalf("%s titles %v, want Basic Options first and Output last", def.ID, titles)
	}
	if len(s.Dock) == 0 || s.Dock[0].Name == "" {
		t.Fatalf("%s command bar missing under Output", def.ID)
	}
	// The command bar is not a pane, and it is not sitting above the output.
	for _, sec := range s.Sections {
		if sec.Name == "Run" || sec.Name == "Apply" || sec.Name == "Live" {
			t.Fatalf("%s left the command bar in the options pane (%q)", def.ID, sec.Name)
		}
	}
	mid := titles[1 : len(titles)-1]
	split := advancedInOwnPane(string(def.ID))
	if split && advanced {
		if len(mid) == 0 || mid[0] != titleAdvanced {
			t.Fatalf("%s with advanced options: %v", def.ID, titles)
		}
		if strings.Count(strings.Join(titles, "\n"), titleAdvanced) != 1 {
			t.Fatalf("%s repeated the advanced pane: %v", def.ID, titles)
		}
		mid = mid[1:]
	} else if !split || !advanced {
		for _, title := range titles {
			if title == titleAdvanced {
				t.Fatalf("%s showed %q (advanced=%v): %v", def.ID, titleAdvanced, advanced, titles)
			}
		}
	}
	for _, title := range mid {
		switch paneKind(title) {
		case "basic", "advanced", "output":
			t.Fatalf("%s extra pane %q is options or output in %v", def.ID, title, titles)
		}
	}
	seen := map[string]bool{}
	for _, title := range titles {
		if seen[title] {
			t.Fatalf("%s repeated pane %q in %v", def.ID, title, titles)
		}
		seen[title] = true
	}
}
