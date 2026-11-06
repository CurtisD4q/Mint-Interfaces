package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestMembershipsFromScores(t *testing.T) {
	scores := []ArchetypeScore{{ID: "search", Score: 50}, {ID: "query", Score: 35}, {ID: "list", Score: 31}, {ID: "filter", Score: 30}, {ID: "ask", Score: 10}}
	ms := membershipsFrom("search", scores)
	var ids []string
	for _, m := range ms {
		ids = append(ids, string(m.ID))
	}
	// 60% of 50 is 30: query, list and filter qualify, but three is the most.
	if strings.Join(ids, " ") != "search query list" {
		t.Fatalf("members %v", ids)
	}
	if ms[0].Weight <= ms[1].Weight {
		t.Fatalf("primary is not the strongest: %+v", ms)
	}
	// One clear winner stays alone.
	if ms := membershipsFrom("list", []ArchetypeScore{{ID: "list", Score: 80}, {ID: "search", Score: 30}}); len(ms) != 1 || ms[0].Weight != 1 {
		t.Fatalf("clear %+v", ms)
	}
	// Small scores never make a membership.
	if ms := membershipsFrom("ask", []ArchetypeScore{{ID: "ask", Score: 12}, {ID: "list", Score: 11}}); len(ms) != 1 {
		t.Fatalf("small %+v", ms)
	}
	// A listed primary that scored low stays first.
	ms = membershipsFrom("session", []ArchetypeScore{{ID: "query", Score: 40}, {ID: "session", Score: 5}})
	if ms[0].ID != "session" || len(ms) != 2 {
		t.Fatalf("listed %+v", ms)
	}
	// No evidence at all: the primary alone.
	if ms := membershipsFrom("general", nil); len(ms) != 1 || ms[0].ID != "general" {
		t.Fatalf("none %+v", ms)
	}
}

func TestChoiceCarriesMemberships(t *testing.T) {
	c := chooseFromScores([]ArchetypeScore{{ID: "convert", Score: 60}, {ID: "pack", Score: 50}, {ID: "ask", Score: 5}})
	if len(c.Members) != 2 || c.Members[0].ID != "convert" || c.Members[1].ID != "pack" {
		t.Fatalf("%+v", c.Members)
	}
}

func TestArchetypeFlagTakesAList(t *testing.T) {
	ms, bad := resolveArchetypeList("toolkit, Records")
	if bad != "" || len(ms) != 2 || ms[0].ID != "toolkit" || ms[1].ID != "records" || ms[0].Weight != ms[1].Weight {
		t.Fatalf("%+v %q", ms, bad)
	}
	if ms, _ := resolveArchetypeList("watch+list"); len(ms) != 2 {
		t.Fatalf("plus %+v", ms)
	}
	if _, bad := resolveArchetypeList("toolkit,nonesuch"); bad != "nonesuch" {
		t.Fatalf("bad %q", bad)
	}
	if _, bad := resolveArchetypeList(" , "); bad == "" {
		t.Fatal("an empty list was accepted")
	}
}

func TestMembersLabel(t *testing.T) {
	if got := membersLabel("list", nil); got != "List" {
		t.Errorf("%q", got)
	}
	got := membersLabel("search", []appkit.Membership{{ID: "search", Weight: 0.58}, {ID: "look-up", Weight: 0.42}})
	if got != "Search 58% + Look up (look-up) 42%" {
		t.Errorf("%q", got)
	}
}

func TestScaffoldWritesMemberships(t *testing.T) {
	spec := appkit.ToolSpec{Name: "box", Bin: "box", ArchetypeID: "search",
		Archetypes: []appkit.Membership{{ID: "search", Weight: 0.58}, {ID: "query", Weight: 0.42}}}
	src, err := renderSpecGo(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `{ID: "search", Weight: 0.58}`) || !strings.Contains(string(src), `{ID: "query", Weight: 0.42}`) {
		t.Fatalf("memberships not written:\n%s", src)
	}
	one, _ := renderSpecGo(appkit.ToolSpec{Name: "a", Bin: "a", ArchetypeID: "list"})
	if strings.Contains(string(one), "Archetypes:") {
		t.Fatal("one archetype wrote a list")
	}
}

// A program is read-only only when every archetype it belongs to is.
func TestReadOnlyNeedsEveryMember(t *testing.T) {
	spec := appkit.ToolSpec{Name: "lst", ArchetypeID: "list", HideTarget: true}
	if !readOnlyArchetype(&spec, nil) {
		t.Fatal("a lister is read-only")
	}
	spec.Archetypes = []appkit.Membership{{ID: "list", Weight: 0.6}, {ID: "settings", Weight: 0.4}}
	if readOnlyArchetype(&spec, nil) {
		t.Fatal("a lister that is also a settings tool was taken as read-only")
	}
}
