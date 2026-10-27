package main

// Learning the template scorer from examples. Runs only when asked:
//
//	MINT_TEMPLATE_LEARN=eval go test -run TestTemplateLearn -v .
//
// The labelled pool is every program on a template list plus the
// benchmark sets; testdata/template_untouched.txt is kept apart and only
// ever measured. Help comes from the same cache as the benchmarks.

import (
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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
	reportSets(t, "hand-written scorer, no lists:", docs, handScorer)
	if mode == "eval" {
		o := trainOpts{L2: 1e-3, Epochs: 40, Rate: 0.3, MinDocs: 2}
		for _, v := range []struct {
			name       string
			body, hand bool
		}{{"A: no whole-help words", false, false}, {"B: A + hand scores", false, true}, {"C: all + hand scores", true, true}} {
			fdocs := toFeatDocsWith(docs, v.body, v.hand)
			cv := crossValidate(fdocs, o)
			for _, tau := range []float64{0, 0.5, 0.7, 0.8} {
				var vv verdicts
				for _, d := range fdocs {
					if r, ok := cv[d.Name]; ok {
						got := r.id
						if r.p < tau {
							got = ""
						}
						vv.add(d.Want, got)
					}
				}
				t.Logf("cv %s sure>=%.1f: %s", v.name, tau, vv)
			}
		}
	}
}

// learnDoc is one program's text, read once.
type learnDoc struct {
	labelled
	text programText
	sub  bool // has subcommands (for the fallback)
}

