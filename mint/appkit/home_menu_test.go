package appkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

func TestHomeMenuLabels(t *testing.T) {
	subs := []SubcommandSpec{
		{Name: "install", Label: "Install", Desc: "Install a package"},
		{Name: "remove", Label: "Remove", Desc: "Remove a package"},
		{Name: "update", Label: "Update", Desc: "Update the lists"},
		{Name: "upgrade", Label: "Upgrade", Desc: "Upgrade packages"},
		{Name: "search", Label: "Search", Desc: "Search packages"},
		{Name: "show", Label: "Show", Desc: "Show a package"},
		{Name: "purge", Label: "Purge", Desc: "Purge a package"},
	}
	findings := &FindingsSpec{Adapter: FindingsLines}
	diffSpec := &DiffSpec{Format: DiffUnified}
	cases := []struct {
		name string
		spec ToolSpec
		want []string
	}{
		{
			name: "ps/list",
			spec: ToolSpec{Name: "ps", Bin: "ps", ArchetypeID: "list"},
			want: []string{"List", "Status", "Presets", "Help", "Live Run & Output", "Run History"},
		},
		{
			name: "systemctl/control",
			spec: ToolSpec{
				Name: "systemctl", Bin: "systemctl", ArchetypeID: "control",
				RowActions: []RowActionSpec{{Label: "Restart", Subcommand: []string{"restart"}, Confirm: true}},
				StatusArgs: []string{"list-units"},
			},
			want: []string{"Control", "Actions", "Presets", "Help", "Live Run & Output", "Run History"},
		},
		{
			name: "apt/records",
			spec: ToolSpec{Name: "apt", Bin: "apt", ArchetypeID: "records", Subcommands: subs},
			want: []string{"Records", "Install", "Remove", "Update", "Upgrade", "Search", "Show", "Presets", "Help", "Live Run & Output", "Run History"},
		},
		{
			name: "nmap/probe",
			spec: ToolSpec{Name: "nmap", Bin: "nmap", ArchetypeID: "probe"},
			want: []string{"Probe", "Presets", "Help", "Live Run & Output", "Output Directory", "Run History"},
		},
		{
			name: "audit",
			spec: ToolSpec{Name: "shellcheck", Bin: "shellcheck", ArchetypeID: "audit", Findings: findings},
			want: []string{"Audit", "Presets", "Help", "Findings", "Live Run & Output", "Output Directory", "Run History"},
		},
		{
			name: "compare",
			spec: ToolSpec{Name: "cmp", Bin: "diff", ArchetypeID: "compare", Diff: diffSpec},
			want: []string{"Compare", "Presets", "Help", "Compare", "Live Run & Output", "Run History"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t, tc.spec)
			got := homeLabels(app.BuildApp())
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("labels\n got %v\nwant %v", got, tc.want)
			}
		})
	}

	apt := newTestApp(t, ToolSpec{Name: "apt", Bin: "apt", ArchetypeID: "records", Subcommands: subs})
	items := apt.BuildApp().Items
	for _, it := range items {
		if it.Label == "Purge" {
			t.Fatal("the 7th subcommand became a home item")
		}
	}
	var opened bool
	for _, it := range items {
		if it.Label == "Install" && it.Open != nil {
			scr := it.Open()
			opened = scr != nil && apt.ActiveSubcommand == "install"
		}
	}
	if !opened {
		t.Fatal("Install did not open the main screen on that subcommand")
	}
}

