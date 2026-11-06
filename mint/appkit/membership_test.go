package appkit

import (
	"math"
	"testing"
)

func TestNormalizeMemberships(t *testing.T) {
	ms := NormalizeMemberships([]Membership{
		{ID: "toolkit", Weight: 3}, {ID: "nope", Weight: 5}, {ID: "records", Weight: 1},
		{ID: "Toolkit", Weight: 9}, {ID: "list", Weight: 0}, {ID: "search", Weight: 2}, {ID: "query", Weight: 1.5},
	})
	if len(ms) != 3 || ms[0].ID != "toolkit" || ms[1].ID != "search" || ms[2].ID != "query" {
		t.Fatalf("%+v", ms)
	}
	sum := 0.0
	for _, m := range ms {
		sum += m.Weight
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("weights sum to %v", sum)
	}
	// A tie keeps the order given, so the primary stays first.
	tie := NormalizeMemberships([]Membership{{ID: "session", Weight: 1}, {ID: "query", Weight: 1}})
	if tie[0].ID != "session" {
		t.Fatalf("tie %+v", tie)
	}
}

func TestSpecMembershipsAndIsA(t *testing.T) {
	old := ToolSpec{ArchetypeID: "list"}
	if ms := old.Memberships(); len(ms) != 1 || ms[0].Weight != 1 || !old.IsA("list") || old.IsA("search") {
		t.Fatalf("one archetype %+v", ms)
	}
	s := ToolSpec{ArchetypeID: "toolkit", Archetypes: []Membership{{ID: "toolkit", Weight: 0.6}, {ID: "settings", Weight: 0.3}, {ID: "audit", Weight: 0.1}}}
	if !s.IsA("toolkit") || !s.IsA("settings") || s.IsA("audit") {
		t.Fatal("IsA: toolkit and settings yes, audit (10%) no")
	}
	if len(s.Members()) != 2 {
		t.Fatalf("members %+v", s.Members())
	}
}

func TestBlendSwitchesOnWhatAStrongSecondaryNeeds(t *testing.T) {
	// ask has no table; list wants one at 1.0. Half and half: 0.5, on.
	s := ToolSpec{Archetypes: []Membership{{ID: "ask", Weight: 0.5}, {ID: "list", Weight: 0.5}}}
	if !s.Blueprint().Has(FeatTable) {
		t.Fatal("a half-list program has no table")
	}
	weak := ToolSpec{Archetypes: []Membership{{ID: "ask", Weight: 0.8}, {ID: "list", Weight: 0.2}}}
	if weak.Blueprint().Has(FeatTable) {
		t.Fatal("a weak secondary switched a table on")
	}
}

func TestApplyMembershipsKeepsThePrimarysWording(t *testing.T) {
	spec := ToolSpec{Name: "x", Bin: "echo"}
	ApplyMemberships(&spec, []Membership{{ID: "session", Weight: 0.5}, {ID: "query", Weight: 0.5}})
	if spec.ArchetypeID != "session" || len(spec.Archetypes) != 2 {
		t.Fatalf("%q %+v", spec.ArchetypeID, spec.Archetypes)
	}
	single := ToolSpec{Name: "y", Bin: "echo"}
	ApplyMemberships(&single, []Membership{{ID: "list", Weight: 1}})
	if single.ArchetypeID != "list" || single.Archetypes != nil {
		t.Fatalf("single %q %+v", single.ArchetypeID, single.Archetypes)
	}
}

// A secondary archetype can raise the data level, never lower it, however
// weak it is.
func TestDataLevelIsTheHighestOfAllArchetypes(t *testing.T) {
	low := ToolSpec{Name: "calc9", Bin: "calc9", Archetypes: []Membership{{ID: "ask", Weight: 0.9}, {ID: "record", Weight: 0.1}}}
	if lvl, _ := ClassifyData(low); lvl != DataSensitive {
		t.Fatalf("a 10%% record archetype gave %q", lvl)
	}
	high := ToolSpec{Name: "conv9", Bin: "conv9", Archetypes: []Membership{{ID: "convert", Weight: 0.5}, {ID: "ask", Weight: 0.5}}}
	single := ToolSpec{Name: "conv9", Bin: "conv9", ArchetypeID: "convert"}
	l1, _ := ClassifyData(high)
	l2, _ := ClassifyData(single)
	if dataRank(l1) < dataRank(l2) {
		t.Fatalf("adding an archetype lowered %q to %q", l2, l1)
	}
}

