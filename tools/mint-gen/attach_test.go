package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestAttachParsedScreens(t *testing.T) {
	follow := appkit.ToolSpec{
		Name: "logtail", ArchetypeID: "follow",
		Flags: []appkit.FlagSpec{{Flag: "-f", Kind: appkit.FlagBool}, {Flag: "--follow", Kind: appkit.FlagBool}},
	}
	attachParsedScreens(&follow, "")
	if follow.Follow == nil || follow.Follow.Flag != "--follow" {
		t.Fatalf("follow: %+v", follow.Follow)
	}

	noFlag := appkit.ToolSpec{Name: "logtail", ArchetypeID: "follow"}
	attachParsedScreens(&noFlag, "")
	if noFlag.Follow != nil {
		t.Fatal("follow without a -f/--follow flag should stay a normal screen")
	}

	serve := appkit.ToolSpec{
		Name: "fileserv", ArchetypeID: "serve",
		Flags: []appkit.FlagSpec{{Flag: "--port", Default: "9000"}, {Flag: "-p"}, {Flag: "--listen"}},
	}
	attachParsedScreens(&serve, "")
	if serve.Service == nil || serve.Service.Adapter != appkit.ServiceGeneric || serve.Service.PortFlag != "--port" || serve.Service.DefaultPort != 9000 {
		t.Fatalf("serve: %+v", serve.Service)
	}

	noPort := appkit.ToolSpec{Name: "fileserv", ArchetypeID: "serve", Flags: []appkit.FlagSpec{{Flag: "--verbose"}}}
	attachParsedScreens(&noPort, "")
	if noPort.Service != nil {
		t.Fatal("serve without a port flag should stay a normal screen")
	}

	quiet := appkit.ToolSpec{Name: "toy", ArchetypeID: "session"}
	attachParsedScreens(&quiet, "Usage: toy\n  --help  show help\n")
	if quiet.Transcript != nil {
		t.Fatal("session without interactive or prompt cues should stay a normal screen")
	}

	session := appkit.ToolSpec{Name: "toyrepl", ArchetypeID: "session"}
	attachParsedScreens(&session, "toyrepl is an interactive command prompt.\n")
	if session.Transcript == nil || session.Transcript.Adapter != "lines" || strings.Join(session.Transcript.QuitWords, ",") != "quit,exit" {
		t.Fatalf("session: %+v", session.Transcript)
	}

	psql := appkit.ToolSpec{Name: "psql", ArchetypeID: "session"}
	attachParsedScreens(&psql, "psql - interactive SQL shell\npsql> ")
	if psql.Transcript == nil || psql.Transcript.Adapter != "psql" || len(psql.Transcript.QuitWords) != 2 {
		t.Fatalf("known session: %+v", psql.Transcript)
	}
}

func TestParseAttachesInteractiveScreens(t *testing.T) {
	follow := ParseManAndHelp("logtail9",
		"NAME\n       logtail9 - follow a growing log file\n\nSYNOPSIS\n       logtail9 [options] [FILE]\n",
		"Usage: logtail9 [options] [FILE]\n  -f, --follow   output appended data as the file grows\n")
	if follow.ArchetypeID != "follow" {
		t.Fatalf("archetype %s, want follow", follow.ArchetypeID)
	}
	if follow.Follow == nil || (follow.Follow.Flag != "--follow" && follow.Follow.Flag != "-f") {
		t.Fatalf("parsed follow: %+v", follow.Follow)
	}

	// A listed follow tool keeps the spec Mint already knows.
	tail := ParseManAndHelp("tail",
		"NAME\n       tail - output the last part of files\n",
		"Usage: tail [options] [FILE]\n  -f, --follow   output appended data as the file grows\n")
	if tail.Follow == nil || len(tail.Follow.FollowArgs) == 0 || tail.Follow.Flag != "" {
		t.Fatalf("known tail: %+v", tail.Follow)
	}

	serve := ParseManAndHelp("fileserv9",
		"NAME\n       fileserv9 - http server that serves files\n\nSYNOPSIS\n       fileserv9 [options]\n",
		"Usage: fileserv9 [options]\n  -p, --port PORT   port to listen on\n")
	if serve.ArchetypeID != "serve" {
		t.Fatalf("archetype %s, want serve", serve.ArchetypeID)
	}
	if serve.Service == nil || serve.Service.Adapter != appkit.ServiceGeneric || (serve.Service.PortFlag != "--port" && serve.Service.PortFlag != "-p") {
		t.Fatalf("parsed serve: %+v", serve.Service)
	}

	session := ParseManAndHelp("toyrepl9",
		"NAME\n       toyrepl9 - interactive SQL shell\n\nDESCRIPTION\n       toyrepl9 is an interactive command prompt.\n",
		"Usage: toyrepl9\nType commands at the prompt. quit or exit leaves.\n")
	if session.ArchetypeID != "session" {
		t.Fatalf("archetype %s, want session", session.ArchetypeID)
	}
	if session.Transcript == nil || session.Transcript.Adapter != "lines" || strings.Join(session.Transcript.QuitWords, ",") != "quit,exit" {
		t.Fatalf("parsed session: %+v", session.Transcript)
	}
}

