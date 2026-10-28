package main

// The archetype benchmark: how well mint-gen recognises the archetype of a
// program it has no list entry for. It runs only when asked:
//
//	MINT_ARCHETYPE_BENCH=1     go test -run TestArchetypeBenchmark -v .   # listed and bench programs
//	MINT_ARCHETYPE_BENCH=final go test -run TestArchetypeBenchmark -v .   # also the untouched and fresh sets
//
// Labels: the archetype lists (mint/appkit/archetypetools.go) and
// testdata/archetype_labels.txt. Every program is classified blind, from
// its help alone, with the lists switched off. The listed and bench
// programs may be looked at while improving the rules; the untouched and
// fresh programs are only ever measured.
//
// Help is read the way mint-gen reads it (CollectHelp, sandboxed) and
// cached in $MINT_TEMPLATE_BENCH_CACHE (an absolute path; default: a folder
// in the system temp dir). No help text is committed.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func benchCache() string {
	if c := os.Getenv("MINT_TEMPLATE_BENCH_CACHE"); c != "" {
		return c
	}
	return filepath.Join(os.TempDir(), "mint-template-bench")
}

// benchHelp returns the program's man and help text, from the cache or
// read now.
func benchHelp(t *testing.T, cache, name string) (man, help string, ok bool) {
	if cache == "" || !filepath.IsAbs(cache) {
		// Never cache into the source folder: help text is not ours to commit.
		t.Fatalf("the help cache must be an absolute path, not %q", cache)
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cache, name+".json")
	if b, err := os.ReadFile(path); err == nil {
		var srcs []HelpSource
		if json.Unmarshal(b, &srcs) == nil {
			m, h := manAndHelp(srcs)
			return m, h, true
		}
	}
	if _, err := exec.LookPath(name); err != nil {
		return "", "", false
	}
	srcs := CollectHelp(name)
	if b, err := json.Marshal(srcs); err == nil {
		_ = os.WriteFile(path, b, 0o600)
	}
	m, h := manAndHelp(srcs)
	return m, h, true
}

type archCase struct {
	Name string
	Want appkit.ArchetypeID
	Set  string // listed, bench, untouched, fresh
}

func (c archCase) tunable() bool { return c.Set == "listed" || c.Set == "bench" }

func loadArchCases(t *testing.T) []archCase {
	var out []archCase
	for _, n := range appkit.ArchetypeToolNames() {
		a, _ := appkit.ArchetypeForTool(n)
		out = append(out, archCase{n, a.ID, "listed"})
	}
	f, err := os.Open("testdata/archetype_labels.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) != 3 {
			t.Fatalf("bad line %q", line)
		}
		if _, ok := appkit.LookupArchetype(fs[1]); !ok {
			t.Fatalf("%s: unknown archetype %q", fs[0], fs[1])
		}
		out = append(out, archCase{fs[0], appkit.ArchetypeID(fs[1]), fs[2]})
	}
	return out
}

type archResult struct {
	archCase
	choice ArchetypeChoice
}

// verdict: right-clear, right-close, wrong-close (flagged), wrong-clear.
func (r archResult) verdict() string {
	right := r.choice.ID == r.Want
	clear := r.choice.Confidence == ConfidenceClear
	switch {
	case right && clear:
		return "right-clear"
	case right:
		return "right-close"
	case !clear:
		return "wrong-close"
	}
	return "wrong-clear"
}

