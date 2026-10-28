package main

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestLayoutDiagnosis(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "diag" {
		t.Skip()
	}
	w := defaultArchWeights
	w.Words.Whole, w.Words.WholeCap = w.Words.Section, w.Words.SectionCap
	group := map[appkit.ArchetypeID]string{}
	for _, d := range appkit.AllArchetypeDefs() {
		group[d.ID] = d.Group
	}
	// keepZones keeps evidence (cues, words, broad) found in the given zones
	// only; related is kept when rel.
	keep := func(ev archEvidence, zones map[int]bool, rel bool) archEvidence {
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
		if !rel {
			e.related = nil
		}
		return e
	}
	sectionZones := map[int]bool{}
	for z := zoneSection; z < numZones; z++ {
		sectionZones[z] = true
	}
	mainZones := map[int]bool{zoneName: true, zoneSummary: true, zoneDescription: true, zoneUsage: true, zoneWhole: true}
	for z := range sectionZones {
		mainZones[z] = true // cues sit in sections; their words are dropped below
	}
	parts := []struct {
		name  string
		zones map[int]bool
	}{
		{"name", map[int]bool{zoneName: true}},
		{"summary", map[int]bool{zoneSummary: true}},
		{"description", map[int]bool{zoneDescription: true}},
		{"usage", map[int]bool{zoneUsage: true}},
		{"whole help", map[int]bool{zoneWhole: true}},
		{"section cues", sectionZones},
	}
	n, wrong := 0, 0
	rank := map[int]int{}
	zero, sameGroup := 0, 0
	conf := map[string]int{}
	partRight := make([]int, len(parts))
	partRightWhenWrong := make([]int, len(parts))
	anyPartWhenWrong := 0
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
		// main: words in sections are dropped, the whole help is the fifth part
		mev := ev
		for _, pe := range []*phraseEvidence{&mev.words, &mev.broad} {
			for z := zoneSection; z < numZones; z++ {
				pe.zones[z] = nil
			}
		}
		s := scoreArchEvidence(w, mev)
		n++
		isWrong := s[0].ID != c.Want
		if isWrong {
			wrong++
			r := 0
			for i, x := range s {
				if x.ID == c.Want {
					r = i + 1
					if x.Score == 0 {
						zero++
					}
				}
			}
			if r > 5 {
				r = 6
			}
			rank[r]++
			if group[s[0].ID] == group[c.Want] {
				sameGroup++
			}
			conf[string(c.Want)+" -> "+string(s[0].ID)]++
		}
		anyRight := false
		for i, pt := range parts {
			ps := scoreArchEvidence(w, keep(mev, pt.zones, false))
			if ps[0].Score > 0 && ps[0].Score > ps[1].Score && ps[0].ID == c.Want {
				partRight[i]++
				if isWrong {
					partRightWhenWrong[i]++
					anyRight = true
				}
			}
		}
		if isWrong && anyRight {
			anyPartWhenWrong++
		}
	}
	t.Logf("main: %d/%d right, %d wrong", n-wrong, n, wrong)
	t.Logf("rank of the right archetype when wrong: 2nd %d, 3rd %d, 4th %d, 5th %d, lower %d", rank[2], rank[3], rank[4], rank[5], rank[6])
	t.Logf("right archetype had no points at all: %d", zero)
	t.Logf("wrong choice in the same group as the right one: %d", sameGroup)
	t.Logf("right when wrong overall, by one part alone (a clear lone winner):")
	for i, pt := range parts {
		t.Logf("  %-13s alone right %3d/%d, and right on %d of the %d wrong", pt.name, partRight[i], n, partRightWhenWrong[i], wrong)
	}
	t.Logf("some part alone was right on %d of the %d wrong", anyPartWhenWrong, wrong)
	type kv struct {
		k string
		v int
	}
	var cs []kv
	for k, v := range conf {
		cs = append(cs, kv{k, v})
	}
	sort.Slice(cs, func(a, b int) bool { return cs[a].v > cs[b].v || cs[a].v == cs[b].v && cs[a].k < cs[b].k })
	for _, x := range cs[:min(20, len(cs))] {
		t.Logf("  %2d  %s", x.v, x.k)
	}
}
