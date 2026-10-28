package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestChoiceConfidence(t *testing.T) {
	for _, c := range []struct {
		lead float64
		want string
	}{{clearLead, ConfidenceClear}, {clearLead + 9, ConfidenceClear}, {clearLead - 1, ConfidenceClose}, {0, ConfidenceClose}} {
		if got := choiceConfidence(c.lead); got != c.want {
			t.Errorf("lead %v: %s, want %s", c.lead, got, c.want)
		}
	}
}

// The build output says how the archetype was chosen, and for a close call
// what else it might be and the commands to check and change it.
func TestPrintArchetypeChoice(t *testing.T) {
	out := func(program string, c ArchetypeChoice, forced string) string {
		var b strings.Builder
		printArchetypeChoice(&b, program, c, forced)
		return b.String()
	}
	if s := out("nmap", ArchetypeChoice{ID: "probe", Confidence: ConfidenceListed}, ""); !strings.Contains(s, "Probe") || !strings.Contains(s, "listed") {
		t.Errorf("listed: %q", s)
	}
	if s := out("tree", ArchetypeChoice{ID: "list", Confidence: ConfidenceClear}, ""); !strings.Contains(s, "clear choice") || !strings.Contains(s, "mint-gen --explain tree") || strings.Contains(s, "CLOSE") {
		t.Errorf("clear: %q", s)
	}
	close := ArchetypeChoice{ID: "convert", Confidence: ConfidenceClose, Alternatives: []appkit.ArchetypeID{"search", "look-up"}}
	s := out("fzf", close, "")
	for _, want := range []string{"CLOSE CALL", "also likely: Search, Look up (look-up)", "mint-gen --explain fzf", "mint-gen --archetype search fzf"} {
		if !strings.Contains(s, want) {
			t.Errorf("close call: missing %q in %q", want, s)
		}
	}
	if s := out("fzf", close, "search"); !strings.Contains(s, "chosen with --archetype") || strings.Contains(s, "CLOSE") {
		t.Errorf("forced: %q", s)
	}
	// A name that is not a plain word is quoted in the commands, and control
	// characters never reach the terminal.
	s = out("my tool;rm -rf ~\x1b[2J", close, "")
	if !strings.Contains(s, "mint-gen --explain 'my tool;rm -rf ~") {
		t.Errorf("not quoted: %q", s)
	}
	if strings.ContainsAny(s, "\x1b\x07") {
		t.Errorf("control characters: %q", s)
	}
	if got := shellWord("it's"); got != `'it'\''s'` {
		t.Errorf("shellWord: %s", got)
	}
}

// --explain shows the archetype, what it means, the evidence, and the
// commands for the alternatives.
func TestExplainArchetypeChoice(t *testing.T) {
	c := chooseArchetype("finder9", manWithName("finder9", "search files for lines that match a pattern", ""), "Usage: finder9 [options] PATTERN [FILE]...\n")
	var b strings.Builder
	explainArchetypeChoice(&b, "finder9", c)
	s := b.String()
	for _, want := range []string{"Archetype: Search", "Finds what matches a pattern.", "Evidence:", "takes a pattern (PATTERN)", "it leads the next by"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	b.Reset()
	explainArchetypeChoice(&b, "nmap", ArchetypeChoice{ID: "probe", Confidence: ConfidenceListed, Why: "listed for nmap"})
	if !strings.Contains(b.String(), "listed for this program") || strings.Contains(b.String(), "Evidence:") {
		t.Errorf("listed: %s", b.String())
	}
}

// --archetype takes an archetype by ID or name; old template IDs are gone.
func TestResolveArchetypeFlag(t *testing.T) {
	for v, want := range map[string]appkit.ArchetypeID{"probe": "probe", "Look up": "look-up", "follow": "follow"} {
		if a, ok := resolveArchetypeFlag(v); !ok || a.ID != want {
			t.Errorf("%q: %s %v, want %s", v, a.ID, ok, want)
		}
	}
	for _, v := range []string{"T12", "network-scanner", "T99"} {
		if _, ok := resolveArchetypeFlag(v); ok {
			t.Errorf("%q accepted", v)
		}
	}
}
