package main

// The interface benchmark: the screen each way of deciding it gives, side
// by side. It runs only when asked:
//
//	MINT_ARCHETYPE_BENCH=interface MINT_INTERFACE_PROGRAMS=/abs/names.txt \
//	MINT_INTERFACE_TRUTH=/abs/truth.json go test -run TestInterfaceBenchmark -v .
//
// MINT_INTERFACE_PROGRAMS lists programs to try (one name per line), on top
// of the benchmark's own. MINT_INTERFACE_TRUTH holds checked answers,
// {"name": {"keeps_running": true, …}}, from the review page. Both live
// outside the repository; help is read and cached as for the archetype
// benchmark, and none of it is committed.
//
// Without checked answers it still compares the two ways on every program
// and checks that answers mode never lowers a data level, drops a secret
// option or removes caution.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// screenQuestions decide the screen's shape; the rest are extras.
var screenQuestions = []appkit.Question{appkit.QKeepsRunning, appkit.QChangesThings, appkit.QTarget, appkit.QSubcommands, appkit.QFullScreen}

// screenOf reads back what a built spec's screen does, as answers.
func screenOf(spec appkit.ToolSpec) map[appkit.Question]bool {
	var b appkit.Blueprint
	if spec.Features != nil {
		b = appkit.Blueprint{Features: spec.Features}
	} else if def, ok := appkit.LookupArchetype(spec.ArchetypeID); ok {
		b = def.Blueprint()
	}
	return map[appkit.Question]bool{
		appkit.QKeepsRunning:  b.Has(appkit.FeatLiveRefresh) || b.Has(appkit.FeatProgress) || b.Has(appkit.FeatFollow) || b.Has(appkit.FeatService) || spec.Follow != nil || spec.Service != nil,
		appkit.QChangesThings: b.Has(appkit.FeatStateEdit) || spec.ConfirmRun || spec.Irreversible,
		appkit.QTarget:        spec.WantTarget,
		appkit.QSubcommands:   len(spec.Subcommands) > 0,
		appkit.QFullScreen:    spec.FullScreen,
		appkit.QWritesFile:    spec.OutputsFile,
		appkit.QTable:         spec.TableOutput,
		appkit.QNeedsRoot:     spec.NeedsRoot,
	}
}

// truthScreen is what checked answers say the screen should do. A program
// that can destroy things should ask before running, like one that
// changes things.
func truthScreen(t map[appkit.Question]bool) map[appkit.Question]bool {
	out := map[appkit.Question]bool{}
	for q, v := range t {
		out[q] = v
	}
	out[appkit.QChangesThings] = t[appkit.QChangesThings] || t[appkit.QCanDestroy]
	return out
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if s := strings.TrimSpace(sc.Text()); s != "" && !strings.HasPrefix(s, "#") {
			out = append(out, strings.Fields(s)[0])
		}
	}
	return out
}

