package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestFitsFromScores(t *testing.T) {
	join := func(ids []appkit.ArchetypeID) string {
		var s []string
		for _, id := range ids {
			s = append(s, string(id))
		}
		return strings.Join(s, " ")
	}
	// A close call: query and list are within 20 points of search; filter
	// is too but three is the most; ask is far behind.
	scores := []ArchetypeScore{{ID: "search", Score: 50}, {ID: "query", Score: 45}, {ID: "list", Score: 35}, {ID: "filter", Score: 33}, {ID: "ask", Score: 10}}
	if got := join(fitsFrom("search", scores)); got != "search query list" {
		t.Fatalf("close call %q", got)
	}
	// A clear choice fits one.
	if got := join(fitsFrom("list", []ArchetypeScore{{ID: "list", Score: 80}, {ID: "search", Score: 55}})); got != "list" {
		t.Fatalf("clear %q", got)
	}
	// Small scores never count, however close.
	if got := join(fitsFrom("ask", []ArchetypeScore{{ID: "ask", Score: 12}, {ID: "list", Score: 11}})); got != "ask" {
		t.Fatalf("small %q", got)
	}
	// A listed main archetype stays first, and the best-scoring one joins it.
	if got := join(fitsFrom("session", []ArchetypeScore{{ID: "query", Score: 40}, {ID: "session", Score: 30}})); got != "session query" {
		t.Fatalf("listed %q", got)
	}
	if got := join(fitsFrom("general", nil)); got != "general" {
		t.Fatalf("none %q", got)
	}
}

// Fitting more than one archetype is exactly a close call.
func TestCloseCallsFitMoreThanOne(t *testing.T) {
	close := chooseFromScores([]ArchetypeScore{{ID: "convert", Score: 60}, {ID: "pack", Score: 50}, {ID: "ask", Score: 5}})
	if close.Confidence != ConfidenceClose || len(close.Fits) != 2 || close.Fits[1] != "pack" {
		t.Fatalf("close %+v", close)
	}
	clear := chooseFromScores([]ArchetypeScore{{ID: "convert", Score: 80}, {ID: "pack", Score: 50}, {ID: "ask", Score: 5}})
	if clear.Confidence != ConfidenceClear || len(clear.Fits) != 1 {
		t.Fatalf("clear %+v", clear)
	}
}

func TestArchetypeFlagTakesAList(t *testing.T) {
	ids, bad := resolveArchetypeList("toolkit, Records")
	if bad != "" || len(ids) != 2 || ids[0] != "toolkit" || ids[1] != "records" {
		t.Fatalf("%v %q", ids, bad)
	}
	if ids, _ := resolveArchetypeList("watch+list"); len(ids) != 2 {
		t.Fatalf("plus %v", ids)
	}
	if _, bad := resolveArchetypeList("toolkit,nonesuch"); bad != "nonesuch" {
		t.Fatalf("bad %q", bad)
	}
	if _, bad := resolveArchetypeList(" , "); bad == "" {
		t.Fatal("an empty list was accepted")
	}
}

func TestFitsLabel(t *testing.T) {
	if got := fitsLabel("list", nil); got != "List" {
		t.Errorf("%q", got)
	}
	if got := fitsLabel("search", []appkit.ArchetypeID{"search", "look-up"}); got != "Search + Look up (look-up)" {
		t.Errorf("%q", got)
	}
}

func TestScaffoldWritesTheArchetypes(t *testing.T) {
	spec := appkit.ToolSpec{Name: "box", Bin: "box", ArchetypeID: "search", Archetypes: []appkit.ArchetypeID{"search", "query"}}
	src, err := renderSpecGo(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `Archetypes:`) || !strings.Contains(string(src), `[]appkit.ArchetypeID{"search", "query"}`) {
		t.Fatalf("archetypes not written:\n%s", src)
	}
	one, _ := renderSpecGo(appkit.ToolSpec{Name: "a", Bin: "a", ArchetypeID: "list"})
	if strings.Contains(string(one), "Archetypes:") {
		t.Fatal("one archetype wrote a list")
	}
}

// A program is read-only only when every archetype it fits is.
func TestReadOnlyNeedsEveryArchetype(t *testing.T) {
	spec := appkit.ToolSpec{Name: "lst", ArchetypeID: "list", HideTarget: true}
	if !readOnlyArchetype(&spec, nil) {
		t.Fatal("a lister is read-only")
	}
	spec.Archetypes = []appkit.ArchetypeID{"list", "settings"}
	if readOnlyArchetype(&spec, nil) {
		t.Fatal("a lister that is also a settings tool was taken as read-only")
	}
}
