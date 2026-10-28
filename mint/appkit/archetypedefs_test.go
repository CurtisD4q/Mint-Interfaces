package appkit

import (
	"strings"
	"testing"
)

// 35: Ask … Session, and General for anything else.
func TestThirtyFiveArchetypes(t *testing.T) {
	defs := AllArchetypeDefs()
	if len(defs) != 35 {
		t.Fatalf("%d archetypes", len(defs))
	}
	seen := map[string]bool{}
	layouts := map[Archetype]bool{}
	for _, l := range AllArchetypes() {
		layouts[l] = true
	}
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
		if len(a.Prefer.Layouts) == 0 {
			t.Errorf("%s: no layout", a.ID)
		}
		for l, w := range a.Prefer.Layouts {
			if !layouts[l] || w <= 0 || w > 1 {
				t.Errorf("%s: layout %s weight %v", a.ID, l, w)
			}
		}
		for f, w := range a.Prefer.Features {
			if w < 0 || w > 1 {
				t.Errorf("%s: feature %s weight %v", a.ID, f, w)
			}
		}
		if !layouts[a.Blueprint().Layout] {
			t.Errorf("%s: blueprint layout %q", a.ID, a.Blueprint().Layout)
		}
		if a.Data != "" && a.Data != DataSensitive && a.Data != DataSecret {
			t.Errorf("%s: data level %q", a.ID, a.Data)
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
	if a, _ := ArchetypeForTool("pkill"); a.ID != "control" {
		t.Errorf("pkill: %s", a.ID)
	}
}

func TestArchetypeApplyTo(t *testing.T) {
	a, _ := LookupArchetype("rules")
	spec := ToolSpec{Name: "fw9", TargetLabel: "mine"}
	a.ApplyTo(&spec)
	if spec.ArchetypeID != "rules" || spec.Archetype != ArchetypeConfig || !spec.TableOutput || !spec.ConfirmRun || !spec.NeedsRoot {
		t.Errorf("rules: %+v", spec)
	}
	if spec.TargetLabel != "mine" {
		t.Error("overwrote the caller's target label")
	}
	w, _ := LookupArchetype("watch")
	spec = ToolSpec{Name: "w9"}
	w.ApplyTo(&spec)
	if spec.Archetype != ArchetypeMonitor || spec.PollInterval <= 0 || spec.ConfirmRun {
		t.Errorf("watch: %+v", spec)
	}
}

// Blending: weight 1 is the archetype unchanged; a style can switch a
// feature on or off by its share.
func TestBlendPreferences(t *testing.T) {
	a, _ := LookupArchetype("probe")
	style := Preferences{Layouts: map[Archetype]float64{}, Features: map[Feature]float64{FeatHistory: 0, FeatCollapsible: 1}}
	if got, want := BlueprintFrom(BlendPreferences(a.Prefer, style, 1)), a.Blueprint(); strings.Join(featNames(got), ",") != strings.Join(featNames(want), ",") {
		t.Errorf("weight 1: %v, want %v", got, want)
	}
	b := BlueprintFrom(BlendPreferences(a.Prefer, style, 0.4))
	if !b.Has(FeatCollapsible) || b.Has(FeatHistory) || b.Layout != ArchetypeScanner {
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

// New applies the archetype; an older app with only a template gets the
// archetype that template stood for, and keeps its data level.
func TestNewAppliesArchetypeOrTemplate(t *testing.T) {
	app := New(ToolSpec{Name: "nmap", Bin: "nmap"})
	if app.Spec.ArchetypeID != "probe" || app.Spec.Archetype != ArchetypeScanner {
		t.Errorf("listed: %s %s", app.Spec.ArchetypeID, app.Spec.Archetype)
	}
	// An archetype named in the spec wins over the list (a --template choice).
	chosen := New(ToolSpec{Name: "nmap", Bin: "nmap", ArchetypeID: "general"})
	if chosen.Spec.ArchetypeID != "general" || chosen.Spec.Archetype != ArchetypeRunner {
		t.Errorf("chosen: %s %s", chosen.Spec.ArchetypeID, chosen.Spec.Archetype)
	}
	old := New(ToolSpec{Name: "x9", Template: "T12"})
	if old.Spec.ArchetypeID != "probe" || old.Spec.Archetype != ArchetypeScanner || !old.Spec.TableOutput {
		t.Errorf("older app: %+v", old.Spec)
	}
	crypto := New(ToolSpec{Name: "x9", Template: "crypto-hash-tool"})
	if crypto.Spec.ArchetypeID != "convert" || crypto.Spec.Data != DataSecret {
		t.Errorf("older crypto app: %s %q", crypto.Spec.ArchetypeID, crypto.Spec.Data)
	}
}