func runArchBench(t *testing.T, keep func(archCase) bool) ([]archResult, []string) {
	var out []archResult
	var missing []string
	for _, c := range loadArchCases(t) {
		if !keep(c) {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			missing = append(missing, c.Name)
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.Related = nil // they come from the lists, which are switched off
		out = append(out, archResult{c, chooseArchetypeFor(p)})
	}
	return out, missing
}

func reportArch(t *testing.T, title string, rs []archResult) {
	n := map[string]int{}
	for _, r := range rs {
		n[r.verdict()]++
	}
	total := len(rs)
	if total == 0 {
		return
	}
	pct := func(k ...string) string {
		s := 0
		for _, x := range k {
			s += n[x]
		}
		return fmt.Sprintf("%3d (%3.0f%%)", s, 100*float64(s)/float64(total))
	}
	t.Logf("%-9s %3d | right %s [clear %s] | wrong %s [flagged %s, marked clear %s]", title, total,
		pct("right-clear", "right-close"), pct("right-clear"), pct("wrong-close", "wrong-clear"), pct("wrong-close"), pct("wrong-clear"))
}

func TestArchetypeBenchmark(t *testing.T) {
	mode := os.Getenv("MINT_ARCHETYPE_BENCH")
	if mode == "" {
		t.Skip("set MINT_ARCHETYPE_BENCH=1 (or final) to run the archetype benchmark")
	}
	final := mode == "final"
	rs, missing := runArchBench(t, func(c archCase) bool { return final || c.tunable() })
	sort.Slice(rs, func(i, j int) bool { return string(rs[i].Want)+rs[i].Name < string(rs[j].Want)+rs[j].Name })
	confusion := map[string]int{}
	for _, r := range rs {
		if r.choice.ID != r.Want && r.tunable() {
			confusion[string(r.Want)+" -> "+string(r.choice.ID)]++
			if os.Getenv("MINT_ARCHETYPE_BENCH_MISSES") != "" {
				top := ""
				for i, s := range r.choice.Scores {
					if i == 3 {
						break
					}
					top += fmt.Sprintf(" %s:%d", s.ID, s.Score)
				}
				t.Logf("  %-6s %-14s want %-10s got %-10s %-5s%s", r.Set, r.Name, r.Want, r.choice.ID, r.choice.Confidence, top)
			}
		}
	}
	for _, set := range []string{"listed", "bench", "untouched", "fresh"} {
		var some []archResult
		for _, r := range rs {
			if r.Set == set {
				some = append(some, r)
			}
		}
		reportArch(t, set, some)
	}
	var tun, untuned []archResult
	for _, r := range rs {
		if r.tunable() {
			tun = append(tun, r)
		} else {
			untuned = append(untuned, r)
		}
	}
	reportArch(t, "tunable", tun)
	reportArch(t, "untuned", untuned)
	var keys []string
	for k := range confusion {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if confusion[keys[i]] != confusion[keys[j]] {
			return confusion[keys[i]] > confusion[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for i, k := range keys {
		if i == 15 {
			break
		}
		t.Logf("  tunable confusion %-26s %d", k, confusion[k])
	}
	if len(missing) > 0 {
		t.Logf("not installed (skipped): %s", strings.Join(missing, " "))
	}
}

// The windowed phrase search finds exactly what the full search finds, on
// every cached program's help (MINT_ARCHETYPE_BENCH=windows).
func TestZoneWindowsMatchFullSearch(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "windows" {
		t.Skip("set MINT_ARCHETYPE_BENCH=windows")
	}
	n := 0
	for _, c := range loadArchCases(t) {
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok {
			continue
		}
		text := man + "\n" + help
		if len(text) <= longZone {
			continue
		}
		n++
		lower := asciiLower(text)
		for _, list := range [][]signal{archetypeSignalList, broadSignalList, dataTopicSignalList} {
			for i, s := range list {
				if full, win := fmt.Sprint(signalLocations(s, text, "")), fmt.Sprint(signalLocations(s, text, lower)); full != win {
					t.Errorf("%s, signal %d (%s): full %.80s window %.80s", c.Name, i, s.label, full, win)
				}
			}
		}
	}
	t.Logf("%d long help texts compared", n)
}

// TestArchetypeCalibration shows, for each lead, how many choices would
// count as clear and how often those are right (MINT_ARCHETYPE_BENCH=
// calibrate; tunable programs only).
func TestArchetypeCalibration(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "calibrate" {
		t.Skip("set MINT_ARCHETYPE_BENCH=calibrate")
	}
	rs, _ := runArchBench(t, func(c archCase) bool { return c.tunable() })
	for _, lead := range []int{1, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 24, 28, 32, 36, 40, 45, 50} {
		var clear, clearRight, close, closeRight int
		for _, r := range rs {
			right := r.choice.ID == r.Want
			if r.choice.Lead >= lead {
				clear++
				if right {
					clearRight++
				}
			} else {
				close++
				if right {
					closeRight++
				}
			}
		}
		t.Logf("lead>=%2d: clear %3d (%3.0f%%) right %3.0f%% | close %3d right %3.0f%%", lead, clear,
			100*float64(clear)/float64(len(rs)), 100*float64(clearRight)/float64(max(clear, 1)), close, 100*float64(closeRight)/float64(max(close, 1)))
	}
}

// TestArchetypeTuning searches the global weights (archWeights) on the
// tunable programs, and estimates how well the search itself generalises
// by 5-fold cross-validation: choose on four fifths, measure on the rest
// (MINT_ARCHETYPE_BENCH=tune).
func TestArchetypeTuning(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "tune" {
		t.Skip("set MINT_ARCHETYPE_BENCH=tune")
	}
	type doc struct {
		c  archCase
		ev archEvidence
	}
	var docs []doc
	for _, c := range loadArchCases(t) {
		if !c.tunable() {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.Related = nil
		docs = append(docs, doc{c, findArchEvidence(p)})
	}
	// The split by where evidence was found stays fixed; the search varies
	// how strong cues count within it and the limits.
	// MINT_ARCHETYPE_SPLIT sets the split, e.g. "usage=30,summary=25,
	// description=20,name=15,sections=10" (sections: each help section
	// read); else the default one.
	base0 := defaultArchWeights
	if spec := os.Getenv("MINT_ARCHETYPE_SPLIT"); spec != "" {
		zone := map[string]int{"name": zoneName, "summary": zoneSummary, "description": zoneDescription, "usage": zoneUsage, "sections": zoneSection}
		for _, kv := range strings.Split(spec, ",") {
			k, v, _ := strings.Cut(strings.TrimSpace(kv), "=")
			z, ok := zone[k]
			n, err := strconv.Atoi(v)
			if !ok || err != nil {
				t.Fatalf("bad split %q", kv)
			}
			base0.CueZone[z] = n
		}
		cz := base0.CueZone
		base0.CueZone = splitZones(cz[zoneName], cz[zoneSummary], cz[zoneDescription], cz[zoneUsage], cz[zoneSection])
		ww := &base0.Words
		ww.Name, ww.Summary, ww.Description, ww.Usage, ww.Section = cz[zoneName], cz[zoneSummary], cz[zoneDescription], cz[zoneUsage], cz[zoneSection]
		base0.Related = cz[zoneSection]
	}
	t.Logf("split: name %d summary %d description %d usage %d each section %d", base0.CueZone[zoneName], base0.CueZone[zoneSummary], base0.CueZone[zoneDescription], base0.CueZone[zoneUsage], base0.CueZone[zoneSection])
	var grid []archWeights
	for _, scale := range []int{50, 100, 150} {
		for _, base := range []int{10, 20} {
			for _, weak := range []int{25, 35, 50} {
				for _, med := range []int{55, 70, 85} {
					for _, weakWord := range []int{30, 40, 55} {
						for _, bodyCap := range []int{2, 3, 5} { // each section's limit, in matches
							for _, share := range []int{0, 30, 60} {
								for _, rel := range []int{0, 1} {
									w := base0
									w.CueStrength = [4]int{base * scale / 100, weak * scale / 100, med * scale / 100, scale}
									ww := &w.Words
									ww.WeakName, ww.WeakSummary, ww.WeakDescription, ww.WeakUsage = ww.Name*weakWord/100, ww.Summary*weakWord/100, ww.Description*weakWord/100, ww.Usage*weakWord/100
									ww.SectionCap = bodyCap * ww.Section
									w.BroadShare, w.Related = share, rel*ww.Section
									grid = append(grid, w)
								}
							}
						}
					}
				}
			}
		}
	}
	// Score every program once per set of weights (in parallel), then
	// cross-validate from the table of right answers.
	ok := make([][]bool, len(grid))
	var wg sync.WaitGroup
	next := make(chan int)
	for g := 0; g < runtime.NumCPU(); g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for gi := range next {
				row := make([]bool, len(docs))
				for i, d := range docs {
					row[i] = scoreArchEvidence(grid[gi], d.ev)[0].ID == d.c.Want
				}
				ok[gi] = row
			}
		}()
	}
	for gi := range grid {
		next <- gi
	}
	close(next)
	wg.Wait()
	right := func(gi int, keep func(int) bool) (n, total int) {
		for i, r := range ok[gi] {
			if !keep(i) {
				continue
			}
			total++
			if r {
				n++
			}
		}
		return
	}
	fold := func(i int) int { return i % 5 }
	// Cross-validation of the search: choose on four folds, count on the
	// fifth.
	cvRight, cvTotal := 0, 0
	for f := 0; f < 5; f++ {
		best, bestN := 0, -1
		for gi := range grid {
			if n, _ := right(gi, func(i int) bool { return fold(i) != f }); n > bestN {
				best, bestN = gi, n
			}
		}
		n, tot := right(best, func(i int) bool { return fold(i) == f })
		cvRight += n
		cvTotal += tot
	}
	n0, tot := 0, len(docs)
	for _, d := range docs {
		if scoreArchEvidence(base0, d.ev)[0].ID == d.c.Want {
			n0++
		}
	}
	t.Logf("this split, default cue strengths: %d/%d (%.1f%%)", n0, tot, 100*float64(n0)/float64(tot))
	t.Logf("5-fold cross-validated search: %d/%d (%.1f%%)", cvRight, cvTotal, 100*float64(cvRight)/float64(cvTotal))
	// The best on all tunable programs.
	type res struct {
		w archWeights
		n int
	}
	var all []res
	for gi, w := range grid {
		n, _ := right(gi, func(int) bool { return true })
		all = append(all, res{w, n})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].n > all[j].n })
	for i := 0; i < 10; i++ {
		w := all[i].w
		t.Logf("%d/%d cue strength %v weak words %d/%d/%d/%d section cap %d broad %d%% related %d", all[i].n, tot,
			w.CueStrength, w.Words.WeakName, w.Words.WeakSummary, w.Words.WeakDescription, w.Words.WeakUsage, w.Words.SectionCap, w.BroadShare, w.Related)
	}
}

// TestSectionSearch tries reading every set of 1 to 4 help sections, each
// as a part of its own, on the tunable programs, and estimates by 5-fold
// cross-validation how well choosing the best set generalises
// (MINT_ARCHETYPE_BENCH=sections).
func TestSectionSearch(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "sections" {
		t.Skip("set MINT_ARCHETYPE_BENCH=sections")
	}
	type doc struct {
		want appkit.ArchetypeID
		ev   archEvidence // with every section read
	}
	var docs []doc
	for _, c := range loadArchCases(t) {
		if !c.tunable() {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.read = helpSections[:]
		docs = append(docs, doc{c.Want, findArchEvidence(p)})
	}
	// with keeps the chosen sections' zones and empties the others.
	with := func(ev archEvidence, chosen []string) archEvidence {
		keep := map[int]bool{}
		for _, s := range chosen {
			keep[zoneOf(s)] = true
		}
		for z := zoneSection; z < numZones; z++ {
			if !keep[z] {
				ev.words.zones[z] = nil
				ev.broad.zones[z] = nil
			}
		}
		return ev
	}
	var subsets [][]string
	var pick func(start int, cur []string)
	pick = func(start int, cur []string) {
		if len(cur) > 0 {
			subsets = append(subsets, append([]string(nil), cur...))
		}
		if len(cur) == 4 {
			return
		}
		for i := start; i < numSections; i++ {
			pick(i+1, append(cur, helpSections[i]))
		}
	}
	pick(0, nil)
	ok := make([][]bool, len(subsets))
	for si, sub := range subsets {
		row := make([]bool, len(docs))
		for i, d := range docs {
			row[i] = scoreArchEvidence(defaultArchWeights, with(d.ev, sub))[0].ID == d.want
		}
		ok[si] = row
	}
	count := func(row []bool, keep func(int) bool) (n int) {
		for i, r := range row {
			if keep(i) && r {
				n++
			}
		}
		return
	}
	nAll, nNone := 0, 0
	for _, d := range docs {
		if scoreArchEvidence(defaultArchWeights, d.ev)[0].ID == d.want {
			nAll++
		}
		if scoreArchEvidence(defaultArchWeights, with(d.ev, nil))[0].ID == d.want {
			nNone++
		}
	}
	all := func(int) bool { return true }
	fold := func(i int) int { return i % 5 }
	cv := 0
	for f := 0; f < 5; f++ {
		best, bestN := 0, -1
		for si := range subsets {
			if n := count(ok[si], func(i int) bool { return fold(i) != f }); n > bestN {
				best, bestN = si, n
			}
		}
		cv += count(ok[best], func(i int) bool { return fold(i) == f })
		t.Logf("fold %d chose %v", f, subsets[best])
	}
	tot := len(docs)
	pct := func(n int) float64 { return 100 * float64(n) / float64(tot) }
	t.Logf("%d sets of 1-4 sections, %d programs", len(subsets), tot)
	t.Logf("all 12 sections:   %d/%d (%.1f%%)", nAll, tot, pct(nAll))
	t.Logf("no sections:       %d/%d (%.1f%%)", nNone, tot, pct(nNone))
	t.Logf("cross-validated choice: %d/%d (%.1f%%)", cv, tot, pct(cv))
	order := make([]int, len(subsets))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		na, nb := count(ok[order[a]], all), count(ok[order[b]], all)
		if na != nb {
			return na > nb
		}
		return len(subsets[order[a]]) < len(subsets[order[b]])
	})
	for _, si := range order[:15] {
		t.Logf("%d/%d %v", count(ok[si], all), tot, subsets[si])
	}
	for si, sub := range subsets {
		if len(sub) == 1 {
			t.Logf("alone: %-16s %d/%d", sub[0], count(ok[si], all), tot)
		}
	}
}

