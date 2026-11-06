package appkit

import "testing"

func TestNormalizeArchetypes(t *testing.T) {
	ids := NormalizeArchetypes([]ArchetypeID{"toolkit", "nope", "Records", "toolkit", "search", "query"})
	if len(ids) != 3 || ids[0] != "toolkit" || ids[1] != "records" || ids[2] != "search" {
		t.Fatalf("%v", ids)
	}
}

func TestArchetypeListAndIsA(t *testing.T) {
	old := ToolSpec{ArchetypeID: "list"}
	if ids := old.ArchetypeList(); len(ids) != 1 || !old.IsA("list") || old.IsA("search") {
		t.Fatalf("one archetype %v", ids)
	}
	s := ToolSpec{ArchetypeID: "toolkit", Archetypes: []ArchetypeID{"toolkit", "settings", "audit"}}
	for _, id := range []ArchetypeID{"toolkit", "settings", "audit"} {
		if !s.IsA(id) {
			t.Errorf("not a %s", id)
		}
	}
	if len(s.Members()) != 3 {
		t.Fatalf("members %+v", s.Members())
	}
}

// Each archetype counts in full: nothing is blended away.
func TestEveryArchetypesFeaturesAreOn(t *testing.T) {
	for _, a := range AllArchetypeDefs() {
		for _, b := range AllArchetypeDefs() {
			if a.ID == b.ID {
				continue
			}
			got := (ToolSpec{Archetypes: []ArchetypeID{a.ID, b.ID}}).Blueprint()
			for _, f := range append(a.Blueprint().Features, b.Blueprint().Features...) {
				if !got.Has(f) {
					t.Fatalf("%s + %s lost %s", a.ID, b.ID, f)
				}
			}
			for _, f := range got.Features {
				if !a.Blueprint().Has(f) && !b.Blueprint().Has(f) {
					t.Fatalf("%s + %s invented %s", a.ID, b.ID, f)
				}
			}
		}
	}
}

func TestApplyArchetypesKeepsTheMainWording(t *testing.T) {
	spec := ToolSpec{Name: "x", Bin: "echo"}
	ApplyArchetypes(&spec, []ArchetypeID{"session", "query"})
	if spec.ArchetypeID != "session" || len(spec.Archetypes) != 2 {
		t.Fatalf("%q %v", spec.ArchetypeID, spec.Archetypes)
	}
	single := ToolSpec{Name: "y", Bin: "echo"}
	ApplyArchetypes(&single, []ArchetypeID{"list"})
	if single.ArchetypeID != "list" || single.Archetypes != nil {
		t.Fatalf("single %q %v", single.ArchetypeID, single.Archetypes)
	}
	// list wants a table; ask does not. A program that is both has one.
	both := ToolSpec{Name: "z", Bin: "echo"}
	ApplyArchetypes(&both, []ArchetypeID{"ask", "list"})
	if !both.TableOutput {
		t.Fatal("ask + list has no table")
	}
}

// A second archetype can raise the data level, never lower it.
func TestDataLevelIsTheHighestOfAllArchetypes(t *testing.T) {
	s := ToolSpec{Name: "calc9", Bin: "calc9", Archetypes: []ArchetypeID{"ask", "record"}}
	if lvl, _ := ClassifyData(s); lvl != DataSensitive {
		t.Fatalf("ask + record gave %q", lvl)
	}
	both := ToolSpec{Name: "conv9", Bin: "conv9", Archetypes: []ArchetypeID{"convert", "ask"}}
	single := ToolSpec{Name: "conv9", Bin: "conv9", ArchetypeID: "convert"}
	l1, _ := ClassifyData(both)
	l2, _ := ClassifyData(single)
	if dataRank(l1) < dataRank(l2) {
		t.Fatalf("adding an archetype lowered %q to %q", l2, l1)
	}
}

func TestBehavioursComeFromEveryArchetype(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "cfg", Bin: "echo", Archetypes: []ArchetypeID{"list", "settings"}})
	if !a.stateEditWanted() {
		t.Fatal("the settings archetype brought no state editor")
	}
	b := newTestApp(t, ToolSpec{Name: "fixer", Bin: "echo", Findings: &FindingsSpec{Adapter: FindingsLines},
		Archetypes: []ArchetypeID{"audit", "edit-files"}})
	var titles []string
	for _, p := range b.rolePanes() {
		titles = append(titles, p.Title)
	}
	if len(titles) == 0 || titles[0] != "Findings" || !b.Spec.IsA("edit-files") {
		t.Fatalf("role panes %v", titles)
	}
}

func TestInterfaceFactsUseEveryArchetype(t *testing.T) {
	s := ToolSpec{Name: "db", Bin: "echo", Archetypes: []ArchetypeID{"query", "session"}}
	f := FactsFromSpec(s)
	if !f.Wants(FeatTranscript) || PlanFor(f).Type != TypeSession {
		t.Fatalf("hints %v plan %s", f.Hints, PlanFor(f).Type)
	}
	if PlanFor(FactsFromSpec(ToolSpec{Name: "db", Bin: "echo", ArchetypeID: "query"})).Type == TypeSession {
		t.Fatal("query alone is a session?")
	}
}

func TestSubcommandsWantedByAnyArchetype(t *testing.T) {
	if !(ToolSpec{Archetypes: []ArchetypeID{"convert", "toolkit"}}).WantsSubcommands() {
		t.Fatal("convert + toolkit does not read commands")
	}
	if (ToolSpec{ArchetypeID: "convert"}).WantsSubcommands() {
		t.Fatal("convert alone reads commands")
	}
}

func TestSpecHasOwnLayout(t *testing.T) {
	suite := ToolSpec{ArchetypeID: "toolkit", Subcommands: []SubcommandSpec{{Name: "run"}}}
	if !SpecHasOwnLayout(suite) {
		t.Error("suite")
	}
	mon := ToolSpec{ArchetypeID: "watch", TableOutput: true, PollInterval: 2000000000}
	if PlanFor(FactsFromSpec(mon)).Type != TypeLiveMonitor || SpecHasOwnLayout(mon) {
		t.Error("a monitor that cannot read its state has its own layout?")
	}
	mon.ReadOnly = true
	if !SpecHasOwnLayout(mon) {
		t.Error("a read-only monitor")
	}
}
