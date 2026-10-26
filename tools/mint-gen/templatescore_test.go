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
		{"proclook", "display Linux processes", "", "T19"},
		{"dirlist", "list directory contents", "", "T25"},
		{"jsrun", "server-side JavaScript runtime", "", "T10"},
		{"imgconv", "image converter and resizer", "", "T06"},
		{"packer", "compress or decompress files", "", "T08"},
		{"sumcheck", "print or check SHA256 checksums", "", "T16"},
		{"lineflip", "write each file with the last line first", "", "T26"},
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
	for _, s := range ScoreTemplates("x", "", nil, body, true) {
		for _, h := range s.Hits {
			t.Errorf("%s scored on %q", s.ID, h)
		}
	}
}

// A long man page cannot outweigh a clear summary.
func TestBodyIsCapped(t *testing.T) {
	body := strings.Repeat("archive compress decompress tarball ", 200)
	got := chooseTemplate("proclook", appkit.ToolSpec{}, manWithName("proclook", "display Linux processes", body), "")
	if got.ID != "T19" {
		t.Fatalf("%s (%s): %v", got.ID, got.Why, got.Scores)
	}
	for _, s := range got.Scores {
		if s.ID == "T08" && s.Score > maxBodyScore {
			t.Fatalf("body score %d over the cap", s.Score)
		}
	}
}

// Suite templates (version control, containers…) need subcommands.
func TestSuiteTemplatesNeedSubcommands(t *testing.T) {
	man := manWithName("vcsish", "record changes: commit, branch and merge", "")
	if got := chooseTemplate("vcsish", appkit.ToolSpec{}, man, ""); got.ID == "T21" {
		t.Fatalf("suite template without subcommands: %s", got.Why)
	}
	help := "Usage: vcsish <command>\n\nAvailable commands:\n  commit    record changes\n  branch    list branches\n  merge     join histories\n"
	if got := chooseTemplate("vcsish", appkit.ToolSpec{}, man, help); got.ID != "T21" {
		t.Fatalf("with subcommands: %s (%s) %v", got.ID, got.Why, got.Scores)
	}
}

// Close scores are not a decision.
func TestNoClearWinnerFallsBack(t *testing.T) {
	scores := []TemplateScore{{ID: "T25", Score: 6}, {ID: "T26", Score: 5}}
	if id, why := pickTemplate(scores); id != "" || !strings.Contains(why, "no clear winner") {
		t.Fatalf("%q %q", id, why)
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
