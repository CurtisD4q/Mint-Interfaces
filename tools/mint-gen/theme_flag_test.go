package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestThemeFlagIsCheckedAndWritten(t *testing.T) {
	if code := runCLI([]string{"--theme", "solarized", "ls"}); code != 2 {
		t.Fatalf("an unknown theme gave exit %d, want 2", code)
	}
	src, err := renderSpecGo(appkit.ToolSpec{Name: "ls", Theme: "nord"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `Theme:             "nord",`) {
		t.Fatalf("the spec does not carry the theme:\n%s", src)
	}
	src, _ = renderSpecGo(appkit.ToolSpec{Name: "ls"})
	if strings.Contains(string(src), "Theme:") {
		t.Fatal("a spec with no theme names one")
	}
	if !knownThemeName("catppuccin") || knownThemeName("../etc") {
		t.Fatal("knownThemeName")
	}
}
