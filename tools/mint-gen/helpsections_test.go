package main

import (
	"strings"
	"testing"
)

func TestSectionKind(t *testing.T) {
	for h, want := range map[string]string{
		"NAME": "", "SYNOPSIS": "", "DESCRIPTION": secMoreDescription, "OPTIONS": secOptions, "GLOBAL OPTIONS": secOptions,
		"COMMANDS": secCommands, "SUBCOMMANDS": secCommands, "EXAMPLES": secExamples, "EXAMPLE": secExamples,
		"ENVIRONMENT VARIABLES": secEnvironment, "FILES": secFiles, "EXIT STATUS": secExitStatus, "RETURN VALUE": secExitStatus,
		"NOTES": secNotes, "CAVEATS": secNotes, "BUGS": secBugs, "REPORTING BUGS": secCredits, "SEE ALSO": secSeeAlso,
		"AUTHORS": secCredits, "COPYRIGHT": secCredits, "HISTORY": secCredits, "HOST DISCOVERY": secOther,
		"Options:": secOptions, "Available Commands:": secCommands, "Examples:": secExamples,
	} {
		if got := sectionKind(h); got != want {
			t.Errorf("%q: %q, want %q", h, got, want)
		}
	}
}

// The help sections leave out what the other parts read: NAME, SYNOPSIS, the first
// paragraph of DESCRIPTION, and the help's usage lines and summary.
func TestHelpSectionsLeaveOutTheOtherParts(t *testing.T) {
	man := "NAME\n       x9 - frobnicate widgets\n\nSYNOPSIS\n       x9 [options] FILE\n\nDESCRIPTION\n       First paragraph here.\n       Still first.\n\n       Second paragraph.\n\nOPTIONS\n       -v     verbose\n\nEXAMPLES\n       x9 file.txt\n\nAUTHOR\n       Someone\n"
	help := "Usage: x9 [options] FILE\nFrobnicate widgets quickly.\n  -q   quiet\n\nCommands:\n  run   run it\n"
	secs := splitHelpSections(cleanLines(man), cleanLines(help))
	all := strings.Join([]string{secs[secMoreDescription], secs[secOptions], secs[secCommands], secs[secExamples], secs[secCredits], secs[secOther]}, "\n")
	for _, gone := range []string{"frobnicate widgets", "[options] FILE", "First paragraph", "Still first", "Frobnicate widgets quickly"} {
		if strings.Contains(all, gone) {
			t.Errorf("%q is in a help section", gone)
		}
	}
	for sec, want := range map[string]string{secMoreDescription: "Second paragraph", secOptions: "-q   quiet", secCommands: "run   run it", secExamples: "x9 file.txt", secCredits: "Someone"} {
		if !strings.Contains(secs[sec], want) {
			t.Errorf("%s: %q missing from %q", sec, want, secs[sec])
		}
	}
	if !strings.Contains(secs[secOptions], "-v     verbose") {
		t.Errorf("man options: %q", secs[secOptions])
	}
}

// Each help section is a part of its own: a phrase there counts in that
// section, up to its own limit, and an unread section counts nothing.
func TestSectionsScoreSeparately(t *testing.T) {
	p := programText{Name: "x9", sections: map[string]string{
		secOptions:  strings.Repeat("archive compress tarball ", 20),
		secExamples: strings.Repeat("archive compress tarball ", 20),
	}}
	score := func(read ...string) int {
		p.read = read
		for _, s := range scoreEvidenceIn(broadSignalList, defaultArchWeights.Words, findEvidenceIn(broadSignalList, p)) {
			if s.ID == "pack" {
				return s.Score
			}
		}
		return 0
	}
	one, two := score(secOptions), score(secOptions, secExamples)
	if one == 0 || one > defaultArchWeights.Words.SectionCap || two != 2*one {
		t.Errorf("options %d, options and examples %d (cap %d)", one, two, defaultArchWeights.Words.SectionCap)
	}
	if got := score(secCredits); got != 0 {
		t.Errorf("an empty section scored %d", got)
	}
	if got := score(); got != 0 {
		t.Errorf("nothing read scored %d", got)
	}
}