func TestApplyChosenArchetypeResetsTarget(t *testing.T) {
	spec := appkit.ToolSpec{Name: "tool", Flags: []appkit.FlagSpec{{Flag: "--port"}}}
	probe, _ := appkit.LookupArchetype("probe")
	probe.ApplyTo(&spec)
	if spec.TargetLabel != "Targets" || spec.HideTarget {
		t.Fatalf("probe: %+v", spec.TargetLabel)
	}
	list, _ := appkit.LookupArchetype("list")
	applyChosenArchetype(&spec, list, "")
	if !spec.HideTarget || spec.TwoInputs || spec.TargetLabel != "" {
		t.Fatalf("list after probe: hide=%v two=%v label=%q", spec.HideTarget, spec.TwoInputs, spec.TargetLabel)
	}
	// A label taken from the usage line is not the archetype's, so it stays.
	spec.TargetLabel = "Target host / network"
	serve, _ := appkit.LookupArchetype("serve")
	applyChosenArchetype(&spec, serve, "")
	if spec.HideTarget || spec.TargetLabel != "Target host / network" || spec.Service == nil || spec.Service.PortFlag != "--port" {
		t.Fatalf("serve kept the usage label and the port: hide=%v label=%q service=%+v", spec.HideTarget, spec.TargetLabel, spec.Service)
	}
}

