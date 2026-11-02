package mint

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

// Every dividing line (under the title, between panes, above the run bar
// and footer, and Divider rows) is drawn in the theme's accent, dimmed, so
// it changes with the theme. Before, it was dim in the terminal's own
// colour whatever the theme.
func TestDividingLinesFollowTheTheme(t *testing.T) {
	oldC, oldBase := C, basePalette
	t.Cleanup(func() { C, basePalette = oldC, oldBase })

	for _, name := range []string{"default", "nord", "gruvbox"} {
		SetPalette(theme.Load(name))
		cv := &canvas{w: 40}
		paintRule(cv, 3, 40)
		if got := cv.b.String(); !strings.Contains(got, C.Accent+C.Dim+"─") {
			t.Fatalf("%s: the rule is not in the theme's accent: %q", name, got)
		}

		// A Divider row, as drawn on a screen and in a pane.
		scr := &Screen{Title: "T", Sections: []Section{{Rows: []Row{Note("a"), Divider(), Note("b")}}}}
		if frame := scr.FrameText(12, 40); !strings.Contains(frame, "─") {
			t.Fatalf("%s: no divider drawn:\n%s", name, frame)
		}
	}
	// Different themes, different lines.
	SetPalette(theme.Load("default"))
	a := ruleStyle()
	SetPalette(theme.Load("nord"))
	if ruleStyle() == a {
		t.Fatal("default and nord draw the rules the same")
	}
}
