package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Help texts here are made up for the tests: no program's real help is
// committed.

const zzMonitorHelp = `Usage: zzmon [OPTIONS]
Show disk activity as a table of columns, refreshed until interrupted.

Options:
  -i, --interval SECONDS   seconds between updates
  -n, --no-header          omit the header line
  -h, --help               show this help
`

const zzInstallerHelp = `Usage: zzpkg COMMAND [PACKAGE...]
Install and remove packages.

Commands:
  install     install packages
  remove      remove packages
  list        list installed packages

Options:
  -y, --yes   answer yes to questions
`

const zzDiffHelp = `Usage: zzcmp [OPTIONS] FILE1 FILE2
Compare two files line by line.

Options:
  -q, --brief   report only when files differ
  -o, --output FILE   write the report to FILE
`

const zzCopyHelp = `Usage: zzcp [OPTIONS] SOURCE DEST
Make a copy of a file.

Options:
  -v, --verbose   explain what is being done
`

func withInterfaceMode(t *testing.T, mode string) {
	t.Helper()
	old, oldPkg := interfaceMode, readPackages
	interfaceMode, readPackages = mode, false
	t.Cleanup(func() { interfaceMode, readPackages = old, oldPkg })
}

func TestDetectAnswersFromHelp(t *testing.T) {
	withInterfaceMode(t, InterfaceArchetype)
	check := func(name, help string, want map[appkit.Question]bool) {
		t.Helper()
		spec := ParseManAndHelp(name, "", help)
		a := detectAnswers(name, "", help, spec)
		for q, w := range want {
			if a[q].Yes != w {
				t.Errorf("%s: %s = %v (%s), want %v", name, q, a[q].Yes, a[q].Why, w)
			}
		}
	}
	check("zzmon", zzMonitorHelp, map[appkit.Question]bool{appkit.QKeepsRunning: true, appkit.QTable: true, appkit.QChangesThings: false, appkit.QTarget: false})
	check("zzpkg", zzInstallerHelp, map[appkit.Question]bool{appkit.QChangesThings: true, appkit.QSubcommands: true, appkit.QKeepsRunning: false})
	check("zzcmp", zzDiffHelp, map[appkit.Question]bool{appkit.QTwoInputs: true, appkit.QTarget: true, appkit.QWritesFile: true})
	check("zzcp", zzCopyHelp, map[appkit.Question]bool{appkit.QTwoInputs: false, appkit.QTarget: true, appkit.QChangesThings: true})
}

func TestPairedInputs(t *testing.T) {
	cases := []struct {
		names []string
		want  bool
	}{
		{[]string{"FILE1", "FILE2"}, true},
		{[]string{"file-1", "file-2"}, true},
		{[]string{"OLD", "NEW"}, true},
		{[]string{"SOURCE", "DEST"}, false},
		{[]string{"PATTERN", "FILE"}, false},
		{[]string{"FILE1"}, false},
	}
	for _, c := range cases {
		var ps []Positional
		for _, n := range c.names {
			ps = append(ps, Positional{Name: n})
		}
		if _, _, ok := pairedInputs(ps); ok != c.want {
			t.Errorf("%v: paired %v, want %v", c.names, ok, c.want)
		}
	}
}

// The default is unchanged: archetype mode sets no features.
func TestArchetypeModeIsTheDefault(t *testing.T) {
	if interfaceMode != InterfaceArchetype {
		t.Fatalf("default mode %q", interfaceMode)
	}
	withInterfaceMode(t, InterfaceArchetype)
	spec := ParseManAndHelp("zzmon", "", zzMonitorHelp)
	if spec.Features != nil || spec.InterfaceGroup != "" {
		t.Errorf("archetype mode set features %v / group %q", spec.Features, spec.InterfaceGroup)
	}
}