func TestScaffoldWritesParsedScreens(t *testing.T) {
	src, err := renderSpecGo(appkit.ToolSpec{
		Name: "logtail", Archetype: appkit.LayoutRunner, ArchetypeID: "follow",
		Follow: &appkit.FollowSpec{Flag: "--follow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if strings.Contains(text, "LayoutRunner") || strings.Contains(text, "Archetype:") {
		t.Fatalf("follow scaffold still names a layout:\n%s", text)
	}
	if !strings.Contains(text, `Flag: "--follow"`) {
		t.Fatalf("follow scaffold:\n%s", text)
	}

	src, err = renderSpecGo(appkit.ToolSpec{
		Name: "toyrepl", ArchetypeID: "session",
		Transcript: &appkit.TranscriptSpec{Adapter: "lines", QuitWords: []string{"quit", "exit"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text = string(src)
	if !strings.Contains(text, `Adapter: "lines"`) || !strings.Contains(text, `"quit"`) || !strings.Contains(text, `"exit"`) {
		t.Fatalf("session scaffold:\n%s", text)
	}

	cmp, _ := appkit.LookupArchetype("compare")
	spec := appkit.ToolSpec{Name: "cmp"}
	cmp.ApplyTo(&spec)
	src, err = renderSpecGo(spec)
	if err != nil {
		t.Fatal(err)
	}
	text = string(src)
	if !strings.Contains(text, "TwoInputs:") || !strings.Contains(text, `SecondTargetLabel:`) || !strings.Contains(text, `"Right"`) {
		t.Fatalf("compare scaffold:\n%s", text)
	}
	if strings.Contains(text, "HideTarget") {
		t.Fatalf("compare should not hide its inputs:\n%s", text)
	}
}

func specWithArchetype(name, id string, flags []appkit.FlagSpec) appkit.ToolSpec {
	spec := appkit.ToolSpec{Name: name, Flags: flags}
	a, ok := appkit.LookupArchetype(id)
	if !ok {
		panic("unknown archetype " + id)
	}
	a.ApplyTo(&spec)
	return spec
}

func attachHelp(spec *appkit.ToolSpec, help string) {
	attachParsedScreens(spec, help)
}

func boolFlag(flag, label string) appkit.FlagSpec {
	return appkit.FlagSpec{Flag: flag, Kind: appkit.FlagBool, Label: label}
}

func TestInferReadOnlyFromHelp(t *testing.T) {
	cases := []struct {
		name, id, help string
		flags          []appkit.FlagSpec
		irreversible   bool
		want           bool
	}{
		{"sampler", "watch", "Usage: sampler [options]\n", nil, false, true},
		{"lister", "list", "Usage: lister [options]\n  --verbose  say more\n", nil, false, true},
		{"fileinfo", "inspect", "Usage: fileinfo FILE\n", nil, false, false},
		{"lscpu", "inspect", "Usage: lscpu [options]\n", nil, false, true},
		{"whois9", "look-up", "Usage: whois9 NAME\n", nil, false, false},
		{"whois9", "look-up", "Usage: whois9 [NAME]\n", nil, false, true},
		{"sampler", "watch", "Usage: sampler DEVICE\n", nil, false, false},
		{"sampler", "watch", "Usage: sampler [options]\n", []appkit.FlagSpec{boolFlag("--force", "Force overwrite")}, false, false},
		{"lister", "list", "Usage: lister [options]\nCommands:\n  list    List the items\n  delete  Delete an item\n", nil, false, false},
		{"sampler", "watch", "Usage: sampler [options]\n", nil, true, false},
		{"findpat", "search", "Usage: findpat [PATTERN]\n", nil, false, true},
		{"findpat", "search", "Usage: findpat PATTERN\n", nil, false, false},
	}
	for _, c := range cases {
		spec := specWithArchetype(c.name, c.id, c.flags)
		spec.Irreversible = c.irreversible
		attachHelp(&spec, c.help)
		if spec.ReadOnly != c.want {
			t.Errorf("%s/%s ReadOnly=%v, want %v", c.id, c.name, spec.ReadOnly, c.want)
		}
	}
}

func TestInferStatusArgsFromSubcommands(t *testing.T) {
	cases := []struct {
		id, help string
		want     []string
	}{
		{"settings", "Usage: tune [options]\nCommands:\n  status  Show current status\n  set     Set a value\n", []string{"status"}},
		{"control", "Usage: unitctl [options]\nCommands:\n  list   List items\n  start  Start one\n", []string{"list"}},
		{"rules", "Usage: rulectl [options]\nCommands:\n  show [name]  Show a rule\n  dump         Dump everything\n", []string{"show"}},
		{"device", "Usage: devctl [options]\nCommands:\n  get <key>  Get one key\n  info       Print information\n", []string{"info"}},
		{"device", "Usage: devctl [options]\nCommands:\n  show <name>  Show one device\n  dump         Dump state\n", []string{"dump"}},
		{"settings", "Usage: tune [options]\nCommands:\n  status  Show status\n  list    List values\n", []string{"status"}},
	}
	for _, c := range cases {
		spec := specWithArchetype("tool", c.id, nil)
		// The usage line has to name the tool, not the archetype.
		help := strings.Replace(c.help, "Usage: ", "Usage: ", 1)
		help = strings.Replace(help, "tune", "tool", 1)
		help = strings.Replace(help, "unitctl", "tool", 1)
		help = strings.Replace(help, "rulectl", "tool", 1)
		help = strings.Replace(help, "devctl", "tool", 1)
		attachHelp(&spec, help)
		if !reflect.DeepEqual(spec.StatusArgs, c.want) {
			t.Errorf("%s: StatusArgs %q, want %q\nhelp:\n%s", c.id, spec.StatusArgs, c.want, help)
		}
		if spec.ReadOnly {
			t.Errorf("%s should not be marked read-only", c.id)
		}
	}
}

func TestInferStatusArgsFromFlags(t *testing.T) {
	cases := []struct {
		id    string
		flags []appkit.FlagSpec
		help  string
		want  []string
	}{
		{"settings", []appkit.FlagSpec{boolFlag("--status", "Show current status")}, "Usage: tool [options]\n", []string{"--status"}},
		{"control", []appkit.FlagSpec{boolFlag("--list-all", "List all entries")}, "Usage: tool [options]\n", []string{"--list-all"}},
		{"rules", []appkit.FlagSpec{boolFlag("-L", "List all rules")}, "Usage: tool [options]\n", []string{"-L"}},
		{"device", []appkit.FlagSpec{boolFlag("--query", "Query connected devices")}, "Usage: tool [options]\n", []string{"--query"}},
		{"device", []appkit.FlagSpec{boolFlag("-q", "be quiet")}, "Usage: tool [options]\n", nil},
		{"settings", []appkit.FlagSpec{boolFlag("-a", "display all settings")}, "Usage: tool [options]\n", []string{"-a"}},
		{"control", []appkit.FlagSpec{boolFlag("-a", "display all settings")}, "Usage: tool [options]\n", nil},
		{"watch", []appkit.FlagSpec{boolFlag("-a", "print all values")}, "Usage: tool DEVICE\n", []string{"-a"}},
		{"list", []appkit.FlagSpec{boolFlag("-a", "show all values")}, "Usage: tool [options]\n", nil},
		{"settings", []appkit.FlagSpec{boolFlag("--status", "Show status"), boolFlag("-a", "display all settings")},
			"Usage: tool [options]\nCommands:\n  status  Show status\n  set     Set a value\n", []string{"status"}},
		{"device", []appkit.FlagSpec{boolFlag("-S", "show the status")}, "Usage: tool [options]\n", []string{"-S"}},
		{"records", []appkit.FlagSpec{boolFlag("--list-rules", "List rules")}, "Usage: tool [options]\n", []string{"--list-rules"}},
	}
	for i, c := range cases {
		spec := specWithArchetype("tool", c.id, c.flags)
		attachHelp(&spec, c.help)
		if !reflect.DeepEqual(spec.StatusArgs, c.want) {
			t.Errorf("case %d %s: StatusArgs %q, want %q (ReadOnly=%v)", i, c.id, spec.StatusArgs, c.want, spec.ReadOnly)
		}
	}
	// A pure watch observer is read-only, so it is polled as itself.
	pure := specWithArchetype("tool", "watch", []appkit.FlagSpec{boolFlag("-a", "display all values")})
	attachHelp(&pure, "Usage: tool [options]\n")
	if !pure.ReadOnly || len(pure.StatusArgs) != 0 {
		t.Fatalf("pure watch: ReadOnly=%v StatusArgs=%v", pure.ReadOnly, pure.StatusArgs)
	}
}

func TestPinnedToolsKeepCuratedStatusArgs(t *testing.T) {
	help := `Usage: %s [options]
Commands:
  status     Show status
  start      Start it
  stop       Stop it
  list       List them
  show       Show one
  info       Print info
`
	want := map[string][]string{
		"systemctl": {"list-units", "--type=service", "--no-pager"},
		"ufw":       {"status", "verbose"},
		"sysctl":    {"-a"},
		"xrandr":    {"--query"},
	}
	control, _ := appkit.LookupArchetype("control")
	for name, status := range want {
		spec := appkit.ToolSpec{Name: name, ReadOnly: true, StatusArgs: []string{"nope"}}
		applyChosenArchetype(&spec, control, fmt.Sprintf(help, name))
		if spec.ReadOnly {
			t.Errorf("%s became read-only", name)
		}
		if !reflect.DeepEqual(spec.StatusArgs, status) {
			t.Errorf("%s status %q, want %q", name, spec.StatusArgs, status)
		}
	}

	spec := appkit.ToolSpec{Name: "systemctl"}
	appkit.ApplySafetyDefaults(&spec)
	kept := append([]string(nil), spec.StatusArgs...)
	def, _ := appkit.LookupArchetype("control")
	def.ApplyTo(&spec)
	attachHelp(&spec, fmt.Sprintf(help, "systemctl"))
	if !reflect.DeepEqual(spec.StatusArgs, kept) {
		t.Fatalf("attach replaced curated status %q with %q", kept, spec.StatusArgs)
	}
}

func TestInferRowActionsFromSubcommands(t *testing.T) {
	control := specWithArchetype("unitctl", "control", nil)
	attachHelp(&control, `Usage: unitctl [options]
Commands:
  logs      Follow logs
  kill      Kill a unit
  disable   Disable a unit
  enable    Enable a unit
  reload    Reload a unit
  restart   Restart a unit
  stop      Stop a unit
  start     Start a unit
  status    Show status
`)
	wantHints := []string{"status", "start", "stop", "restart", "reload", "enable", "disable", "kill", "logs"}
	if got := actionHints(control.RowActions); !reflect.DeepEqual(got, wantHints) {
		t.Fatalf("control hints %q, want %q", got, wantHints)
	}
	for _, a := range control.RowActions {
		confirm := a.Hint == "stop" || a.Hint == "restart" || a.Hint == "disable" || a.Hint == "kill"
		if a.Confirm != confirm || a.Column != 0 || !reflect.DeepEqual(a.Subcommand, []string{a.Hint}) {
			t.Errorf("control %s: %+v", a.Hint, a)
		}
		if a.Label == "" {
			t.Errorf("control %s has an empty label", a.Hint)
		}
	}

	check := func(id, help string, want []string, confirm map[string]bool) {
		t.Helper()
		spec := specWithArchetype("tool", id, nil)
		attachHelp(&spec, help)
		if got := actionHints(spec.RowActions); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s hints %q, want %q", id, got, want)
		}
		for _, a := range spec.RowActions {
			if a.Confirm != confirm[a.Hint] || a.Column != 0 {
				t.Errorf("%s %s: confirm %v column %d", id, a.Hint, a.Confirm, a.Column)
			}
		}
	}
	check("records", "Usage: tool [options]\nCommands:\n  remove  Remove a record\n  update  Update a record\n  show    Show a record\n",
		[]string{"show", "update", "remove"}, map[string]bool{"show": false, "update": true, "remove": true})
	check("schedule", "Usage: tool [options]\nCommands:\n  delete  Delete a job\n  run     Run a job\n  info    Show a job\n",
		[]string{"info", "run", "delete"}, map[string]bool{"info": false, "run": false, "delete": true})
	check("rules", "Usage: tool [options]\nCommands:\n  delete  Delete a rule\n  deny    Deny a rule\n  allow   Allow a rule\n  status  Show status\n",
		[]string{"status", "allow", "deny", "delete"}, map[string]bool{"status": false, "allow": false, "deny": true, "delete": true})
	check("history", "Usage: tool [options]\nCommands:\n  revert  Revert a snapshot\n  diff    Show a diff\n",
		[]string{"diff", "revert"}, map[string]bool{"diff": false, "revert": true})
	check("device", "Usage: tool [options]\nCommands:\n  remove   Remove a device\n  disable  Disable a device\n  enable   Enable a device\n  show     Show a device\n",
		[]string{"show", "enable", "disable", "remove"}, map[string]bool{"show": false, "enable": false, "disable": true, "remove": true})
}

func TestInferRowActionsFromFlags(t *testing.T) {
	spec := specWithArchetype("unitctl", "control", []appkit.FlagSpec{
		boolFlag("--show", "Show one item"),
		{Flag: "-d", Kind: appkit.FlagText, Label: "Delete an entry"},
		boolFlag("-r", "Recursive directory traversal"),
		boolFlag("--remove", "remove files recursively from directories"),
		boolFlag("--kill", "Kill a process"),
		{Flag: "--stop", Kind: appkit.FlagText, Label: "Stop a service"},
		{Flag: "-D", Kind: appkit.FlagText, Label: "remove a rule"},
	})
	attachHelp(&spec, "Usage: unitctl [options]\n")
	var subs []string
	for _, a := range spec.RowActions {
		if a.Column != 0 {
			t.Errorf("%s column %d", a.Hint, a.Column)
		}
		subs = append(subs, strings.Join(a.Subcommand, " "))
	}
	if got, want := subs, []string{"--show", "-d", "-D", "--kill", "--stop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("flag actions %q, want %q", got, want)
	}
	for _, a := range spec.RowActions {
		wantConfirm := a.Subcommand[0] != "--show"
		if a.Confirm != wantConfirm {
			t.Errorf("%s confirm %v", a.Subcommand[0], a.Confirm)
		}
	}

	// A matching subcommand wins; the delete flag is not also added.
	both := specWithArchetype("unitctl", "control", []appkit.FlagSpec{
		{Flag: "--delete", Kind: appkit.FlagText, Label: "Delete an entry"},
	})
	attachHelp(&both, "Usage: unitctl [options]\nCommands:\n  status  Show status\n  stop    Stop a unit\n")
	if got := actionHints(both.RowActions); !reflect.DeepEqual(got, []string{"status", "stop"}) {
		t.Fatalf("subcommands should win: %q", got)
	}
}

func TestApplyChosenArchetypeRecomputesSafety(t *testing.T) {
	watch, _ := appkit.LookupArchetype("watch")
	control, _ := appkit.LookupArchetype("control")
	spec := appkit.ToolSpec{Name: "sampler"}
	applyChosenArchetype(&spec, watch, "Usage: sampler [options]\n")
	if !spec.ReadOnly || len(spec.StatusArgs) != 0 || len(spec.RowActions) != 0 {
		t.Fatalf("watch observer: ro=%v status=%v actions=%v", spec.ReadOnly, spec.StatusArgs, spec.RowActions)
	}

	help := `Usage: sampler [options]
Commands:
  stop     Stop one
  start    Start one
  status   Show status
`
	applyChosenArchetype(&spec, control, help)
	if spec.ReadOnly || !reflect.DeepEqual(spec.StatusArgs, []string{"status"}) {
		t.Fatalf("control: ro=%v status=%v", spec.ReadOnly, spec.StatusArgs)
	}
	if got := actionHints(spec.RowActions); !reflect.DeepEqual(got, []string{"status", "start", "stop"}) {
		t.Fatalf("control actions %q", got)
	}
	if !spec.RowActions[2].Confirm || spec.RowActions[0].Confirm {
		t.Fatalf("confirm flags: %+v", spec.RowActions)
	}

	applyChosenArchetype(&spec, watch, "Usage: sampler [options]\n")
	if !spec.ReadOnly || len(spec.StatusArgs) != 0 || len(spec.RowActions) != 0 {
		t.Fatalf("back to watch: ro=%v status=%v actions=%v", spec.ReadOnly, spec.StatusArgs, spec.RowActions)
	}

	sys := appkit.ToolSpec{Name: "systemctl", ReadOnly: true, StatusArgs: []string{"status"},
		RowActions: []appkit.RowActionSpec{{Label: "Gone", Hint: "gone"}}}
	applyChosenArchetype(&sys, control, help)
	if sys.ReadOnly || !reflect.DeepEqual(sys.StatusArgs, []string{"list-units", "--type=service", "--no-pager"}) {
		t.Fatalf("systemctl: ro=%v status=%v", sys.ReadOnly, sys.StatusArgs)
	}
}

func TestSafetyReportShowsInferredState(t *testing.T) {
	spec := specWithArchetype("sampler", "watch", nil)
	attachHelp(&spec, "Usage: sampler [options]\n")
	if !strings.Contains(spec.SafetyReport().Running, "only reports state") {
		t.Fatalf("watch running %q", spec.SafetyReport().Running)
	}

	spec = specWithArchetype("unitctl", "control", nil)
	attachHelp(&spec, "Usage: unitctl [options]\nCommands:\n  status  Show status\n  stop    Stop a unit\n")
	r := spec.SafetyReport()
	if !strings.Contains(r.Running, "unitctl status") {
		t.Fatalf("running %q", r.Running)
	}
	if len(r.RowActions) != 2 {
		t.Fatalf("report actions %+v", r.RowActions)
	}
	var b strings.Builder
	printSafetyReport(&b, spec)
	out := b.String()
	for _, want := range []string{"Row actions:", "Status", "Stop (confirm)", "unitctl status"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	plain := appkit.ToolSpec{Name: "unitctl", ArchetypeID: "control"}
	appkit.ApplySafetyDefaults(&plain)
	if len(safetyRows(spec)) <= len(safetyRows(plain)) {
		t.Fatal("safety rows omitted the row-action line")
	}
	var io strings.Builder
	printIOModel(&io, IOModel{Tool: "unitctl"}, spec)
	if !strings.Contains(io.String(), "Stop (confirm)") {
		t.Fatalf("explain:\n%s", io.String())
	}
}

func TestRenderSpecGoWritesSafetyFields(t *testing.T) {
	src, err := renderSpecGo(appkit.ToolSpec{
		Name: "unitctl", Archetype: appkit.LayoutMonitor, ArchetypeID: "control",
		ReadOnly:   true,
		StatusArgs: []string{"status", "--no-pager"},
		RowActions: []appkit.RowActionSpec{
			{Label: "Status", Hint: "status", Subcommand: []string{"status"}, Column: 0, Confirm: false},
			{Label: "Stop", Hint: "stop", Subcommand: []string{"stop"}, Column: 0, Confirm: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, want := range []string{
		"ReadOnly:", "true",
		"StatusArgs:", `"status"`, `"--no-pager"`,
		"RowActions:", "RowActionSpec",
		`Label: "Status"`, `Label: "Stop"`,
		`Subcommand: []string{"status"}`, `Subcommand: []string{"stop"}`,
		"Column: 0", "Confirm: false", "Confirm: true",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q\n%s", want, text)
		}
	}

	dir := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	full := append(append([]byte{}, src...), []byte("\nfunc main() {}\n")...)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), full, 0o644); err != nil {
		t.Fatal(err)
	}
	mod := "module example.com/speccompile\n\ngo 1.21\n\nrequire github.com/CurtisD4q/Mint-Interfaces v0.0.0\n\nreplace github.com/CurtisD4q/Mint-Interfaces => " + repo + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "out"), ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated spec does not compile: %v\n%s\n%s", err, out, text)
	}
}

func actionHints(actions []appkit.RowActionSpec) []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = a.Hint
	}
	return out
}
