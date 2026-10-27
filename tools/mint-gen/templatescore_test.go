package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func manWithName(name, summary, body string) string {
	return "NAME\n       " + name + " - " + summary + "\n\nDESCRIPTION\n       " + body + "\n"
}

// Names below are not in any template's tool list, so the scorer decides.
func TestChooseTemplateFromSummary(t *testing.T) {
	cases := []struct {
		name, summary, body, want string
	}{
		{"proclook", "interactive process viewer", "", "T19"},
		{"proclist", "display Linux processes", "", "T05"}, // only ordinary words: too weak to decide
		{"dirlist", "list directory contents", "", "T25"},
		{"jsrun", "server-side JavaScript runtime", "", "T10"},
		{"imgconv", "image converter and resizer", "", "T06"},
		{"packer", "compress or decompress files", "", "T08"},
		{"sumcheck", "print or check SHA256 checksums", "", "T16"},
		{"lineflip", "a stream editor for filtering and transforming text", "", "T26"},
		{"fsmount", "mount a filesystem", "", "T24"},
		{"nothing", "do something unusual", "", "T05"}, // no evidence: the general runner
	}
	for _, c := range cases {
		got := chooseTemplate(c.name, appkit.ToolSpec{Name: c.name}, manWithName(c.name, c.summary, c.body), "")
		if got.ID != c.want {
			t.Errorf("%s (%q): %s (%s), want %s; scores %v", c.name, c.summary, got.ID, got.Why, c.want, got.Scores)
		}
	}
}

// Whole words only: words hidden inside other words score nothing.
func TestTemplateWordsAreWholeWords(t *testing.T) {
	body := "rapid toolchain csvkit unarchived instances repository-free pidgin"
	for _, s := range ScoreTemplates("x", "", nil, body, nil) {
		for _, h := range s.Hits {
			t.Errorf("%s scored on %q", s.ID, h)
		}
	}
}

// A long man page cannot outweigh a clear summary.
func TestBodyIsCapped(t *testing.T) {
	body := "Shows the running processes.\n\nOPTIONS\n       " + strings.Repeat("archive compress decompress tarball ", 200)
	got := chooseTemplate("proclook", appkit.ToolSpec{}, manWithName("proclook", "interactive process viewer", body), "")
	if got.ID == "T08" {
		t.Fatalf("the long man page won: %s (%s): %v", got.ID, got.Why, got.Scores)
	}
	for _, s := range got.Scores {
		if s.ID == "T08" && s.Score > defaultWeights.BodyCap {
			t.Fatalf("body score %d over the cap", s.Score)
		}
	}
}

// The opening of the DESCRIPTION is read on its own, each idea once,
// however long it is; option names in it are not prose.
func TestDescriptionZone(t *testing.T) {
	man := manWithName("vcs9", "manage things", "Vcs9 is a version control system. "+strings.Repeat("It keeps every revision. ", 30)+"See --kill-line.")
	got := scoreTemplateChoice("vcs9", man, "")
	if got.ID != "T21" {
		t.Fatalf("%s (%s) %v", got.ID, got.Why, got.Scores)
	}
	for _, s := range got.Scores {
		n := 0
		for _, h := range s.Hits {
			if strings.HasPrefix(h, "description: ") {
				n++
			}
		}
		if s.ID == "T21" && n != 2 { // version control, revision
			t.Fatalf("description hits: %v", s.Hits)
		}
		if s.ID == "T19" {
			t.Fatalf("an option name counted as prose: %v", s.Hits)
		}
	}
	if d := extractDescription(cleanLines(man)); len(d) > 600 {
		t.Fatalf("description %d characters", len(d))
	}
}

// A signal word joined to an option or key name by a hyphen is not prose.
func TestHyphenatedNamesAreNotProse(t *testing.T) {
	body := "  --smart-case   case-insensitive unless\n  --layout=TYPE\n  ctrl-k  kill-line\n"
	if s := ScoreTemplates("finder9", "", nil, body, nil); len(s) != 0 {
		t.Fatalf("option names scored: %v", s)
	}
	// In a usage line, "--json" is still a hint.
	if s := ScoreTemplates("q9", "", []string{"q9 [--json] FILE"}, "", nil); len(s) == 0 || s[0].ID != "T26" {
		t.Fatalf("usage: %v", s)
	}
}

