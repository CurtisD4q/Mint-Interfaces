package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// How a template choice is shown: one line in the build output (two for a
// close call, with what else it might be and how to change it), and the
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

func templateLabel(id string) string {
	tpl, ok := appkit.LookupTemplate(id)
	if !ok {
		return id
	}
	return fmt.Sprintf("%s %s", tpl.ID, tpl.Slug)
}

// printTemplateChoice writes the build output's template line. forced is
// the --template value, when one was given.
func printTemplateChoice(w io.Writer, program string, c TemplateChoice, forced string) {
	p := func(format string, a ...any) { fmt.Fprint(w, safeTerminalText(fmt.Sprintf(format, a...))+"\n") }
	prog := shellWord(program)
	if forced != "" {
		tpl, _ := appkit.LookupTemplate(forced)
		p("Template: %s (%s), chosen with --template", templateLabel(forced), tpl.Archetype)
		return
	}
	switch c.Confidence {
	case ConfidenceListed:
		p("Template: %s (%s), listed for this program", templateLabel(c.ID), c.Archetype)
	case ConfidenceClear:
		p("Template: %s (%s archetype), a clear choice. Why: mint-gen --explain %s", templateLabel(c.ID), c.Archetype, prog)
	default:
		also := ""
		if len(c.Alternatives) > 0 {
			var names []string
			for _, a := range c.Alternatives {
				names = append(names, templateLabel(a))
			}
			also = "; also likely: " + strings.Join(names, ", ")
		}
		p("Template: %s (%s archetype), a CLOSE CALL%s", templateLabel(c.ID), c.Archetype, also)
		change := "mint-gen --list-templates"
		if len(c.Alternatives) > 0 {
			change = fmt.Sprintf("mint-gen --template %s %s", c.Alternatives[0], prog)
		}
		p("  Check why: mint-gen --explain %s    Change it: %s", prog, change)
	}
}

// explainTemplateChoice writes the whole reasoning, for --explain.
func explainTemplateChoice(w io.Writer, program string, c TemplateChoice) {
	p := func(format string, a ...any) { fmt.Fprint(w, safeTerminalText(fmt.Sprintf(format, a...))+"\n") }
	prog := shellWord(program)
	conf := map[string]string{
		ConfidenceListed: "listed for this program",
		ConfidenceClear:  "a clear choice",
		ConfidenceClose:  "a close call: check it",
	}[c.Confidence]
	p("  Template: %s, %s", templateLabel(c.ID), conf)
	if c.Confidence == ConfidenceListed {
		p("            %s", c.Why)
		return
	}
	for i, a := range c.Archetypes {
		if i == 3 {
			break
		}
		head := "            "
		if i == 0 {
			head = "  Archetype:"
		}
		hits := a.Hits
		if len(hits) > 4 {
			hits = append(hits[:4:4], "…")
		}
		p("%s %-9s %3d  %s", head, a.Archetype, a.Score, strings.Join(hits, "; "))
	}
	p("            (it leads the next archetype by %d)", c.ArchetypeLead)
	n := 0
	for _, sc := range c.Scores {
		tpl, ok := appkit.LookupTemplate(sc.ID)
		if !ok || tpl.Archetype != c.Archetype {
			continue
		}
		head := "            "
		if n == 0 {
			head = "  Within it:"
		}
		hits := sc.Hits
		if len(hits) > 4 {
			hits = append(hits[:4:4], "…")
		}
		p("%s %-24s %3d  %s", head, templateLabel(sc.ID), sc.Score, strings.Join(hits, ", "))
		if n++; n == 3 {
			break
		}
	}
	if n == 0 {
		p("  Within it: no template of the %s archetype had word evidence, so its most general one", c.Archetype)
	}
	if c.DecidedBy != "" {
		p("            chosen over the next by %s (a lead of %d)", c.DecidedBy, c.TemplateLead)
	}
	for _, a := range c.Alternatives {
		p("  Instead:   mint-gen --template %s %s   (%s)", a, prog, templateLabel(a))
	}
}
