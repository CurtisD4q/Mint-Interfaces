package appkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildArgsAndProfiles(t *testing.T) {
	tmp := t.TempDir()
	spec := ToolSpec{
		Name:              "demo",
		Bin:               "echo",
		OutputsFile:       true,
		OutputFlag:        "-o",
		DefaultExt:        "txt",
		DefaultResultsDir: tmp,
		Profiles: []ProfileSpec{
			{
				Name:      "Quick",
				Flags:     map[string]string{"-v": "true", "-n": "3"},
				ExtraArgs: []string{"--fast"},
			},
			{
				Name:  "Minimal",
				Flags: map[string]string{"-v": "false"},
			},
		},
		Flags: []FlagSpec{
			{Flag: "-v", Kind: FlagBool, Label: "Verbose"},
			{Flag: "-n", Kind: FlagInt, Label: "Count", Min: 1, Max: 10},
			{Flag: "--format", Join: "=", Kind: FlagChoice, Choices: []Choice{{Label: "JSON", Value: "json"}}},
		},
	}

	app := New(spec)
	app.ConfigDir = filepath.Join(tmp, "cfg")
	app.DataDir = filepath.Join(tmp, "data")
	app.Target = "example.com"
	app.OutputFile = "scan.txt"
	app.Values["--format"] = "json"

	args, outPath := app.BuildArgs()
	wantOut := filepath.Join(tmp, "scan.txt")
	if outPath != wantOut {
		t.Fatalf("outPath = %q, want %q", outPath, wantOut)
	}

	cmdline := strings.Join(args, " ")
	for _, token := range []string{"-v", "-n 3", "--format=json", "-o " + wantOut, "--fast", "example.com"} {
		if !strings.Contains(cmdline, token) {
			t.Fatalf("cmdline %q missing %q", cmdline, token)
		}
	}

	if !app.ApplyProfile("Minimal") {
		t.Fatalf("ApplyProfile(Minimal) returned false")
	}
	if app.Values["-v"] != "false" {
		t.Fatalf("expected -v to be false after Minimal profile, got %q", app.Values["-v"])
	}
}

func TestStartJobAndDirectoryListing(t *testing.T) {
	tmp := t.TempDir()
	spec := ToolSpec{
		Name:              "echotest",
		Bin:               "echo",
		OutputsFile:       true,
		OutputFlag:        "", // capture stdout to file
		DefaultExt:        "log",
		DefaultResultsDir: tmp,
		KeepHistory:       true,
		Flags: []FlagSpec{
			{Flag: "-n", Kind: FlagBool, Label: "No newline"},
		},
	}

	app := New(spec)
	app.ConfigDir = filepath.Join(tmp, "cfg")
	app.DataDir = filepath.Join(tmp, "data")
	app.Target = "hello-from-appkit"
	app.OutputFile = "hello.log"

	job, err := app.StartJob()
	if err != nil {
		t.Fatalf("StartJob error: %v", err)
	}

	select {
	case <-job.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for job")
	}

	outFile := filepath.Join(tmp, "hello.log")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected output file %s to exist: %v", outFile, err)
	}
	if !strings.Contains(string(data), "hello-from-appkit") {
		t.Fatalf("unexpected file contents: %q", string(data))
	}

	entries, err := app.ListResultsDir()
	if err != nil {
		t.Fatalf("ListResultsDir error: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "hello.log" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected hello.log in ListResultsDir, got %+v", entries)
	}
}

