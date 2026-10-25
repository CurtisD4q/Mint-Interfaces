package mint

import (
	"fmt"
	"strings"
	"testing"
)

func TestCollapsibleFlattenAndOpenPtr(t *testing.T) {
	open := false
	sec := Section{
		Name: "Main",
		Rows: []Row{
			Collapsible(CollapsibleOpts{
				ID:    "adv",
				Title: "Advanced Flags",
				Open:  &open,
				Rows: []Row{
					Checkbox(CheckboxOpts{Label: "Verbose"}),
					Text(TextOpts{Label: "Port"}),
				},
			}),
		},
	}
	st := &uiState{row: 1, collapsed: map[string]bool{}}

	// Initially collapsed (open == false): only Main section header + Collapsible header
	items := st.flatten([]Section{sec}, 80, 0)
	if len(items) != 2 {
		t.Fatalf("collapsed items len = %d, want 2", len(items))
	}
	if !items[1].collapsible || !items[1].collapsed || items[1].hidden != 2 {
		t.Fatalf("header item = %+v", items[1])
	}

	// Open via pointer
	open = true
	items = st.flatten([]Section{sec}, 80, 0)
	if len(items) != 4 {
		t.Fatalf("expanded items len = %d, want 4", len(items))
	}
	if items[1].collapsed {
		t.Fatalf("expected header to be unfolded")
	}
}

func TestCollapsibleLargeTextChunking(t *testing.T) {
	var rawLines []string
	for i := 1; i <= 250; i++ {
		rawLines = append(rawLines, fmt.Sprintf("line %d of output", i))
	}
	open := true
	row := Collapsible(CollapsibleOpts{
		ID:        "big",
		Title:     "Large Output",
		Open:      &open,
		Lines:     func() []string { return rawLines },
		ChunkSize: 100,
	})
	st := &uiState{row: 1, collapsed: map[string]bool{}}
	items := st.flatten([]Section{{Rows: []Row{row}}}, 80, 0)

	// Should contain:
	// 0: "Large Output" collapsible header (badge "250 lines")
	// 1: "Open full text in viewer" action
	// 2: "Lines 1–100" collapsed chunk header
	// 3: "Lines 101–200" collapsed chunk header
	// 4: "Lines 201–250" collapsed chunk header
	if len(items) != 5 {
		t.Fatalf("chunked items len = %d, want 5", len(items))
	}
	if items[0].badge != "250 lines" {
		t.Fatalf("top badge = %q, want %q", items[0].badge, "250 lines")
	}
	if !strings.HasPrefix(items[2].name, "Lines 1") || !items[2].collapsed {
		t.Fatalf("chunk 1 = %+v", items[2])
	}
}
