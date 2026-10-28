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

// The archetype body is the more description, options, environment and
// other sections only; the whole help is still there for the data topics.
func TestArchBodyIsTheBodySections(t *testing.T) {
	man := "NAME\n       x9 - frobnicate widgets\n\nDESCRIPTION\n       First paragraph.\n\n       Second paragraph.\n\nOPTIONS\n       -v     verbose\n\nHOST DISCOVERY\n       ownword\n\nENVIRONMENT\n       X9_HOME   envword\n\nEXAMPLES\n       exampleword\n\nBUGS\n       bugword\n\nSEE ALSO\n       seeword(1)\n\nAUTHOR\n       authorword\n"
	help := "Usage: x9 [options]\nFrobnicate widgets.\nCommands:\n  commandword   do it\nReport bugs to:\n  reportword\n"
	p := programTextFrom("x9", man, help)
	for _, w := range []string{"Second paragraph", "verbose", "ownword", "envword"} {
		if !strings.Contains(p.ArchBody, w) {
			t.Errorf("archetype body lost %q", w)
		}
	}
	for _, w := range []string{"First paragraph", "exampleword", "bugword", "seeword", "authorword", "commandword", "reportword"} {
		if strings.Contains(p.ArchBody, w) {
			t.Errorf("archetype body keeps %q", w)
		}
	}
	for _, w := range []string{"exampleword", "bugword", "seeword", "authorword", "commandword"} {
		if !strings.Contains(p.Body, w) {
			t.Errorf("whole help lost %q", w)
		}
	}
}

// In the help, an option's line is an option under any heading, but not in
// a list of commands.
func TestHelpFlagLinesAreOptions(t *testing.T) {
	help := "Usage: x9 [options]\nFrobnicate widgets.\nOutput control:\n  -q   quiet output\nExamples:\n  -v   be verbose\n  x9 file\nCommands:\n  run   run it\n"
	secs := splitHelpSections(nil, cleanLines(help))
	for _, w := range []string{"-q   quiet output", "-v   be verbose"} {
		if !strings.Contains(secs[secOptions], w) {
			t.Errorf("options lost %q: %q", w, secs[secOptions])
		}
	}
	if !strings.Contains(secs[secExamples], "x9 file") || strings.Contains(secs[secExamples], "be verbose") {
		t.Errorf("examples: %q", secs[secExamples])
	}
	if !strings.Contains(secs[secCommands], "run   run it") {
		t.Errorf("commands: %q", secs[secCommands])
	}
}
