package main

import (
	"regexp"
	"strings"
)

// scanGenericOptionLine reads an option line the other patterns miss. It
// is tried last (see scanOptionLine), and reads:
//
//	names        -d --decompress, -v|--verbose, --verbose or -v,
//	             --help, -h, and single-dash long names (-delete, -sdel)
//	a value      <num hosts>, [protocol list], {a|s|t}, NUM, L, #, file
//	a separator  two spaces or a tab, ":" or " - ", or one space before a
//	             capital letter
//
// A line that reads as a sentence ("--fs to get an overview of…") gives
// nothing, and so does a line with no separator after its names unless it
// ends there (an option alone on its line, its description below).
func scanGenericOptionLine(line string) []OptionText {
	if len(line) > maxOptionLine {
		return nil
	}
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	if indent < 1 || indent > 12 {
		return nil
	}
	rest := line[indent:]
	var names []string
	ph := ""
	for {
		m := reGenericName.FindStringSubmatch(rest)
		if m == nil {
			break
		}
		names = append(names, m[1])
		if ph == "" && m[2] != "" {
			ph = m[2]
		}
		rest = rest[len(m[0]):]
		sep := reGenericNameSep.FindString(rest)
		if sep == "" || !strings.HasPrefix(rest[len(sep):], "-") {
			break
		}
		rest = rest[len(sep):]
	}
	if len(names) == 0 {
		return nil
	}
	// A value after the names (or the same value again, after a second
	// name: "--interface=<s> or -I <s>").
	if m := reGenericValue.FindStringSubmatch(rest); m != nil {
		if ph == "" {
			ph = m[1]
		}
		rest = rest[len(m[0]):]
	}
	// find's -exec and -execdir end their command with ";" or "{} +".
	if m := reExecEnd.FindString(rest); m != "" && ph != "" {
		ph += " " + strings.TrimSpace(m)
		rest = rest[len(m):]
	}
	// A typed value in its own column, as ffmpeg and ffprobe list theirs:
	// "-colors            <string>     ED..V....... set the colors".
	if m := reTypeColumn.FindStringSubmatch(rest); m != nil && ph == "" {
		ph = m[1]
		rest = "  " + rest[len(m[0]):]
	}
	desc := ""
	switch {
	case strings.TrimSpace(rest) == "":
	case reGenericDescSep.MatchString(rest):
		desc = strings.TrimSpace(reGenericDescSep.ReplaceAllString(rest, ""))
	case reGenericCapital.MatchString(rest) && (ph != "" || len(names) > 1):
		desc = strings.TrimSpace(rest)
	default:
		return nil // a sentence, not an option line
	}
	// A lower-case value counts only when something clearly ends it.
	if ph != "" && reLowerWord.MatchString(ph) && strings.TrimSpace(rest) != "" && desc == "" {
		return nil
	}
	if reGenericProse.MatchString(line) && desc == "" {
		return nil
	}
	out := OptionText{Names: names[:1], Placeholder: ParsePlaceholder(ph), Desc: desc}
	// Short name first, then the first long name, as elsewhere.
	short, long := "", ""
	for _, n := range names {
		switch {
		case !strings.HasPrefix(n, "--") && len(n) == 2 && short == "":
			short = n
		case strings.HasPrefix(n, "--") && long == "":
			long = n
		}
	}
	switch {
	case short != "" && long != "":
		out.Names = []string{short, long}
	case short != "":
		out.Names = []string{short}
	case long != "":
		out.Names = []string{long}
	}
	return []OptionText{out}
}

var (
	// One name, with a value joined to it: "--color[=WHEN]", "--x=#",
	// "-T<0-5>", "-PO[protocol list]". A name never ends in "-" or ".".
	reGenericName = regexp.MustCompile(`^(--?[A-Za-z0-9](?:[A-Za-z0-9_.+-]*[A-Za-z0-9_])?)((?:=[^\s,|]+)|\[[^\]]{1,32}\]|<[^>]{1,32}>)?`)
	// Between two names: ", ", ",", "|", " or ", or spaces.
	reGenericNameSep = regexp.MustCompile(`^(?:\s*,\s*|\s*\|\s*|\s+or\s+|\s+)`)
	// A value after the names: <num hosts>, [chain [rulenum]], {a|b}, NUM,
	// L, #, or a lower-case word.
	reGenericValue = regexp.MustCompile(`^[ =]((?:<[^>]{1,40}>|\[[^\]]{1,40}\]|\{[^}\s]{1,40}\}|[A-Z][A-Z0-9_:./#-]{0,31}|#n?|[a-z][a-z0-9_:./-]{0,31}(?:\[[^\]\s]{1,16}\])?)(?:\.\.\.)?)`)
	// The end of the names and value, and the start of the description.
	reGenericDescSep = regexp.MustCompile(`^(?:\s*:\s+|\s{2,}|\t+|\s+-\s+)`)
	reGenericCapital = regexp.MustCompile(`^ [A-Z][a-z]`)
	reExecEnd        = regexp.MustCompile(`^\s+(?:\\?;|\{\}\s*\+|\{\}\s*\\?;)`)
	reTypeColumn     = regexp.MustCompile(`^\s{2,}(<[a-z][a-z0-9_]{1,15}>)(?:\s{2,}|\s*$)`)
	reLowerWord      = regexp.MustCompile(`^[a-z]`)
	// A sentence that starts with an option name: "--fs to get an overview
	// of", "--table-* options. Use this", "-n is ignored when".
	reGenericProse = regexp.MustCompile(`^\s*--?[A-Za-z0-9][A-Za-z0-9_.*=-]*[.,]?\s+(?:[a-z]+[,.]?\s+){2}[a-z]`)
)
