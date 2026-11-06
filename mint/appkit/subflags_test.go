package appkit

import (
	"strings"
	"testing"
)

func cmdSpec() ToolSpec {
	return ToolSpec{
		Name: "dock", Bin: "echo", ArchetypeID: "toolkit",
		Flags: []FlagSpec{{Flag: "--debug", Kind: FlagBool, Label: "Debug", Section: "Global options"}},
		Subcommands: []SubcommandSpec{
			{Name: "run", Label: "Run", Args: []string{"run"}, Flags: []FlagSpec{
				{Flag: "--rm", Kind: FlagBool, Label: "Remove when done", Section: "Run options"},
				{Flag: "-f", Kind: FlagFile, Label: "File", Section: "Run options"},
				{Flag: "--name", Kind: FlagText, Label: "Name", Section: "Run options", Default: "box"},
				{Flag: "--debug", Kind: FlagBool, Label: "Debug again"}, // the program's own: dropped
			}},
			{Name: "logs", Label: "Logs", Args: []string{"logs"}, Flags: []FlagSpec{
				{Flag: "-f", Kind: FlagBool, Label: "Follow", Section: "Logs options"},
				{Flag: "--token", Kind: FlagSecret, Label: "Token", Section: "Logs options"},
			}},
		},
	}
}

func TestFlagsAreTheSelectedCommandsThenTheProgramsOwn(t *testing.T) {
	a := newTestApp(t, cmdSpec())
	var got []string
	for _, f := range a.flags() {
		got = append(got, f.Flag)
	}
	if strings.Join(got, " ") != "--rm -f --name --debug" {
		t.Fatalf("flags %v", got)
	}
	if a.Values["--name"] != "box" || a.Values["--rm"] != "false" {
		t.Fatalf("defaults %v", a.Values)
	}
	all := a.allFlags()
	if len(all) != 1+3+2 {
		t.Fatalf("all flags %d", len(all))
	}
}

func TestSwitchingCommandsKeepsEachCommandsValues(t *testing.T) {
	a := newTestApp(t, cmdSpec())
	a.Values["--rm"] = "true"
	a.Values["-f"] = "/tmp/build.txt"
	args, _ := a.BuildArgs()
	line := strings.Join(args, " ")
	if !strings.HasPrefix(line, "run ") || !strings.Contains(line, "--rm") || !strings.Contains(line, "-f /tmp/build.txt") {
		t.Fatalf("run line %q", line)
	}
	// logs -f is a switch, not run's file: nothing carries over.
	a.useSubcommand("logs")
	args, _ = a.BuildArgs()
	line = strings.Join(args, " ")
	if line != "logs" {
		t.Fatalf("logs line %q", line)
	}
	if a.Values["-f"] != "false" {
		t.Fatalf("logs -f is %q, want its own default", a.Values["-f"])
	}
	a.Values["-f"] = "true"
	if line = strings.Join(must(a.BuildArgs()), " "); line != "logs -f" {
		t.Fatalf("logs -f line %q", line)
	}
	// Back to run: its values come back, logs' are put aside.
	a.useSubcommand("run")
	line = strings.Join(must(a.BuildArgs()), " ")
	if !strings.Contains(line, "--rm") || !strings.Contains(line, "-f /tmp/build.txt") {
		t.Fatalf("run again %q", line)
	}
	a.useSubcommand("logs")
	if a.Values["-f"] != "true" {
		t.Fatalf("logs -f was not kept: %q", a.Values["-f"])
	}
}

func must(args []string, _ string) []string { return args }

func TestASecretIsForgottenWhenAnotherCommandIsSelected(t *testing.T) {
	a := newTestApp(t, cmdSpec())
	a.useSubcommand("logs")
	if err := a.SetSecret(FlagSpec{Flag: "--token", Kind: FlagSecret}.SecretKey(), "hunter22"); err != nil {
		t.Skipf("no secret store here: %v", err)
	}
	if !a.hasSecret(FlagSpec{Flag: "--token", Kind: FlagSecret}.SecretKey()) {
		t.Fatal("secret not stored")
	}
	a.useSubcommand("run")
	if a.hasSecret(FlagSpec{Flag: "--token", Kind: FlagSecret}.SecretKey()) {
		t.Fatal("the logs token survived switching to run")
	}
	if strings.Contains(strings.Join(must(a.BuildArgs()), " "), "hunter22") {
		t.Fatal("the token reached another command")
	}
}

func TestSuiteShowsTheSelectedCommandsOptions(t *testing.T) {
	a := newTestApp(t, cmdSpec())
	labels := func() string {
		var out []string
		for _, sec := range a.MainScreen().Panes[0].Sections {
			out = append(out, sec.Name)
			for _, r := range sec.Rows {
				out = append(out, rowField(r, "label"))
			}
		}
		return strings.Join(out, "|")
	}
	got := labels()
	if !strings.Contains(got, "Run options") || !strings.Contains(got, "Remove when done") || !strings.Contains(got, "Global options") || strings.Contains(got, "Follow") {
		t.Fatalf("run shows %q", got)
	}
	if strings.Index(got, "Run options") > strings.Index(got, "Global options") {
		t.Fatalf("the command's own options should come first: %q", got)
	}
	for _, r := range a.MainScreen().Sections[0].Rows {
		if rowField(r, "id") == "suite:command:logs" {
			activateRow(t, r)
		}
	}
	got = labels()
	if !strings.Contains(got, "Follow") || strings.Contains(got, "Remove when done") {
		t.Fatalf("logs shows %q", got)
	}
}

func TestPresetRestoreSelectsItsCommandProperly(t *testing.T) {
	a := newTestApp(t, cmdSpec())
	a.Values["--rm"] = "true"
	a.useSubcommand("logs")
	a.useSubcommand("run")
	if a.Values["--rm"] != "true" {
		t.Fatal("setup")
	}
	a.useSubcommand("logs")
	if _, ok := a.Values["--rm"]; ok {
		t.Fatal("run's value stayed in Values while logs is selected")
	}
}

func TestGlobalOptionsGoBeforeTheCommandWhenTheUsageSaysSo(t *testing.T) {
	spec := cmdSpec()
	spec.GlobalFlagsFirst = true
	a := newTestApp(t, spec)
	a.Values["--debug"] = "true"
	a.Values["--rm"] = "true"
	a.ExtraArgs = "--privileged"
	a.Target = "alpine"
	line := strings.Join(must(a.BuildArgs()), " ")
	if !strings.HasPrefix(line, "--debug run --rm") || !strings.HasSuffix(line, "--privileged alpine") {
		t.Fatalf("line %q", line)
	}
	if n := a.argPrefixLen(); n != 2 {
		t.Fatalf("prefix %d, want 2 (--debug run)", n)
	}
	// Without the usage saying so, everything stays after the command word.
	b := newTestApp(t, cmdSpec())
	b.Values["--debug"] = "true"
	if line := strings.Join(must(b.BuildArgs()), " "); !strings.HasPrefix(line, "run ") || !strings.Contains(line, "--debug") {
		t.Fatalf("default line %q", line)
	}
}
