package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Help texts here are made up for the tests: no program's real help is
// committed.

func noPackages(t *testing.T) {
	t.Helper()
	old := readPackages
	readPackages = false
	t.Cleanup(func() { readPackages = old })
}

func TestFullScreenSignal(t *testing.T) {
	noPackages(t)
	man := "ZZVIEW(1)    User Commands    ZZVIEW(1)\n\nNAME\n       zzview - view files\n\nSYNOPSIS\n       zzview [FILE]\n\nKEY BINDINGS\n       q      quit\n"
	spec := ParseManAndHelp("zzview", man, "")
	if !spec.FullScreen {
		t.Error("a man page with a KEY BINDINGS section is full-screen")
	}
	spec = ParseManAndHelp("zzmenu", "", "Usage: zzmenu [OPTIONS]\nA terminal user interface for menus.\n")
	if !spec.FullScreen {
		t.Error("a summary saying terminal user interface is full-screen")
	}
	spec = ParseManAndHelp("zzcat", "", "Usage: zzcat [FILE]\nPrint files.\n\nOptions:\n  -n   number lines\n")
	if spec.FullScreen {
		t.Error("a plain filter is not full-screen")
	}
}

func TestChangesSignalAddsCaution(t *testing.T) {
	noPackages(t)
	for _, h := range []string{
		"Usage: zzpkg COMMAND\nInstall and remove packages.\n",
		"Usage: zzlocale [OPTIONS]\nQuery or change the system locale.\n",
		"Usage: zzwipe FILE\nOverwrite a file to hide its contents.\n",
	} {
		spec := ParseManAndHelp("zzx", "", h)
		if !spec.ConfirmRun || spec.ReadOnly {
			t.Errorf("%q: confirm %v, read-only %v", strings.SplitN(h, "\n", 3)[1], spec.ConfirmRun, spec.ReadOnly)
		}
	}
	for _, h := range []string{
		"Usage: zzed FILE\nA stream editor for text.\n",
		"Usage: zzcp SOURCE DEST\nCopy files and directories.\n",
	} {
		if readSignals("zzy", "", h).Changes {
			t.Errorf("%q: plain file work is not a clear change", strings.SplitN(h, "\n", 3)[1])
		}
	}
}

// A read-only setting a person checked is never undone by a signal.
func TestCheckedReadOnlyKept(t *testing.T) {
	var name string
	for _, n := range appkit.ArchetypeToolNames() {
		if appkit.IsReadOnlyTool(n) {
			name = n
			break
		}
	}
	if name == "" {
		t.Skip("no checked read-only tool")
	}
	spec := appkit.ToolSpec{Name: name, ReadOnly: true}
	applySignals(&spec, behaviourSignals{Changes: true, ChangesWhy: "test"})
	if !spec.ReadOnly || !spec.ConfirmRun {
		t.Errorf("%s: read-only %v, confirm %v", name, spec.ReadOnly, spec.ConfirmRun)
	}
}

func TestSignalNotes(t *testing.T) {
	notes := signalNotes("list", behaviourSignals{Changes: true, ChangesWhy: "its summary says install"})
	if len(notes) != 1 || !strings.Contains(notes[0], "only reads") {
		t.Errorf("list that changes things: %v", notes)
	}
	notes = signalNotes("toolkit", behaviourSignals{Changes: true, ChangesWhy: "its summary says install"})
	if len(notes) != 1 || strings.Contains(notes[0], "only reads") {
		t.Errorf("toolkit that changes things: %v", notes)
	}
	notes = signalNotes("read", behaviourSignals{MaybeFullScreen: true, FullScreenWhy: "its package uses ncurses"})
	if len(notes) != 1 || !strings.Contains(notes[0], "may draw") {
		t.Errorf("maybe full-screen: %v", notes)
	}
	var b bytes.Buffer
	printSignalNotes(&b, []string{"one\x1b[31m", "two"})
	if strings.Contains(b.String(), "\x1b") || !strings.Contains(b.String(), "Note:") {
		t.Errorf("notes must be printed safely: %q", b.String())
	}
}

// Prompt-style programs (nslookup, fdisk) have an INTERACTIVE COMMANDS
// section but run fine one-shot: not full-screen.
func TestInteractiveCommandsIsNotFullScreen(t *testing.T) {
	noPackages(t)
	man := "ZZLOOK(1)    User Commands    ZZLOOK(1)\n\nNAME\n       zzlook - query name servers\n\nINTERACTIVE COMMANDS\n       server NAME\n"
	if readSignals("zzlook", man, "").FullScreen {
		t.Error("an INTERACTIVE COMMANDS section alone is not full-screen")
	}
}

func TestScaffoldWritesFullScreen(t *testing.T) {
	b, err := renderSpecGo(appkit.ToolSpec{Name: "zz", Title: "zz", FullScreen: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "FullScreen:        true") {
		t.Error("scaffold output lacks FullScreen")
	}
}
