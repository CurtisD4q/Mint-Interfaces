package appkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
			want: []string{"List", "Status", "Presets", "Manual", "Live Run & Output", "Run History"},
		},
		{
			name: "systemctl/control",
			spec: ToolSpec{
				Name: "systemctl", Bin: "systemctl", ArchetypeID: "control",
				RowActions: []RowActionSpec{{Label: "Restart", Subcommand: []string{"restart"}, Confirm: true}},
				StatusArgs: []string{"list-units"},
			},
			want: []string{"Control", "Actions", "Presets", "Manual", "Live Run & Output", "Run History"},
		},
		{
			name: "apt/records",
			spec: ToolSpec{Name: "apt", Bin: "apt", ArchetypeID: "records", Subcommands: subs},
			want: []string{"Records", "Install", "Remove", "Update", "Upgrade", "Search", "Show", "Presets", "Manual", "Live Run & Output", "Run History"},
		},
		{
			name: "nmap/probe",
			spec: ToolSpec{Name: "nmap", Bin: "nmap", ArchetypeID: "probe"},
			want: []string{"Probe", "Presets", "Manual", "Live Run & Output", "Output Directory", "Run History"},
		},
		{
			name: "audit",
			spec: ToolSpec{Name: "shellcheck", Bin: "shellcheck", ArchetypeID: "audit", Findings: findings},
			want: []string{"Audit", "Presets", "Manual", "Findings", "Live Run & Output", "Output Directory", "Run History"},
		},
		{
			name: "compare",
			spec: ToolSpec{Name: "cmp", Bin: "diff", ArchetypeID: "compare", Diff: diffSpec},
			want: []string{"Compare", "Presets", "Manual", "Compare", "Live Run & Output", "Run History"},
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
	dir := helpTestDir(t)
	writeExe(t, filepath.Join(dir, "man"), "#!/bin/sh\necho 'no manual entry'\n")
	prependPath(t, dir)
	bin := filepath.Join(dir, "zzminthelpfix")
	argsLog := filepath.Join(dir, "args")
	allowHelpUserToWrite(t, argsLog)
	script := "#!/bin/sh\necho \"$*\" >> " + argsLog + "\ncase \"$1\" in\n--help) exit 0 ;;\n-h) echo 'fallback help text'; exit 0 ;;\nesac\n"
	writeExe(t, bin, script)
	text, how := captureToolHelp(bin)
	if how != "-h" || !strings.Contains(text, "fallback help text") {
		t.Fatalf("how %q text %q", how, text)
	}
	logged, _ := os.ReadFile(argsLog)
	if strings.Contains(string(logged), "--man") {
		t.Fatalf("manual was fetched with --man:\n%s", logged)
	}
	if !strings.Contains(string(logged), "--help") || !strings.Contains(string(logged), "-h") {
		t.Fatalf("expected --help then -h, got:\n%s", logged)
	}
	a := newTestApp(t, ToolSpec{Name: "zzminthelpfix", Bin: bin, ArchetypeID: "list"})
	scr := a.ManualScreen()
	if scr == nil || !strings.Contains(scr.Title, "Manual") || strings.Contains(scr.Title, "--help") || strings.Contains(scr.Title, "-h") {
		t.Fatalf("manual screen title %q", scr.Title)
	}
	if got := viewerText(t, scr); !strings.Contains(got, "fallback help text") {
		t.Fatalf("viewer %q", got)
	}
}

func TestEveryToolHomeHasManual(t *testing.T) {
	for _, def := range AllArchetypeDefs() {
		app := newTestApp(t, ToolSpec{Name: string(def.ID), Bin: "echo", ArchetypeID: string(def.ID)}).BuildApp()
		var saw bool
		for _, it := range app.Items {
			if it.Label == "Help" {
				t.Fatalf("%s home still has Help: %v", def.ID, homeLabels(app))
			}
			if it.Label != "Manual" {
				continue
			}
			saw = true
			if it.Desc != "Read the full manual (--man)" || strings.Contains(it.Desc, "--help") {
				t.Fatalf("%s manual desc %q", def.ID, it.Desc)
			}
			if it.Open == nil {
				t.Fatalf("%s Manual does not open", def.ID)
			}
		}
		if !saw {
			t.Fatalf("%s home has no Manual: %v", def.ID, homeLabels(app))
		}
	}
}

func TestManualWidthFollowsTerminal(t *testing.T) {
	dir := helpTestDir(t)
	writeExe(t, filepath.Join(dir, "man"), "#!/bin/sh\nprintf '%s\\n' NAME '    wide - manual' \"WIDTH=$MANWIDTH\"\n")
	prependPath(t, dir)
	bin := filepath.Join(dir, "wide")
	writeExe(t, bin, "#!/bin/sh\necho 'not the manual'\n")
	text, how := captureToolHelpArgs(bin, nil, 64)
	if how != "man" || !strings.Contains(text, "WIDTH=64") {
		t.Fatalf("how %q text %q", how, text)
	}
	if strings.Contains(text, "WIDTH=100") {
		t.Fatal("MANWIDTH is still the old hardcoded 100")
	}
}

