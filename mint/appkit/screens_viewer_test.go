package appkit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

func rowBool(r mint.Row, name string) bool {
	v := reflect.ValueOf(r).FieldByName(name)
	if !v.IsValid() || v.Kind() != reflect.Bool {
		return false
	}
	return v.Bool()
}

func outputTable(t *testing.T, id, text string) mint.Row {
	t.Helper()
	a := newTestApp(t, ToolSpec{
		Name:        "tool-" + id,
		Bin:         "echo",
		ArchetypeID: id,
		TableOutput: true,
	})
	a.SetSnapshotText(text)
	scr := a.MainScreen()
	if row, ok := findTableRow(scr); ok {
		return row
	}
	t.Fatalf("%s: no table row in %v", id, sectionNames(scr))
	return mint.Row{}
}

func findTableRow(scr *mint.Screen) (mint.Row, bool) {
	for _, s := range scr.Sections {
		for _, r := range s.Rows {
			if rowField(r, "wtype") == "table" {
				return r, true
			}
		}
	}
	for _, p := range scr.Panes {
		if p == nil {
			continue
		}
		for _, s := range p.Sections {
			for _, r := range s.Rows {
				if rowField(r, "wtype") == "table" {
					return r, true
				}
			}
		}
	}
	return mint.Row{}, false
}

func TestWideArchetypesUseTheTableViewer(t *testing.T) {
	const text = "NAME  VALUE\nalpha  one\nbeta   two\n"
	for _, id := range []string{"list", "inspect", "records", "query", "search"} {
		if !rowBool(outputTable(t, id, text), "viewer") {
			t.Errorf("%s: Viewer is off", id)
		}
	}
	if rowBool(outputTable(t, "probe", text), "viewer") {
		t.Error("probe: Viewer is on; only the wide archetypes scroll sideways")
	}
}

// df and ps stay at their natural column widths on a narrow terminal and
// scroll sideways. A table that is not one of those archetypes is squeezed
// to the width instead, and scrolling it changes nothing.
func TestWideOutputScrollsOnANarrowTerminal(t *testing.T) {
	const token = "not-squeeze"
	samples := []string{
		"FILESYSTEM  SIZE  USED  AVAIL  USE  MOUNTED\n" +
			"/dev/sda1   100G  80G   20G    80%  /mnt/data/very-long-mount-point-that-must-scroll-sideways-not-squeeze\n" +
			"tmpfs       8G    1M    8G     1%   /run\n",
		"USER    PID   CPU  MEM  COMMAND\n" +
			"root    1     0.0  0.1  /sbin/init --with /mnt/data/very-long-command-line-that-must-scroll-sideways-not-squeeze\n" +
			"curtis  4242  1.2  3.4  /usr/bin/vim src/mint/screen.go\n",
	}
	const width = 60
	for _, text := range samples {
		viewer := outputTable(t, "list", text)
		fitted := outputTable(t, "probe", text)
		if !rowBool(viewer, "viewer") || rowBool(fitted, "viewer") {
			t.Fatalf("viewer flags list=%v probe=%v", rowBool(viewer, "viewer"), rowBool(fitted, "viewer"))
		}

		first := mint.TableLines(viewer, width, 0)
		firstText := strings.Join(first, "\n")
		if strings.Contains(firstText, token) {
			t.Fatalf("width %d already shows %q; the columns were squeezed in:\n%s", width, token, firstText)
		}
		var scrolled string
		changed := false
		prev := firstText
		for skip := 4; skip <= 160; skip += 4 {
			lines := mint.TableLines(viewer, width, skip)
			got := strings.Join(lines, "\n")
			if got != prev {
				changed = true
			}
			prev = got
			scrolled += got + "\n"
			for _, line := range lines {
				if util.VLen(line) > width {
					t.Errorf("viewer line is %d cells at width %d: %q", util.VLen(line), width, line)
				}
			}
		}
		if !changed {
			t.Fatal("scrolling sideways did not change the viewer table")
		}
		if !strings.Contains(scrolled, token) {
			t.Fatalf("scrolling a width-%d viewer never reached %q:\n%s", width, token, scrolled)
		}

		fit := mint.TableLines(fitted, width, 0)
		fitText := strings.Join(fit, "\n")
		if strings.Contains(fitText, token) {
			t.Fatalf("fitted table kept %q at width %d:\n%s", token, width, fitText)
		}
		for _, line := range fit {
			if util.VLen(line) > width {
				t.Errorf("fitted line is %d cells: %q", util.VLen(line), line)
			}
		}
		if again := strings.Join(mint.TableLines(fitted, width, 40), "\n"); again != fitText {
			t.Fatalf("fitted table scrolled sideways:\n%s\n%s", fitText, again)
		}
	}
}
