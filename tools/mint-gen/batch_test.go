package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Help texts here are made up for the tests: no program's real help is
// committed.

const zzMonManWithBatch = `ZZMON(1)    User Commands    ZZMON(1)

NAME
       zzmon - show processes

SYNOPSIS
       zzmon [OPTIONS]

OPTIONS
       -b, --batch
              Batch mode: print plain text to standard output.
       -n NUM
              Number of iterations before exiting.
       -d SECS
              Delay between updates.

KEY BINDINGS
       q      quit
`

func TestKnownBatchModes(t *testing.T) {
	b, ok := findBatchMode("top", appkit.ToolSpec{})
	if !ok || !b.Poll || strings.Join(b.Args, " ") != "-b -n 1" {
		t.Errorf("top: %+v %v", b, ok)
	}
	b, ok = findBatchMode("lynx", appkit.ToolSpec{})
	if !ok || b.Poll || strings.Join(b.Args, " ") != "-dump" {
		t.Errorf("lynx: %+v %v", b, ok)
	}
}

func TestBatchModeFromOptions(t *testing.T) {
	noPackages(t)
	spec := ParseManAndHelp("zzmon", zzMonManWithBatch, "")
	if spec.FullScreen {
		t.Error("a full-screen program with a batch mode should run in it, not be marked full-screen")
	}
	if strings.Join(spec.BatchArgs, " ") != "-b -n 1" {
		t.Errorf("batch args %q", spec.BatchArgs)
	}
	if spec.ArchetypeID != "watch" || !spec.ReadOnly {
		t.Errorf("one round and exit is a read-only live view: %s, read-only %v", spec.ArchetypeID, spec.ReadOnly)
	}
	for _, f := range spec.Flags {
		if f.Flag == "-b" || f.Flag == "-n" {
			t.Errorf("batch option %s is still on the form", f.Flag)
		}
	}
	// Without a way to stop after a round, it runs once and is not polled.
	once := strings.Replace(zzMonManWithBatch, "       -n NUM\n              Number of iterations before exiting.\n", "", 1)
	spec = ParseManAndHelp("zzmon", once, "")
	if strings.Join(spec.BatchArgs, " ") != "-b" || spec.ArchetypeID == "watch" && spec.ReadOnly {
		t.Errorf("batch without rounds: args %q, archetype %s, read-only %v", spec.BatchArgs, spec.ArchetypeID, spec.ReadOnly)
	}
}

// A plain program is never put in batch mode, whatever its options.
func TestBatchModeOnlyForFullScreen(t *testing.T) {
	noPackages(t)
	spec := ParseManAndHelp("zzzip", "", "Usage: zzzip [OPTIONS] FILE\nCompress files.\n\nOptions:\n  -c, --stdout   write to standard output\n")
	if len(spec.BatchArgs) > 0 {
		t.Errorf("a compressor's --stdout is not a batch mode: %q", spec.BatchArgs)
	}
}

func TestScaffoldWritesBatchArgs(t *testing.T) {
	b, err := renderSpecGo(appkit.ToolSpec{Name: "zz", Title: "zz", BatchArgs: []string{"-b", "-n", "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `BatchArgs:`) || !strings.Contains(string(b), `"-n"`) {
		t.Errorf("scaffold output lacks BatchArgs:\n%s", b)
	}
}

// "Print version and exit" is not a batch mode.
func TestVersionIsNotBatch(t *testing.T) {
	spec := appkit.ToolSpec{Flags: []appkit.FlagSpec{{Flag: "-v", Kind: appkit.FlagBool, Label: "Print version information and exit."}}}
	if b, ok := findBatchMode("zzview", spec); ok {
		t.Errorf("version option taken as batch mode: %+v", b)
	}
}

// An option that only applies within batch mode does not turn it on.
func TestInBatchModeIsNotBatch(t *testing.T) {
	spec := appkit.ToolSpec{Flags: []appkit.FlagSpec{{Flag: "-p", Kind: appkit.FlagBool, Label: "Do not show progress in non-interactive mode"}}}
	if b, ok := findBatchMode("zzdu", spec); ok {
		t.Errorf("a within-mode option taken as batch mode: %+v", b)
	}
	if _, ok := findBatchMode("neomutt", appkit.ToolSpec{Flags: []appkit.FlagSpec{{Flag: "-B", Kind: appkit.FlagBool, Label: "Run in batch mode"}}}); ok {
		t.Error("a mailer's batch mode sends mail: not a batch mode for Mint")
	}
}
