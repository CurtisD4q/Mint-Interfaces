package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func formSummary(f UsageForm) string {
	var parts []string
	for _, p := range f.Positionals {
		parts = append(parts, p.Name+"="+string(p.Role))
	}
	return strings.Join(parts, " ")
}

func TestParseSynopsis(t *testing.T) {
	cases := []struct {
		tool, help string
		want       []string // one per usage form
	}{
		{"cp", "Usage: cp [OPTION]... [-T] SOURCE DEST\n  or:  cp [OPTION]... SOURCE... DIRECTORY\n  or:  cp [OPTION]... -t DIRECTORY SOURCE...",
			[]string{"SOURCE=input-file DEST=output-file", "SOURCE=input-file DIRECTORY=output-dir", "SOURCE=input-file"}},
		{"ln", "Usage: ln [OPTION]... [-T] TARGET LINK_NAME", []string{"TARGET=input-file LINK_NAME=output-file"}},
		{"ffmpeg", "usage: ffmpeg [options] [[infile options] -i infile]... {[outfile options] outfile}...", []string{"outfile=output-file"}},
		{"curl", "Usage: curl [options...] <url>", []string{"url="}},
		{"jq", "Usage:\tjq [options] <jq filter> [file...]", []string{"jq filter= file=input-file"}},
		{"grep", "Usage: grep [OPTION]... PATTERNS [FILE]...", []string{"PATTERNS= FILE=input-file"}},
		{"dd", "Usage: dd [OPERAND]...\n  or:  dd OPTION", []string{"", ""}},
		{"other", "Usage: someone-else FILE", nil}, // not this program's line
	}
	for _, c := range cases {
		s := ParseSynopsis(c.tool, nil, cleanLines(c.help))
		var got []string
		for _, f := range s.Forms {
			got = append(got, formSummary(f))
		}
		if strings.Join(got, " | ") != strings.Join(c.want, " | ") {
			t.Errorf("%s:\n got  %q\n want %q", c.tool, got, c.want)
		}
	}
	spec := ParseManAndHelp("cp", "", "Usage: cp SOURCE DEST\n")
	if len(spec.PositionalRoles) != 2 || spec.PositionalRoles[0] != appkit.RoleInputFile || spec.PositionalRoles[1] != appkit.RoleOutputFile {
		t.Fatalf("cp roles %v", spec.PositionalRoles)
	}
	s := ParseSynopsis("ffmpeg", nil, cleanLines("usage: ffmpeg [options] {[outfile options] outfile}..."))
	if p, _, ok := s.OutputPositional(); !ok || p.Name != "outfile" || p.Role != appkit.RoleOutputFile || !p.Repeat {
		t.Fatalf("ffmpeg output positional: %+v %v", p, ok)
	}
}

// Hostile or huge usage text is bounded.
func TestParseSynopsisLimits(t *testing.T) {
	long := "Usage: x " + strings.Repeat("A ", 5000)
	if s := ParseSynopsis("x", nil, []string{long}); len(s.Forms) != 0 {
		t.Fatal("over-long usage line read")
	}
	many := strings.Repeat("Usage: x FILE\n", 100)
	var lines []string
	for i, l := range strings.Split(many, "\n") {
		lines = append(lines, l+strings.Repeat(" ", i%7)+string(rune('a'+i%26)))
	}
	if s := ParseSynopsis("x", nil, lines); len(s.Forms) > maxUsageLines {
		t.Fatalf("%d forms", len(s.Forms))
	}
	toks := tokenizeUsage(strings.Repeat("[", 10000) + strings.Repeat("B ", 100))
	if len(toks) > maxTokens {
		t.Fatalf("%d tokens", len(toks))
	}
}