// TestExtraSectionSearch compares, on the tunable programs, the five parts
// of main (the whole help as the fifth part) and grouped sections (chosen
// sections read together as the fifth part), each with 1 or 2 of the best
// single sections (options, more description) added as separate parts of
// their own. Choices are made by 5-fold cross-validation.
func TestExtraSectionSearch(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "extras" {
		t.Skip("set MINT_ARCHETYPE_BENCH=extras")
	}
	type doc struct {
		want appkit.ArchetypeID
		ev   archEvidence // whole help and every section read
	}
	var docs []doc
	for _, c := range loadArchCases(t) {
		if !c.tunable() {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.readWhole, p.read = true, helpSections[:]
		docs = append(docs, doc{c.Want, findArchEvidence(p)})
	}
	w := defaultArchWeights
	w.Words.Whole, w.Words.WholeCap = w.Words.Section, w.Words.SectionCap
	// fifth part: group nil means the whole help (main); otherwise the
	// group's sections are read together as one part. extra sections are
	// separate parts. Every other section is left out.
	shape := func(pe phraseEvidence, group, extra []string) phraseEvidence {
		out := pe
		if group != nil {
			merged := map[int]int{}
			for _, s := range group {
				for i, n := range pe.zones[zoneOf(s)] {
					merged[i] += n
				}
			}
			out.zones[zoneWhole] = merged
		}
		keep := map[int]bool{}
		for _, s := range extra {
			keep[zoneOf(s)] = true
		}
		for z := zoneSection; z < numZones; z++ {
			if !keep[z] {
				out.zones[z] = nil
			}
		}
		return out
	}
	type setup struct {
		group, extra []string
		ok           []bool
	}
	run := func(group, extra []string) setup {
		s := setup{group, extra, make([]bool, len(docs))}
		for i, d := range docs {
			ev := d.ev
			ev.words = shape(ev.words, group, extra)
			ev.broad = shape(ev.broad, group, extra)
			s.ok[i] = scoreArchEvidence(w, ev)[0].ID == d.want
		}
		return s
	}
	count := func(row []bool, keep func(int) bool) (n int) {
		for i, r := range row {
			if keep(i) && r {
				n++
			}
		}
		return
	}
	all := func(int) bool { return true }
	tot := len(docs)
	pct := func(n int) float64 { return 100 * float64(n) / float64(tot) }
	// cv picks, in each fold, the setup that is right most often on the
	// other four folds and counts it on the fold left out.
	cv := func(name string, ss []setup) {
		n := 0
		var chose []string
		for f := 0; f < 5; f++ {
			best, bestN := 0, -1
			for si := range ss {
				if c := count(ss[si].ok, func(i int) bool { return i%5 != f }); c > bestN {
					best, bestN = si, c
				}
			}
			n += count(ss[best].ok, func(i int) bool { return i%5 == f })
			chose = append(chose, fmt.Sprintf("%v+%v", ss[best].group, ss[best].extra))
		}
		order := make([]int, len(ss))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return count(ss[order[a]].ok, all) > count(ss[order[b]].ok, all) })
		t.Logf("%-28s fair %d/%d (%.1f%%)", name, n, tot, pct(n))
		for _, si := range order[:min(3, len(ss))] {
			t.Logf("    best %d/%d group %v extra %v", count(ss[si].ok, all), tot, ss[si].group, ss[si].extra)
		}
		t.Logf("    folds chose %v", chose)
	}
	top := []string{secOptions, secMoreDescription}
	base := run(nil, nil)
	t.Logf("main as it is:               %d/%d (%.1f%%)", count(base.ok, all), tot, pct(count(base.ok, all)))
	groupsWithout := func(extra []string) [][]string {
		out := map[bool]bool{}
		_ = out
		skip := map[string]bool{}
		for _, s := range extra {
			skip[s] = true
		}
		var rest []string
		for _, s := range helpSections {
			if !skip[s] {
				rest = append(rest, s)
			}
		}
		var subs [][]string
		var pick func(start int, cur []string)
		pick = func(start int, cur []string) {
			if len(cur) > 0 {
				subs = append(subs, append([]string(nil), cur...))
			}
			if len(cur) == 4 {
				return
			}
			for i := start; i < len(rest); i++ {
				pick(i+1, append(cur, rest[i]))
			}
		}
		pick(0, nil)
		return subs
	}
	var grouped0 []setup
	for _, g := range groupsWithout(nil) {
		grouped0 = append(grouped0, run(g, nil))
	}
	cv("grouped sections", grouped0)
	var grouped1, main1 []setup
	for _, e := range top {
		extra := []string{e}
		main1 = append(main1, run(nil, extra))
		for _, g := range groupsWithout(extra) {
			grouped1 = append(grouped1, run(g, extra))
		}
	}
	cv("grouped sections + 1", grouped1)
	cv("main + 1", main1)
	var grouped2 []setup
	for _, g := range groupsWithout(top) {
		grouped2 = append(grouped2, run(g, top))
	}
	cv("grouped sections + 2", grouped2)
	main2 := run(nil, top)
	t.Logf("main + 2 %v:  %d/%d (%.1f%%), nothing chosen so this is fair", top, count(main2.ok, all), tot, pct(count(main2.ok, all)))
}

