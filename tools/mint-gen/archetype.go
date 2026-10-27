package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Choosing a template for a program no list names happens in two steps,
// always both:
//
//  1. The archetype: how the program works. A suite has subcommands or
//     modes (git, tar, crontab), a monitor repeats or follows (vmstat 2,
//     journalctl -f), a scanner takes hosts or targets, an explorer takes
//     a pattern, query or device and shows what it finds, a config tool
//     gets and sets values, and a runner does a job on its input. This
//     comes from the program's behaviour (usage lines, options,
//     subcommands) and from the words for the templates of each archetype.
//  2. The template within that archetype, by the scorer's word evidence,
//     with tie-breakers that trust the summary and description over the
//     long body (see pickWithinArchetype).

// ArchetypeScore is how well an archetype fits a program, and why.
type ArchetypeScore struct {
	Archetype appkit.Archetype
	Score     int
	Hits      []string
}

var archetypeOrder = []appkit.Archetype{
	appkit.ArchetypeConfig, appkit.ArchetypeRunner, appkit.ArchetypeScanner,
	appkit.ArchetypeMonitor, appkit.ArchetypeExplorer, appkit.ArchetypeSuite,
}

// archetypeDefault is the template used when the archetype is clear but no
// template of it has any word evidence: the most general one.
var archetypeDefault = map[appkit.Archetype]string{
	appkit.ArchetypeConfig:   "T02",
	appkit.ArchetypeRunner:   "T05",
	appkit.ArchetypeScanner:  "T12",
	appkit.ArchetypeMonitor:  "T17",
	appkit.ArchetypeExplorer: "T25",
	appkit.ArchetypeSuite:    "T21",
}

// Behaviour points (see archetypeBehaviour).
const (
	behaveStrong = 4
	behaveMedium = 3
	behaveWeak   = 1
)

var (
	// A value that repeats the program: "vmstat [delay [count]]", "-n, --interval SECS", "--watch".
	reRepeatWord = regexp.MustCompile(`(?i)\b(?:interval|delay|refresh|every\s+\d*\s*seconds?|seconds\s+between|repeat(?:ed|edly)?\s+every|update\s+every|watch)\b`)
	reFollowOpt  = regexp.MustCompile(`(?m)^\s{0,12}(?:-f,\s*)?--follow\b`)
	// Argument names that are hosts or targets.
	reTargetArg = regexp.MustCompile(`(?i)^(?:host|hosts|hostname|target|targets|address|addr|ip|ipaddr|ip-address|domain|network|net|server|url|urls|uri|destination|dest-host)$`)
	// Argument names that are a pattern, query or device to look into.
	reQueryArg  = regexp.MustCompile(`(?i)^(?:pattern|patterns|query|expression|expr|regex|regexp|filter|keyword|keywords|search|term|name|names|filename-pattern)$`)
	reDeviceArg = regexp.MustCompile(`(?i)^(?:device|devices|disk|disks|partition|blockdev|mountpoint|dev)$`)
	// Get and set: "get KEY", "set KEY VALUE", "key=value", "NAME=VALUE".
	reGetSetWord = regexp.MustCompile(`(?i)\b(?:get|set|unset|reset)\b`)
	reKeyValue   = regexp.MustCompile(`(?i)\b(?:key|name|variable|param|parameter|setting|option)\s*=\s*(?:value|val)\b|\b(?:key|parameter|setting)\s+(?:value|val)\b`)
	// A section of options that are modes: "Main operation mode:", "Commands:", "Actions:".
	reModeSection = regexp.MustCompile(`(?i)\b(?:operation|operations|mode|modes|command|commands|action|actions|function|functions)\b`)
	// A usage form that starts with a single option (crontab -l, crontab -e).
	reUsageLeadOpt = regexp.MustCompile(`^\S+\s+(?:\[[^\]]*\]\s+)*(-{1,2}[A-Za-z][-A-Za-z]*)`)
)