func TestAll28TemplatesAnd6Archetypes(t *testing.T) {
	templates := AllTemplates()
	if len(templates) != 28 {
		t.Fatalf("len(AllTemplates()) = %d, want 28", len(templates))
	}

	archetypes := AllArchetypes()
	if len(archetypes) != 6 {
		t.Fatalf("len(AllArchetypes()) = %d, want 6", len(archetypes))
	}

	mixins := AllMixins()
	if len(mixins) != 8 {
		t.Fatalf("len(AllMixins()) = %d, want 8", len(mixins))
	}

	seenIDs := map[TemplateID]bool{}
	seenSlugs := map[string]bool{}
	seenArch := map[Archetype]int{}

	for _, tpl := range templates {
		if tpl.ID == "" || tpl.Slug == "" || tpl.Name == "" {
			t.Fatalf("template missing required identity fields: %+v", tpl)
		}
		if seenIDs[tpl.ID] {
			t.Fatalf("duplicate TemplateID: %s", tpl.ID)
		}
		if seenSlugs[tpl.Slug] {
			t.Fatalf("duplicate Slug: %s", tpl.Slug)
		}
		seenIDs[tpl.ID] = true
		seenSlugs[tpl.Slug] = true
		seenArch[tpl.Archetype]++

		if byID, ok := LookupTemplate(string(tpl.ID)); !ok || byID.Slug != tpl.Slug {
			t.Fatalf("LookupTemplate(%q) failed", tpl.ID)
		}
		if bySlug, ok := LookupTemplate(tpl.Slug); !ok || bySlug.ID != tpl.ID {
			t.Fatalf("LookupTemplate(%q) failed", tpl.Slug)
		}

		for _, ex := range tpl.ExampleTools {
			bin := strings.Fields(ex)[0]
			if matched, ok := TemplateForTool(bin); !ok {
				t.Fatalf("TemplateForTool(%q) returned false", bin)
			} else if matched.ID == "" {
				t.Fatalf("TemplateForTool(%q) returned empty ID", bin)
			}
		}

		spec := ToolSpec{Name: "test-" + strings.ToLower(string(tpl.ID)), Bin: "echo", Template: string(tpl.ID)}
		app := New(spec)
		if app.Spec.Archetype != tpl.Archetype {
			t.Fatalf("template %s: app.Spec.Archetype = %q, want %q", tpl.ID, app.Spec.Archetype, tpl.Archetype)
		}
		scr := app.MainScreen()
		if scr == nil || len(scr.Sections) == 0 {
			t.Fatalf("template %s (%s): MainScreen returned empty screen", tpl.ID, tpl.Archetype)
		}
		mintApp := app.BuildApp()
		if mintApp == nil || len(mintApp.Items) < 3 {
			t.Fatalf("template %s: BuildApp returned insufficient HomeItems", tpl.ID)
		}
	}

	for _, arch := range archetypes {
		if seenArch[arch] == 0 {
			t.Fatalf("archetype %q has 0 templates assigned", arch)
		}
	}
}

func TestMixinsAndDynamicChoicesAndSplitSections(t *testing.T) {
	spec := ToolSpec{
		Name:      "net-demo",
		Bin:       "echo",
		Template:  "T12", // network-scanner: Scanner + Table + Collapsible + FileOutput + Profiles + RootAuth + History
		NeedsRoot: true,
		Flags: []FlagSpec{
			{
				Flag:       "-i",
				Kind:       FlagChoice,
				Label:      "Interface",
				ChoicesCmd: []string{"echo", "lo\neth0\nwlan0"},
			},
		},
	}

	app := New(spec)
	if len(app.Spec.Flags[0].Choices) < 3 {
		t.Fatalf("expected ResolveDynamicChoices to populate choices from echo, got %+v", app.Spec.Flags[0].Choices)
	}

	enabled := app.Spec.EnabledMixins()
	names := enabled.Names()
	if !enabled.WithTableOutput || !enabled.WithCollapsibleOutput || !enabled.WithFileOutput || !enabled.WithRootAuth || !enabled.WithDynamicChoices {
		t.Fatalf("unexpected enabled mixins for T12 + ChoicesCmd: %+v (%v)", enabled, names)
	}

	// Mint never wraps commands in sudo: root comes from starting the app with sudo.
	if preview := app.CommandPreview(); strings.Contains(preview, "sudo") {
		t.Fatalf("CommandPreview should not add sudo, got %q", preview)
	}

	rows := SplitSections([]string{
		"NAME",
		"    nmap - Network exploration tool",
		"SYNOPSIS",
		"    nmap [Scan Type] [Options] {target specification}",
		"DESCRIPTION",
		"    Nmap ('Network Mapper') is an open source tool.",
	}, 50)
	if len(rows) != 3 {
		t.Fatalf("expected 3 collapsible section rows from SplitSections, got %d", len(rows))
	}
}

func TestRepeatableKVFlags(t *testing.T) {
	spec := ToolSpec{
		Name: "curl",
		Flags: []FlagSpec{
			{Flag: "-H", Kind: FlagText, Label: "Header (-H)", Hint: "key=value (repeatable)"},
		},
	}
	app := New(spec)
	app.Values["-H"] = "Accept: application/json\nX-Token: abc"
	args, _ := app.BuildArgs()
	want := []string{"-H", "Accept: application/json", "-H", "X-Token: abc"}
	if len(args) != len(want) {
		t.Fatalf("expected %v, got %v", want, args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("at %d: expected %q, got %q", i, want[i], args[i])
		}
	}
}