func TestInterfaceBenchmark(t *testing.T) {
	if os.Getenv("MINT_ARCHETYPE_BENCH") != "interface" {
		t.Skip("set MINT_ARCHETYPE_BENCH=interface to compare the two ways of deciding a screen")
	}
	for _, env := range []string{"MINT_INTERFACE_PROGRAMS", "MINT_INTERFACE_TRUTH"} {
		if p := os.Getenv(env); p != "" && !strings.HasPrefix(p, "/") {
			t.Fatalf("%s must be an absolute path outside the repository", env)
		}
	}
	truth := map[string]map[appkit.Question]bool{}
	if p := os.Getenv("MINT_INTERFACE_TRUTH"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var raw map[string]map[string]bool
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatal(err)
		}
		for n, m := range raw {
			truth[n] = map[appkit.Question]bool{}
			for q, v := range m {
				truth[n][appkit.Question(q)] = v
			}
		}
	}
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, c := range loadArchCases(t) {
		add(c.Name)
	}
	for _, n := range readLines(os.Getenv("MINT_INTERFACE_PROGRAMS")) {
		add(n)
	}
	for n := range truth {
		add(n)
	}
	sort.Strings(names)
	old := interfaceMode
	defer func() { interfaceMode = old }()

	type row struct {
		name     string
		before   appkit.ToolSpec
		after    appkit.ToolSpec
		detected appkit.Answers
	}
	var rows []row
	unsafe := 0
	for _, n := range names {
		man, help, ok := benchHelp(t, benchCache(), n)
		if !ok || strings.TrimSpace(man+help) == "" {
			continue
		}
		interfaceMode = InterfaceArchetype
		before := ParseManAndHelp(n, man, help)
		interfaceMode = InterfaceAnswers
		after := ParseManAndHelp(n, man, help)
		if after.InterfaceGroup == "" {
			continue // listed: both ways give the checked setup
		}
		// Safety: answers mode never lowers a data level, drops a secret
		// option or removes caution.
		secrets := func(s appkit.ToolSpec) (k int) {
			for _, f := range s.Flags {
				if f.Kind == appkit.FlagSecret {
					k++
				}
			}
			return
		}
		switch {
		case dataRank(after.Data) < dataRank(before.Data):
			t.Errorf("%s: data level %q lowered to %q", n, before.Data, after.Data)
			unsafe++
		case secrets(after) < secrets(before):
			t.Errorf("%s: secret options %d -> %d", n, secrets(before), secrets(after))
			unsafe++
		case before.NeedsRoot && !after.NeedsRoot, before.ConfirmRun && !after.ConfirmRun, before.Irreversible && !after.Irreversible:
			t.Errorf("%s: caution removed", n)
			unsafe++
		case after.ReadOnly && !before.ReadOnly:
			t.Errorf("%s: answers mode made it read-only", n)
			unsafe++
		}
		rows = append(rows, row{n, before, after, detectAnswers(n, man, help, after)})
	}
	t.Logf("%d unlisted programs built both ways; safety problems: %d", len(rows), unsafe)

	// How often the two ways differ, per screen question.
	diff := map[appkit.Question]int{}
	anyDiff := 0
	groups := map[string]int{}
	for _, r := range rows {
		a, b := screenOf(r.before), screenOf(r.after)
		d := false
		for _, q := range screenQuestions {
			if a[q] != b[q] {
				diff[q]++
				d = true
			}
		}
		if d {
			anyDiff++
		}
		groups[r.after.InterfaceGroup]++
	}
	t.Logf("screens that differ between the two ways: %d of %d", anyDiff, len(rows))
	for _, q := range screenQuestions {
		t.Logf("  %-14s differs for %d", q, diff[q])
	}

	// Against checked answers.
	var checked []row
	for _, r := range rows {
		if _, ok := truth[r.name]; ok {
			checked = append(checked, r)
		}
	}
	if len(checked) == 0 {
		t.Logf("no checked answers yet (MINT_INTERFACE_TRUTH): nothing to score")
		return
	}
	score := func(get func(row) appkit.ToolSpec) (screenRight int, perQ map[appkit.Question]int) {
		perQ = map[appkit.Question]int{}
		for _, r := range checked {
			got, want := screenOf(get(r)), truthScreen(truth[r.name])
			all := true
			for _, q := range append(append([]appkit.Question{}, screenQuestions...), appkit.QWritesFile, appkit.QTable, appkit.QNeedsRoot) {
				if got[q] == want[q] {
					perQ[q]++
				} else if containsQ(screenQuestions, q) {
					all = false
				}
			}
			if all {
				screenRight++
			}
		}
		return
	}
	oldRight, oldQ := score(func(r row) appkit.ToolSpec { return r.before })
	newRight, newQ := score(func(r row) appkit.ToolSpec { return r.after })
	groupRight := 0
	for _, r := range checked {
		want := appkit.PlaceInterface(answersOf(truth[r.name]))
		if want.ID == r.after.InterfaceGroup {
			groupRight++
		}
	}
	n := len(checked)
	pc := func(k int) string { return fmt.Sprintf("%3d (%3.0f%%)", k, 100*float64(k)/float64(n)) }
	t.Logf("checked programs: %d", n)
	t.Logf("%-16s %-14s %-14s", "", "archetype", "answers")
	t.Logf("%-16s %-14s %-14s", "screen right", pc(oldRight), pc(newRight))
	for _, q := range append(append([]appkit.Question{}, screenQuestions...), appkit.QWritesFile, appkit.QTable, appkit.QNeedsRoot) {
		t.Logf("  %-14s %-14s %-14s", q, pc(oldQ[q]), pc(newQ[q]))
	}
	t.Logf("group right (answers vs checked answers): %s", pc(groupRight))
}

func containsQ(qs []appkit.Question, q appkit.Question) bool {
	for _, x := range qs {
		if x == q {
			return true
		}
	}
	return false
}

func answersOf(m map[appkit.Question]bool) appkit.Answers {
	a := appkit.Answers{}
	for _, q := range appkit.Questions {
		a[q] = appkit.Answer{Yes: m[q], Sure: appkit.SureStrong}
	}
	return a
}