// The longer phrase is the meaning: "SQL injection" is security evidence,
// not also "SQL" data evidence.
func TestLongerPhraseWins(t *testing.T) {
	for _, sc := range ScoreTemplates("x", "automatic SQL injection and takeover tool", nil, "", nil) {
		if sc.ID == "T26" {
			t.Fatalf("SQL counted inside SQL injection: %v", sc.Hits)
		}
	}
}

// Word endings: "viruses", "encryption", "benchmarking" match.
func TestWordEndings(t *testing.T) {
	cases := map[string]string{"scan files for viruses": "T15", "OpenPGP encryption and signing tool": "T16",
		"command-line benchmarking tool": "T11", "read Info documents": "T27"}
	for summary, want := range cases {
		scores := ScoreTemplates("x", summary, nil, "", nil)
		if len(scores) == 0 || scores[0].ID != want {
			t.Errorf("%q: %v, want %s first", summary, scores, want)
		}
	}
}

// Weak, everyday words never count in the body.
func TestWeakWordsNotInBody(t *testing.T) {
	body := strings.Repeat("lines fields columns process signal search build script query database ", 20)
	if s := ScoreTemplates("x", "", nil, body, nil); len(s) != 0 {
		t.Fatalf("weak body words scored: %v", s)
	}
}

// Related programs named in SEE ALSO count for their templates.
func TestRelatedPrograms(t *testing.T) {
	man := "NAME\n       lookup2 - query servers\n\nSEE ALSO\n       host(1), nslookup(1), named(8), resolv.conf(5).\n\nBUGS\n       none(1)\n"
	rel := seeAlsoPrograms(man)
	if strings.Join(rel, ",") != "host,nslookup,named" {
		t.Fatalf("related: %v", rel)
	}
	got := scoreTemplateChoice("lookup2", man, "")
	if len(got.Scores) == 0 || got.Scores[0].ID != "T14" || !strings.Contains(strings.Join(got.Scores[0].Hits, ","), "related: host,related: nslookup") {
		t.Fatalf("%s (%s) %v", got.ID, got.Why, got.Scores)
	}
	// A program is never its own evidence.
	if s := ScoreTemplates("dig", "", nil, "", []string{"dig"}); len(s) != 0 {
		t.Fatalf("self counted: %v", s)
	}
}

// A program's own name in its help is not evidence: "crontab" is a T28
// signal, but crontab's help naming itself says nothing new. Other words
// (and the name inside a longer word or path) still count.
func TestOwnNameIsNotEvidence(t *testing.T) {
	if s := ScoreTemplates("crontab", "", []string{"crontab [-u user] file", "crontab -l"}, "crontab crontab", nil); len(s) != 0 {
		t.Fatalf("own name counted: %v", s)
	}
	s := ScoreTemplates("crontab", "", []string{"crontab [-u user] file"}, "edit the cron table", nil)
	if len(s) == 0 || s[0].ID != "T28" {
		t.Fatalf("other words lost: %v", s)
	}
	if s := ScoreTemplates("cron2", "", nil, "reads /etc/crontab and runs crontab jobs", nil); len(s) == 0 || s[0].ID != "T28" {
		t.Fatalf("another program's name lost: %v", s)
	}
}

// Close scores are not a decision.
func TestNoClearWinnerFallsBack(t *testing.T) {
	scores := []TemplateScore{{ID: "T25", Score: 6}, {ID: "T26", Score: 5}}
	if id, why := pickTemplate(scores); id != "" || !strings.Contains(why, "no clear winner") {
		t.Fatalf("%q %q", id, why)
	}
	if id, _ := pickTemplate([]TemplateScore{{ID: "T19", Score: 7}, {ID: "T26", Score: 5}}); id != "" {
		t.Fatal("a 2-point lead accepted")
	}
	if id, _ := pickTemplate([]TemplateScore{{ID: "T19", Score: 3}}); id != "" {
		t.Fatal("too little evidence accepted")
	}
	if id, _ := pickTemplate([]TemplateScore{{ID: "T19", Score: 8}, {ID: "T26", Score: 3}}); id != "T19" {
		t.Fatal("clear winner refused")
	}
}