func TestAnswersModeBuildsTheScreen(t *testing.T) {
	withInterfaceMode(t, InterfaceAnswers)
	spec := ParseManAndHelp("zzmon", "", zzMonitorHelp)
	b := appkit.Blueprint{Features: spec.Features}
	if spec.InterfaceGroup == "" || !b.Has(appkit.FeatLiveRefresh) || !b.Has(appkit.FeatTable) {
		t.Errorf("zzmon: group %q, features %v", spec.InterfaceGroup, spec.Features)
	}
	if spec.ArchetypeID == "" {
		t.Error("the archetype still words the screen")
	}
	spec = ParseManAndHelp("zzpkg", "", zzInstallerHelp)
	b = appkit.Blueprint{Features: spec.Features}
	if !b.Has(appkit.FeatConfirm) || b.Has(appkit.FeatLiveRefresh) || spec.ReadOnly {
		t.Errorf("zzpkg changes things: features %v, read-only %v", spec.Features, spec.ReadOnly)
	}
	if len(spec.Subcommands) == 0 {
		t.Error("zzpkg: no subcommand tabs")
	}
}

// Listed programs keep their checked setup in either mode.
func TestAnswersModeLeavesListedProgramsAlone(t *testing.T) {
	withInterfaceMode(t, InterfaceAnswers)
	names := appkit.ArchetypeToolNames()
	if len(names) == 0 {
		t.Fatal("no listed programs")
	}
	spec := ParseManAndHelp(names[0], "", zzMonitorHelp)
	if spec.Features != nil || spec.InterfaceGroup != "" {
		t.Errorf("%s is listed, but answers mode set features %v / group %q", names[0], spec.Features, spec.InterfaceGroup)
	}
}

// Answers mode never lowers a data level, never drops a secret, and never
// removes caution the archetype mode gives.
func TestAnswersModeKeepsSafety(t *testing.T) {
	helps := map[string]string{
		"zzmon": zzMonitorHelp, "zzpkg": zzInstallerHelp, "zzcmp": zzDiffHelp, "zzcp": zzCopyHelp,
		"zzvault": "Usage: zzvault [OPTIONS] NAME\nRead a secret from the password store.\n\nOptions:\n  -p, --password PASSWORD   the master password\n  --token TOKEN             an API token\n",
		"zzcap":   "Usage: zzcap [OPTIONS]\nCapture network packets until interrupted. Must be run as root.\n\nOptions:\n  -i, --interface IFACE   interface to listen on\n  -w FILE                 write packets to FILE\n",
	}
	withInterfaceMode(t, InterfaceArchetype) // restores both settings afterwards
	for _, mode := range []string{InterfaceAnswers, InterfaceBlend} {
		for name, help := range helps {
			interfaceMode = InterfaceArchetype
			before := ParseManAndHelp(name, "", help)
			interfaceMode = mode
			after := ParseManAndHelp(name, "", help)
			if dataRank(after.Data) < dataRank(before.Data) {
				t.Errorf("%s: data level %q lowered to %q", name, before.Data, after.Data)
			}
			secrets := func(s appkit.ToolSpec) (n int) {
				for _, f := range s.Flags {
					if f.Kind == appkit.FlagSecret {
						n++
					}
				}
				return
			}
			if secrets(after) < secrets(before) {
				t.Errorf("%s: secret options %d -> %d", name, secrets(before), secrets(after))
			}
			if before.NeedsRoot && !after.NeedsRoot || before.ConfirmRun && !after.ConfirmRun || before.Irreversible && !after.Irreversible {
				t.Errorf("%s: caution removed: root %v->%v confirm %v->%v", name, before.NeedsRoot, after.NeedsRoot, before.ConfirmRun, after.ConfirmRun)
			}
			if after.ReadOnly && !before.ReadOnly {
				t.Errorf("%s: %s mode made it read-only", name, mode)
			}
		}
	}
}

