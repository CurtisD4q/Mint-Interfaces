package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

// mint-gen --theme NAME saves the theme for every Mint app, and builds
// nothing into an app.
func TestThemeFlagSetsTheThemeForEveryApp(t *testing.T) {
	old := theme.ConfigPath
	theme.ConfigPath = filepath.Join(t.TempDir(), "theme.conf")
	t.Cleanup(func() { theme.ConfigPath = old })

	if code := runCLI([]string{"--theme", "Nord"}); code != 0 {
		t.Fatalf("mint-gen --theme Nord gave exit %d", code)
	}
	if got := theme.Current(); got != "nord" {
		t.Fatalf("saved theme %q, want nord", got)
	}
	if code := runCLI([]string{"--theme", "solarized"}); code != 2 {
		t.Fatalf("an unknown theme gave exit %d, want 2", code)
	}
	if got := theme.Current(); got != "nord" {
		t.Fatalf("an unknown theme changed the saved one to %q", got)
	}
	if _, err := os.Stat(theme.ConfigPath); err != nil {
		t.Fatal(err)
	}
	src, _ := renderSpecGo(appkit.ToolSpec{Name: "ls"})
	if strings.Contains(string(src), "Theme") {
		t.Fatal("an app's spec names a theme")
	}
	if !knownThemeName("catppuccin") || knownThemeName("../etc") {
		t.Fatal("knownThemeName")
	}
}