func TestBehavioursComeFromEveryMember(t *testing.T) {
	// A lister that is also a settings tool edits state.
	a := newTestApp(t, ToolSpec{Name: "cfg", Bin: "echo", Archetypes: []Membership{{ID: "list", Weight: 0.6}, {ID: "settings", Weight: 0.4}}})
	if !a.stateEditWanted() {
		t.Fatal("the settings membership brought no state editor")
	}
	// An audit that also edits files has both panes.
	b := newTestApp(t, ToolSpec{Name: "fixer", Bin: "echo", Findings: &FindingsSpec{Adapter: FindingsLines},
		Archetypes: []Membership{{ID: "audit", Weight: 0.5}, {ID: "edit-files", Weight: 0.5}}})
	var titles []string
	for _, p := range b.rolePanes() {
		titles = append(titles, p.Title)
	}
	if len(titles) == 0 || titles[0] != "Findings" {
		t.Fatalf("role panes %v", titles)
	}
	if !b.Spec.IsA("edit-files") {
		t.Fatal("edit-files membership lost")
	}
}

func TestInterfaceFactsUseTheBlend(t *testing.T) {
	// session wants a transcript; a program that is only partly a session
	// (25% or more) still gets it in the hints.
	s := ToolSpec{Name: "db", Bin: "echo", Archetypes: []Membership{{ID: "query", Weight: 0.5}, {ID: "session", Weight: 0.5}}}
	f := FactsFromSpec(s)
	if !f.Wants(FeatTranscript) || PlanFor(f).Type != TypeSession {
		t.Fatalf("hints %v plan %s", f.Hints, PlanFor(f).Type)
	}
	q := ToolSpec{Name: "db", Bin: "echo", ArchetypeID: "query"}
	if PlanFor(FactsFromSpec(q)).Type == TypeSession {
		t.Fatal("query alone is a session?")
	}
}

func TestSubcommandsWantedByAnyMember(t *testing.T) {
	s := ToolSpec{Archetypes: []Membership{{ID: "convert", Weight: 0.6}, {ID: "toolkit", Weight: 0.4}}}
	if !s.WantsSubcommands() {
		t.Fatal("a 40% toolkit does not read commands")
	}
	if (ToolSpec{ArchetypeID: "convert"}).WantsSubcommands() {
		t.Fatal("convert alone reads commands")
	}
}

// Belonging to more archetypes only ever adds features.
func TestAMembershipNeverRemovesThePrimarysFeatures(t *testing.T) {
	for _, primary := range AllArchetypeDefs() {
		for _, other := range AllArchetypeDefs() {
			if other.ID == primary.ID {
				continue
			}
			s := ToolSpec{Archetypes: []Membership{{ID: primary.ID, Weight: 0.4}, {ID: other.ID, Weight: 0.35}, {ID: "general", Weight: 0.25}}}
			got := s.Blueprint()
			for _, f := range primary.Blueprint().Features {
				if !got.Has(f) {
					t.Fatalf("%s + %s lost %s", primary.ID, other.ID, f)
				}
			}
		}
	}
}

func TestSpecHasOwnLayout(t *testing.T) {
	suite := ToolSpec{ArchetypeID: "toolkit", Subcommands: []SubcommandSpec{{Name: "run"}}}
	if !SpecHasOwnLayout(suite) {
		t.Error("suite")
	}
	follow := ToolSpec{ArchetypeID: "watch", TableOutput: true, PollInterval: 2000000000}
	if PlanFor(FactsFromSpec(follow)).Type != TypeLiveMonitor || SpecHasOwnLayout(follow) {
		t.Error("a monitor that cannot read its state has its own layout?")
	}
	follow.ReadOnly = true
	if !SpecHasOwnLayout(follow) {
		t.Error("a read-only monitor")
	}
}
