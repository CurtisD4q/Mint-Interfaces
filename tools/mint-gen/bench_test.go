package main

// The template benchmark: how well the scorer classifies real programs it
// has no list entry for. It runs only when asked:
//
//	MINT_TEMPLATE_BENCH=1 go test -run TestTemplateBenchmark -v .
//
// For each program in testdata/template_benchmark.txt that is installed,
// it reads the help the way mint-gen does (CollectHelp, sandboxed) and
// classifies it blind, with the template tool lists switched off. Help is
// cached in $MINT_TEMPLATE_BENCH_CACHE (default: a folder in the system
// temp dir) so runs are quick and repeatable; delete it to re-read.
//
// Programs are split into a dev set (tune on it) and a held-out set (never
// looked at while tuning) by a hash of the name, so the held-out numbers
// show how the scorer does on programs it was not tuned for.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type benchCase struct {
	Name, Want, Source string
}

func loadBenchCases(t *testing.T) []benchCase {
	f, err := os.Open("testdata/template_benchmark.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []benchCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			t.Fatalf("bad line %q", line)
		}
		out = append(out, benchCase{f[0], f[1], f[2]})
	}
	return out
}

// heldOut puts about 30% of programs in the held-out set, the same ones
// every run.
func heldOut(name string) bool {
	h := fnv.New32a()
	h.Write([]byte(name))
	return h.Sum32()%10 < 3
}

// benchHelp returns the program's man and help text, from the cache or
// read now.
func benchHelp(t *testing.T, cache, name string) (man, help string, ok bool) {
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

type benchResult struct {
	benchCase
	Got     string
	Verdict string // correct, fallback-correct, abstained, wrong
	Why     string
	Top     string
}

func TestTemplateBenchmark(t *testing.T) {
	if os.Getenv("MINT_TEMPLATE_BENCH") == "" {
		t.Skip("set MINT_TEMPLATE_BENCH=1 to run the template benchmark")
	}
	cache := os.Getenv("MINT_TEMPLATE_BENCH_CACHE")
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "mint-template-bench")
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}

	var results []benchResult
	var missing []string
	for _, c := range loadBenchCases(t) {
		man, help, ok := benchHelp(t, cache, c.Name)
		if !ok {
			missing = append(missing, c.Name)
			continue
		}
		choice := scoreTemplateChoice(c.Name, man, help)
		r := benchResult{benchCase: c, Got: choice.ID, Why: choice.Why}
		fellBack := !strings.HasPrefix(choice.Why, "best evidence") // no clear winner: the fallback was used
		switch {
		case choice.ID == c.Want && !fellBack:
			r.Verdict = "correct"
		case choice.ID == c.Want:
			r.Verdict = "fallback-correct"
		case fellBack:
			r.Verdict = "abstained"
		default:
			r.Verdict = "wrong"
		}
		var top []string
		for i, s := range choice.Scores {
			if i == 3 {
				break
			}
			top = append(top, fmt.Sprintf("%s:%d", s.ID, s.Score))
		}
		r.Top = strings.Join(top, " ")
		results = append(results, r)
	}

	report := func(title string, keep func(benchResult) bool) {
		n := map[string]int{}
		total := 0
		for _, r := range results {
			if keep(r) {
				n[r.Verdict]++
				total++
			}
		}
		if total == 0 {
			return
		}
		pct := func(k string) string { return fmt.Sprintf("%2d (%3.0f%%)", n[k], 100*float64(n[k])/float64(total)) }
		t.Logf("%-9s %3d programs | correct %s | correct by fallback %s | abstained %s | WRONG %s",
			title, total, pct("correct"), pct("fallback-correct"), pct("abstained"), pct("wrong"))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Want+results[i].Name < results[j].Want+results[j].Name })
	if os.Getenv("MINT_TEMPLATE_BENCH") == "all" {
		for _, r := range results {
			set := "dev"
			if heldOut(r.Name) {
				set = "held-out"
			}
			t.Logf("%-8s %-13s want %s got %s  %-16s %s   [%s]", set, r.Name, r.Want, r.Got, r.Verdict, r.Top, r.Why)
		}
	} else {
		for _, r := range results {
			if !heldOut(r.Name) && (r.Verdict == "wrong" || r.Verdict == "abstained") {
				t.Logf("dev      %-13s want %s got %s  %-16s %s   [%s]", r.Name, r.Want, r.Got, r.Verdict, r.Top, r.Why)
			}
		}
	}
	report("dev", func(r benchResult) bool { return !heldOut(r.Name) })
	report("held-out", func(r benchResult) bool { return heldOut(r.Name) })
	report("all", func(benchResult) bool { return true })
	if len(missing) > 0 {
		t.Logf("not installed (skipped): %s", strings.Join(missing, " "))
	}
}

