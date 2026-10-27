package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func archOf(t *testing.T, name, man, help string) ArchetypeChoice {
	t.Helper()
	return chooseArchetypeFor(programTextFrom(name, man, help))
}

// Each behaviour cue points to its archetype on its own, with no words
// to help.
func TestArchetypeBehaviourCues(t *testing.T) {
	for _, c := range []struct {
		name, help string
		want       appkit.ArchetypeID
	}{
		{"x1", "Usage: x1 [options] HOST...\n", "probe"},
		{"x2", "Usage: x2 [options] URL\n", "transfer"},
		{"x3", "Usage: x3 [options] PATTERN [FILE]...\n", "search"},
		{"x4", "Usage: x4 [options] FILE1 FILE2\n", "compare"},
		{"x5", "Usage: x5 [options] COMMAND [ARG]...\n", "wrap"},
		{"x6", "Usage: x6 [options] [delay [count]]\n", "watch"},
		{"x7", "Usage: x7 [options] TIME\n", "schedule"},
		{"x8", "Usage: x8 [options] PAGE\n", "read"},
		{"x9", "Usage: x9 [options] KEY=VALUE\n", "settings"},
		{"x10", "Usage: x10 [options]\n  -f, --follow   output appended data as the file grows\n", "follow"},
		{"x11", "Usage: x11 [options]\n  -l, --listen ADDR   listen on ADDR and serve files\n", "serve"},
		{"x12", "Commands:\n  start   Start it\n  stop    Stop it\n  status  Show status\n  restart Restart it\n", "control"},
		{"x13", "Commands:\n  install  Install a package\n  remove   Remove a package\n  list     List packages\n", "records"},
		{"x14", "Commands:\n  commit   Record changes\n  log      Show history\n  checkout Switch\n", "history"},
		{"x15", "Commands:\n  alpha    Do alpha\n  beta     Do beta\n  gamma    Do gamma\n", "toolkit"},
	} {
		if got := archOf(t, c.name, "", c.help); got.ID != c.want {
			t.Errorf("%s: %s, want %s\n%v", c.name, got.ID, c.want, got.Scores[:3])
		}
	}
}

// A COMMAND in the usage of a program with subcommands is one of them,
// not a command the program runs.
func TestCommandOfASuiteIsNotWrap(t *testing.T) {
	// Subcommands of no particular kind (a toolkit), so only the rule keeps
	// "COMMAND [ARGS]" from counting as a wrapped command.
	help := "Usage: suite9 COMMAND [ARGS]...\n\nCommands:\n  alpha    Do alpha\n  beta     Do beta\n  gamma    Do gamma\n"
	if got := archOf(t, "suite9", "", help); got.ID == "wrap" {
		t.Fatalf("a suite taken for a wrapper: %v", got.Scores[:3])
	}
}

// A summary that lists plural things lists ("show the logical volumes");
// one that reports on a single item inspects ("display information about
// an object file"). Where a summary says both ("display information about
// logical volumes") the two can tie, and the choice is a close call.
func TestListsOverInspects(t *testing.T) {
	if got := archOf(t, "vols9", "NAME\n       vols9 - show the logical volumes\n", ""); got.ID != "list" {
		t.Errorf("plural: %s %v", got.ID, got.Scores[:3])
	}
	// Both readings in one summary: the list one wins, not both.
	if got := archOf(t, "mounts9", "NAME\n       mounts9 - show details of the mounted volumes\n", ""); got.ID != "list" {
		t.Errorf("both: %s %v", got.ID, got.Scores[:3])
	}
	if got := archOf(t, "elf9", "NAME\n       elf9 - display information about an object file\n", ""); got.ID != "inspect" {
		t.Errorf("single: %s %v", got.ID, got.Scores[:3])
	}
}

// Common words do not decide alone: "display", "process", "scan" in a long
// body are weak.
func TestCommonWordsAreWeakInTheBody(t *testing.T) {
	body := strings.Repeat("display the process list and scan the server log. ", 30)
	c := archOf(t, "plain9", "NAME\n       plain9 - convert images between formats\n\nOPTIONS\n       "+body, "")
	if c.ID != "convert" {
		t.Errorf("%s %v", c.ID, c.Scores[:4])
	}
}

// What a program's help is about can raise its data level, never lower it.
func TestTopicRaisesDataLevel(t *testing.T) {
	spec := ParseManAndHelp("keytool9", "NAME\n       keytool9 - encrypt and decrypt files with OpenPGP keys and signatures\n", "")
	if spec.Data != appkit.DataSecret {
		t.Errorf("keys: %q (%s)", spec.Data, spec.ArchetypeID)
	}
	// pass is classified on its own as secret; help about logs (sensitive)
	// does not lower that.
	if spec := ParseManAndHelp("pass", "NAME\n       pass - read system log messages from the journal and syslog\n", ""); spec.Data != appkit.DataSecret {
		t.Errorf("pass: %q", spec.Data)
	}
	if spec := ParseManAndHelp("calc9", "NAME\n       calc9 - an arbitrary precision calculator\n", ""); spec.Data != "" && spec.Data != appkit.DataNormal {
		t.Errorf("calculator: %q", spec.Data)
	}
}

// Every archetype has phrases, and every phrase names a real archetype.
func TestArchetypeSignalsCoverAll(t *testing.T) {
	for _, a := range appkit.AllArchetypeDefs() {
		if a.ID != "general" && len(archetypeSignals[string(a.ID)]) == 0 {
			t.Errorf("%s has no phrases", a.ID)
		}
	}
	for id := range archetypeSignals {
		if _, ok := appkit.LookupArchetype(id); !ok {
			t.Errorf("phrases for unknown archetype %q", id)
		}
	}
}
