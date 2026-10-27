package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestChoiceConfidence(t *testing.T) {
	for _, c := range []struct {
		arch, tpl int
		want      string
	}{{3, 3, ConfidenceClear}, {9, 9, ConfidenceClear}, {2, 9, ConfidenceClose}, {9, 2, ConfidenceClose}, {0, 0, ConfidenceClose}} {
		if got := choiceConfidence(c.arch, c.tpl); got != c.want {
			t.Errorf("leads %d/%d: %s, want %s", c.arch, c.tpl, got, c.want)
		}
	}
	// A clear archetype with no word evidence for any of its templates gets
	// the archetype's general template, and that is a close call.
	c := chooseTemplate("tick9", appkit.ToolSpec{}, "", "Usage: tick9 [-v] [delay [count]]\n")
	if c.Archetype != appkit.ArchetypeMonitor || c.ArchetypeLead < clearArchetypeLead || c.Confidence != ConfidenceClose {
		t.Errorf("fallback within a clear archetype: %+v", c)
	}
}

// The build output says how the template was chosen, and for a close call
// what else it might be and the commands to check and change it.
func TestPrintTemplateChoice(t *testing.T) {
	out := func(program string, c TemplateChoice, forced string) string {
		var b strings.Builder
		printTemplateChoice(&b, program, c, forced)
		return b.String()
	}
	if s := out("nmap", TemplateChoice{ID: "T12", Archetype: appkit.ArchetypeScanner, Confidence: ConfidenceListed}, ""); !strings.Contains(s, "T12 network-scanner") || !strings.Contains(s, "listed") {
		t.Errorf("listed: %q", s)
	}
	if s := out("tree", TemplateChoice{ID: "T25", Archetype: appkit.ArchetypeExplorer, Confidence: ConfidenceClear}, ""); !strings.Contains(s, "clear choice") || !strings.Contains(s, "mint-gen --explain tree") || strings.Contains(s, "CLOSE") {
		t.Errorf("clear: %q", s)
	}
	close := TemplateChoice{ID: "T06", Archetype: appkit.ArchetypeRunner, Confidence: ConfidenceClose, Alternatives: []string{"T25", "T24"}}
	s := out("fzf", close, "")
	for _, want := range []string{"CLOSE CALL", "also likely: T25 content-searcher, T24 disk-inspector", "mint-gen --explain fzf", "mint-gen --template T25 fzf"} {
		if !strings.Contains(s, want) {
			t.Errorf("close call: missing %q in %q", want, s)
		}
	}
	if s := out("fzf", close, "T25"); !strings.Contains(s, "chosen with --template") || strings.Contains(s, "CLOSE") {
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

// --explain shows the archetypes, the templates within the chosen one, and
// the commands for the alternatives.
func TestExplainTemplateChoice(t *testing.T) {
	c := chooseTemplate("finder9", appkit.ToolSpec{}, manWithName("finder9", "search files for lines that match a pattern", ""), "Usage: finder9 [options] PATTERN [FILE]...\n")
	var b strings.Builder
	explainTemplateChoice(&b, "finder9", c)
	s := b.String()
	for _, want := range []string{"Template: T25", "Archetype: explorer", "takes a pattern or query", "Within it:", "it leads the next archetype by"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	b.Reset()
	explainTemplateChoice(&b, "nmap", TemplateChoice{ID: "T12", Confidence: ConfidenceListed, Why: "listed for nmap"})
	if !strings.Contains(b.String(), "listed for this program") || strings.Contains(b.String(), "Archetype:") {
		t.Errorf("listed: %s", b.String())
	}
}