func TestManualOpensViewLogWithFullText(t *testing.T) {
	dir := helpTestDir(t)
	t.Setenv("SECRET", "hunter2")
	writeExe(t, filepath.Join(dir, "man"), `#!/bin/sh
printf '%s\n' 'NAME' '    longman - a long manual' "WIDTH=$MANWIDTH" "SECRET=${SECRET-unset}" "TTY=$(test -t 0 && echo yes || echo no)"
st=$(dd bs=1 count=1 2>/dev/null | wc -c | tr -d ' ')
printf '%s\n' "STDIN=$st"
dd if=/dev/zero bs=1024 count=2500 2>/dev/null | tr '\0' 'x' | fold -w 80
printf '%s\n' 'ENDMARKER-PR85'
`)
	prependPath(t, dir)
	bin := filepath.Join(dir, "longman")
	writeExe(t, bin, "#!/bin/sh\necho 'short help that must not replace the manual'\n")
	a := newTestApp(t, ToolSpec{Name: "longman", Bin: bin, ArchetypeID: "list"})
	scr := a.ManualScreen()
	if _, ok := scr.ViewerLines(); !ok {
		t.Fatal("Manual did not open ViewLog")
	}
	if strings.Contains(scr.Title, "--help") || !strings.Contains(scr.Title, "Manual") {
		t.Fatalf("title %q", scr.Title)
	}
	text := viewerText(t, scr)
	if !strings.Contains(text, "ENDMARKER-PR85") {
		t.Fatalf("manual truncated, len %d, tail %q", len(text), textTail(text))
	}
	if len(text) <= 2<<20 {
		t.Fatalf("manual len %d is still within the old 2MB cap", len(text))
	}
	_, cols := mint.TermSize()
	if !strings.Contains(text, "WIDTH="+strconv.Itoa(cols)) {
		t.Fatalf("MANWIDTH is not the terminal width %d:\n%s", cols, textHead(text))
	}
	if strings.Contains(text, "hunter2") || !strings.Contains(text, "SECRET=unset") {
		t.Fatalf("environment leaked into the manual:\n%s", textHead(text))
	}
	if !strings.Contains(text, "TTY=no") || !strings.Contains(text, "STDIN=0") {
		t.Fatalf("manual capture is not sandboxed:\n%s", textHead(text))
	}
	if strings.Contains(text, "short help") {
		t.Fatalf("fell through to --help:\n%s", textHead(text))
	}
}