func learnDocs(t *testing.T) []learnDoc {
	var out []learnDoc
	for _, l := range learnPool(t) {
		man, help, ok := benchHelp(t, learnCache(), l.Name)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		p := programTextFrom(l.Name, man, help)
		p.Related = nil // they come from the lists, which are what is being replaced
		ml, hl := cleanLines(man), cleanLines(help)
		out = append(out, learnDoc{labelled: l, text: p, sub: hasSubcommands(man, help, ml, hl)})
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

// ---- softmax regression ----

// maxBodyFeatures caps how many whole-help features a program keeps (its
// strongest), so training stays quick and a huge man page stays one vote.
const maxBodyFeatures = 400

type featDoc struct {
	learnDoc
	names []string
	vals  []float64
}

func toFeatDocs(docs []learnDoc) []featDoc { return toFeatDocsWith(docs, true, false) }

// toFeatDocsWith picks feature groups: words from the whole help, and the
// hand-written scorer's per-template scores.
func toFeatDocsWith(docs []learnDoc, bodyWords, hand bool) []featDoc {
	out := make([]featDoc, len(docs))
	for i, d := range docs {
		f := templateFeatures(d.text)
		if !bodyWords {
			for k := range f {
				if strings.HasPrefix(k, featBody) {
					delete(f, k)
				}
			}
		}
		if hand {
			for _, sc := range scoreEvidence(defaultWeights, findEvidence(d.text)) {
				f["h:"+sc.ID] = float64(sc.Score) / 10
			}
		}
		names, vals := capFeatures(f, maxBodyFeatures)
		out[i] = featDoc{d, names, vals}
	}
	return out
}

type softmaxModel struct {
	classes []string
	vocab   map[string]int
	w       []float64 // feature*K + class
	bias    []float64
}

type trainOpts struct {
	L2      float64
	Epochs  int
	Rate    float64
	MinDocs int // a feature must appear in this many training programs
}

func templateClasses() []string {
	var classes []string
	for _, tpl := range appkit.AllTemplates() {
		classes = append(classes, string(tpl.ID))
	}
	return classes
}

func trainSoftmax(docs []featDoc, o trainOpts) *softmaxModel {
	classes := templateClasses()
	K := len(classes)
	cls := map[string]int{}
	for i, c := range classes {
		cls[c] = i
	}
	df := map[string]int{}
	for _, d := range docs {
		for _, n := range d.names {
			df[n]++
		}
	}
	var names []string
	for n, c := range df {
		if c >= o.MinDocs {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	m := &softmaxModel{classes: classes, vocab: map[string]int{}, w: make([]float64, len(names)*K), bias: make([]float64, K)}
	for i, n := range names {
		m.vocab[n] = i
	}
	type idoc struct {
		idx  []int
		vals []float64
		y    int
	}
	ids := make([]idoc, len(docs))
	for i, d := range docs {
		ids[i].y = cls[d.Want]
		for j, n := range d.names {
			if f, ok := m.vocab[n]; ok {
				ids[i].idx = append(ids[i].idx, f)
				ids[i].vals = append(ids[i].vals, d.vals[j])
			}
		}
	}
	g2 := make([]float64, len(m.w))
	b2 := make([]float64, K)
	p := make([]float64, K)
	for ep := 0; ep < o.Epochs; ep++ {
		for _, d := range ids {
			m.probsIdx(d.idx, d.vals, p)
			for k := 0; k < K; k++ {
				g := p[k]
				if k == d.y {
					g--
				}
				b2[k] += g * g
				m.bias[k] -= o.Rate * g / (1e-8 + math.Sqrt(b2[k]))
			}
			for j, f := range d.idx {
				base := f * K
				for k := 0; k < K; k++ {
					g := p[k] * d.vals[j]
					if k == d.y {
						g -= d.vals[j]
					}
					g += o.L2 * m.w[base+k]
					g2[base+k] += g * g
					m.w[base+k] -= o.Rate * g / (1e-8 + math.Sqrt(g2[base+k]))
				}
			}
		}
	}
	return m
}

func (m *softmaxModel) probsIdx(idx []int, vals []float64, p []float64) {
	K := len(p)
	copy(p, m.bias)
	for j, f := range idx {
		base := f * K
		for k := 0; k < K; k++ {
			p[k] += m.w[base+k] * vals[j]
		}
	}
	softmaxInPlace(p)
}

// predict returns the best template and its probability.
func (m *softmaxModel) predict(d featDoc) (string, float64) {
	var idx []int
	var vals []float64
	for j, n := range d.names {
		if f, ok := m.vocab[n]; ok {
			idx = append(idx, f)
			vals = append(vals, d.vals[j])
		}
	}
	p := make([]float64, len(m.classes))
	m.probsIdx(idx, vals, p)
	best := 0
	for k := range p {
		if p[k] > p[best] {
			best = k
		}
	}
	return m.classes[best], p[best]
}

// crossValidate predicts every non-untouched program with a model that
// never saw it (10 folds by name), returning the template and probability.
func crossValidate(docs []featDoc, o trainOpts) map[string]struct {
	id string
	p  float64
} {
	out := map[string]struct {
		id string
		p  float64
	}{}
	fold := func(n string) int { h := fnv.New32a(); h.Write([]byte(n)); return int(h.Sum32() % 10) }
	for f := 0; f < 10; f++ {
		var train, test []featDoc
		for _, d := range docs {
			if d.Untouched {
				continue
			}
			if fold(d.Name) == f {
				test = append(test, d)
			} else {
				train = append(train, d)
			}
		}
		m := trainSoftmax(train, o)
		for _, d := range test {
			id, p := m.predict(d)
			out[d.Name] = struct {
				id string
				p  float64
			}{id, p}
		}
	}
	return out
}

// ---- similarity methods (few examples per template) ----

func cosine(a, b map[string]float64) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	var dot, na, nb float64
	for k, v := range a {
		dot += v * b[k]
		na += v * v
	}
	for _, v := range b {
		nb += v * v
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

func docMap(d featDoc, idf map[string]float64) map[string]float64 {
	m := map[string]float64{}
	for j, n := range d.names {
		if w, ok := idf[n]; ok {
			m[n] = d.vals[j] * w
		}
	}
	return m
}

func idfOf(train []featDoc) map[string]float64 {
	df := map[string]int{}
	for _, d := range train {
		for _, n := range d.names {
			df[n]++
		}
	}
	idf := map[string]float64{}
	for n, c := range df {
		if c >= 2 {
			idf[n] = math.Log(float64(len(train)+1) / float64(c))
		}
	}
	return idf
}

// simPredict classifies test docs by similarity to training docs: the
// template whose average (k=0) or whose k nearest programs are closest.
// The confidence is the lead of the best template over the next.
func simPredict(train, test []featDoc, k int) map[string]struct {
	id   string
	lead float64
} {
	idf := idfOf(train)
	tm := make([]map[string]float64, len(train))
	for i, d := range train {
		tm[i] = docMap(d, idf)
	}
	cent := map[string]map[string]float64{}
	if k == 0 {
		for i, d := range train {
			c := cent[d.Want]
			if c == nil {
				c = map[string]float64{}
				cent[d.Want] = c
			}
			nm := tm[i]
			var n float64
			for _, v := range nm {
				n += v * v
			}
			n = math.Sqrt(n)
			for f, v := range nm {
				if n > 0 {
					c[f] += v / n
				}
			}
		}
	}
	out := map[string]struct {
		id   string
		lead float64
	}{}
	for _, d := range test {
		q := docMap(d, idf)
		score := map[string]float64{}
		if k == 0 {
			for id, c := range cent {
				score[id] = cosine(q, c)
			}
		} else {
			type nb struct {
				s  float64
				id string
			}
			var all []nb
			for i, td := range train {
				all = append(all, nb{cosine(q, tm[i]), td.Want})
			}
			sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })
			for i := 0; i < k && i < len(all); i++ {
				score[all[i].id] += all[i].s
			}
		}
		var ids []string
		for id := range score {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			if score[ids[i]] != score[ids[j]] {
				return score[ids[i]] > score[ids[j]]
			}
			return ids[i] < ids[j]
		})
		r := struct {
			id   string
			lead float64
		}{}
		if len(ids) > 0 {
			r.id = ids[0]
			r.lead = score[ids[0]]
			if len(ids) > 1 {
				r.lead -= score[ids[1]]
			}
		}
		out[d.Name] = r
	}
	return out
}

func simCrossValidate(docs []featDoc, k int) map[string]struct {
	id   string
	lead float64
} {
	out := map[string]struct {
		id   string
		lead float64
	}{}
	fold := func(n string) int { h := fnv.New32a(); h.Write([]byte(n)); return int(h.Sum32() % 10) }
	for f := 0; f < 10; f++ {
		var train, test []featDoc
		for _, d := range docs {
			if d.Untouched {
				continue
			}
			if fold(d.Name) == f {
				test = append(test, d)
			} else {
				train = append(train, d)
			}
		}
		for n, r := range simPredict(train, test, k) {
			out[n] = r
		}
	}
	return out
}

// cachedFeatDocs computes feature docs once per variant and keeps them in
// the help cache folder, so experiments do not re-read every man page.
func cachedFeatDocs(t *testing.T, docs []learnDoc, body, hand bool) []featDoc {
	path := filepath.Join(learnCache(), fmt.Sprintf("features-body%v-hand%v.gob", body, hand))
	type stored struct {
		Name  string
		Names []string
		Vals  []float64
	}
	byName := map[string]learnDoc{}
	for _, d := range docs {
		byName[d.Name] = d
	}
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		var st []stored
		if gob.NewDecoder(f).Decode(&st) == nil && len(st) == len(docs) {
			out := make([]featDoc, 0, len(st))
			for _, s := range st {
				out = append(out, featDoc{byName[s.Name], s.Names, s.Vals})
			}
			return out
		}
	}
	out := toFeatDocsWith(docs, body, hand)
	st := make([]stored, len(out))
	for i, d := range out {
		st[i] = stored{d.Name, d.names, d.vals}
	}
	if f, err := os.Create(path); err == nil {
		_ = gob.NewEncoder(f).Encode(st)
		f.Close()
	}
	return out
}
