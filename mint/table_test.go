package mint

import (
	"strings"
	"testing"
)

func TestParseTableNmapAndWhitespace(t *testing.T) {
	nmapOut := []string{
		"PORT     STATE SERVICE VERSION",
		"22/tcp   open  ssh     OpenSSH 9.6p1 Ubuntu",
		"80/tcp   open  http    nginx 1.24.0",
		"443/tcp  open  ssl/http nginx 1.24.0",
	}
	hdrs, rows := ParseTable(nmapOut)
	if len(hdrs) != 4 || hdrs[0] != "PORT" || hdrs[3] != "VERSION" {
		t.Fatalf("nmap headers = %v", hdrs)
	}
	if len(rows) != 3 || rows[0][0] != "22/tcp" || rows[0][3] != "OpenSSH 9.6p1 Ubuntu" {
		t.Fatalf("nmap rows = %v", rows)
	}
}

func TestParseTablePipeAndBoxDrawing(t *testing.T) {
	boxOut := []string{
		"┌────────┬────────┬───────┐",
		"│ Name   │ Status │ Count │",
		"├────────┼────────┼───────┤",
		"│ alpha  │ ready  │ 12    │",
		"│ beta   │ active │ 4     │",
		"└────────┴────────┴───────┘",
	}
	hdrs, rows := ParseTable(boxOut)
	if len(hdrs) != 3 || hdrs[0] != "Name" || hdrs[1] != "Status" || hdrs[2] != "Count" {
		t.Fatalf("box headers = %v", hdrs)
	}
	if len(rows) != 2 || rows[1][0] != "beta" || rows[1][2] != "4" {
		t.Fatalf("box rows = %v", rows)
	}
}

func TestTableRowExpansion(t *testing.T) {
	var selectedIdx int = -1
	var selectedRow []string
	tbl := Table(TableOpts{
		Headers: []string{"PORT", "STATE", "SERVICE"},
		Rows: [][]string{
			{"22/tcp", "open", "ssh"},
			{"80/tcp", "open", "http"},
		},
		OnSelect: func(_ *Ctx, idx int, row []string) {
			selectedIdx = idx
			selectedRow = row
		},
	})
	st := &uiState{row: 1, collapsed: map[string]bool{}}
	items := st.flatten([]Section{{Rows: []Row{tbl}}}, 80, 0)
	// Header label + rule note + 2 data rows = 4 items
	if len(items) != 4 {
		t.Fatalf("expanded table items = %d, want 4", len(items))
	}
	if !strings.Contains(items[0].label, "PORT") || !strings.Contains(items[0].label, "SERVICE") {
		t.Fatalf("header label = %q", items[0].label)
	}
	if items[2].wtype != "tablerow" || !items[2].isSelectable() {
		t.Fatalf("data row 0 = %+v", items[2])
	}
	items[3].onActivate(&Ctx{})
	if selectedIdx != 1 || len(selectedRow) != 3 || selectedRow[0] != "80/tcp" {
		t.Fatalf("OnSelect got idx=%d row=%v", selectedIdx, selectedRow)
	}
}