func TestScaffoldWritesInterfaceFields(t *testing.T) {
	spec := appkit.ToolSpec{Name: "zz", Title: "zz", InterfaceGroup: "live", Features: []appkit.Feature{appkit.FeatLiveRefresh, appkit.FeatTable}, FullScreen: true}
	b, err := renderSpecGo(spec)
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{`InterfaceGroup:    "live"`, `Features:          []appkit.Feature{"live-refresh", "table"}`, `FullScreen:        true`} {
		if !strings.Contains(src, want) {
			t.Errorf("scaffold output lacks %s", want)
		}
	}
}

func TestAnswerProbs(t *testing.T) {
	a := appkit.Answers{
		appkit.QKeepsRunning: {Yes: true, Sure: appkit.SureStrong},
		appkit.QTable:        {Yes: true, Sure: appkit.SureWeak},
		appkit.QSubcommands:  {Yes: false, Sure: appkit.SureStrong},
		appkit.QTarget:       {Yes: false, Sure: appkit.SureNone},
		appkit.QFullScreen:   {Yes: false, Sure: appkit.SureWeak},
	}
	p := answerProbs(a)
	if !(p[appkit.QKeepsRunning] > p[appkit.QTable] && p[appkit.QTable] > 0.5 && p[appkit.QSubcommands] < 0.1) {
		t.Errorf("strong yes > hint > 0.5 > strong no: %v", p)
	}
	if p[appkit.QTarget] != answerBase[appkit.QTarget] || p[appkit.QFullScreen] >= answerBase[appkit.QFullScreen] {
		t.Errorf("no evidence gives the base rate, silence less: %v", p)
	}
}

func TestKeywordGroupProbs(t *testing.T) {
	sum := func(m map[string]float64) (s float64) {
		for _, v := range m {
			s += v
		}
		return
	}
	listed := ArchetypeChoice{ID: "watch", Confidence: ConfidenceListed}
	if ap := archetypeProbs(listed); ap["watch"] != 1 {
		t.Errorf("a listed archetype is certain: %v", ap)
	}
	if s := sum(keywordGroupProbs(listed)); s < 0.999 || s > 1.001 {
		t.Errorf("K(g) sums to %v", s)
	}
	close := ArchetypeChoice{ID: "watch", Confidence: ConfidenceClose, Scores: []ArchetypeScore{{ID: "watch", Score: 50}, {ID: "list", Score: 45}, {ID: "convert", Score: 0}}}
	ap := archetypeProbs(close)
	if !(ap["watch"] > ap["list"] && ap["list"] > ap["convert"]) {
		t.Errorf("probabilities follow the points: %v", ap)
	}
	if s := sum(keywordGroupProbs(close)); s < 0.999 || s > 1.001 {
		t.Errorf("K(g) sums to %v", s)
	}
	if len(groupBridge) != 35 {
		t.Errorf("the bridge has %d archetypes, want 35", len(groupBridge))
	}
}

func TestBlendModeBuildsTheScreenSafely(t *testing.T) {
	withInterfaceMode(t, InterfaceBlend)
	spec := ParseManAndHelp("zzmon", "", zzMonitorHelp)
	if spec.InterfaceGroup == "" || spec.Features == nil {
		t.Fatalf("blend mode set no group: %+v", spec.InterfaceGroup)
	}
	spec = ParseManAndHelp("zzpkg", "", zzInstallerHelp)
	b := appkit.Blueprint{Features: spec.Features}
	if !spec.ConfirmRun || b.Has(appkit.FeatLiveRefresh) || spec.ReadOnly {
		t.Errorf("zzpkg changes things: confirm %v, features %v, read-only %v", spec.ConfirmRun, spec.Features, spec.ReadOnly)
	}
}

func TestAnswerWeightFlag(t *testing.T) {
	old := blendAnswerWeight
	t.Cleanup(func() { blendAnswerWeight, interfaceMode = old, InterfaceArchetype })
	for _, bad := range []string{"1.5", "-0.1", "NaN"} {
		if code := runCLI([]string{"--interface", "blend", "--answer-weight", bad, "--list-archetypes"}); code != 2 {
			t.Errorf("--answer-weight %s: exit %d, want 2", bad, code)
		}
	}
}
