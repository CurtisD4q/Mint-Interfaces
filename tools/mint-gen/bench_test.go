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
					top += fmt.Sprintf(" %s:%s", s.ID, points(s.Score))
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
	for _, lead := range []float64{1, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 45, 50} {
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
		t.Logf("lead>=%4.1f: clear %3d (%3.0f%%) right %3.0f%% | close %3d right %3.0f%%", lead, clear,
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
	// description=20,name=15,body=10"; else the default one.
	base0 := defaultArchWeights
	if spec := os.Getenv("MINT_ARCHETYPE_SPLIT"); spec != "" {
		zone := map[string]int{"name": zoneName, "summary": zoneSummary, "description": zoneDescription, "usage": zoneUsage, "body": zoneBody}
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
		base0.Words.Name, base0.Words.Summary, base0.Words.Description, base0.Words.Usage, base0.Words.Body = cz[zoneName], cz[zoneSummary], cz[zoneDescription], cz[zoneUsage], cz[zoneBody]
		base0.Related = float64(cz[zoneBody])
	}
	t.Logf("split: name %d summary %d description %d usage %d body %d", base0.CueZone[zoneName], base0.CueZone[zoneSummary], base0.CueZone[zoneDescription], base0.CueZone[zoneUsage], base0.CueZone[zoneBody])
	var grid []archWeights
	for _, scale := range []int{50, 100, 150} {
		for _, base := range []int{10, 20} {
			for _, weak := range []int{25, 35, 50} {
				for _, med := range []int{55, 70, 85} {
					for _, weakWord := range []int{30, 40, 55} {
						for _, bodyCap := range []int{2, 3, 5} {
							for _, share := range []int{0, 30, 60} {
								for _, rel := range []int{0, 1} {
									w := base0
									w.CueStrength = [4]int{base * scale / 100, weak * scale / 100, med * scale / 100, scale}
									ww := &w.Words
									ww.WeakName, ww.WeakSummary, ww.WeakDescription, ww.WeakUsage = ww.Name*weakWord/100, ww.Summary*weakWord/100, ww.Description*weakWord/100, ww.Usage*weakWord/100
									ww.BodyCap = bodyCap * ww.Body
									w.BroadShare, w.Related = float64(share), float64(rel*ww.Body)
									grid = append(grid, w)
								}
							}
						}
					}
				}
			}
		}
	}
	// The scorer before the split, for comparison: cue points by strength
	// alone, phrases by the old part points.
	prev := defaultArchWeights
	prev.CueZone = [numZones]int{100, 100, 100, 100, 100}
	prev.CueStrength = [4]int{1, 2, 4, 6}
	prev.Words = defaultWeights
	prev.Words.BodyCap = 3
	prev.BroadShare = 0
	prev.Related = 2
	grid = append(grid, prev)
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
	if np, _ := right(len(grid)-1, func(int) bool { return true }); true {
		t.Logf("current scorer (PR #60): %d/%d (%.1f%%)", np, tot, 100*float64(np)/float64(tot))
	}
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
		t.Logf("%d/%d cue strength %v weak words %d/%d/%d/%d bodycap %d broad %v%% related %v", all[i].n, tot,
			w.CueStrength, w.Words.WeakName, w.Words.WeakSummary, w.Words.WeakDescription, w.Words.WeakUsage, w.Words.BodyCap, w.BroadShare, w.Related)
	}
}
