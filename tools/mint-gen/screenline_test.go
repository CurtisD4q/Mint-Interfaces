package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestInterfaceTypeLine(t *testing.T) {
	suite := appkit.ToolSpec{Name: "box", Bin: "box", ArchetypeID: "toolkit", Subcommands: []appkit.SubcommandSpec{{Name: "run"}}}
	if got := interfaceTypeLine(suite); !strings.HasPrefix(got, "Command suite (command-suite), because it has commands") || strings.Contains(got, "classic") {
		t.Errorf("suite: %q", got)
	}
	mon := appkit.ToolSpec{Name: "free", Bin: "free", ArchetypeID: "watch", ReadOnly: true, StatusArgs: []string{"-m"}, PollInterval: 2 * time.Second, TableOutput: true}
	if got := interfaceTypeLine(mon); !strings.HasPrefix(got, "Live monitor (live-monitor)") {
		t.Errorf("monitor: %q", got)
	}
	plain := appkit.ToolSpec{Name: "calc", Bin: "calc", ArchetypeID: "ask"}
	if got := interfaceTypeLine(plain); !strings.Contains(got, "Quick answer") || !strings.Contains(got, "classic screen for now") {
		t.Errorf("quick answer: %q", got)
	}
	t.Setenv("MINT_CLASSIC_LAYOUT", "1")
	if got := interfaceTypeLine(suite); !strings.Contains(got, "MINT_CLASSIC_LAYOUT=1") {
		t.Errorf("override not mentioned: %q", got)
	}
}

func TestPrintInterfaceTypeIsCleaned(t *testing.T) {
	var b bytes.Buffer
	printInterfaceType(&b, appkit.ToolSpec{Name: "x\x1b[31m", Bin: "x"}, "  ")
	if strings.Contains(b.String(), "\x1b") || !strings.HasPrefix(b.String(), "  Screen:    ") {
		t.Errorf("%q", b.String())
	}
}
