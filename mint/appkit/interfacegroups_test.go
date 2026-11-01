package appkit

import "testing"

func answersFrom(yes map[Question]bool, sure string) Answers {
	a := Answers{}
	for _, q := range Questions {
		a[q] = Answer{Yes: yes[q], Sure: sure}
	}
	return a
}

// Every possible set of answers falls in exactly one group.
func TestInterfaceGroupsCoverEveryAnswerSet(t *testing.T) {
	if len(interfaceGroups) != 22 {
		t.Fatalf("%d groups, want 22", len(interfaceGroups))
	}
	ids := map[string]bool{}
	for _, g := range interfaceGroups {
		if ids[g.ID] {
			t.Fatalf("duplicate group %q", g.ID)
		}
		ids[g.ID] = true
		for q := range g.Typical {
			if _, ok := g.Rule[q]; ok {
				t.Errorf("%s: %s is both a rule and a typical answer", g.ID, q)
			}
		}
	}
	for mask := 0; mask < 1<<len(Questions); mask++ {
		yes := map[Question]bool{}
		for i, q := range Questions {
			yes[q] = mask&(1<<i) != 0
		}
		n := 0
		for _, g := range interfaceGroups {
			if g.Matches(yes) {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("answers %v fall in %d groups, want 1", yes, n)
		}
	}
}

func TestPlaceInterfaceExamples(t *testing.T) {
	cases := []struct {
		yes  map[Question]bool
		want string
	}{
		{map[Question]bool{}, "once"},
		{map[Question]bool{QTarget: true}, "once-target"},
		{map[Question]bool{QKeepsRunning: true, QTable: true}, "live"},
		{map[Question]bool{QChangesThings: true, QSubcommands: true}, "change-commands"},
		{map[Question]bool{QKeepsRunning: true, QFullScreen: true}, "live-screen"},
		{map[Question]bool{QKeepsRunning: true, QChangesThings: true}, "live-change"},
	}
	for _, c := range cases {
		if g := PlaceInterface(answersFrom(c.yes, SureStrong)); g.ID != c.want {
			t.Errorf("%v: group %s, want %s", c.yes, g.ID, c.want)
		}
	}
}

// A group's typical answer fills in only where the help said nothing: it
// never takes a yes away, and never overrides a clear no.
func TestEffectiveAnswersOnlyAdd(t *testing.T) {
	g, _ := LookupInterfaceGroup("once-target") // typical: no root, no two inputs
	a := answersFrom(map[Question]bool{QTarget: true}, SureStrong)
	a[QNeedsRoot] = Answer{Yes: true, Sure: SureWeak}
	if !a.Effective(g)[QNeedsRoot] {
		t.Error("a weak yes to needs root was dropped")
	}
	g, _ = LookupInterfaceGroup("live-change") // typical: takes a target
	a = answersFrom(map[Question]bool{QKeepsRunning: true, QChangesThings: true}, SureStrong)
	a[QTarget] = Answer{Yes: false, Sure: SureWeak}
	if !a.Effective(g)[QTarget] {
		t.Error("where the help said nothing, the group's usual yes should fill in")
	}
	a[QTarget] = Answer{Yes: false, Sure: SureStrong}
	if a.Effective(g)[QTarget] {
		t.Error("a clear no was overridden")
	}
}

func TestBlueprintFromAnswers(t *testing.T) {
	b := BlueprintFromAnswers(map[Question]bool{QKeepsRunning: true, QTable: true}, false)
	if !b.Has(FeatLiveRefresh) || b.Has(FeatConfirm) {
		t.Errorf("a live table: %v", b.Features)
	}
	b = BlueprintFromAnswers(map[Question]bool{QKeepsRunning: true, QTable: true, QChangesThings: true}, false)
	if b.Has(FeatLiveRefresh) || !b.Has(FeatProgress) || !b.Has(FeatConfirm) {
		t.Errorf("a program that changes things must not be re-run on a timer: %v", b.Features)
	}
	b = BlueprintFromAnswers(map[Question]bool{QKeepsRunning: true, QTable: true, QCanDestroy: true}, true)
	if b.Has(FeatLiveRefresh) || !b.Has(FeatConfirm) {
		t.Errorf("a program that destroys things by purpose must not be re-run on a timer: %v", b.Features)
	}
	b = BlueprintFromAnswers(map[Question]bool{QKeepsRunning: true, QTable: true, QCanDestroy: true}, false)
	if !b.Has(FeatLiveRefresh) || !b.Has(FeatConfirm) {
		t.Errorf("a risky option alone still asks before running, and may refresh: %v", b.Features)
	}
	b = BlueprintFromAnswers(map[Question]bool{QKeepsRunning: true, QTable: true, QFullScreen: true}, false)
	if b.Has(FeatLiveRefresh) || b.Has(FeatProgress) {
		t.Errorf("a full-screen program is not run inside Mint: %v", b.Features)
	}
}

func TestApplyInterface(t *testing.T) {
	spec := ToolSpec{Name: "x", ReadOnly: true}
	a := answersFrom(map[Question]bool{QChangesThings: true}, SureStrong)
	g := ApplyInterface(&spec, a)
	if g.ID != "change" || spec.InterfaceGroup != "change" {
		t.Fatalf("group %s / %s", g.ID, spec.InterfaceGroup)
	}
	if spec.ReadOnly || !spec.ConfirmRun {
		t.Errorf("a program that changes things: read-only %v, confirm %v", spec.ReadOnly, spec.ConfirmRun)
	}
	if spec.Features == nil || (Blueprint{Features: spec.Features}).Has(FeatStateEdit) {
		t.Errorf("no way to read state, so no state editor: %v", spec.Features)
	}
	spec = ToolSpec{Name: "x", StatusArgs: []string{"show"}}
	ApplyInterface(&spec, a)
	if !(Blueprint{Features: spec.Features}).Has(FeatStateEdit) {
		t.Errorf("with a status command it reads, edits and applies: %v", spec.Features)
	}
	// An explicit target label keeps the target box.
	spec = ToolSpec{Name: "x", TargetLabel: "Host"}
	ApplyInterface(&spec, answersFrom(map[Question]bool{}, SureStrong))
	if !spec.WantTarget || spec.HideTarget {
		t.Error("an explicit target label was hidden")
	}
	// Caution already set is never removed.
	spec = ToolSpec{Name: "x", NeedsRoot: true, ConfirmRun: true}
	ApplyInterface(&spec, answersFrom(map[Question]bool{}, SureStrong))
	if !spec.NeedsRoot || !spec.ConfirmRun {
		t.Error("caution set by the caller was removed")
	}
	// Full-screen.
	spec = ToolSpec{Name: "x"}
	ApplyInterface(&spec, answersFrom(map[Question]bool{QFullScreen: true}, SureStrong))
	if !spec.FullScreen {
		t.Error("full-screen not set")
	}
}

// Features, when set, decide the screen instead of the archetype.
func TestSpecFeaturesDecideTheScreen(t *testing.T) {
	app := &ToolApp{Spec: ToolSpec{Name: "x", ArchetypeID: "settings", Features: []Feature{FeatCollapsible}}}
	if app.executionMode() != execOnce {
		t.Errorf("features without state edit still gave mode %v (from the archetype)", app.executionMode())
	}
	app = &ToolApp{Spec: ToolSpec{Name: "x", ArchetypeID: "settings"}}
	if app.executionMode() != execState {
		t.Errorf("without features the archetype decides: mode %v", app.executionMode())
	}
	app = &ToolApp{Spec: ToolSpec{Name: "x", ArchetypeID: "list", Features: []Feature{FeatLiveRefresh, FeatTable}}}
	if app.executionMode() != execPoll {
		t.Errorf("live refresh from features: mode %v", app.executionMode())
	}
}
