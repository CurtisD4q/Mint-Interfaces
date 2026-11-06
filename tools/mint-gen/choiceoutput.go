package main

import (
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// How an archetype choice is shown: one line in the build output (two for
// a close call, with what else it might be and how to change it), and the
// whole reasoning in --explain. Everything printed is cleaned of control
// characters first, since parts of it come from a program's help.

var reShellSafe = regexp.MustCompile(`^[A-Za-z0-9._+/-]+$`)

// shellWord quotes a program name for a command the user may copy.
func shellWord(s string) string {
	if reShellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// archetypeLabel is "Look up (look-up)".
func archetypeLabel(id appkit.ArchetypeID) string {
	a, ok := appkit.LookupArchetype(string(id))
	if !ok {
		return string(id)
	}
	if strings.EqualFold(a.Name, string(a.ID)) {
		return a.Name
	}
	return fmt.Sprintf("%s (%s)", a.Name, a.ID)
}

// membersLabel is the archetypes a choice belongs to: "Toolkit" for one,
// "Toolkit 70% + Records (records) 30%" for several.
func membersLabel(id appkit.ArchetypeID, ms []appkit.Membership) string {
	if len(ms) <= 1 {
		return archetypeLabel(id)
	}
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = fmt.Sprintf("%s %d%%", archetypeLabel(m.ID), int(math.Round(m.Weight*100)))
	}
	return strings.Join(parts, " + ")
}

// printArchetypeChoice writes the build output's archetype line. forced is
// the --archetype value, when one was given.
func printArchetypeChoice(w io.Writer, program string, c ArchetypeChoice, forced string) {
	p := func(format string, a ...any) { fmt.Fprint(w, safeTerminalText(fmt.Sprintf(format, a...))+"\n") }
	prog := shellWord(program)
	if forced != "" {
		ms, _ := resolveArchetypeList(forced)
		if len(ms) > 0 {
			p("Archetype: %s, chosen with --archetype", membersLabel(ms[0].ID, ms))
		}
		return
	}
	switch c.Confidence {
	case ConfidenceListed:
		p("Archetype: %s, listed for this program", membersLabel(c.ID, c.Members))
	case ConfidenceClear:
		p("Archetype: %s, a clear choice. Why: mint-gen --explain %s", membersLabel(c.ID, c.Members), prog)
	default:
		also := ""
		if len(c.Alternatives) > 0 {
			var names []string
			for _, a := range c.Alternatives {
				if isMember(a, c.Members) {
					continue // already one of its archetypes
				}
				names = append(names, archetypeLabel(a))
			}
			if len(names) > 0 {
				also = "; also likely: " + strings.Join(names, ", ")
			}
		}
		p("Archetype: %s, a CLOSE CALL%s", membersLabel(c.ID, c.Members), also)
		change := "mint-gen --list-archetypes"
		if len(c.Alternatives) > 0 {
			change = fmt.Sprintf("mint-gen --archetype %s %s", c.Alternatives[0], prog)
		}
		p("  Check why: mint-gen --explain %s    Change it: %s", prog, change)
	}
}

// explainArchetypeChoice writes the whole reasoning, for --explain.
func explainArchetypeChoice(w io.Writer, program string, c ArchetypeChoice) {
	p := func(format string, a ...any) { fmt.Fprint(w, safeTerminalText(fmt.Sprintf(format, a...))+"\n") }
	prog := shellWord(program)
	conf := map[string]string{
		ConfidenceListed: "listed for this program",
		ConfidenceClear:  "a clear choice",
		ConfidenceClose:  "a close call: check it",
	}[c.Confidence]
	p("  Archetype: %s, %s", archetypeLabel(c.ID), conf)
	if len(c.Members) > 1 {
		p("  Belongs to: %s (each scored at least %d%% of the best; weights by score)", membersLabel(c.ID, c.Members), int(secondaryShare*100))
	}
	if a, ok := appkit.LookupArchetype(string(c.ID)); ok {
		clause := func(s string) string { return lowerFirst(strings.TrimSuffix(strings.TrimSpace(s), ".")) }
		p("             %s It takes %s; runs %s; shows %s.", a.Is, clause(a.Takes), clause(a.Runs), clause(a.Shows))
	}
	if c.Confidence == ConfidenceListed {
		p("             %s", c.Why)
		return
	}
	for i, s := range c.Scores {
		if i == 4 || s.Score <= 0 {
			break
		}
		head := "             "
		if i == 0 {
			head = "  Evidence:  "
		}
		hits := s.Hits
		if len(hits) > 5 {
			hits = append(hits[:5:5], "…")
		}
		p("%s%-12s %5s  %s", head, s.ID, points(s.Score), strings.Join(hits, "; "))
	}
	p("             (it leads the next by %s; %s or more is a clear choice)", points(c.Lead), points(clearLead))
	for _, a := range c.Alternatives {
		p("  Instead:    mint-gen --archetype %s %s   (%s)", a, prog, archetypeLabel(a))
	}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// resolveArchetypeFlag reads one archetype's ID or name ("probe", "Look up").
func resolveArchetypeFlag(v string) (appkit.ArchetypeDef, bool) {
	return appkit.LookupArchetype(v)
}

// resolveArchetypeList reads an --archetype value: one archetype, or
// several separated by commas or "+" ("toolkit,records"), the first being
// the primary. They share the weight equally. bad is the first name that is
// not an archetype.
func resolveArchetypeList(v string) (ms []appkit.Membership, bad string) {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '+' })
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		a, ok := resolveArchetypeFlag(p)
		if !ok {
			return nil, p
		}
		ms = append(ms, appkit.Membership{ID: a.ID, Weight: 1})
	}
	if ms = appkit.NormalizeMemberships(ms); len(ms) == 0 {
		if strings.TrimSpace(v) == "" {
			v = "(empty)"
		}
		return nil, v
	}
	return ms, ""
}

func isMember(id appkit.ArchetypeID, ms []appkit.Membership) bool {
	for _, m := range ms {
		if m.ID == id {
			return true
		}
	}
	return false
}
