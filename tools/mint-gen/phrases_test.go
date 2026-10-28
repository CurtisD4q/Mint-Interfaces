package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func manWithName(name, summary, body string) string {
	return "NAME\n       " + name + " - " + summary + "\n\nDESCRIPTION\n       " + body + "\n"
}

// scoreBroad scores the archetypes' broad phrases (subjects) alone.
func scoreBroad(name, summary string, usage []string, body string) []PhraseScore {
	p := programText{Name: name, Summary: summary, Usage: usage, Body: body}
	return scoreEvidenceIn(broadSignalList, defaultWeights, findEvidenceIn(broadSignalList, p))
}

// broadChoiceOf is the clear winner of the broad phrases, or "".
func broadChoiceOf(name, man, help string) (id, why string, scores []PhraseScore) {
	p := programTextFrom(name, man, help)
	scores = scoreEvidenceIn(broadSignalList, defaultWeights, findEvidenceIn(broadSignalList, p))
	id, why = pickGroup(scores)
	return id, why, scores
}

// Whole words only: words hidden inside other words score nothing.
func TestPhrasesAreWholeWords(t *testing.T) {
	body := "rapid toolchain csvkit unarchived instances repository-free pidgin"
	for _, s := range scoreBroad("x", "", nil, body) {
		for _, h := range s.Hits {
			t.Errorf("%s scored on %q", s.ID, h)
		}
	}
}

// A long man page cannot outweigh a clear summary.
func TestBodyIsCapped(t *testing.T) {
	body := "Shows the running processes.\n\nOPTIONS\n       " + strings.Repeat("archive compress decompress tarball ", 200)
	id, why, scores := broadChoiceOf("proclook", manWithName("proclook", "interactive process viewer", body), "")
	if id == "pack" {
		t.Fatalf("the long man page won: %s (%s): %v", id, why, scores)
	}
	for _, s := range scores {
		if s.ID == "pack" && s.Score > defaultWeights.BodyCap {
			t.Fatalf("body score %d over the cap", s.Score)
		}
	}
}

