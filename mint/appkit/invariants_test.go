package appkit

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// frameOverflow replays a frame onto an h×w terminal and returns the first
// text drawn outside it.
func frameOverflow(frame string, h, w int) string {
	r, c := 1, 1
	for i := 0; i < len(frame); {
		if frame[i] == 0x1b && i+1 < len(frame) && frame[i+1] == '[' {
			j := i + 2
			for j < len(frame) && (frame[j] < 0x40 || frame[j] > 0x7e) {
				j++
			}
			if j < len(frame) && frame[j] == 'H' {
				r, c = 1, 1
				if parts := strings.Split(frame[i+2:j], ";"); len(parts) == 2 {
					r, c = atoiTest(parts[0]), atoiTest(parts[1])
				}
			}
			i = j + 1
			continue
		}
		if frame[i] == 0x1b && i+1 < len(frame) && frame[i+1] == ']' {
			j := i + 2
			for j < len(frame) && frame[j] != 0x07 && !(frame[j] == 0x1b && j+1 < len(frame) && frame[j+1] == '\\') {
				j++
			}
			if j < len(frame) && frame[j] == 0x1b {
				j++
			}
			i = j + 1
			continue
		}
		ru, size := utf8.DecodeRuneInString(frame[i:])
		wd := util.VLen(string(ru))
		if wd > 0 && (r < 1 || r > h || c < 1 || c+wd-1 > w) {
			end := i + 30
			if end > len(frame) {
				end = len(frame)
			}
			return frame[i:end]
		}
		c += wd
		i += size
	}
	return ""
}

var testSizes = [][2]int{{12, 48}, {16, 59}, {20, 60}, {24, 80}, {30, 120}, {50, 200}}

// Every archetype's screen, at every size, stays inside the terminal.
func TestEveryArchetypeDrawsInsideTheTerminal(t *testing.T) {
	for _, d := range AllArchetypeDefs() {
		a := newTestApp(t, ToolSpec{Name: string(d.ID), Bin: "echo", ArchetypeID: string(d.ID),
			Flags: []FlagSpec{{Flag: "--a-very-long-option-name-indeed", Kind: FlagText, Label: strings.Repeat("Long label ", 12), Section: "Options"}}})
		for _, sz := range testSizes {
			if bad := frameOverflow(a.MainScreen().FrameText(sz[0], sz[1]), sz[0], sz[1]); bad != "" {
				t.Errorf("%s at %dx%d draws outside: %q", d.ID, sz[0], sz[1], bad)
			}
		}
	}
}

// The new layouts, at every size.
func TestInterfaceLayoutsDrawInsideTheTerminal(t *testing.T) {
	specs := map[string]ToolSpec{"suite": suiteSpec(), "monitor": monitorSpec(), "commands": cmdSpec()}
	for name, spec := range specs {
		a := newTestApp(t, spec)
		for _, sz := range testSizes {
			if bad := frameOverflow(a.MainScreen().FrameText(sz[0], sz[1]), sz[0], sz[1]); bad != "" {
				t.Errorf("%s at %dx%d draws outside: %q", name, sz[0], sz[1], bad)
			}
		}
	}
}

// Every pair of archetypes: it draws inside the terminal, and its data
// level is at least the higher of the two alone.
func TestEveryArchetypePairIsSafe(t *testing.T) {
	defs := AllArchetypeDefs()
	for _, x := range defs {
		for _, y := range defs {
			if x.ID == y.ID {
				continue
			}
			spec := ToolSpec{Name: "pair", Bin: "pair", Archetypes: []ArchetypeID{x.ID, y.ID}}
			lx, _ := ClassifyData(ToolSpec{Name: "pair", Bin: "pair", ArchetypeID: string(x.ID)})
			ly, _ := ClassifyData(ToolSpec{Name: "pair", Bin: "pair", ArchetypeID: string(y.ID)})
			lp, _ := ClassifyData(spec)
			if dataRank(lp) < dataRank(lx) || dataRank(lp) < dataRank(ly) {
				t.Fatalf("%s + %s is %q, below %q / %q", x.ID, y.ID, lp, lx, ly)
			}
			if _, ok := LookupInterfaceType(PlanFor(FactsFromSpec(spec)).Type); !ok {
				t.Fatalf("%s + %s has no interface type", x.ID, y.ID)
			}
		}
	}
	// Drawing every pair is slow; every pair with the first archetype of
	// each group is enough to reach every layout path.
	seen := map[string]bool{}
	for _, x := range defs {
		if seen[x.Group] {
			continue
		}
		seen[x.Group] = true
		for _, y := range defs {
			if x.ID == y.ID {
				continue
			}
			a := newTestApp(t, ToolSpec{Name: "pair", Bin: "echo", Archetypes: []ArchetypeID{x.ID, y.ID}})
			if bad := frameOverflow(a.MainScreen().FrameText(24, 80), 24, 80); bad != "" {
				t.Errorf("%s + %s draws outside: %q", x.ID, y.ID, bad)
			}
		}
	}
}

// Every combination of the ten answers with every archetype's hints gets
// one registered type, the first rule that matches.
func TestEveryFactsCombinationHasOneType(t *testing.T) {
	for _, d := range AllArchetypeDefs() {
		for bits := 0; bits < 1<<10; bits++ {
			b := func(i int) bool { return bits&(1<<i) != 0 }
			f := Facts{KeepsRunning: b(0), ChangesState: b(1), TakesTarget: b(2), TwoInputs: b(3), Subcommands: b(4),
				WritesFile: b(5), TableOutput: b(6), NeedsRoot: b(7), Destructive: b(8), FullScreen: b(9),
				Hints: d.Blueprint().Features, Weights: d.Prefer.Features}
			p := PlanFor(f)
			if _, ok := LookupInterfaceType(p.Type); !ok || p.Why == "" {
				t.Fatalf("%s %010b: %+v", d.ID, bits, p)
			}
			if f.FullScreen && p.Type != TypeLauncher {
				t.Fatalf("%s %010b: a full-screen program is %s", d.ID, bits, p.Type)
			}
		}
	}
}
