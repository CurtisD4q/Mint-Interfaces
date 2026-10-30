package mint

import "testing"

// ViewLog's windowing used to count characters once panned, so wide text
// overflowed the pane. It now measures cells, through util.SliceCols.
func TestViewLogPaneIsWidthCorrect(t *testing.T) {
	for _, s := range []string{
		"日本語のテキストです",
		"mixed 日本 text",
		"e\u0301 combining",
		"👨\u200d👩\u200d👧 emoji",
	} {
		for _, off := range []int{0, 1, 2, 5} {
			for _, w := range []int{2, 4, 10} {
				if got := VLen(sliceCols(s, off, w)); got > w {
					t.Errorf("sliceCols(%q, %d, %d) is %d cells, want at most %d", s, off, w, got, w)
				}
			}
		}
	}
}
