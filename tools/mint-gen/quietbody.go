package main

import "strings"

// Kinds of help section, by heading (see headingKind).
const (
	kindCredits  = "credits" // authors, copyright, history, reporting bugs, licence…
	kindSeeAlso  = "see also"
	kindBugs     = "bugs"
	kindNotes    = "notes"
	kindExit     = "exit status"
	kindExamples = "examples"
	kindOther    = "other"
)

// bodyLeavesOut are the help sections the archetype scorer does not read
// as part of the body: they say who wrote a program, where to report its
// bugs and which other programs to read about, not what it does, and in
// tests reading them made choices no better or worse. SEE ALSO still
// counts through the programs it names (relatedArchetypes). The data
// topics read the whole help (topicDataLevel), so this can never lower a
// data level.
var bodyLeavesOut = map[string]bool{kindCredits: true, kindSeeAlso: true, kindBugs: true}

// headingKind says what kind of section a man or help heading starts.
func headingKind(heading string) string {
	h := strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(heading), ":")))
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(h, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("REPORTING", "REPORT BUG", "AUTHOR", "COPYRIGHT", "HISTORY", "COLOPHON", "AVAILABILITY", "LICEN", "ACKNOWLEDG", "MAINTAINER", "THANKS"):
		return kindCredits
	case has("SEE ALSO"):
		return kindSeeAlso
	case has("BUG"):
		return kindBugs
	case has("EXAMPLE"):
		return kindExamples
	case has("EXIT STATUS", "RETURN VALUE", "DIAGNOSTIC"):
		return kindExit
	case has("NOTE", "CAVEAT"):
		return kindNotes
	}
	return kindOther
}

// quietBody is the man page and help without the sections in
// bodyLeavesOut. lines are the cleaned lines of text, one for each of its
// lines, used to find the headings.
func quietBody(manText, helpText string, manLines, helpLines []string) string {
	keep := func(text string, lines []string, isHeading func(string) bool) string {
		raw := strings.Split(text, "\n")
		if len(raw) != len(lines) {
			return text // cannot line them up; read it all
		}
		var b strings.Builder
		skip := false
		for i, l := range raw {
			if isHeading(lines[i]) {
				skip = bodyLeavesOut[headingKind(lines[i])]
			}
			if !skip {
				b.WriteString(l)
				b.WriteByte('\n')
			}
		}
		return strings.TrimSuffix(b.String(), "\n")
	}
	return keep(manText, manLines, isManSectionHeader) + "\n" + keep(helpText, helpLines, isHelpSectionHeader)
}