// benchInput is one program's evidence, prepared once for tuning.
type benchInput struct {
	benchCase
	ev     templateEvidence
	hasSub bool
}

func loadBenchInputs(t *testing.T, cache string) []benchInput {
	var out []benchInput
	for _, c := range loadBenchCases(t) {
		man, help, ok := benchHelp(t, cache, c.Name)
		if !ok {
			continue
		}
		ml, hl := cleanLines(man), cleanLines(help)
		ev := findEvidence(c.Name, extractSummary(c.Name, ml, hl), usageLines(c.Name, ml, hl), man+"\n"+help, seeAlsoPrograms(man))
		out = append(out, benchInput{benchCase: c, ev: ev, hasSub: hasSubcommands(man, help, ml, hl)})
	}
	return out
}

// benchScore classifies inputs with w: correct, abstained, wrong.
func benchScore(w scoreWeights, inputs []benchInput) (correct, abstained, wrong int) {
	for _, in := range inputs {
		id, _ := pickTemplateWith(w, scoreEvidence(w, in.ev))
		switch {
		case id == "":
			fallback := "T05"
			if in.hasSub {
				fallback = "T21"
			}
			if fallback == in.Want {
				correct++
			} else {
				abstained++
			}
		case id == in.Want:
			correct++
		default:
			wrong++
		}
	}
	return
}

// TestTemplateTuning searches weights on the dev programs only, ranking by
// correct - 2*wrong (a wrong template is worse than falling back):
//
//	MINT_TEMPLATE_TUNE=1 go test -run TestTemplateTuning -v .
func TestTemplateTuning(t *testing.T) {
	if os.Getenv("MINT_TEMPLATE_TUNE") == "" {
		t.Skip("set MINT_TEMPLATE_TUNE=1 to search the scorer's weights")
	}
	cache := os.Getenv("MINT_TEMPLATE_BENCH_CACHE")
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "mint-template-bench")
	}
	var dev []benchInput
	for _, in := range loadBenchInputs(t, cache) {
		if !heldOut(in.Name) {
			dev = append(dev, in)
		}
	}
	type result struct {
		w                         scoreWeights
		correct, abstained, wrong int
	}
	var results []result
	base := defaultWeights
	for _, summ := range []int{4, 5, 6} {
		for _, weakSumm := range []int{2, 3, 4} {
			for _, usage := range []int{1, 2, 3} {
				for _, bodyCap := range []int{2, 3, 4, 6} {
					for _, rel := range []int{0, 1, 2, 3} {
						for _, relCap := range []int{2, 4, 6} {
							for _, minScore := range []int{3, 4, 5} {
								for _, margin := range []int{1, 2, 3} {
									w := base
									w.Summary, w.WeakSummary, w.Usage, w.BodyCap = summ, weakSumm, usage, bodyCap
									w.Related, w.RelatedCap, w.MinScore, w.MinMargin = rel, relCap, minScore, margin
									c, a, x := benchScore(w, dev)
									results = append(results, result{w, c, a, x})
								}
							}
						}
					}
				}
			}
		}
	}
	obj := func(r result) int { return r.correct - 2*r.wrong }
	sort.SliceStable(results, func(i, j int) bool {
		if obj(results[i]) != obj(results[j]) {
			return obj(results[i]) > obj(results[j])
		}
		return results[i].wrong < results[j].wrong
	})
	c, a, x := benchScore(defaultWeights, dev)
	t.Logf("current defaults: correct %d abstained %d wrong %d (objective %d) of %d dev programs", c, a, x, c-2*x, len(dev))
	for i, r := range results {
		if i == 15 {
			break
		}
		t.Logf("objective %3d  correct %2d abstained %2d wrong %2d  %+v", obj(r), r.correct, r.abstained, r.wrong, r.w)
	}
}