// The opening of the DESCRIPTION is read on its own, each idea once,
// however long it is; option names in it are not prose.
func TestDescriptionZone(t *testing.T) {
	man := manWithName("vcs9", "manage things", "Vcs9 is a version control system. "+strings.Repeat("It keeps every revision. ", 30)+"See --kill-line.")
	id, why, scores := broadChoiceOf("vcs9", man, "")
	if id != "history" {
		t.Fatalf("%s (%s) %v", id, why, scores)
	}
	for _, s := range scores {
		n := 0
		for _, h := range s.Hits {
			if strings.HasPrefix(h, "description: ") {
				n++
			}
		}
		if s.ID == "history" && n != 2 { // version control, revision
			t.Fatalf("description hits: %v", s.Hits)
		}
		if s.ID == "list" {
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
	if s := scoreBroad("finder9", "", nil, body); len(s) != 0 {
		t.Fatalf("option names scored: %v", s)
	}
	// In a usage line, "--json" is still a hint.
	if s := scoreBroad("q9", "", []string{"q9 [--json] FILE"}, ""); len(s) == 0 || s[0].ID != "query" {
		t.Fatalf("usage: %v", s)
	}
}

// The longer phrase is the meaning: "SQL injection" is security evidence,
// not also "SQL" data evidence.
func TestLongerPhraseWins(t *testing.T) {
	for _, sc := range scoreBroad("x", "automatic SQL injection and takeover tool", nil, "") {
		if sc.ID == "query" {
			t.Fatalf("SQL counted inside SQL injection: %v", sc.Hits)
		}
	}
}

// Word endings: "viruses", "encryption", "benchmarking" match.
func TestWordEndings(t *testing.T) {
	cases := map[string]string{"scan files for viruses": "audit", "OpenPGP encryption and signing tool": "convert",
		"command-line benchmarking tool": "measure", "read Info documents": "read"}
	for summary, want := range cases {
		scores := scoreBroad("x", summary, nil, "")
		if len(scores) == 0 || scores[0].ID != want {
			t.Errorf("%q: %v, want %s first", summary, scores, want)
		}
	}
}

// Weak, everyday words never count in the body.
func TestWeakWordsNotInBody(t *testing.T) {
	body := strings.Repeat("lines fields columns process signal search build script query database ", 20)
	if s := scoreBroad("x", "", nil, body); len(s) != 0 {
		t.Fatalf("weak body words scored: %v", s)
	}
}

// Related programs named in SEE ALSO count for their archetypes.
func TestRelatedPrograms(t *testing.T) {
	man := "NAME\n       lookup2 - query servers\n\nSEE ALSO\n       host(1), nslookup(1), named(8), resolv.conf(5).\n\nBUGS\n       none(1)\n"
	rel := seeAlsoPrograms(man)
	if strings.Join(rel, ",") != "host,nslookup,named" {
		t.Fatalf("related: %v", rel)
	}
	if got := relatedArchetypes("lookup2", rel); strings.Join(got["look-up"], ",") != "host,nslookup" {
		t.Fatalf("related archetypes: %v", got)
	}
	// A program is never its own evidence.
	if got := relatedArchetypes("dig", []string{"dig"}); len(got) != 0 {
		t.Fatalf("self counted: %v", got)
	}
}

// A program's own name in its help is not evidence: "crontab" is a
// Schedule phrase, but crontab's help naming itself says nothing new. Other words
// (and the name inside a longer word or path) still count.
func TestOwnNameIsNotEvidence(t *testing.T) {
	if s := scoreBroad("crontab", "", []string{"crontab [-u user] file", "crontab -l"}, "crontab crontab"); len(s) != 0 {
		t.Fatalf("own name counted: %v", s)
	}
	s := scoreBroad("crontab", "", []string{"crontab [-u user] file"}, "edit the cron table")
	if len(s) == 0 || s[0].ID != "schedule" {
		t.Fatalf("other words lost: %v", s)
	}
	if s := scoreBroad("cron2", "", nil, "reads /etc/crontab and runs crontab jobs"); len(s) == 0 || s[0].ID != "schedule" {
		t.Fatalf("another program's name lost: %v", s)
	}
}

// Close scores are not a decision.
func TestNoClearWinnerFallsBack(t *testing.T) {
	scores := []PhraseScore{{ID: "search", Score: 6}, {ID: "query", Score: 5}}
	if id, why := pickGroup(scores); id != "" || !strings.Contains(why, "no clear winner") {
		t.Fatalf("%q %q", id, why)
	}
	if id, _ := pickGroup([]PhraseScore{{ID: "list", Score: 7}, {ID: "query", Score: 5}}); id != "" {
		t.Fatal("a 2-point lead accepted")
	}
	if id, _ := pickGroup([]PhraseScore{{ID: "list", Score: 3}}); id != "" {
		t.Fatal("too little evidence accepted")
	}
	if id, _ := pickGroup([]PhraseScore{{ID: "list", Score: 8}, {ID: "query", Score: 3}}); id != "list" {
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
	if id, why, scores := broadChoiceOf("showit", "", help); id != "" {
		t.Fatalf("one idea decided: %s (%s) %v", id, why, scores)
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
	for _, s := range scoreBroad("mgr9", "Management user interface", nil, "") {
		if s.ID == "rules" {
			t.Fatalf("user interface counted as a network interface: %v", s.Hits)
		}
	}
	for _, s := range scoreBroad("cli9", "a command line tool", nil, "") {
		if s.ID == "query" {
			t.Fatalf("command line counted as text lines: %v", s.Hits)
		}
	}
	s := scoreBroad("sensors", "Print sensors information", nil, "")
	if len(s) == 0 || s[0].ID != "watch" {
		t.Fatalf("name in summary: %v", s)
	}
}

// In a long text the phrase search runs only around each phrase's first
// word, and finds exactly what a search of the whole text finds.
func TestWindowedSearchMatchesFull(t *testing.T) {
	var b strings.Builder
	for b.Len() <= longZone {
		b.WriteString("filler text about nothing in particular. ")
	}
	b.WriteString("PACKET CAPTURE and Captures, xcapture, capturex, packet-capture, packet   capture,\n")
	b.WriteString("queries query QUERIES; sql injection; SQL; compress compressed decompression; ")
	b.WriteString("archive archive archive; port scanner; port; portable; i/o statistics; s.m.a.r.t data")
	text := b.String()
	lower := asciiLower(text)
	if len(lower) != len(text) {
		t.Fatal("lower case changed the length")
	}
	found := 0
	var all []signal
	for _, list := range [][]signal{archetypeSignalList, broadSignalList, topicContestList} {
		all = append(all, list...)
	}
	for i, s := range all {
		full := fmt.Sprint(signalLocations(s, text, ""))
		win := fmt.Sprint(signalLocations(s, text, lower))
		if full != win {
			t.Fatalf("signal %d (%s): full %s, windowed %s", i, s.label, full, win)
		}
		if full != "[]" {
			found++
		}
	}
	if found < 8 {
		t.Fatalf("only %d signals matched; the test text is not testing much", found)
	}
}

// What a program's help is about can raise its data level: keys and
// passwords make it secret, captured traffic or logs sensitive. A topic
// decides only when it clearly leads the other subjects, and SEE ALSO
// counts for it.
func TestTopicDataLevel(t *testing.T) {
	level := func(name, man, help string) string {
		l, _ := topicDataLevel(programTextFrom(name, man, help))
		return l
	}
	if got := level("seal9", manWithName("seal9", "OpenPGP encryption and signing tool", "Encrypt files with a passphrase or a private key."), ""); got != appkit.DataSecret {
		t.Errorf("keys: %q", got)
	}
	if got := level("sniff9", manWithName("sniff9", "capture network traffic to a pcap file", "A packet sniffer."), ""); got != appkit.DataSensitive {
		t.Errorf("traffic: %q", got)
	}
	// A package manager that verifies checksums is about packages.
	pm := manWithName("pkg9", "package manager", "Install packages, remove packages and upgrade packages. Package signatures and checksums are verified.")
	if got := level("pkg9", pm, ""); got != "" {
		t.Errorf("package manager: %q", got)
	}
	// SEE ALSO naming packet capture programs counts for captured traffic.
	man := manWithName("flow9", "reconstruct TCP flows", "Reads flows.") + "\nSEE ALSO\n       tcpdump(1), tshark(1)\n"
	if got := level("flow9", man, ""); got != appkit.DataSensitive {
		t.Errorf("related: %q", got)
	}
}
