package appkit

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// answerSet is one line of testdata/answer_sets.txt: how many programs of an
// archetype gave one set of the ten answers.
type answerSet struct {
	arch  ArchetypeID
	bits  string
	count int
}

func loadAnswerSets(t *testing.T) []answerSet {
	t.Helper()
	f, err := os.Open("testdata/answer_sets.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []answerSet
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) != 3 || len(fs[1]) != 10 {
			t.Fatalf("bad line %q", line)
		}
		n, err := strconv.Atoi(fs[2])
		if err != nil {
			t.Fatalf("bad count in %q", line)
		}
		out = append(out, answerSet{ArchetypeID(fs[0]), fs[1], n})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// factsOf builds facts from the ten answers in the order the data file
// lists them, with the archetype's wanted features as hints.
func factsOf(t *testing.T, s answerSet) Facts {
	t.Helper()
	b := func(i int) bool { return s.bits[i] == '1' }
	f := Facts{
		KeepsRunning: b(0), ChangesState: b(1), TakesTarget: b(2), TwoInputs: b(3), Subcommands: b(4),
		WritesFile: b(5), TableOutput: b(6), NeedsRoot: b(7), Destructive: b(8), FullScreen: b(9),
	}
	def, ok := LookupArchetype(string(s.arch))
	if !ok {
		t.Fatalf("unknown archetype %q in the data", s.arch)
	}
	f.Hints = def.Blueprint().Features
	f.Weights = def.Prefer.Features
	return f
}

func TestInterfaceTypeTableIsWellFormed(t *testing.T) {
	seen := map[InterfaceTypeID]bool{}
	for i, it := range interfaceTypes {
		if it.ID == "" || it.Name == "" || it.Is == "" || it.Why == "" || it.When == nil {
			t.Errorf("type %d is incomplete: %+v", i, it.ID)
		}
		if seen[it.ID] {
			t.Errorf("type %q is listed twice", it.ID)
		}
		seen[it.ID] = true
	}
	if n := len(interfaceTypes); n < 10 || n > 12 {
		t.Errorf("%d types, want 10 to 12", n)
	}
	// The last rule catches everything, so every set of facts has a type.
	if !interfaceTypes[len(interfaceTypes)-1].When(Facts{}) {
		t.Error("the last rule must match every set of facts")
	}
}

// Every one of the 1,076 programs lands in exactly one registered type, and
// the type does not depend on how many other rules would also match.
func TestEveryAnswerSetHasOneType(t *testing.T) {
	sets := loadAnswerSets(t)
	total := 0
	for _, s := range sets {
		total += s.count
		f := factsOf(t, s)
		p := PlanFor(f)
		if _, ok := LookupInterfaceType(p.Type); !ok {
			t.Errorf("%s %s: type %q is not registered", s.arch, s.bits, p.Type)
		}
		first := ""
		for _, it := range interfaceTypes {
			if it.When(f) {
				first = string(it.ID)
				break
			}
		}
		if first != string(p.Type) {
			t.Errorf("%s %s: plan says %s, the first matching rule is %s", s.arch, s.bits, p.Type, first)
		}
		if p.Why == "" {
			t.Errorf("%s %s: no reason", s.arch, s.bits)
		}
	}
	if total != 1076 {
		t.Errorf("%d programs in the data, want 1076", total)
	}
}

func TestNoInterfaceTypeIsEmpty(t *testing.T) {
	counts := map[InterfaceTypeID]int{}
	for _, s := range loadAnswerSets(t) {
		counts[PlanFor(factsOf(t, s)).Type] += s.count
	}
	for _, it := range interfaceTypes {
		if counts[it.ID] == 0 {
			t.Errorf("no program lands in %s", it.ID)
		}
	}
}

// TestInterfaceTypeGolden keeps the spread of programs over the types, and
// each archetype's types, in a file that shows up in review when a rule
// moves programs. Run with MINT_UPDATE_GOLDEN=1 to rewrite it.
func TestInterfaceTypeGolden(t *testing.T) {
	counts := map[InterfaceTypeID]int{}
	byArch := map[ArchetypeID]map[InterfaceTypeID]int{}
	total := 0
	for _, s := range loadAnswerSets(t) {
		ty := PlanFor(factsOf(t, s)).Type
		counts[ty] += s.count
		if byArch[s.arch] == nil {
			byArch[s.arch] = map[InterfaceTypeID]int{}
		}
		byArch[s.arch][ty] += s.count
		total += s.count
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Programs by interface type (%d programs)\n", total)
	for _, it := range interfaceTypes {
		fmt.Fprintf(&b, "%-16s %4d  %2d%%\n", it.ID, counts[it.ID], 100*counts[it.ID]/total)
	}
	b.WriteString("\n# Archetype: types its programs land in\n")
	var archs []string
	for a := range byArch {
		archs = append(archs, string(a))
	}
	sort.Strings(archs)
	for _, a := range archs {
		m := byArch[ArchetypeID(a)]
		var ids []string
		for id := range m {
			ids = append(ids, string(id))
		}
		sort.Slice(ids, func(i, j int) bool {
			if m[InterfaceTypeID(ids[i])] != m[InterfaceTypeID(ids[j])] {
				return m[InterfaceTypeID(ids[i])] > m[InterfaceTypeID(ids[j])]
			}
			return ids[i] < ids[j]
		})
		var parts []string
		for _, id := range ids {
			parts = append(parts, fmt.Sprintf("%s %d", id, m[InterfaceTypeID(id)]))
		}
		fmt.Fprintf(&b, "%-11s %s\n", a, strings.Join(parts, ", "))
	}
	const path = "testdata/interface_types.golden"
	if os.Getenv("MINT_UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with MINT_UPDATE_GOLDEN=1 to create it)", err)
	}
	if string(want) != b.String() {
		t.Errorf("interface types moved; review and run with MINT_UPDATE_GOLDEN=1:\n%s", b.String())
	}
}

func TestPlanExtrasWrapAnyType(t *testing.T) {
	p := PlanFor(Facts{TakesTarget: true, WritesFile: true, NeedsRoot: true, Destructive: true, Hints: []Feature{FeatHistory, FeatTable}})
	if p.Type != TypeQuickAnswer {
		t.Fatalf("type %s", p.Type)
	}
	if got := p.Signature(); got != "quick-answer+file-output,root,confirm,history" {
		t.Errorf("signature %q", got)
	}
	// Extras never change the type.
	q := PlanFor(Facts{TakesTarget: true})
	if q.Type != p.Type {
		t.Errorf("extras changed the type: %s vs %s", q.Type, p.Type)
	}
}

func TestPlanRuleOrder(t *testing.T) {
	cases := []struct {
		name string
		f    Facts
		want InterfaceTypeID
	}{
		{"full-screen beats everything", Facts{FullScreen: true, KeepsRunning: true, Subcommands: true}, TypeLauncher},
		{"keeps running and changes things", Facts{KeepsRunning: true, ChangesState: true, Subcommands: true}, TypeServiceControl},
		{"keeps running with rows", Facts{KeepsRunning: true, TableOutput: true}, TypeLiveMonitor},
		{"keeps running alone", Facts{KeepsRunning: true, TakesTarget: true}, TypeLogFollower},
		{"commands beat two inputs", Facts{Subcommands: true, TwoInputs: true}, TypeCommandSuite},
		{"two inputs", Facts{TwoInputs: true, TakesTarget: true}, TypeCompare},
		{"state editor has no target", Facts{ChangesState: true}, TypeStateEditor},
		{"action has a target", Facts{ChangesState: true, TakesTarget: true}, TypeAction},
		{"table", Facts{TableOutput: true, TakesTarget: true}, TypeResultTable},
		{"report from a firm archetype", Facts{TakesTarget: true, Hints: []Feature{FeatCollapsible}, Weights: map[Feature]float64{FeatCollapsible: 0.9}}, TypeReport},
		{"a mild preference is not a report", Facts{TakesTarget: true, Hints: []Feature{FeatCollapsible}, Weights: map[Feature]float64{FeatCollapsible: 0.6}}, TypeQuickAnswer},
		{"session from the archetype", Facts{KeepsRunning: true, Hints: []Feature{FeatTranscript}}, TypeSession},
		{"service from the archetype", Facts{KeepsRunning: true, Hints: []Feature{FeatService}}, TypeServiceControl},
		{"a hint alone does not make it run", Facts{Hints: []Feature{FeatService}}, TypeQuickAnswer},
		{"default", Facts{}, TypeQuickAnswer},
	}
	for _, c := range cases {
		if got := PlanFor(c.f).Type; got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFactsFromSpecAndAppPlan(t *testing.T) {
	spec := ToolSpec{Name: "top", Bin: "echo", ArchetypeID: "watch", PollInterval: 2000000000, TableOutput: true}
	f := FactsFromSpec(spec)
	if !f.KeepsRunning || !f.TableOutput {
		t.Fatalf("facts %+v", f)
	}
	if got := newTestApp(t, spec).InterfacePlan().Type; got != TypeLiveMonitor {
		t.Errorf("watch app is %s", got)
	}
	if got := newTestApp(t, ToolSpec{Name: "calc", Bin: "echo", ArchetypeID: "ask"}).InterfacePlan().Type; got != TypeQuickAnswer {
		t.Errorf("ask app is %s", got)
	}
	if got := newTestApp(t, ToolSpec{Name: "htop", Bin: "echo", FullScreen: true}).InterfacePlan().Type; got != TypeLauncher {
		t.Errorf("full-screen app is %s", got)
	}
	if got := newTestApp(t, ToolSpec{Name: "git", Bin: "echo", Subcommands: []SubcommandSpec{{Name: "status"}}}).InterfacePlan().Type; got != TypeCommandSuite {
		t.Errorf("app with commands is %s", got)
	}
}

func TestReadOnlyStatusArgsDoNotChangeState(t *testing.T) {
	mon := ToolSpec{Name: "free", Bin: "echo", ArchetypeID: "watch", ReadOnly: true, StatusArgs: []string{"-m"}, PollInterval: 2000000000, TableOutput: true}
	if f := FactsFromSpec(mon); f.ChangesState {
		t.Fatal("a read-only monitor was taken to change things")
	}
	if got := PlanFor(FactsFromSpec(mon)).Type; got != TypeLiveMonitor {
		t.Errorf("read-only monitor is %s", got)
	}
	ctl := ToolSpec{Name: "svc", Bin: "echo", ArchetypeID: "control", StatusArgs: []string{"status"}, PollInterval: 2000000000}
	if f := FactsFromSpec(ctl); !f.ChangesState {
		t.Fatal("a controller with status arguments does not change things?")
	}
}

// Choosing a type must not change what MainScreen draws yet.
func TestPlanDoesNotChangeTheScreen(t *testing.T) {
	for _, d := range AllArchetypeDefs() {
		a := newTestApp(t, ToolSpec{Name: string(d.ID), Bin: "echo", ArchetypeID: string(d.ID)})
		before := a.MainScreen().FrameText(30, 100)
		_ = a.InterfacePlan()
		if after := a.MainScreen().FrameText(30, 100); after != before {
			t.Errorf("%s: screen changed after asking for the plan", d.ID)
		}
	}
}
