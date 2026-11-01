package mint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

// withThemeFile points theme.conf at a temporary file and restores the
// palette afterwards.
func withThemeFile(t *testing.T) string {
	t.Helper()
	oldPath, oldC, oldBase, oldShown := theme.ConfigPath, C, basePalette, shownTheme
	path := filepath.Join(t.TempDir(), "theme.conf")
	theme.ConfigPath = path
	t.Cleanup(func() { theme.ConfigPath, C, basePalette, shownTheme = oldPath, oldC, oldBase, oldShown })
	return path
}

func TestHomeListEndsWithTheme(t *testing.T) {
	a := &App{Items: []HomeItem{{Label: "Run"}}}
	items := a.items()
	if len(items) != 2 || items[1].Label != "Theme" || items[1].Open == nil {
		t.Fatalf("items = %+v", items)
	}
	if len(a.Items) != 1 {
		t.Fatal("items() changed the app's own list")
	}
	if a.findItem("theme") != 2 {
		t.Fatal("--screen Theme does not find the Theme item")
	}
	a.NoThemeItem = true
	if len(a.items()) != 1 {
		t.Fatal("NoThemeItem still adds Theme")
	}
	own := &App{Items: []HomeItem{{Label: "theme"}}}
	if len(own.items()) != 1 {
		t.Fatal("an app's own Theme item got a second one")
	}
}

func TestThemeScreenPreviewsSavesAndRestores(t *testing.T) {
	path := withThemeFile(t)
	SetPalette(theme.Load("default"))
	shownTheme = ""
	start := C

	scr := ThemeScreen()
	radio := scr.Sections[0].Rows[1]
	var nordLabel string
	for _, o := range radio.options {
		if strings.HasPrefix(o, "nord ") {
			nordLabel = o
		}
	}
	radio.setStr(nordLabel)
	if C == start {
		t.Fatal("choosing nord did not show it")
	}
	if radio.getStr() != nordLabel {
		t.Fatalf("radio shows %q", radio.getStr())
	}
	// Leaving without saving puts the old colours back and saves nothing.
	scr.onLeave()
	if C != start {
		t.Fatal("leaving without saving kept the preview")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a preview wrote theme.conf")
	}

	// Saving keeps it.
	scr = ThemeScreen()
	scr.Sections[0].Rows[1].setStr(nordLabel)
	nord := C
	scr.Sections[1].Rows[0].onActivate(&Ctx{})
	if theme.Current() != "nord" {
		t.Fatalf("saved theme = %q", theme.Current())
	}
	scr.onLeave()
	if C != nord {
		t.Fatal("leaving after saving dropped the saved theme")
	}
}

func TestThemeFlag(t *testing.T) {
	cases := []struct {
		args []string
		name string
		rest int
		ok   bool
	}{
		{[]string{"--theme", "nord"}, "nord", 0, true},
		{[]string{"--theme=dracula", "--screen", "Run"}, "dracula", 2, true},
		{[]string{"--theme"}, "", 0, true},
		{[]string{"--screen", "Run"}, "", 2, false},
	}
	for _, c := range cases {
		name, rest, ok := themeFlag(c.args)
		if name != c.name || len(rest) != c.rest || ok != c.ok {
			t.Errorf("themeFlag(%q) = %q %q %v", c.args, name, rest, ok)
		}
	}
}

func TestThemeFlagAppliesForOneRunThenOtherFlags(t *testing.T) {
	path := withThemeFile(t)
	SetPalette(theme.Load("default"))
	shownTheme = ""
	start := C
	a := &App{Items: []HomeItem{{Label: "Run"}}}
	done, item := a.handleArgs([]string{"--theme", "gruvbox", "--screen", "Run"}, "◈ T", "t", "")
	if done || item != 1 {
		t.Fatalf("done=%v item=%d", done, item)
	}
	if C == start {
		t.Fatal("--theme did not apply")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("--theme saved the theme")
	}
	if knownTheme("../../etc/passwd") || applyTheme("nope") {
		t.Fatal("an unknown theme name was accepted")
	}
}

func TestHelpListsThemeFlag(t *testing.T) {
	a := &App{Items: []HomeItem{{Label: "Run"}}}
	out := captureStdout(t, func() { a.handleArgs([]string{"--help"}, "◈ T", "t", "") })
	if !strings.Contains(out, "--theme NAME") || !strings.Contains(out, "Theme") {
		t.Fatalf("help:\n%s", out)
	}
}

func TestThemeScreenShowsTheFlagTheme(t *testing.T) {
	withThemeFile(t)
	if !applyTheme("dracula") {
		t.Fatal("dracula")
	}
	scr := ThemeScreen()
	if got := scr.Sections[0].Rows[1].getStr(); !strings.HasPrefix(got, "dracula ") {
		t.Fatalf("selected %q, want dracula (from --theme)", got)
	}
	if hint := scr.Sections[1].Rows[0].getStr(); hint != "not saved" {
		t.Fatalf("save hint %q", hint)
	}
}