// archetypeBehaviour scores each archetype by how the program works.
func archetypeBehaviour(p programText) map[appkit.Archetype]*ArchetypeScore {
	out := map[appkit.Archetype]*ArchetypeScore{}
	add := func(a appkit.Archetype, pts int, why string) {
		s := out[a]
		if s == nil {
			s = &ArchetypeScore{Archetype: a}
			out[a] = s
		}
		s.Score += pts
		s.Hits = append(s.Hits, why)
	}
	manLines, helpLines := p.manLines, p.helpLines
	usage := strings.Join(p.Usage, "\n")

	// Suite: subcommands, a section of mode options, or several usage
	// forms each led by a different option.
	if subs := extractSubcommands(manLines, helpLines); len(subs) >= 2 {
		add(appkit.ArchetypeSuite, behaveStrong, fmt.Sprintf("has %d subcommands", len(subs)))
	}
	modes := map[string]int{}
	for _, f := range p.options {
		if reModeSection.MatchString(f.Section) && !strings.EqualFold(f.Section, "Options") {
			modes[f.Section]++
		}
	}
	for sec, n := range modes {
		if n >= 3 {
			add(appkit.ArchetypeSuite, behaveMedium, fmt.Sprintf("options grouped as modes (%q)", sec))
			break
		}
	}
	lead := map[string]bool{}
	for _, u := range p.Usage {
		if m := reUsageLeadOpt.FindStringSubmatch(strings.TrimSpace(u)); m != nil {
			lead[m[1]] = true
		}
	}
	if len(lead) >= 3 {
		add(appkit.ArchetypeSuite, behaveMedium, fmt.Sprintf("%d usage forms, each led by a different option", len(lead)))
	}

	// Monitor: repeats on an interval, or follows.
	if reRepeatWord.MatchString(usage) {
		add(appkit.ArchetypeMonitor, behaveStrong, "its usage repeats on an interval")
	} else if n := countOptionsMatching(p, reRepeatWord); n > 0 {
		add(appkit.ArchetypeMonitor, behaveMedium, "an option repeats it on an interval")
	}
	if reFollowOpt.MatchString(p.Body) {
		add(appkit.ArchetypeMonitor, behaveMedium, "it can follow (--follow)")
	}

	// Scanner and explorer: what its arguments are.
	for _, a := range usageArgNames(p.Usage) {
		switch {
		case reTargetArg.MatchString(a):
			add(appkit.ArchetypeScanner, behaveMedium, "takes a host or target ("+a+")")
		case reQueryArg.MatchString(a):
			add(appkit.ArchetypeExplorer, behaveMedium, "takes a pattern or query ("+a+")")
		case reDeviceArg.MatchString(a):
			add(appkit.ArchetypeExplorer, behaveMedium, "takes a device ("+a+")")
		}
	}

	// Config: gets and sets values.
	if reKeyValue.MatchString(usage) {
		add(appkit.ArchetypeConfig, behaveStrong, "its usage sets key=value")
	}
	if n := len(reGetSetWord.FindAllString(usage, -1)); n >= 2 {
		add(appkit.ArchetypeConfig, behaveMedium, "its usage gets and sets values")
	}

	// Runner: the usual kind of program, when nothing else stands out.
	add(appkit.ArchetypeRunner, behaveWeak, "runs a job (the usual kind)")
	for a, s := range out {
		if len(s.Hits) > 0 && a != appkit.ArchetypeRunner {
			s.Hits = dedupe(s.Hits)
		}
	}
	return out
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// usageArgNames lists the argument words of the usage forms, lower case,
// once each: "vmstat [delay [count]]" -> delay, count.
func usageArgNames(usage []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range usage {
		toks := strings.Fields(u)
		for i, tok := range toks {
			if i == 0 || strings.HasPrefix(strings.Trim(tok, "[]{}()<>|."), "-") {
				continue
			}
			w := strings.ToLower(strings.Trim(tok, "[]{}()<>|.,'\""))
			if w == "" || seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// countOptionsMatching counts option definitions whose name, value or
// description match re.
func countOptionsMatching(p programText, re *regexp.Regexp) int {
	n := 0
	for _, f := range p.options {
		if re.MatchString(f.Flag + " " + f.Hint + " " + f.Label) {
			n++
		}
	}
	return n
}

// archetypeWordEvidence is, for each archetype, the best word score among
// its templates.
func archetypeWordEvidence(scores []TemplateScore) map[appkit.Archetype]TemplateScore {
	out := map[appkit.Archetype]TemplateScore{}
	for _, s := range scores {
		tpl, ok := appkit.LookupTemplate(s.ID)
		if !ok {
			continue
		}
		if best, ok := out[tpl.Archetype]; !ok || s.Score > best.Score {
			out[tpl.Archetype] = s
		}
	}
	return out
}

// detectArchetype ranks the archetypes for a program, best first. Ties go
// to the order of archetypeOrder, so the result is the same every run.
func detectArchetype(p programText, scores []TemplateScore) []ArchetypeScore {
	beh := archetypeBehaviour(p)
	words := archetypeWordEvidence(scores)
	var out []ArchetypeScore
	for _, a := range archetypeOrder {
		s := ArchetypeScore{Archetype: a}
		if b := beh[a]; b != nil {
			s.Score += b.Score
			s.Hits = append(s.Hits, b.Hits...)
		}
		if w, ok := words[a]; ok && w.Score > 0 {
			s.Score += w.Score
			s.Hits = append(s.Hits, fmt.Sprintf("words for %s (%d)", w.ID, w.Score))
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// tieKey is how two templates of one archetype are compared: points from
// the summary and description first (what the program says it is), then
// all points, then how many parts of the help had evidence, then the most
// specific phrase (the most words).
type tieKey struct {
	top, total, zones, specific int
}

func (a tieKey) beats(b tieKey) bool {
	switch {
	case a.top != b.top:
		return a.top > b.top
	case a.total != b.total:
		return a.total > b.total
	case a.zones != b.zones:
		return a.zones > b.zones
	}
	return a.specific > b.specific
}

func templateTieKey(s TemplateScore) tieKey {
	k := tieKey{total: s.Score}
	zones := map[string]bool{}
	for _, h := range s.Hits {
		zone, phrase, _ := strings.Cut(h, ": ")
		zones[zone] = true
		if zone == "summary" || zone == "description" || zone == "name" {
			k.top++
		}
		phrase, _, _ = strings.Cut(phrase, " ×")
		if n := len(strings.Fields(phrase)); n > k.specific {
			k.specific = n
		}
	}
	k.zones = len(zones)
	return k
}

// pickWithinArchetype chooses the template of archetype a with the best
// evidence (see tieKey), or the archetype's default when none has any.
func pickWithinArchetype(a appkit.Archetype, scores []TemplateScore) (string, string) {
	best, bestKey, found := "", tieKey{}, false
	for _, s := range scores {
		tpl, ok := appkit.LookupTemplate(s.ID)
		if !ok || tpl.Archetype != a || s.Score <= 0 {
			continue
		}
		k := templateTieKey(s)
		if !found || k.beats(bestKey) || (!bestKey.beats(k) && s.ID < best) {
			best, bestKey, found = s.ID, k, true
		}
	}
	if !found {
		return archetypeDefault[a], "no template of it has word evidence; its most general template"
	}
	return best, fmt.Sprintf("best evidence among %s templates (%d points)", a, bestKey.total)
}

// chooseByArchetype is the two-step choice for a program no list names.
func chooseByArchetype(p programText) TemplateChoice {
	return chooseByArchetypeScores(p, scoreEvidence(defaultWeights, findEvidence(p)))
}

// chooseByArchetypeScores is chooseByArchetype with the word scores given.
func chooseByArchetypeScores(p programText, scores []TemplateScore) TemplateChoice {
	arch := detectArchetype(p, scores)
	a := arch[0]
	id, why := pickWithinArchetype(a.Archetype, scores)
	return TemplateChoice{
		ID:         id,
		Why:        fmt.Sprintf("archetype %s (%d: %s); %s", a.Archetype, a.Score, strings.Join(a.Hits, ", "), why),
		Scores:     scores,
		Archetypes: arch,
	}
}
