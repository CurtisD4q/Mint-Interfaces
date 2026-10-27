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
	"sort"
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
		for _, list := range [][]signal{templateSignalList, archetypeSignalList} {
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
	for lead := 1; lead <= 12; lead++ {
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
	var grid []archWeights
	for _, weak := range []int{1, 2} {
		for _, med := range []int{3, 4, 5} {
			for _, strong := range []int{6, 8} {
				for _, summ := range []int{4, 5, 6} {
					for _, desc := range []int{2, 3, 4} {
						for _, usage := range []int{1, 2, 3} {
							for _, bodyCap := range []int{2, 3, 5} {
								for _, share := range []int{0, 30, 60, 100} {
									for _, rel := range []int{0, 2} {
										w := defaultArchWeights
										w.Cue = [4]int{1, weak, med, strong}
										w.Words.Summary, w.Words.Description, w.Words.Usage, w.Words.BodyCap = summ, desc, usage, bodyCap
										w.TemplateShare, w.Related = share, rel
										grid = append(grid, w)
									}
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
		if scoreArchEvidence(defaultArchWeights, d.ev)[0].ID == d.c.Want {
			n0++
		}
	}
	t.Logf("current weights: %d/%d (%.1f%%)", n0, tot, 100*float64(n0)/float64(tot))
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
		t.Logf("%d/%d cue %v summary %d desc %d usage %d bodycap %d template %d%% related %d", all[i].n, tot,
			w.Cue, w.Words.Summary, w.Words.Description, w.Words.Usage, w.Words.BodyCap, w.TemplateShare, w.Related)
	}
}