func TestToolkitManualUsesSelectedSubcommand(t *testing.T) {
	dir := helpTestDir(t)
	log := filepath.Join(dir, "calls")
	allowHelpUserToWrite(t, log)
	writeExe(t, filepath.Join(dir, "man"), `#!/bin/sh
printf '%s\n' "$*" >> `+fmt.Sprintf("%q", log)+`
page=
for a in "$@"; do page=$a; done
case "$page" in
  docker-run) printf '%s\n' NAME '    docker-run - run a container' SUB-MANUAL ;;
  docker) printf '%s\n' NAME '    docker - the tool' TOOL-MANUAL ;;
  *) printf '%s\n' "no manual for $page" ;;
esac
`)
	writeExe(t, filepath.Join(dir, "docker"), `#!/bin/sh
printf '%s\n' "$*" >> `+fmt.Sprintf("%q", log)+`
case "$1:$2" in
  run:--help) echo 'run long help' ;;
  run:-h) echo 'run short help' ;;
  --help) echo 'tool long help' ;;
  -h) echo 'tool short help' ;;
  *) echo "unexpected $*" ;;
esac
`)
	prependPath(t, dir)
	bin := filepath.Join(dir, "docker")
	spec := ToolSpec{
		Name: "docker", Bin: bin, ArchetypeID: "toolkit",
		Subcommands: []SubcommandSpec{{Name: "run", Label: "Run", Desc: "Run a container"}},
	}
	a := newTestApp(t, spec)
	if a.ActiveSubcommand != "run" {
		t.Fatalf("toolkit starts on %q", a.ActiveSubcommand)
	}
	scr := a.ManualScreen()
	text := viewerText(t, scr)
	if !strings.Contains(text, "SUB-MANUAL") || strings.Contains(text, "TOOL-MANUAL") {
		t.Fatalf("selected subcommand manual:\n%s", text)
	}
	if !strings.Contains(scr.Title, "docker run") || !strings.Contains(scr.Title, "Manual") {
		t.Fatalf("title %q", scr.Title)
	}

	b := newTestApp(t, spec)
	b.ActiveSubcommand = ""
	if got := viewerText(t, b.ManualScreen()); !strings.Contains(got, "TOOL-MANUAL") || strings.Contains(got, "SUB-MANUAL") {
		t.Fatalf("tool manual with no subcommand:\n%s", got)
	}

	// No subcommand page: ask the tool with --help, never --man.
	resetHelpLog(t, log)
	writeExe(t, filepath.Join(dir, "man"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+fmt.Sprintf("%q", log)+"\necho 'no manual entry'\n")
	c := newTestApp(t, spec)
	c.ActiveSubcommand = "run"
	got := viewerText(t, c.ManualScreen())
	if !strings.Contains(got, "run long help") {
		t.Fatalf("subcommand --help fallback:\n%s", got)
	}
	logged, _ := os.ReadFile(log)
	if strings.Contains(string(logged), "--man") {
		t.Fatalf("fetched the subcommand manual with --man:\n%s", logged)
	}
	if !strings.Contains(string(logged), "run --help") {
		t.Fatalf("subcommand was not asked with --help:\n%s", logged)
	}

	// A records tool keeps the tool manual even when a subcommand is selected.
	resetHelpLog(t, log)
	writeExe(t, filepath.Join(dir, "man"), `#!/bin/sh
printf '%s\n' "$*" >> `+fmt.Sprintf("%q", log)+`
page=
for a in "$@"; do page=$a; done
case "$page" in
  docker-run) printf '%s\n' NAME '    docker-run - run a container' SUB-MANUAL ;;
  docker) printf '%s\n' NAME '    docker - the tool' TOOL-MANUAL ;;
  *) printf '%s\n' "no manual for $page" ;;
esac
`)
	rec := newTestApp(t, ToolSpec{
		Name: "docker", Bin: bin, ArchetypeID: "records",
		Subcommands: []SubcommandSpec{{Name: "run", Label: "Run"}},
	})
	rec.ActiveSubcommand = "run"
	if got := viewerText(t, rec.ManualScreen()); !strings.Contains(got, "TOOL-MANUAL") || strings.Contains(got, "SUB-MANUAL") {
		t.Fatalf("records used the subcommand manual:\n%s", got)
	}
}

// helpTestDir is a folder for fake programs the manual runs. As root, the
// manual runs them as nobody (help.go drops privileges), and t.TempDir is
// private to root, so the folder is made readable by everyone instead.
func helpTestDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() != 0 {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp("", "mint-help-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// allowHelpUserToWrite creates path so that a fake program run by the
// manual (as nobody, when the tests run as root) can append to it.
func allowHelpUserToWrite(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() != 0 {
		return
	}
	if err := os.WriteFile(path, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
}

// resetHelpLog empties a log the fake programs append to, keeping it
// writable for them (see allowHelpUserToWrite).
func resetHelpLog(t *testing.T, path string) {
	t.Helper()
	_ = os.Remove(path)
	allowHelpUserToWrite(t, path)
}

func writeExe(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func prependPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func viewerText(t *testing.T, scr *mint.Screen) string {
	t.Helper()
	lines, ok := scr.ViewerLines()
	if !ok {
		t.Fatalf("screen %q is not ViewLog", scr.Title)
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func textHead(s string) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

func textTail(s string) string {
	if len(s) > 200 {
		return s[len(s)-200:]
	}
	return s
}

func TestSplitScreensForArchetypes(t *testing.T) {
	// A table opens its own row in a screen of its own. These are in the
	// 22, so the options stay in one pane and the output is the last pane.
	for _, id := range []string{"list", "records", "search"} {
		s := newTestApp(t, ToolSpec{Name: id, Bin: "echo", ArchetypeID: id}).MainScreen()
		got := orderedPaneTitles(s)
		if len(got) != 2 || got[0] != titleBasic || got[1] != titleOutput {
			t.Fatalf("%s panes %v, want Basic Options then Output", id, got)
		}
	}
	cmp := newTestApp(t, ToolSpec{Name: "cmp", Bin: "echo", ArchetypeID: "compare"})
	cs := cmp.MainScreen()
	if got := orderedPaneTitles(cs); len(got) != 2 || got[0] != titleBasic || got[1] != titleOutput {
		t.Fatalf("compare panes %v", got)
	}
	if !hasSection(cs, "Inputs") || !hasSection(cs, "Differences") {
		t.Fatalf("compare sections %v", sectionNames(cs))
	}
	// No options on this run screen, so the live output is the screen
	// itself. The recorded runs are listed there, not in a second pane.
	run := newTestApp(t, ToolSpec{Name: "ps", Bin: "ps", ArchetypeID: "list"}).RunScreen()
	if len(run.Panes) != 0 {
		t.Fatalf("run panes %v, want the output on the screen", paneTitles(run))
	}
	if !hasSection(run, "Captured Output") {
		t.Fatalf("run sections %v", sectionNames(run))
	}
	plain := newTestApp(t, ToolSpec{Name: "x", Bin: "echo"}).MainScreen()
	if got := orderedPaneTitles(plain); len(got) != 2 || got[0] != titleBasic || got[1] != titleOutput {
		t.Fatalf("plain panes %v", got)
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
