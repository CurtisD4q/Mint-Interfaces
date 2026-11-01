package main

import (
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// A contrast link settles a close call between its two archetypes by their
// contrast words, and does nothing when they are not the top two.
func TestContrastSettlesLinkedNeighbours(t *testing.T) {
	p := programTextFrom("rec9", "NAME\n       rec9 - capture audio from the microphone and save it\n", "")
	close := []ArchetypeScore{{ID: "convert", Score: 50}, {ID: "record", Score: 45}, {ID: "pack", Score: 10}}
	if got := applyContrast(p, close); got[0].ID != "record" {
		t.Errorf("record vs convert: %v", got[0])
	}
	other := []ArchetypeScore{{ID: "convert", Score: 50}, {ID: "pack", Score: 45}, {ID: "record", Score: 10}}
	if got := applyContrast(p, other); got[0].ID != "convert" || got[0].Score != 50 {
		t.Errorf("unlinked pair changed: %v", got[0])
	}
	for _, l := range contrastLinks {
		for _, id := range []appkit.ArchetypeID{l.a, l.b} {
			if _, ok := appkit.LookupArchetype(string(id)); !ok {
				t.Errorf("link names unknown archetype %q", id)
			}
		}
	}
}