func TestLooksLikeSummary(t *testing.T) {
	cases := map[string]bool{
		"Or:  /usr/bin/ln [OPTION]... TARGET":                              false,
		"Usage: top [options]":                                             false,
		"top [options]":                                                    false,
		"Pdftotext version 24.02.0":                                        false,
		"Copyright (C) 2023 Free Software Foundation":                      false,
		"SUMMARY OF LESS COMMANDS":                                         false,
		"Input options:":                                                   false,
		"Concatenate FILE(s) to standard output.":                          true,
		"tar saves many files together into a single tape or disk archive": true,
		"Print the first 10 lines of each FILE to standard output.":        true,
	}
	for line, want := range cases {
		name := "tool"
		switch {
		case strings.Contains(line, "/usr/bin/ln"):
			name = "ln"
		case strings.Contains(line, "top ["):
			name = "top"
		case strings.HasPrefix(line, "Pdftotext"):
			name = "pdftotext"
		case strings.HasPrefix(line, "tar "):
			name = "tar"
		}
		if got := looksLikeSummary(name, line); got != want {
			t.Errorf("%q: %v", line, got)
		}
	}
}

// Word forms of one idea count once: "line"/"lines" alone decide nothing.
func TestWordFormsCountOnce(t *testing.T) {
	help := "Usage: showit FILE\n  -n   number the lines\n  -s   squeeze each blank line\n  -w   wrap long lines at each line end"
	got := chooseTemplate("showit", appkit.ToolSpec{}, "", help)
	if got.ID != "T05" {
		t.Fatalf("one idea decided the template: %s (%s) %v", got.ID, got.Why, got.Scores)
	}
}

// A summary that says nothing about the program gives way to one that
// does: the help's first line, then the DESCRIPTION's first sentence.
func TestSummaryFallbacks(t *testing.T) {
	for _, c := range []struct{ name, man, help, want string }{
		{"bench9", "NAME\n       bench9 - Bench9\n\nDESCRIPTION\n       A command-line benchmarking tool.\n", "", "A command-line benchmarking tool"},
		{"pm9", "NAME\n       pm9 - PM9 Command Reference\n\nDESCRIPTION\n       PM9 is a package manager for RPM-based dis-\n       tributions. It does more.\n", "", "PM9 is a package manager for RPM-based distributions"},
		{"ctl9", "NAME\n       ctl9 - Ctl9 Documentation\n", "ctl9 -- control applications run by daemon9 from the cmd line.\n\nUsage: ctl9 [options]\n", "Control applications run by daemon9 from the cmd line."},
		// A usage fragment is not a summary.
		{"pages9", "", "((-u|--update) | [-p|--platform PLATFORM] [-L LOCALE]\npages9 shows simple help pages\n", "Pages9 shows simple help pages"},
		// A good NAME line is kept.
		{"grab9", "NAME\n       grab9 - download files from the web\n\nDESCRIPTION\n       Grab9 is a program.\n", "", "Download files from the web"},
	} {
		if got := extractSummary(c.name, cleanLines(c.man), cleanLines(c.help)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// A copyright line that looks like "name - description" is not one.
	if looksLikeSummary("tool9", "tool9 - Copyright (c) 2020 Someone") {
		t.Error("copyright line taken as a summary")
	}
}

// "user interface" is not a network interface and "command line" is not
// a line of text, and the program's name counts in its own summary.
func TestNeutralPhrasesAndNameInSummary(t *testing.T) {
	for _, s := range ScoreTemplates("mgr9", "Management user interface", nil, "", nil) {
		if s.ID == "T04" {
			t.Fatalf("user interface counted as a network interface: %v", s.Hits)
		}
	}
	for _, s := range ScoreTemplates("cli9", "a command line tool", nil, "", nil) {
		if s.ID == "T26" {
			t.Fatalf("command line counted as text lines: %v", s.Hits)
		}
	}
	s := ScoreTemplates("sensors", "Print sensors information", nil, "", nil)
	if len(s) == 0 || s[0].ID != "T17" {
		t.Fatalf("name in summary: %v", s)
	}
}