func TestPresetsSaveLoadDeleteSkipsSecrets(t *testing.T) {
	a := newTestApp(t, ToolSpec{
		Name: "box", Bin: "echo", ArchetypeID: "list",
		Flags: []FlagSpec{
			{Flag: "-n", Kind: FlagText, Label: "Count"},
			{Flag: "--password", Kind: FlagSecret, Label: "Password"},
		},
	})
	a.Values["-n"] = "4"
	a.Values["--password"] = "s3cr3t"
	a.Target = "unit"
	a.ExtraArgs = "--password=s3cr3t"
	if !a.SavePreset("wide") {
		t.Fatal("save failed")
	}
	raw, err := os.ReadFile(a.presetsPath())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(a.presetsPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	if strings.Contains(string(raw), "s3cr3t") {
		t.Fatalf("secret stored:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"wide"`) || !strings.Contains(string(raw), `"4"`) {
		t.Fatalf("preset missing values:\n%s", raw)
	}
	hist := a.historyPath()
	if filepath.Dir(a.presetsPath()) != filepath.Dir(hist) {
		t.Fatalf("preset dir %s history dir %s", a.presetsPath(), hist)
	}

	var saved []savedPreset
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].Values["--password"] != "" || saved[0].Values["-n"] != "4" {
		t.Fatalf("saved values %+v", saved)
	}
	a.Values["-n"] = "1"
	if !a.ApplyPreset("wide") || a.Values["-n"] != "4" {
		t.Fatalf("apply: %#v", a.Values)
	}

	b := newTestApp(t, ToolSpec{Name: "box", Bin: "echo", ArchetypeID: "list"})
	b.DataDir = a.DataDir
	b.loadPresets()
	if got := b.Presets(); len(got) != 1 || got[0].Name != "wide" || got[0].Values["-n"] != "4" {
		t.Fatalf("loaded %+v", got)
	}
	if !a.DeletePreset("wide") || len(a.Presets()) != 0 {
		t.Fatalf("after delete %+v", a.Presets())
	}
	left, _ := os.ReadFile(a.presetsPath())
	if strings.Contains(string(left), "wide") {
		t.Fatalf("deleted preset still on disk:\n%s", left)
	}
}

func TestActionsUseConfirm(t *testing.T) {
	bin, log := fakeTool(t, "ctl", "")
	a := newTestApp(t, ToolSpec{
		Name: "ctl", Bin: bin, ArchetypeID: "control",
		RowActions: []RowActionSpec{{
			Label: "Restart", Hint: "restart", Subcommand: []string{"restart"}, Confirm: true, Column: 0,
		}},
	})
	asked := 0
	prev := confirmAction
	t.Cleanup(func() { confirmAction = prev })
	confirmAction = func(c *mint.Ctx, title, body string) bool {
		asked++
		if !strings.Contains(title, "Restart") || !strings.Contains(body, "nginx.service") {
			t.Fatalf("confirm %q %q", title, body)
		}
		return false
	}
	a.invokeRowAction(&mint.Ctx{}, a.Spec.RowActions[0], "nginx.service", "nginx.service", false)
	if asked != 1 {
		t.Fatalf("confirm calls %d", asked)
	}
	if runs(log) != "" {
		t.Fatalf("ran despite cancelling confirm:\n%s", runs(log))
	}

	confirmAction = func(c *mint.Ctx, title, body string) bool { return true }
	a.invokeRowAction(&mint.Ctx{}, a.Spec.RowActions[0], "nginx.service", "nginx.service", false)
	if !strings.Contains(runs(log), "restart nginx.service") {
		t.Fatalf("command: %s", runs(log))
	}

	// Irreversible asks even when the action itself does not.
	a.Spec.Irreversible = true
	a.Spec.RowActions[0].Confirm = false
	asked = 0
	confirmAction = func(c *mint.Ctx, title, body string) bool {
		asked++
		return false
	}
	a.invokeRowAction(&mint.Ctx{}, a.Spec.RowActions[0], "nginx.service", "nginx.service", false)
	if asked != 1 {
		t.Fatal("irreversible action skipped confirm")
	}

	labels := strings.Join(screenLabels(a.ActionsScreen()), "\n")
	if !strings.Contains(labels, "Restart") {
		t.Fatalf("actions screen: %s", labels)
	}
}

func TestHelpFallsBackToShortFlag(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "zzminthelpfix")
	script := "#!/bin/sh\ncase \"$1\" in\n--help) exit 0 ;;\n-h) echo 'fallback help text'; exit 0 ;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	text, how := captureToolHelp(bin)
	if how != "-h" || !strings.Contains(text, "fallback help text") {
		t.Fatalf("how %q text %q", how, text)
	}
	a := newTestApp(t, ToolSpec{Name: "zzminthelpfix", Bin: bin, ArchetypeID: "list"})
	scr := a.HelpScreen()
	if scr == nil || !strings.Contains(scr.Title, "-h") {
		t.Fatalf("help screen title %q", scr.Title)
	}
	if scr.Title == "" {
		t.Fatal("empty help screen")
	}
}

func TestSplitScreensForArchetypes(t *testing.T) {
	list := newTestApp(t, ToolSpec{Name: "ps", Bin: "ps", ArchetypeID: "list"})
	scr := list.MainScreen()
	if len(scr.Panes) != 2 || scr.Panes[0].Title != "Details" || scr.Panes[1].Title != "Actions" {
		t.Fatalf("list panes %+v", paneTitles(scr))
	}
	for _, id := range []string{"records", "search"} {
		s := newTestApp(t, ToolSpec{Name: id, Bin: "echo", ArchetypeID: id}).MainScreen()
		if len(s.Panes) != 2 || s.Panes[0].Title != "Details" || s.Panes[1].Title != "Actions" {
			t.Fatalf("%s panes %v", id, paneTitles(s))
		}
	}
	cmp := newTestApp(t, ToolSpec{Name: "cmp", Bin: "echo", ArchetypeID: "compare"})
	cs := cmp.MainScreen()
	if len(cs.Panes) != 1 || cs.Panes[0].Title != "Differences" {
		t.Fatalf("compare panes %v", paneTitles(cs))
	}
	if !hasSection(cs, "Inputs") || !hasSection(cs, "Differences") {
		t.Fatalf("compare sections %v", sectionNames(cs))
	}
	run := list.RunScreen()
	if len(run.Panes) != 2 || run.Panes[0].Title != "Live output" || run.Panes[1].Title != "Recent runs" {
		t.Fatalf("run panes %v", paneTitles(run))
	}
	plain := newTestApp(t, ToolSpec{Name: "x", Bin: "echo"}).MainScreen()
	if len(plain.Panes) != 0 {
		t.Fatalf("plain screen gained panes %v", paneTitles(plain))
	}
}

func paneTitles(s *mint.Screen) []string {
	var out []string
	for _, p := range s.Panes {
		if p != nil {
			out = append(out, p.Title)
		}
	}
	return out
}
