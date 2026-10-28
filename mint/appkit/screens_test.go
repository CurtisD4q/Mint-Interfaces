package appkit

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

func rowField(r mint.Row, name string) string {
	v := reflect.ValueOf(r).FieldByName(name)
	if !v.IsValid() || v.Kind() != reflect.String {
		return ""
	}
	return v.String()
}

func screenRowIDs(scr *mint.Screen) []string {
	var out []string
	for _, s := range scr.Sections {
		for _, r := range s.Rows {
			if id := rowField(r, "id"); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

func screenLabels(scr *mint.Screen) []string {
	var out []string
	for _, s := range scr.Sections {
		for _, r := range s.Rows {
			if l := rowField(r, "label"); l != "" {
				out = append(out, l)
			}
		}
	}
	return out
}

func hasSection(scr *mint.Screen, name string) bool {
	for _, s := range scr.Sections {
		if s.Name == name {
			return true
		}
	}
	return false
}

func TestLayoutAliasesAndJSON(t *testing.T) {
	if LayoutRunner != ArchetypeRunner || LayoutScanner != ArchetypeScanner {
		t.Fatal("layout aliases drifted")
	}
	var fromOld ToolSpec
	if err := json.Unmarshal([]byte(`{"name":"t","archetype":"scanner"}`), &fromOld); err != nil || fromOld.Archetype != LayoutScanner {
		t.Fatalf("archetype key: %v %+v", err, fromOld)
	}
	var fromNew ToolSpec
	if err := json.Unmarshal([]byte(`{"name":"t","layout":"explorer"}`), &fromNew); err != nil || fromNew.Archetype != LayoutExplorer {
		t.Fatalf("layout key: %v %+v", err, fromNew)
	}
	var both ToolSpec
	if err := json.Unmarshal([]byte(`{"name":"t","archetype":"runner","layout":"suite"}`), &both); err != nil || both.Archetype != LayoutRunner {
		t.Fatalf("archetype should win: %v %+v", err, both)
	}
}

func TestEveryArchetypeHasLabels(t *testing.T) {
	if _, ok := ArchetypeLabelsFor(""); ok {
		t.Fatal("empty id has labels")
	}
	for _, d := range AllArchetypeDefs() {
		l, ok := ArchetypeLabelsFor(string(d.ID))
		if !ok || l.Verb == "" || l.Input == "" || l.Output == "" || l.Title == "" || l.Home == "" {
			t.Errorf("%s: %+v ok=%v", d.ID, l, ok)
		}
	}
}

func TestFeatTargetHidesTheBox(t *testing.T) {
	for _, id := range []string{"list", "watch", "generate", "control"} {
		app := newTestApp(t, ToolSpec{Name: "t-" + id, Bin: "echo", ArchetypeID: id})
		if !app.Spec.HideTarget || app.Spec.WantTarget || app.Spec.TwoInputs {
			t.Fatalf("%s: hide=%v want=%v two=%v", id, app.Spec.HideTarget, app.Spec.WantTarget, app.Spec.TwoInputs)
		}
		ids := screenRowIDs(app.MainScreen())
		for _, idRow := range ids {
			if idRow == "target" || strings.HasSuffix(idRow, ":target") || strings.HasSuffix(idRow, "target2") {
				t.Fatalf("%s: target row %q in %v", id, idRow, ids)
			}
		}
	}
	// A label the caller set is kept, and the box stays.
	shown := newTestApp(t, ToolSpec{Name: "ls", Bin: "echo", ArchetypeID: "list", TargetLabel: "Path"})
	if shown.Spec.HideTarget || shown.Spec.TargetLabel != "Path" {
		t.Fatalf("explicit label: hide=%v label=%q", shown.Spec.HideTarget, shown.Spec.TargetLabel)
	}
	if !strings.Contains(strings.Join(screenLabels(shown.MainScreen()), "\n"), "Path") {
		t.Fatal("explicit target label was not drawn")
	}
}

func TestFeatTwoInputs(t *testing.T) {
	cases := []struct {
		id, left, right string
	}{
		{"compare", "Left", "Right"},
		{"mirror", "Source", "Destination"},
		{"transfer", "URL or remote", "Destination"},
	}
	for _, tc := range cases {
		app := newTestApp(t, ToolSpec{Name: "t-" + tc.id, Bin: "echo", ArchetypeID: tc.id})
		if !app.Spec.TwoInputs || app.Spec.HideTarget || !app.Spec.WantTarget {
			t.Fatalf("%s: two=%v hide=%v want=%v", tc.id, app.Spec.TwoInputs, app.Spec.HideTarget, app.Spec.WantTarget)
		}
		if app.Spec.TargetLabel != tc.left || app.Spec.SecondTargetLabel != tc.right {
			t.Fatalf("%s: labels %q / %q", tc.id, app.Spec.TargetLabel, app.Spec.SecondTargetLabel)
		}
		labels := strings.Join(screenLabels(app.MainScreen()), "\n")
		if !strings.Contains(labels, tc.left) || !strings.Contains(labels, tc.right) {
			t.Fatalf("%s: boxes missing from %q", tc.id, labels)
		}
		ids := screenRowIDs(app.MainScreen())
		if !containsAll(ids, "target", "target2") {
			t.Fatalf("%s: ids %v", tc.id, ids)
		}
		app.Target = "left-in"
		app.SecondTarget = "right-in"
		args, _ := app.BuildArgs()
		got := strings.Join(args, " ")
		if !strings.Contains(got, "left-in") || !strings.Contains(got, "right-in") {
			t.Fatalf("%s: args %q", tc.id, got)
		}
	}
}

func TestArchetypeLabelsReplaceLayoutLabels(t *testing.T) {
	compare := newTestApp(t, ToolSpec{Name: "cmp", Bin: "echo", ArchetypeID: "compare"})
	scr := compare.MainScreen()
	if scr.Title != "cmp — Compare" {
		t.Fatalf("compare title %q", scr.Title)
	}
	if !hasSection(scr, "Inputs") || !hasSection(scr, "Differences") {
		t.Fatalf("compare sections: %v", sectionNames(scr))
	}
	labels := strings.Join(screenLabels(scr), "\n")
	if strings.Contains(labels, "Run query") || !strings.Contains(labels, "Compare (cmp)") {
		t.Fatalf("compare button: %q", labels)
	}

	record := newTestApp(t, ToolSpec{Name: "cap", Bin: "echo", ArchetypeID: "record"})
	rec := record.MainScreen()
	if !hasSection(rec, "Capture Output") || hasSection(rec, "Discovered Targets & Ports") {
		t.Fatalf("record sections: %v", sectionNames(rec))
	}
	if rec.Title != "cap — Record" {
		t.Fatalf("record title %q", rec.Title)
	}

	device := newTestApp(t, ToolSpec{Name: "als", Bin: "echo", ArchetypeID: "device"})
	dev := device.MainScreen()
	devLabels := strings.Join(screenLabels(dev), "\n")
	if strings.Contains(devLabels, "Apply configuration") || !strings.Contains(devLabels, "Apply (als)") {
		t.Fatalf("device button: %q", devLabels)
	}
	if !hasSection(dev, "Device") || !hasSection(dev, "Current state") {
		t.Fatalf("device sections: %v", sectionNames(dev))
	}

	probe := newTestApp(t, ToolSpec{Name: "scan", Bin: "echo", ArchetypeID: "probe"})
	pr := probe.MainScreen()
	if !hasSection(pr, "Discovered Targets") || pr.Title != "scan — Probe" {
		t.Fatalf("probe: title %q sections %v", pr.Title, sectionNames(pr))
	}
	home := probe.BuildApp()
	if !homeHas(home, "Probe") || homeHas(home, "Target & Live Scanner") {
		t.Fatalf("probe home: %+v", homeLabels(home))
	}

	// No archetype id: the layout's own labels.
	plain := newTestApp(t, ToolSpec{Name: "x", Bin: "echo", Archetype: LayoutExplorer})
	plainScr := plain.MainScreen()
	if plainScr.Title != "x — Explorer" || !hasSection(plainScr, "Search & Query") || !hasSection(plainScr, "Query Results") {
		t.Fatalf("plain explorer: title %q sections %v", plainScr.Title, sectionNames(plainScr))
	}
	plainLabels := strings.Join(screenLabels(plainScr), "\n")
	if !strings.Contains(plainLabels, "Run query (x)") {
		t.Fatalf("plain explorer button: %q", plainLabels)
	}
	if !homeHas(plain.BuildApp(), "Search & Query Explorer") {
		t.Fatalf("plain home: %v", homeLabels(plain.BuildApp()))
	}
}

func TestRunnerFoldsScannerAndExplorer(t *testing.T) {
	scan := newTestApp(t, ToolSpec{Name: "s", Bin: "echo", Archetype: LayoutScanner})
	if scan.ScannerScreen().Title != scan.RunnerScreen().Title {
		t.Fatal("scanner screen diverged from runner")
	}
	if !hasSection(scan.MainScreen(), "Discovered Targets & Ports") {
		t.Fatal("scanner layout lost its results section")
	}
	ids := screenRowIDs(scan.MainScreen())
	for _, id := range ids {
		if id == "profile" {
			t.Fatal("preset picker without profiles")
		}
	}

	exp := newTestApp(t, ToolSpec{Name: "e", Bin: "echo", Archetype: LayoutExplorer})
	if !hasSection(exp.ExplorerScreen(), "Query Results") {
		t.Fatal("explorer screen lost its results section")
	}

	// FeatProfiles (probe) shows the picker; generate does not.
	if !containsAll(screenRowIDs(newTestApp(t, ToolSpec{Name: "p", Bin: "echo", ArchetypeID: "probe"}).MainScreen()), "profile") {
		t.Fatal("probe should offer presets")
	}
	gen := newTestApp(t, ToolSpec{Name: "g", Bin: "echo", ArchetypeID: "generate"})
	for _, id := range screenRowIDs(gen.MainScreen()) {
		if id == "profile" {
			t.Fatal("generate has no FeatProfiles, but a preset picker was drawn")
		}
	}
}

func TestSuiteWithoutSubcommandsUsesRunner(t *testing.T) {
	empty := newTestApp(t, ToolSpec{Name: "pk", Bin: "echo", ArchetypeID: "pack"})
	scr := empty.MainScreen()
	if hasSection(scr, "Subcommand Tabs (ArchetypeSuite)") {
		t.Fatal("empty suite still shows the layout's tab heading")
	}
	for _, id := range screenRowIDs(scr) {
		if id == "suite:subcommand" {
			t.Fatal("empty suite invented a subcommand picker")
		}
	}
	if scr.Title != "pk — Pack" {
		t.Fatalf("empty pack title %q", scr.Title)
	}

	with := newTestApp(t, ToolSpec{
		Name: "gitx", Bin: "echo", ArchetypeID: "toolkit",
		Subcommands: []SubcommandSpec{{Name: "status", Label: "Status", Desc: "show"}},
	})
	suite := with.MainScreen()
	if !hasSection(suite, "Toolkit") || hasSection(suite, "Subcommand Tabs (ArchetypeSuite)") {
		t.Fatalf("suite sections: %v", sectionNames(suite))
	}
	if !containsAll(screenRowIDs(suite), "suite:subcommand") {
		t.Fatal("suite with subcommands lost its picker")
	}
	if !strings.Contains(strings.Join(screenLabels(suite), "\n"), "Run (gitx)") {
		t.Fatalf("suite button: %q", screenLabels(suite))
	}

	// No archetype id keeps the layout heading.
	generic := newTestApp(t, ToolSpec{
		Name: "box", Bin: "echo", Archetype: LayoutSuite,
		Subcommands: []SubcommandSpec{{Name: "default", Label: "Default"}},
	})
	if !hasSection(generic.MainScreen(), "Subcommand Tabs (ArchetypeSuite)") {
		t.Fatalf("generic suite sections: %v", sectionNames(generic.MainScreen()))
	}
}

func sectionNames(scr *mint.Screen) []string {
	var out []string
	for _, s := range scr.Sections {
		out = append(out, s.Name)
	}
	return out
}

func containsAll(have []string, want ...string) bool {
	got := map[string]bool{}
	for _, h := range have {
		got[h] = true
	}
	for _, w := range want {
		if !got[w] {
			return false
		}
	}
	return true
}

func homeLabels(app *mint.App) []string {
	var out []string
	for _, it := range app.Items {
		out = append(out, it.Label)
	}
	return out
}

func homeHas(app *mint.App, label string) bool {
	for _, l := range homeLabels(app) {
		if l == label {
			return true
		}
	}
	return false
}
