package main

import (
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
