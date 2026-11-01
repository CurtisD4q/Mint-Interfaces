package appkit

import (
	"strings"
	"testing"
)

// 36: Ask … Session, and General for anything else.
func TestThirtyFiveArchetypes(t *testing.T) {
	defs := AllArchetypeDefs()
	if len(defs) != 36 {
		t.Fatalf("%d archetypes", len(defs))
	}
	seen := map[string]bool{}
	for _, a := range defs {
		keys := map[string]bool{archetypeKey(string(a.ID)): true, archetypeKey(a.Name): true}
		for k := range keys {
			if seen[k] {
				t.Errorf("%s: name or ID also used by another archetype", a.ID)
			}
			seen[k] = true
		}
		if a.Name == "" || a.Group == "" || a.Is == "" || a.Takes == "" || a.Runs == "" || a.Shows == "" {
			t.Errorf("%s: a guide field is empty: %+v", a.ID, a)
		}
		if len(a.Prefer.Features) == 0 {
			t.Errorf("%s: no features", a.ID)
		}
		for f, w := range a.Prefer.Features {
			if w < 0 || w > 1 {
				t.Errorf("%s: feature %s weight %v", a.ID, f, w)
			}
		}
		if a.Data != "" && a.Data != DataSensitive && a.Data != DataSecret {
			t.Errorf("%s: data level %q", a.ID, a.Data)
		}
		if len(a.Blueprint().Features) == 0 {
			t.Errorf("%s: blueprint has no features", a.ID)
		}
	}
	for _, id := range []string{"settings", "device", "rules"} {
		b := mustArchetype(t, id).Blueprint()
		if !b.Has(FeatStateEdit) {
			t.Errorf("%s: state edit is off", id)
		}
	}
}

func TestLookupArchetype(t *testing.T) {
	for _, s := range []string{"look-up", "Look up", "LOOKUP", " look_up "} {
		if a, ok := LookupArchetype(s); !ok || a.ID != "look-up" {
			t.Errorf("%q: %v %v", s, a.ID, ok)
		}
	}
	if _, ok := LookupArchetype("nonsense"); ok {
		t.Error("found nonsense")
	}
}

// Every listed program names a real archetype.
func TestArchetypeListsAndTemplates(t *testing.T) {
	for _, n := range ArchetypeToolNames() {
		if _, ok := ArchetypeForTool(n); !ok {
			t.Errorf("%s: unknown archetype %q", n, archetypeTools[n])
		}
	}
	if a, _ := ArchetypeForTool("/usr/bin/nmap"); a.ID != "probe" {
		t.Errorf("nmap: %s", a.ID)
	}
	if a, _ := ArchetypeForTool("pkill"); a.ID != "signal" {
		t.Errorf("pkill: %s", a.ID)
	}
}

func TestArchetypeApplyTo(t *testing.T) {
	a := mustArchetype(t, "rules")
	spec := ToolSpec{Name: "fw9", TargetLabel: "mine"}
	a.ApplyTo(&spec)
	if spec.ArchetypeID != "rules" || !spec.TableOutput || !spec.ConfirmRun || !spec.NeedsRoot || !a.Blueprint().Has(FeatStateEdit) {
		t.Errorf("rules: %+v", spec)
	}
	if spec.TargetLabel != "mine" {
		t.Error("overwrote the caller's target label")
	}
	w := mustArchetype(t, "watch")
	spec = ToolSpec{Name: "w9"}
	w.ApplyTo(&spec)
	if spec.PollInterval <= 0 || spec.ConfirmRun || !w.Blueprint().Has(FeatLiveRefresh) {
		t.Errorf("watch: %+v", spec)
	}
}

func mustArchetype(t *testing.T, id string) ArchetypeDef {
	t.Helper()
	a, ok := LookupArchetype(id)
	if !ok {
		t.Fatalf("missing archetype %s", id)
	}
	return a
}

// Blending: weight 1 is the archetype unchanged; a style can switch a
// feature on or off by its share.
func TestBlendPreferences(t *testing.T) {
	a, _ := LookupArchetype("probe")
	style := Preferences{Features: map[Feature]float64{FeatHistory: 0, FeatCollapsible: 1}}
	if got, want := BlueprintFrom(BlendPreferences(a.Prefer, style, 1)), a.Blueprint(); strings.Join(featNames(got), ",") != strings.Join(featNames(want), ",") {
		t.Errorf("weight 1: %v, want %v", got, want)
	}
	b := BlueprintFrom(BlendPreferences(a.Prefer, style, 0.4))
	if !b.Has(FeatCollapsible) || b.Has(FeatHistory) {
		t.Errorf("weight 0.4: %+v", b)
	}
}

func featNames(b Blueprint) []string {
	var out []string
	for _, f := range b.Features {
		out = append(out, string(f))
	}
	return out
}

// Moving to archetypes never lowers a program's data level: programs the
// old templates protected more than their archetype does are classified
// on their own, and unlisted programs get their archetype's.
func TestArchetypeDataLevels(t *testing.T) {
	// base64 was a crypto-template program: still secret, though its
	// archetype (Convert) has no level of its own.
	if lvl, _ := ClassifyData(ToolSpec{Name: "base64", ArchetypeID: "convert"}); lvl != DataSecret {
		t.Errorf("base64: %q", lvl)
	}
	if lvl, _ := ClassifyData(ToolSpec{Name: "capture9", ArchetypeID: "record"}); lvl != DataSensitive {
		t.Errorf("record: %q", lvl)
	}
	if lvl, _ := ClassifyData(ToolSpec{Name: "calc9", ArchetypeID: "ask"}); lvl != DataNormal {
		t.Errorf("ask: %q", lvl)
	}
}

// New applies the archetype named in the spec, else the listed one.
func TestNewAppliesArchetype(t *testing.T) {
	app := New(ToolSpec{Name: "nmap", Bin: "nmap"})
	if app.Spec.ArchetypeID != "probe" || !app.blueprint().Has(FeatTable) {
		t.Errorf("listed: %s", app.Spec.ArchetypeID)
	}
	// An archetype named in the spec wins over the list (an --archetype choice).
	chosen := New(ToolSpec{Name: "nmap", Bin: "nmap", ArchetypeID: "general"})
	if chosen.Spec.ArchetypeID != "general" || chosen.blueprint().Has(FeatTable) {
		t.Errorf("chosen: %s table=%v", chosen.Spec.ArchetypeID, chosen.blueprint().Has(FeatTable))
	}
}
