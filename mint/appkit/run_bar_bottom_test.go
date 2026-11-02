package appkit

import (
	"regexp"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

var reANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// screenRows draws s and returns each terminal row as plain text.
func screenRows(s *mint.Screen, h, w int) []string {
	rows := make([]string, h+1)
	raw := s.FrameText(h, w)
	re := regexp.MustCompile(`\x1b\[(\d+);(\d+)H`)
	locs := re.FindAllStringSubmatchIndex(raw, -1)
	for i, l := range locs {
		var row int
		for _, c := range raw[l[2]:l[3]] {
			row = row*10 + int(c-'0')
		}
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		if row >= 1 && row <= h {
			rows[row] += reANSI.ReplaceAllString(raw[l[1]:end], "")
		}
	}
	return rows
}

// For every archetype, at every height a Mint app draws at, the section
// with the command preview and the run (apply, or poll) button is the
// bottom pane: both rows are on screen, and nothing from the panes above
// is drawn below them.
func TestRunSectionIsAlwaysTheBottomPane(t *testing.T) {
	type tc struct {
		name string
		scr  *mint.Screen
	}
	var cases []tc
	for _, def := range AllArchetypeDefs() {
		for _, advanced := range []bool{false, true} {
			cases = append(cases, tc{string(def.ID), screenFor(t, def, advanced)})
		}
	}
	// A live view that really polls, with the Live section at the bottom.
	live := newTestApp(t, ToolSpec{Name: "free", Bin: "echo", ArchetypeID: "watch", ReadOnly: true,
		Flags: []FlagSpec{{Flag: "-b", Kind: FlagBool, Label: "Bytes", Section: "Options"}}})
	cases = append(cases, tc{"watch (polling)", live.MainScreen()})

	buttons := regexp.MustCompile(`(\brun$|\bapply$|^Poll now)`)
	for _, c := range cases {
		for _, h := range []int{12, 13, 14, 16, 20, 24, 30, 48} {
			rows := screenRows(c.scr, h, 100)
			preview, button, lastPane := 0, 0, 0
			for i, r := range rows {
				switch {
				case strings.Contains(r, "Command preview"):
					preview = i
				case buttons.MatchString(strings.Trim(r, "│ ")) && preview > 0 && button == 0:
					button = i
				}
				if strings.Contains(r, "Basic Options") || strings.Contains(r, "Advanced + Additional") || strings.Contains(r, "Output") && !strings.Contains(r, "preview") {
					lastPane = i
				}
			}
			if preview == 0 || button == 0 {
				t.Errorf("%s at %d rows: preview row %d, button row %d\n%s", c.name, h, preview, button, strings.Join(rows, "\n"))
				continue
			}
			if lastPane > preview {
				t.Errorf("%s at %d rows: a pane is drawn below the run section (row %d > %d)", c.name, h, lastPane, preview)
			}
			for _, p := range c.scr.Panes {
				for _, sec := range p.Sections {
					if sec.Name == "Run" || sec.Name == "Apply" || sec.Name == "Live" {
						t.Errorf("%s: run section %q inside pane %q", c.name, sec.Name, p.Title)
					}
				}
			}
		}
	}
}
