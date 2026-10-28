package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// TestVoteLayoutSearch tries "parts vote" layouts on top of main's points:
// each part (name, summary, description, usage, the whole help, and the
// behaviour cues in the help sections) is scored alone, and an archetype
// that is a part's clear winner gets that part's vote; or each part adds
// a rank-fusion share by where it ranks an archetype. The vote values and
// fusion settings are chosen by 5-fold cross-validation.
func TestVoteLayoutSearch(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "votes" {
		t.Skip("set MINT_ARCHETYPE_BENCH=votes")
	}
	defs := appkit.AllArchetypeDefs()
	idx := map[appkit.ArchetypeID]int{}
	for i, d := range defs {
		idx[d.ID] = i
	}
	w := defaultArchWeights
	w.Words.Whole, w.Words.WholeCap = w.Words.Section, w.Words.SectionCap
	sectionZones := map[int]bool{}
	for z := zoneSection; z < numZones; z++ {
		sectionZones[z] = true
	}
	parts := []struct {
		name  string
		zones map[int]bool
	}{
		{"name", map[int]bool{zoneName: true}},
		{"summary", map[int]bool{zoneSummary: true}},
		{"description", map[int]bool{zoneDescription: true}},
		{"usage", map[int]bool{zoneUsage: true}},
		{"whole", map[int]bool{zoneWhole: true}},
		{"cues", sectionZones},
	}
	keep := func(ev archEvidence, zones map[int]bool) archEvidence {
		e := ev
		e.cues = map[appkit.ArchetypeID][]cueHit{}
		for id, hs := range ev.cues {
			for _, h := range hs {
				if zones[h.zone] {
					e.cues[id] = append(e.cues[id], h)
				}
			}
		}
		for _, pe := range []*phraseEvidence{&e.words, &e.broad} {
			for z := range pe.zones {
				if !zones[z] {
					pe.zones[z] = nil
				}
			}
		}
		e.related = nil
		return e
	}
	vec := func(s []ArchetypeScore) []float64 {
		v := make([]float64, len(defs))
		for _, x := range s {
			v[idx[x.ID]] = float64(x.Score)
		}
		return v
	}
	type doc struct {
		want  int
		total []float64
		top   []int       // per part: its clear winner, or -1
		rank  [][]int     // per part, per archetype: 1-based rank, 0 = no points
		pts   [][]float64 // per part scores
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
		for _, pe := range []*phraseEvidence{&ev.words, &ev.broad} {
			for z := zoneSection; z < numZones; z++ {
				pe.zones[z] = nil // main: section words are read in the whole help
			}
		}
		d := doc{want: idx[c.Want], total: vec(scoreArchEvidence(w, ev))}
		for _, pt := range parts {
			v := vec(scoreArchEvidence(w, keep(ev, pt.zones)))
			order := make([]int, len(v))
			for i := range order {
				order[i] = i
			}
			sort.SliceStable(order, func(a, b int) bool { return v[order[a]] > v[order[b]] })
			top := -1
			if v[order[0]] > 0 && v[order[0]] > v[order[1]] {
				top = order[0]
			}
			r := make([]int, len(v))
			for k, i := range order {
				if v[i] > 0 {
					r[i] = k + 1
				}
			}
			d.top = append(d.top, top)
			d.rank = append(d.rank, r)
			d.pts = append(d.pts, v)
		}
		docs = append(docs, d)
	}
	tot := len(docs)
	pct := func(n int) float64 { return 100 * float64(n) / float64(tot) }
	// precision of each part's clear winner
	for pi, pt := range parts {
		has, right := 0, 0
		for _, d := range docs {
			if d.top[pi] >= 0 {
				has++
				if d.top[pi] == d.want {
					right++
				}
			}
		}
		t.Logf("%-12s has a clear winner on %3d, right on %3d (%.0f%%)", pt.name, has, right, 100*float64(right)/float64(max(has, 1)))
	}
	type res struct {
		right bool
		lead  float64
	}
	pick := func(fin []float64, want int) res {
		best, second := 0, -1
		for i := 1; i < len(fin); i++ {
			if fin[i] > fin[best] {
				best, second = i, best
			} else if second < 0 || fin[i] > fin[second] {
				second = i
			}
		}
		return res{best == want, fin[best] - fin[second]}
	}
	type layout struct {
		name string
		run  func(d doc) res
	}
	var layouts []layout
	layouts = append(layouts, layout{"main", func(d doc) res { return pick(d.total, d.want) }})
	vals := []float64{0, 10, 20, 40}
	var votes [6]float64
	var rec func(i int)
	rec = func(i int) {
		if i == len(parts) {
			v := votes
			layouts = append(layouts, layout{fmt.Sprintf("votes %v", v), func(d doc) res {
				fin := append([]float64(nil), d.total...)
				for pi, top := range d.top {
					if top >= 0 {
						fin[top] += v[pi]
					}
				}
				return pick(fin, d.want)
			}})
			return
		}
		for _, x := range vals {
			votes[i] = x
			rec(i + 1)
		}
	}
	rec(0)
	nVotes := len(layouts)
	partPts := []float64{15, 25, 20, 30, 10, 10}
	for _, k := range []float64{1, 2, 4, 8} {
		for _, lam := range []float64{0.5, 1, 2, 4, 8} {
			for _, only := range []bool{false, true} {
				k, lam, only := k, lam, only
				name := fmt.Sprintf("fusion k=%g ×%g", k, lam)
				if only {
					name += " alone"
				}
				layouts = append(layouts, layout{name, func(d doc) res {
					fin := make([]float64, len(d.total))
					if !only {
						copy(fin, d.total)
					}
					for pi := range parts {
						for a, r := range d.rank[pi] {
							if r > 0 {
								fin[a] += lam * partPts[pi] * (k + 1) / (k + float64(r))
							}
						}
					}
					return pick(fin, d.want)
				}})
			}
		}
	}
	// Learned layout: each part's points are scaled by a learned factor and
	// each archetype gets a learned offset (correcting archetypes that win
	// too often), trained by an averaged margin perceptron on the training
	// folds only. Settings are chosen by an inner cross-validation within the
	// training folds, so the score on the held-out fold is a fair one.
	type cfg struct {
		parts, bias bool
		margin      float64
	}
	var cfgs []cfg
	for _, m := range []float64{0, 10, 32} {
		cfgs = append(cfgs, cfg{false, true, m}, cfg{true, false, m}, cfg{true, true, m})
	}
	nA := len(defs)
	type model struct {
		u []float64
		b []float64
	}
	score := func(m model, d doc) []float64 {
		fin := append([]float64(nil), d.total...)
		for pi := range parts {
			for a := 0; a < nA; a++ {
				fin[a] += (m.u[pi] - 1) * d.pts[pi][a]
			}
		}
		for a := range fin {
			fin[a] += m.b[a]
		}
		return fin
	}
	train := func(c cfg, ds []doc) model {
		cur := model{make([]float64, len(parts)), make([]float64, nA)}
		sum := model{make([]float64, len(parts)), make([]float64, nA)}
		for pi := range cur.u {
			cur.u[pi] = 1
		}
		steps := 0
		for ep := 0; ep < 20; ep++ {
			for _, d := range ds {
				fin := score(cur, d)
				pred := -1
				for a := range fin {
					if a != d.want && (pred < 0 || fin[a] > fin[pred]) {
						pred = a
					}
				}
				if fin[d.want]-fin[pred] < c.margin {
					if c.bias {
						cur.b[d.want]++
						cur.b[pred]--
					}
					if c.parts {
						for pi := range cur.u {
							cur.u[pi] += 0.002 * (d.pts[pi][d.want] - d.pts[pi][pred])
						}
					}
				}
				for pi := range cur.u {
					sum.u[pi] += cur.u[pi]
				}
				for a := range cur.b {
					sum.b[a] += cur.b[a]
				}
				steps++
			}
		}
		for pi := range sum.u {
			sum.u[pi] /= float64(steps)
		}
		for a := range sum.b {
			sum.b[a] /= float64(steps)
		}
		return sum
	}
	learned := make([]res, len(docs))
	perCfg := make([][]res, len(cfgs))
	for ci := range cfgs {
		perCfg[ci] = make([]res, len(docs))
	}
	var learnChose []string
	for f := 0; f < 5; f++ {
		var tr []doc
		for i, d := range docs {
			if i%5 != f {
				tr = append(tr, d)
			}
		}
		for ci, c := range cfgs {
			m := train(c, tr)
			for i, d := range docs {
				if i%5 == f {
					perCfg[ci][i] = pick(score(m, d), d.want)
				}
			}
		}
		best, bestN := 0, -1
		for ci, c := range cfgs {
			n := 0
			for g := 0; g < 4; g++ {
				var itr, ite []doc
				for j, d := range tr {
					if j%4 == g {
						ite = append(ite, d)
					} else {
						itr = append(itr, d)
					}
				}
				m := train(c, itr)
				for _, d := range ite {
					if pick(score(m, d), d.want).right {
						n++
					}
				}
			}
			if n > bestN {
				best, bestN = ci, n
			}
		}
		learnChose = append(learnChose, fmt.Sprintf("%+v", cfgs[best]))
		for i := range docs {
			if i%5 == f {
				learned[i] = perCfg[best][i]
			}
		}
		if f == 0 {
			m := train(cfgs[best], tr)
			t.Logf("fold 0 learned part factors %.2f", m.u)
			type ab struct {
				id string
				b  float64
			}
			var bs []ab
			for a, v := range m.b {
				bs = append(bs, ab{string(defs[a].ID), v})
			}
			sort.Slice(bs, func(x, y int) bool { return bs[x].b < bs[y].b })
			t.Logf("fold 0 most lowered %v", bs[:5])
			t.Logf("fold 0 most raised  %v", bs[len(bs)-5:])
		}
	}
	results := make([][]res, len(layouts))
	for li, l := range layouts {
		results[li] = make([]res, len(docs))
		for i, d := range docs {
			results[li][i] = l.run(d)
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
	cvOver := func(from, to int) (int, []string) {
		n := 0
		var chose []string
		for f := 0; f < 5; f++ {
			best, bestN := -1, -1
			for li := from; li < to; li++ {
				if c := right(results[li], func(i int) bool { return i%5 != f }); c > bestN {
					best, bestN = li, c
				}
			}
			n += right(results[best], func(i int) bool { return i%5 == f })
			chose = append(chose, layouts[best].name)
		}
		return n, chose
	}
	t.Logf("main: %d/%d (%.1f%%)", right(results[0], all), tot, pct(right(results[0], all)))
	n, chose := cvOver(1, nVotes)
	t.Logf("votes  [name summary description usage whole cues], fair %d/%d (%.1f%%)  folds chose %v", n, tot, pct(n), chose)
	n, chose = cvOver(nVotes, len(layouts))
	t.Logf("fusion, fair %d/%d (%.1f%%)  folds chose %v", n, tot, pct(n), chose)
	n, chose = cvOver(0, len(layouts))
	t.Logf("any, fair %d/%d (%.1f%%)  folds chose %v", n, tot, pct(n), chose)
	nl := 0
	for _, r := range learned {
		if r.right {
			nl++
		}
	}
	t.Logf("learned (nested cross-validation), fair %d/%d (%.1f%%)  folds chose %v", nl, tot, pct(nl), learnChose)
	for ci, c := range cfgs {
		n := 0
		for _, r := range perCfg[ci] {
			if r.right {
				n++
			}
		}
		t.Logf("  learned %+v alone: %d/%d", c, n, tot)
	}
	layouts = append(layouts, layout{"learned (fair)", nil})
	results = append(results, learned)
	order := make([]int, len(layouts))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return right(results[order[a]], all) > right(results[order[b]], all) })
	for _, li := range order[:12] {
		t.Logf("  best seen %d/%d %s", right(results[li], all), tot, layouts[li].name)
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
		for l := 0.0; l <= 600; l += 0.5 {
			a, b, _, _ := split(rs, l)
			if a+b > 0 && a*100 >= 85*(a+b) {
				cal = l
				break
			}
		}
		a, b, c, d := split(rs, cal)
		t.Logf("  %-34s at 32: %3d %3d %3d %3d | at %5.1f: %3d %3d %3d %3d", layouts[li].name, cr, cw, mr, mw, cal, a, b, c, d)
	}
	t.Log("clear right, clear wrong, maybe right, maybe wrong:")
	conf(0)
	conf(len(layouts) - 1)
	for _, li := range order[:3] {
		conf(li)
	}
}
