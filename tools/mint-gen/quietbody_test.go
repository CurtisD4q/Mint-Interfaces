package main

import (
	"strings"
	"testing"
)

// The archetype body leaves out credits, SEE ALSO and BUGS, in the man
// page and the help, and keeps the rest; the data topics still read it all.
func TestQuietBodyLeavesOutNoise(t *testing.T) {
	man := "NAME\n       tool9 - do a thing\nDESCRIPTION\n       Does the thing.\nOPTIONS\n       -x  extract\nBUGS\n       bugword\nSEE ALSO\n       seeword(1)\nAUTHOR\n       authorword\nEXAMPLES\n       exampleword\n"
	help := "Usage: tool9 [-x]\nOptions:\n  -x  extract\nReport bugs to:\n  reportword\n"
	p := programTextFrom("tool9", man, help)
	for _, w := range []string{"bugword", "seeword", "authorword", "reportword"} {
		if strings.Contains(p.ArchBody, w) {
			t.Errorf("archetype body keeps %q", w)
		}
		if !strings.Contains(p.Body, w) {
			t.Errorf("whole body lost %q", w)
		}
	}
	for _, w := range []string{"Does the thing", "extract", "exampleword"} {
		if !strings.Contains(p.ArchBody, w) {
			t.Errorf("archetype body lost %q", w)
		}
	}
	for h, want := range map[string]string{"REPORTING BUGS": kindCredits, "BUGS": kindBugs, "SEE ALSO": kindSeeAlso, "AUTHORS": kindCredits, "COPYRIGHT": kindCredits, "OPTIONS": kindOther, "Report bugs to:": kindCredits} {
		if got := headingKind(h); got != want {
			t.Errorf("headingKind(%q) = %q, want %q", h, got, want)
		}
	}
}
