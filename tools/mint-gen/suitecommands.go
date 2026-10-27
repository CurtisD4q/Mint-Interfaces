package main

import (
	"regexp"
	"strings"
)

// suiteCommandNames reads the names of a program's subcommands (or of its
// mode options, for programs like gpg whose "Commands:" are options), in
// the many ways programs list them, for the archetype step:
//
//	Commands:            git:  These are common Git commands…
//	  start  Start it          clone      Clone a repository
//	apt:  list - list packages        gh:  auth:   Authenticate gh
//	svn:  Available subcommands:      openssl:  Standard commands
//	         add                       asn1parse   ca   ciphers
//	ip:   OBJECT := { address | link | route }
//	cargo:  build, b    Compile the current package  (a name and its aliases)
//
// It reads the help first, and the man page only when the help has none.
// It is separate from extractSubcommands, which builds the app's tabs.
func suiteCommandNames(manLines, helpLines []string) []string {
	if names := commandNamesIn(helpLines); len(names) >= 2 {
		return names
	}
	return commandNamesIn(manLines)
}

var (
	reCmdHeading  = regexp.MustCompile(`(?i)\b(?:sub)?commands\b`)
	reCmdLine     = regexp.MustCompile(`^\s{1,10}([a-z][a-z0-9_-]{1,24})(?:,\s*[a-z][a-z0-9_-]{0,24})*(?::\s|:$|\s+-\s|\s{2,}|\s*\(|\s*$)`)
	reCmdOption   = regexp.MustCompile(`^\s{1,10}(?:-\w,\s*)?--([a-z][a-z0-9-]{1,30})\b`)
	reCmdColumns  = regexp.MustCompile(`^\s*[a-z][a-z0-9_.-]+(?:\s{2,}[a-z][a-z0-9_.-]+){2,}\s*$`)
	reCmdBraces   = regexp.MustCompile(`\{\s*([a-z][\w-]*(?:\s*\|\s*[a-z][\w-]*){2,})\s*\|?\s*\}`)
	reCmdAllCaps  = regexp.MustCompile(`^[A-Z][A-Z ]{3,}$`)
	reOptsHeading = regexp.MustCompile(`(?i)^\s*(?:global\s+|common\s+|general\s+)?(?:options|flags|arguments)\s*:?\s*$`)
)

// Words that a wrapped sentence can leave at the start of an indented line.
var cmdFiller = map[string]bool{"a": true, "an": true, "the": true, "is": true, "are": true, "to": true, "of": true,
	"and": true, "or": true, "in": true, "on": true, "for": true, "with": true, "by": true, "it": true, "if": true,
	"be": true, "as": true, "at": true, "this": true, "that": true, "which": true, "from": true, "not": true,
	"usage": true, "example": true, "examples": true, "see": true, "note": true, "default": true, "help": true, "version": true}

func commandNamesIn(lines []string) []string {
	var out []string
	seen := map[string]bool{}
	addName := func(n string) {
		n = strings.ToLower(n)
		if len(n) < 2 || cmdFiller[n] || seen[n] || len(out) >= 80 {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	// { a | b | c } in a usage line or an "OBJECT := { ... }" definition
	// (joined across the lines it wraps over), not in an option's
	// description, where braces list the option's values.
	for i, l := range lines {
		low := strings.ToLower(strings.TrimSpace(l))
		if !strings.Contains(l, ":=") && !strings.HasPrefix(low, "usage") {
			continue
		}
		// Join the next lines only while a brace is still open.
		text := l
		for j := i + 1; j < len(lines) && j < i+6 && strings.Count(text, "{") > strings.Count(text, "}"); j++ {
			text += " " + lines[j]
		}
		for _, m := range reCmdBraces.FindAllStringSubmatch(text, 2) {
			for _, n := range strings.Split(m[1], "|") {
				addName(strings.TrimSpace(n))
			}
		}
	}
	in, misses := false, 0
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		head := !strings.HasPrefix(t, "-") && len(strings.Fields(t)) <= 10 && len(t) < 80
		switch {
		case head && reCmdHeading.MatchString(t) && (!strings.HasPrefix(l, " ") || strings.HasSuffix(t, ":") || reCmdAllCaps.MatchString(t)):
			in, misses = true, 0
			continue
		case in && (reOptsHeading.MatchString(t) || (reCmdAllCaps.MatchString(t) && !reCmdHeading.MatchString(t))):
			in = false
			continue
		}
		if !in {
			continue
		}
		switch {
		case reCmdColumns.MatchString(l):
			for _, n := range strings.Fields(t) {
				addName(n)
			}
		case reCmdLine.MatchString(l):
			addName(reCmdLine.FindStringSubmatch(l)[1])
		case reCmdOption.MatchString(l):
			addName(reCmdOption.FindStringSubmatch(l)[1])
		default:
			// A sub-heading ("start a working area") or a wrapped
			// description; too many in a row end the list.
			if misses++; misses > 12 {
				in = false
			}
			continue
		}
		misses = 0
	}
	return out
}