// TestSwapBodySearch drops the whole help (the fifth part) and reads chosen
// help sections in its place, each as a separate part. Nothing is chosen
// from these programs, so each score is a fair one.
func TestSwapBodySearch(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "swap" {
		t.Skip("set MINT_ARCHETYPE_BENCH=swap")
	}
	type doc struct {
		want appkit.ArchetypeID
		ev   archEvidence
	}
	var docs []doc
	for _, c := range loadArchCases(t) {
		if !c.tunable() {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.readWhole, p.read = true, helpSections[:]
		docs = append(docs, doc{c.Want, findArchEvidence(p)})
	}
	w := defaultArchWeights
	w.Words.Whole, w.Words.WholeCap = w.Words.Section, w.Words.SectionCap
	shape := func(pe phraseEvidence, whole bool, parts []string) phraseEvidence {
		keep := map[int]bool{zoneWhole: whole}
		for _, s := range parts {
			keep[zoneOf(s)] = true
		}
		for z := zoneWhole; z < numZones; z++ {
			if !keep[z] {
				pe.zones[z] = nil
			}
		}
		return pe
	}
	tot := len(docs)
	try := func(name string, whole bool, parts ...string) {
		n := 0
		for _, d := range docs {
			ev := d.ev
			ev.words = shape(ev.words, whole, parts)
			ev.broad = shape(ev.broad, whole, parts)
			if scoreArchEvidence(w, ev)[0].ID == d.want {
				n++
			}
		}
		t.Logf("%-46s %d/%d (%.1f%%)", name, n, tot, 100*float64(n)/float64(tot))
	}
	try("main: whole help", true)
	try("no fifth part", false)
	try("other", false, secOther)
	try("other + commands", false, secOther, secCommands)
	try("options", false, secOptions)
	try("more description", false, secMoreDescription)
	try("options + more description", false, secOptions, secMoreDescription)
	try("options + more description + other", false, secOptions, secMoreDescription, secOther)
	try("options + more description + other + commands", false, secOptions, secMoreDescription, secOther, secCommands)
}

// TestConfidenceSplit shows, for main and each alternative tried, how many
// choices were clear and right, clear but wrong, close (a maybe) and right,
// and close and wrong, at the shipped clear lead and at a lead calibrated
// so that clear choices are right at least 85% of the time.
func TestConfidenceSplit(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "confidence" {
		t.Skip("set MINT_ARCHETYPE_BENCH=confidence")
	}
	type doc struct {
		want appkit.ArchetypeID
		ev   archEvidence
	}
	var docs []doc
	for _, c := range loadArchCases(t) {
		if !c.tunable() {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.readWhole, p.read = true, helpSections[:]
		docs = append(docs, doc{c.Want, findArchEvidence(p)})
	}
	w := defaultArchWeights
	w.Words.Whole, w.Words.WholeCap = w.Words.Section, w.Words.SectionCap
	// fifth: "whole" (main), "group" (group read as one part) or "" (none).
	shape := func(pe phraseEvidence, fifth string, group, parts []string) phraseEvidence {
		if fifth == "group" {
			merged := map[int]int{}
			for _, s := range group {
				for i, n := range pe.zones[zoneOf(s)] {
					merged[i] += n
				}
			}
			pe.zones[zoneWhole] = merged
		} else if fifth == "" {
			pe.zones[zoneWhole] = nil
		}
		keep := map[int]bool{}
		for _, s := range parts {
			keep[zoneOf(s)] = true
		}
		for z := zoneSection; z < numZones; z++ {
			if !keep[z] {
				pe.zones[z] = nil
			}
		}
		return pe
	}
	type res struct {
		right bool
		lead  int
	}
	split := func(rs []res, lead int) (cr, cw, mr, mw int) {
		for _, r := range rs {
			switch {
			case r.lead >= lead && r.right:
				cr++
			case r.lead >= lead:
				cw++
			case r.right:
				mr++
			default:
				mw++
			}
		}
		return
	}
	t.Logf("%-44s %-22s %s", "", "clear lead 32", "calibrated (clear >= 85% right)")
	t.Logf("%-44s %5s %5s %5s %5s | %4s %5s %5s %5s %5s", "setup", "clrOK", "clrNO", "mayOK", "mayNO", "lead", "clrOK", "clrNO", "mayOK", "mayNO")
	try := func(name, fifth string, group []string, parts ...string) {
		rs := make([]res, len(docs))
		for i, d := range docs {
			ev := d.ev
			ev.words = shape(ev.words, fifth, group, parts)
			ev.broad = shape(ev.broad, fifth, group, parts)
			s := scoreArchEvidence(w, ev)
			rs[i] = res{s[0].ID == d.want, s[0].Score - s[1].Score}
		}
		cr, cw, mr, mw := split(rs, clearLead)
		cal := 0
		for l := 0; l <= 400; l++ {
			a, b, _, _ := split(rs, l)
			if a+b > 0 && a*100 >= 85*(a+b) {
				cal = l
				break
			}
		}
		a, b, c, d := split(rs, cal)
		t.Logf("%-44s %5d %5d %5d %5d | %4d %5d %5d %5d %5d", name, cr, cw, mr, mw, cal, a, b, c, d)
	}
	md, op, ot, cm := secMoreDescription, secOptions, secOther, secCommands
	try("main: whole help", "whole", nil)
	try("main + more description", "whole", nil, md)
	try("main + options", "whole", nil, op)
	try("main + options + more description", "whole", nil, op, md)
	try("grouped [md op environment other]", "group", []string{md, op, secEnvironment, ot})
	try("grouped [md cmd examples notes] + options", "group", []string{md, cm, secExamples, secNotes}, op)
	try("grouped [files exit notes] + op + md", "group", []string{secFiles, secExitStatus, secNotes}, op, md)
	try("all 12 separate", "", nil, helpSections[:]...)
	try("separate [op files exit notes]", "", nil, op, secFiles, secExitStatus, secNotes)
	try("swap: other", "", nil, ot)
	try("swap: other + commands", "", nil, ot, cm)
	try("swap: op + md", "", nil, op, md)
	try("swap: op + md + other", "", nil, op, md, ot)
	try("no fifth part", "", nil)
}

// TestNewLayoutSearch tries new ways to use the fifth part. The four front
// parts (name, summary, description, usage), the behaviour cues and SEE
// ALSO are scored as now; the fifth part's points per archetype are then
// added in one of these ways:
//
//	plain   the points ×m (m = 1 is how they are added now)
//	topK    only to the K archetypes the front parts rank highest
//	agree   only to archetypes the front parts gave some points
//	share   F points shared out in proportion to the fifth part's points,
//	        so a long help with something for everyone gives each little
//	share+topK, share+agree  both
//
// The fifth part is the whole help (main), all sections read together, a
// group of sections, or "breadth": a phrase counts once per section it
// appears in, however often it is repeated there.
// Settings are chosen by 5-fold cross-validation.
func TestNewLayoutSearch(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "layouts" {
		t.Skip("set MINT_ARCHETYPE_BENCH=layouts")
	}
	defs := appkit.AllArchetypeDefs()
	idx := map[appkit.ArchetypeID]int{}
	for i, d := range defs {
		idx[d.ID] = i
	}
	w := defaultArchWeights
	w.Words.Whole, w.Words.WholeCap = w.Words.Section, w.Words.SectionCap
	md, op, ot, en := secMoreDescription, secOptions, secOther, secEnvironment
	group4 := []string{md, op, en, ot}
	type source struct {
		name  string
		fair  bool // nothing in it was chosen on these programs
		build func(pe phraseEvidence) map[int]int
	}
	sum := func(secs []string) func(pe phraseEvidence) map[int]int {
		return func(pe phraseEvidence) map[int]int {
			m := map[int]int{}
			for _, s := range secs {
				for i, n := range pe.zones[zoneOf(s)] {
					m[i] += n
				}
			}
			return m
		}
	}
	breadth := func(secs []string) func(pe phraseEvidence) map[int]int {
		return func(pe phraseEvidence) map[int]int {
			m := map[int]int{}
			for _, s := range secs {
				for i, n := range pe.zones[zoneOf(s)] {
					if n > 0 {
						m[i]++
					}
				}
			}
			return m
		}
	}
	sources := []source{
		{"whole help", true, func(pe phraseEvidence) map[int]int { return pe.zones[zoneWhole] }},
		{"all sections together", true, sum(helpSections[:])},
		{"breadth, all sections", true, breadth(helpSections[:])},
		{"group [md op env other]", false, sum(group4)},
		{"breadth [md op env other]", false, breadth(group4)},
	}
	vec := func(s []ArchetypeScore) []float64 {
		v := make([]float64, len(defs))
		for _, x := range s {
			v[idx[x.ID]] = float64(x.Score)
		}
		return v
	}
	type doc struct {
		want    int
		front   []float64
		contrib [][]float64 // per source
	}
	var docs []doc
	for _, c := range loadArchCases(t) {
		if !c.tunable() {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.readWhole, p.read = true, helpSections[:]
		ev := findArchEvidence(p)
		with := func(build func(phraseEvidence) map[int]int) archEvidence {
			e := ev
			for _, pe := range []*phraseEvidence{&e.words, &e.broad} {
				var z map[int]int
				if build != nil {
					z = build(*pe)
				}
				for k := zoneSection; k < numZones; k++ {
					pe.zones[k] = nil
				}
				pe.zones[zoneWhole] = z
			}
			return e
		}
		d := doc{want: idx[c.Want], front: vec(scoreArchEvidence(w, with(nil)))}
		for _, s := range sources {
			full := vec(scoreArchEvidence(w, with(s.build)))
			cv := make([]float64, len(defs))
			for i := range cv {
				cv[i] = full[i] - d.front[i]
			}
			d.contrib = append(d.contrib, cv)
		}
		docs = append(docs, d)
	}
	type layout struct {
		name string
		src  int
		mode string
		m    float64
		k, f int
	}
	var layouts []layout
	for si, s := range sources {
		for _, m := range []float64{0.5, 0.75, 1, 1.25, 1.5, 2} {
			layouts = append(layouts, layout{fmt.Sprintf("%s: plain ×%.2f", s.name, m), si, "plain", m, 0, 0})
			layouts = append(layouts, layout{fmt.Sprintf("%s: agree ×%.2f", s.name, m), si, "agree", m, 0, 0})
			for _, k := range []int{2, 3, 4, 6} {
				layouts = append(layouts, layout{fmt.Sprintf("%s: top%d ×%.2f", s.name, k, m), si, "topK", m, k, 0})
			}
		}
		for _, f := range []int{20, 30, 40, 50, 60, 80, 100} {
			layouts = append(layouts, layout{fmt.Sprintf("%s: share %d", s.name, f), si, "share", 0, 0, f})
			layouts = append(layouts, layout{fmt.Sprintf("%s: share %d + agree", s.name, f), si, "share+agree", 0, 0, f})
			for _, k := range []int{2, 3, 4, 6} {
				layouts = append(layouts, layout{fmt.Sprintf("%s: share %d + top%d", s.name, f, k), si, "share+topK", 0, k, f})
			}
		}
	}
	family := func(l layout) string {
		return strings.TrimPrefix(l.mode, "")
	}
	type res struct {
		right bool
		lead  float64
	}
	apply := func(l layout, d doc) res {
		c := d.contrib[l.src]
		fin := append([]float64(nil), d.front...)
		allowed := make([]bool, len(fin))
		switch l.mode {
		case "topK", "share+topK":
			order := make([]int, len(fin))
			for i := range order {
				order[i] = i
			}
			sort.SliceStable(order, func(a, b int) bool { return d.front[order[a]] > d.front[order[b]] })
			for _, i := range order[:l.k] {
				allowed[i] = true
			}
		case "agree", "share+agree":
			for i, v := range d.front {
				allowed[i] = v > 0
			}
		default:
			for i := range allowed {
				allowed[i] = true
			}
		}
		if strings.HasPrefix(l.mode, "share") {
			tot := 0.0
			for i, v := range c {
				if allowed[i] && v > 0 {
					tot += v
				}
			}
			for i, v := range c {
				if allowed[i] && v > 0 {
					fin[i] += float64(l.f) * v / tot
				}
			}
		} else {
			for i, v := range c {
				if allowed[i] {
					fin[i] += l.m * v
				}
			}
		}
		best, second := 0, -1
		for i := 1; i < len(fin); i++ {
			if fin[i] > fin[best] {
				best, second = i, best
			} else if second < 0 || fin[i] > fin[second] {
				second = i
			}
		}
		return res{best == d.want, fin[best] - fin[second]}
	}
	results := make([][]res, len(layouts))
	for li, l := range layouts {
		results[li] = make([]res, len(docs))
		for i, d := range docs {
			results[li][i] = apply(l, d)
		}
	}
	right := func(rs []res, keep func(int) bool) (n int) {
		for i, r := range rs {
			if keep(i) && r.right {
				n++
			}
		}
		return
	}
	all := func(int) bool { return true }
	tot := len(docs)
	pct := func(n int) float64 { return 100 * float64(n) / float64(tot) }
	cvOver := func(pick func(layout) bool) (int, []string) {
		n := 0
		var chose []string
		for f := 0; f < 5; f++ {
			best, bestN := -1, -1
			for li, l := range layouts {
				if !pick(l) {
					continue
				}
				if c := right(results[li], func(i int) bool { return i%5 != f }); c > bestN {
					best, bestN = li, c
				}
			}
			n += right(results[best], func(i int) bool { return i%5 == f })
			chose = append(chose, layouts[best].name)
		}
		return n, chose
	}
	find := func(name string) int {
		for li, l := range layouts {
			if l.name == name {
				return li
			}
		}
		t.Fatalf("no layout %q", name)
		return -1
	}
	mainLi, grpLi := find("whole help: plain ×1.00"), find("group [md op env other]: plain ×1.00")
	t.Logf("%d programs, %d layouts", tot, len(layouts))
	t.Logf("main (whole help, plain ×1):              %d/%d (%.1f%%)", right(results[mainLi], all), tot, pct(right(results[mainLi], all)))
	t.Logf("grouped [md op env other] (plain ×1):     %d/%d (%.1f%%), group chosen on these programs", right(results[grpLi], all), tot, pct(right(results[grpLi], all)))
	t.Log("fair score of each way (settings and fifth part chosen by cross-validation, sources with no choice made on these programs):")
	modes := []string{"plain", "agree", "topK", "share", "share+agree", "share+topK"}
	for _, m := range modes {
		n, chose := cvOver(func(l layout) bool { return family(l) == m && sources[l.src].fair })
		t.Logf("  %-12s %d/%d (%.1f%%)  folds chose %v", m, n, tot, pct(n), chose)
	}
	n, chose := cvOver(func(l layout) bool { return sources[l.src].fair })
	t.Logf("  ANY (fair sources)  %d/%d (%.1f%%)  folds chose %v", n, tot, pct(n), chose)
	n, chose = cvOver(func(layout) bool { return true })
	t.Logf("  ANY (incl. group)   %d/%d (%.1f%%)  folds chose %v", n, tot, pct(n), chose)
	order := make([]int, len(layouts))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return right(results[order[a]], all) > right(results[order[b]], all) })
	t.Log("best seen (on these programs, so optimistic):")
	for _, li := range order[:20] {
		t.Logf("  %d/%d %s", right(results[li], all), tot, layouts[li].name)
	}
	split := func(rs []res, lead float64) (cr, cw, mr, mw int) {
		for _, r := range rs {
			switch {
			case r.lead >= lead && r.right:
				cr++
			case r.lead >= lead:
				cw++
			case r.right:
				mr++
			default:
				mw++
			}
		}
		return
	}
	conf := func(li int) {
		rs := results[li]
		cr, cw, mr, mw := split(rs, float64(clearLead))
		cal := 0.0
		for l := 0.0; l <= 400; l += 0.5 {
			a, b, _, _ := split(rs, l)
			if a+b > 0 && a*100 >= 85*(a+b) {
				cal = l
				break
			}
		}
		a, b, c, d := split(rs, cal)
		t.Logf("  %-46s at 32: %3d %3d %3d %3d | at %5.1f: %3d %3d %3d %3d", layouts[li].name, cr, cw, mr, mw, cal, a, b, c, d)
	}
	t.Log("clear right, clear wrong, maybe right, maybe wrong:")
	conf(mainLi)
	conf(grpLi)
	for _, li := range order[:8] {
		conf(li)
	}
	if v := os.Getenv("MINT_LAYOUT"); v != "" {
		conf(find(v))
	}
}

// TestGroupedPlusSearch compares main, main without its noise sections,
// grouped sections, and grouped sections with options and more
// description also read as separate parts (kept in the group too, or
// moved out of it), with how many clear and close choices are right.
func TestGroupedPlusSearch(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "grouped-plus" {
		t.Skip("set MINT_ARCHETYPE_BENCH=grouped-plus")
	}
	type doc struct {
		want appkit.ArchetypeID
		ev   archEvidence
	}
	var docs []doc
	for _, c := range loadArchCases(t) {
		if !c.tunable() {
			continue
		}
		man, help, ok := benchHelp(t, benchCache(), c.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(c.Name, man, help)
		p.readWhole, p.read = true, helpSections[:]
		docs = append(docs, doc{c.Want, findArchEvidence(p)})
	}
	w := defaultArchWeights
	w.Words.Whole, w.Words.WholeCap = w.Words.Section, w.Words.SectionCap
	md, op := secMoreDescription, secOptions
	group := []string{md, op, secEnvironment, secOther}
	without := func(drop ...string) []string {
		var out []string
		for _, s := range group {
			if !slices.Contains(drop, s) {
				out = append(out, s)
			}
		}
		return out
	}
	// fifth: "whole", "quiet" (whole less credits, SEE ALSO and BUGS) or
	// "group" (the sections in grp read together); parts are separate.
	shape := func(pe phraseEvidence, fifth string, grp, parts []string) phraseEvidence {
		switch fifth {
		case "group":
			merged := map[int]int{}
			for _, s := range grp {
				for i, n := range pe.zones[zoneOf(s)] {
					merged[i] += n
				}
			}
			pe.zones[zoneWhole] = merged
		case "quiet":
			merged := map[int]int{}
			for i, n := range pe.zones[zoneWhole] {
				merged[i] = n
			}
			for _, s := range []string{secCredits, secSeeAlso, secBugs} {
				for i, n := range pe.zones[zoneOf(s)] {
					merged[i] -= n
				}
			}
			for i, n := range merged {
				if n <= 0 {
					delete(merged, i)
				}
			}
			pe.zones[zoneWhole] = merged
		}
		keep := map[int]bool{}
		for _, s := range parts {
			keep[zoneOf(s)] = true
		}
		for z := zoneSection; z < numZones; z++ {
			if !keep[z] {
				pe.zones[z] = nil
			}
		}
		return pe
	}
	type res struct {
		right bool
		lead  int
	}
	split := func(rs []res, lead int) (cr, cw, mr, mw int) {
		for _, r := range rs {
			switch {
			case r.lead >= lead && r.right:
				cr++
			case r.lead >= lead:
				cw++
			case r.right:
				mr++
			default:
				mw++
			}
		}
		return
	}
	t.Logf("%-52s %5s | %-23s | %s", "", "", "clear lead 32", "clear line set for 85% right")
	t.Logf("%-52s %5s | %5s %5s %5s %5s | %4s %5s %5s %5s %5s", "setup", "right", "clrOK", "clrNO", "mayOK", "mayNO", "lead", "clrOK", "clrNO", "mayOK", "mayNO")
	try := func(name, fifth string, grp []string, parts ...string) {
		rs := make([]res, len(docs))
		n := 0
		for i, d := range docs {
			ev := d.ev
			ev.words = shape(ev.words, fifth, grp, parts)
			ev.broad = shape(ev.broad, fifth, grp, parts)
			s := scoreArchEvidence(w, ev)
			rs[i] = res{s[0].ID == d.want, s[0].Score - s[1].Score}
			if rs[i].right {
				n++
			}
		}
		cr, cw, mr, mw := split(rs, clearLead)
		cal := 0
		for l := 0; l <= 400; l++ {
			a, b, _, _ := split(rs, l)
			if a+b > 0 && a*100 >= 85*(a+b) {
				cal = l
				break
			}
		}
		a, b, c, d := split(rs, cal)
		t.Logf("%-52s %5d | %5d %5d %5d %5d | %4d %5d %5d %5d %5d", name, n, cr, cw, mr, mw, cal, a, b, c, d)
	}
	try("main (whole help)", "whole", nil)
	try("main without credits, SEE ALSO, BUGS", "quiet", nil)
	try("grouped [md op env other]", "group", group)
	try("grouped + options (options read twice)", "group", group, op)
	try("grouped [md env other] + options", "group", without(op), op)
	try("grouped + options + md (both read twice)", "group", group, op, md)
	try("grouped [env other] + options + md", "group", without(op, md), op, md)
}
