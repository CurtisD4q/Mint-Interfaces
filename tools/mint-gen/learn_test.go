package main

// Learning the template scorer from examples. Runs only when asked:
//
//	MINT_TEMPLATE_LEARN=eval go test -run TestTemplateLearn -v .
//
// The labelled pool is every program on a template list plus the
// benchmark sets; testdata/template_untouched.txt is kept apart and only
// ever measured. Help comes from the same cache as the benchmarks.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

type labelled struct {
	Name, Want string
	Listed     bool // on a template list
	Untouched  bool // in the untouched test set
}

// learnPool is every labelled program once: the template lists, the
// benchmark sets, and (marked) the untouched set.
func learnPool(t *testing.T) []labelled {
	byName := map[string]*labelled{}
	var order []string
	add := func(l labelled) {
		if p, ok := byName[l.Name]; ok {
			if p.Want != l.Want {
				t.Fatalf("%s labelled %s and %s", l.Name, p.Want, l.Want)
			}
			p.Listed = p.Listed || l.Listed
			return
		}
		byName[l.Name] = &l
		order = append(order, l.Name)
	}
	for _, tpl := range appkit.AllTemplates() {
		for _, n := range tpl.ExampleTools {
			add(labelled{Name: n, Want: string(tpl.ID), Listed: true})
		}
	}
	for _, c := range loadBenchCases(t) {
		add(labelled{Name: c.Name, Want: c.Want})
	}
	for _, c := range loadBenchFile(t, "testdata/template_untouched.txt", true) {
		if _, ok := byName[c.Name]; ok {
			t.Fatalf("%s is in the untouched set and elsewhere", c.Name)
		}
		add(labelled{Name: c.Name, Want: c.Want, Untouched: true})
	}
	out := make([]labelled, 0, len(order))
	for _, n := range order {
		out = append(out, *byName[n])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func learnCache() string {
	if c := os.Getenv("MINT_TEMPLATE_BENCH_CACHE"); c != "" {
		return c
	}
	return filepath.Join(os.TempDir(), "mint-template-bench")
}

func TestTemplateLearn(t *testing.T) {
	mode := os.Getenv("MINT_TEMPLATE_LEARN")
	if mode == "" {
		t.Skip("set MINT_TEMPLATE_LEARN=collect|eval|train to learn the template scorer")
	}
	pool := learnPool(t)
	var have, missing []string
	for _, l := range pool {
		if _, _, ok := benchHelp(t, learnCache(), l.Name); ok {
			have = append(have, l.Name)
		} else {
			missing = append(missing, l.Name)
		}
	}
	t.Logf("%d labelled programs, help for %d; not installed: %s", len(pool), len(have), strings.Join(missing, " "))
	if mode == "collect" {
		return
	}
	docs := learnDocs(t)
	showUntouched := mode == "final"
	if names := os.Getenv("MINT_TEMPLATE_LEARN_ONLY"); names != "" {
		keep := map[string]bool{}
		for _, n := range strings.Fields(names) {
			keep[n] = true
		}
		var some []learnDoc
		for _, d := range docs {
			if keep[d.Name] {
				some = append(some, d)
			}
		}
		docs = some
	}
	archOf := func(id string) appkit.Archetype { tpl, _ := appkit.LookupTemplate(id); return tpl.Archetype }
	var archRight, archAll, archUntRight, archUnt int
	var nowPool, newPool, nowUnt, newUnt verdicts
	confusion := map[string]int{}
	var misses []string
	for _, d := range docs {
		scores := d.scores
		nowID, _ := pickTemplate(scores)
		if nowID == "" {
			nowID = "T05"
			if d.sub {
				nowID = "T21"
			}
		}
		c := chooseByArchetypeEvidence(d.text, d.ev, scores)
		gotArch := c.Archetypes[0].Archetype
		if d.Untouched {
			archUnt++
			if gotArch == archOf(d.Want) {
				archUntRight++
			}
			nowUnt.add(d.Want, nowID)
			newUnt.add(d.Want, c.ID)
			continue
		}
		archAll++
		if gotArch == archOf(d.Want) {
			archRight++
			if c.ID != d.Want {
				misses = append(misses, fmt.Sprintf("WITHIN %-14s want %s got %s  [%s]", d.Name, d.Want, c.ID, c.Why))
			}
		} else {
			confusion[string(archOf(d.Want))+" -> "+string(gotArch)]++
			misses = append(misses, fmt.Sprintf("%-14s want %s/%s got %s/%s  [%s]", d.Name, d.Want, archOf(d.Want), c.ID, gotArch, c.Why))
		}
		nowPool.add(d.Want, nowID)
		newPool.add(d.Want, c.ID)
	}
	t.Logf("archetype right: pool %d/%d (%.0f%%)", archRight, archAll, 100*float64(archRight)/float64(archAll))
	t.Logf("pool, today's scorer (always answers): %s", nowPool)
	t.Logf("pool, archetype first:                 %s", newPool)
	var keys []string
	for k := range confusion {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return confusion[keys[i]] > confusion[keys[j]] })
	for _, k := range keys {
		t.Logf("  archetype %s: %d", k, confusion[k])
	}
	if os.Getenv("MINT_TEMPLATE_LEARN_MISSES") != "" {
		for _, m := range misses {
			t.Logf("  %.230s", m)
		}
	}
	if showUntouched {
		t.Logf("archetype right: untouched %d/%d (%.0f%%)", archUntRight, archUnt, 100*float64(archUntRight)/float64(archUnt))
		t.Logf("untouched, today's scorer (always answers): %s", nowUnt)
		t.Logf("untouched, archetype first:                 %s", newUnt)
	}
}

// learnDoc is one program's text, read once.
type learnDoc struct {
	labelled
	text   programText
	sub    bool             // has subcommands (for the fallback)
	scores []TemplateScore  // the word scores, computed once
	ev     templateEvidence // the evidence they came from
}

func learnDocs(t *testing.T) []learnDoc {
	var out []learnDoc
	keep := map[string]bool{}
	for _, n := range strings.Fields(os.Getenv("MINT_TEMPLATE_LEARN_ONLY")) {
		keep[n] = true
	}
	for _, l := range learnPool(t) {
		if len(keep) > 0 && !keep[l.Name] {
			continue
		}
		start := time.Now()
		man, help, ok := benchHelp(t, learnCache(), l.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(l.Name, man, help)
		p.Related = nil // they come from the lists, which are what is being replaced
		ml, hl := cleanLines(man), cleanLines(help)
		out = append(out, learnDoc{labelled: l, text: p, sub: hasSubcommands(man, help, ml, hl),
			ev: findEvidence(p)})
		out[len(out)-1].scores = scoreEvidence(defaultWeights, out[len(out)-1].ev)
		if d := time.Since(start); d > 2*time.Second {
			t.Logf("slow: %s %v (%d KB)", l.Name, d.Round(time.Second), len(man+help)/1024)
		}
	}
	return out
}

// verdicts counts correct / abstained / wrong.
type verdicts struct{ correct, abstained, wrong int }

func (v *verdicts) add(want, got string) {
	switch {
	case got == "":
		v.abstained++
	case got == want:
		v.correct++
	default:
		v.wrong++
	}
}

func (v verdicts) String() string {
	n := float64(v.correct + v.abstained + v.wrong)
	return fmt.Sprintf("%3d programs | right %3d (%3.0f%%) | not sure %3d (%3.0f%%) | WRONG %3d (%3.0f%%)",
		int(n), v.correct, 100*float64(v.correct)/n, v.abstained, 100*float64(v.abstained)/n, v.wrong, 100*float64(v.wrong)/n)
}

// handScorer is today's scorer with no lists: a template, or "" when not sure.
func handScorer(d learnDoc) string {
	id, _ := pickTemplate(scoreEvidence(defaultWeights, findEvidence(d.text)))
	return id
}

func reportSets(t *testing.T, title string, docs []learnDoc, pick func(learnDoc) string) {
	var listed, other, untouched verdicts
	for _, d := range docs {
		got := pick(d)
		switch {
		case d.Untouched:
			untouched.add(d.Want, got)
		case d.Listed:
			listed.add(d.Want, got)
		default:
			other.add(d.Want, got)
		}
	}
	t.Logf("%s", title)
	t.Logf("  listed     %s", listed)
	t.Logf("  benchmarks %s", other)
	t.Logf("  untouched  %s", untouched)
}

// The windowed search finds exactly what the full search finds, on every
// cached program's help (MINT_TEMPLATE_LEARN=windows).
func TestZoneWindowsMatchFullSearch(t *testing.T) {
	if os.Getenv("MINT_TEMPLATE_LEARN") != "windows" {
		t.Skip("set MINT_TEMPLATE_LEARN=windows")
	}
	n, diff := 0, 0
	for _, l := range learnPool(t) {
		man, help, ok := benchHelp(t, learnCache(), l.Name)
		if !ok {
			continue
		}
		text := man + "\n" + help
		if len(text) <= longZone {
			continue
		}
		n++
		lower := asciiLower(text)
		for i, s := range templateSignalList {
			full := fmt.Sprint(signalLocations(s, text, ""))
			win := fmt.Sprint(signalLocations(s, text, lower))
			if full != win {
				diff++
				t.Errorf("%s, signal %d (%s): full %.80s window %.80s", l.Name, i, s.label, full, win)
				break
			}
		}
	}
	t.Logf("%d long help texts compared, %d differ", n, diff)
}
