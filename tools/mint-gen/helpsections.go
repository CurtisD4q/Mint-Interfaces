package main

import (
	"strings"
)

// Help sections: the parts of a program's man page and --help that the
// name, summary, description and usage do not already read.
const (
	secMoreDescription = "more description" // DESCRIPTION after its first paragraph, and the help's opening text
	secOptions         = "options"
	secCommands        = "commands"
	secExamples        = "examples"
	secEnvironment     = "environment"
	secFiles           = "files"
	secExitStatus      = "exit status"
	secNotes           = "notes"
	secBugs            = "bugs"
	secSeeAlso         = "see also"
	secCredits         = "credits" // authors, copyright, history, reporting bugs…
	secOther           = "other"   // headings of the program's own ("HOST DISCOVERY")
)

// bodySections are the help sections the archetype phrases are read in as
// the fifth part (the body), together: more description, options,
// environment and the program's own headings. The others (commands,
// examples, files, exit status, notes, bugs, see also, credits) say little
// about what kind of program it is, and reading them made choices no
// better (tested on 351 programs, MINT_ARCHETYPE_BENCH=sections and
// grouped-plus on the body-sections branch). Behaviour cues and the data
// topics still read the whole help, so this can never lower a data level
// through them.
var bodySections = []string{secMoreDescription, secOptions, secEnvironment, secOther}

// archBody is the text of the body sections of a program's help.
func archBody(manLines, helpLines []string) string {
	secs := splitHelpSections(manLines, helpLines)
	var parts []string
	for _, s := range bodySections {
		if t := secs[s]; t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

// sectionKind says which body section a heading belongs to; "" for the
// parts read elsewhere (NAME, SYNOPSIS).
func sectionKind(heading string) string {
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
	case h == "NAME" || h == "SYNOPSIS" || h == "USAGE":
		return ""
	case h == "DESCRIPTION":
		return secMoreDescription
	case has("REPORTING", "AUTHOR", "COPYRIGHT", "HISTORY", "COLOPHON", "AVAILABILITY", "LICEN", "ACKNOWLEDG", "STANDARDS", "CONFORMING", "VERSIONS", "MAINTAINER", "THANKS"):
		return secCredits
	case has("SEE ALSO"):
		return secSeeAlso
	case has("EXAMPLE"):
		return secExamples
	case has("COMMAND"):
		return secCommands
	case has("OPTION", "FLAG", "ARGUMENT", "PARAMETER", "SWITCH"):
		return secOptions
	case has("ENVIRONMENT"):
		return secEnvironment
	case has("EXIT", "RETURN VALUE", "DIAGNOSTIC", "ERRORS"):
		return secExitStatus
	case has("FILE"):
		return secFiles
	case has("NOTE", "CAVEAT", "WARNING", "LIMITATION"):
		return secNotes
	case has("BUG"):
		return secBugs
	}
	return secOther
}

// splitHelpSections splits the man page and --help into help sections,
// leaving out what the other parts read: NAME and SYNOPSIS, the first paragraph of
// DESCRIPTION, and the help's usage lines and summary line.
func splitHelpSections(manLines, helpLines []string) map[string]string {
	out := map[string]*strings.Builder{}
	add := func(sec, line string) {
		if sec == "" {
			return
		}
		if out[sec] == nil {
			out[sec] = &strings.Builder{}
		}
		out[sec].WriteString(line)
		out[sec].WriteByte('\n')
	}

	// The man page, by its headings. DESCRIPTION's first paragraph is the
	// description part.
	sec, firstPara, seenText := "", false, false
	for _, l := range manLines {
		if isManSectionHeader(l) {
			sec = sectionKind(l)
			firstPara, seenText = sec == secMoreDescription, false
			continue
		}
		t := strings.TrimSpace(l)
		if firstPara {
			if t == "" {
				if seenText {
					firstPara = false
				}
				continue
			}
			seenText = true
			continue
		}
		add(sec, l)
	}

	// The help: before its first heading come the usage lines, the summary
	// and often an introduction; option lines there are options.
	sec, summarySeen := secMoreDescription, false
	headed := false
	for _, l := range helpLines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		lower := strings.ToLower(t)
		if isHelpSectionHeader(l) {
			sec, headed = sectionKind(t), true
			if sec == secMoreDescription || sec == "" {
				sec = secOther
			}
			continue
		}
		if !headed {
			switch {
			case strings.HasPrefix(lower, "usage:") || strings.HasPrefix(lower, "or:") || strings.HasPrefix(lower, "synopsis"):
				continue
			case strings.HasPrefix(t, "-"):
				add(secOptions, l)
				continue
			case !summarySeen:
				summarySeen = true // the summary part reads it
				continue
			}
		}
		// An option's line is an option under any heading ("Output
		// control:", "Examples:"), except in a list of commands.
		if strings.HasPrefix(t, "-") && sec != secCommands {
			add(secOptions, l)
			continue
		}
		add(sec, l)
	}
	res := map[string]string{}
	for k, b := range out {
		res[k] = b.String()
	}
	return res
}
