package runner

import (
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// A line with something masked keeps no colours.
func TestMaskedLinesLoseColours(t *testing.T) {
	j := &Job{c: Cmd{MaxLines: 10, Redact: func(s string) string {
		if s == "token=abc" {
			return "token=***"
		}
		return s
	}}}
	j.appendLine("token=abc", []mint.ColourRun{{Start: 6, End: 9, FG: 2}}, false, false)
	j.appendLine("plain", []mint.ColourRun{{Start: 0, End: 5, FG: 3}}, false, false)
	ll := j.Lines()
	if ll[0].Text != "token=***" || ll[0].Colours != nil {
		t.Errorf("masked line: %+v", ll[0])
	}
	if len(ll[1].Colours) != 1 {
		t.Errorf("unmasked line lost its colours: %+v", ll[1])
	}
}
