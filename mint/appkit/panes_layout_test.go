package appkit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/fileplan"
)

func assertPaneCap(t *testing.T, s *mint.Screen) {
	t.Helper()
	if len(s.Sections) > maxPaneSections {
		t.Fatalf("top pane has %d sections", len(s.Sections))
	}
	for _, p := range s.Panes {
		if p == nil {
			continue
		}
		if len(p.Sections) > maxPaneSections {
			t.Fatalf("pane %q has %d sections", p.Title, len(p.Sections))
		}
		if len(p.Sections) == 0 {
			t.Fatalf("pane %q is empty", p.Title)
		}
	}
}

func assertPaneTitles(t *testing.T, s *mint.Screen, want ...string) {
	t.Helper()
	assertPaneCap(t, s)
	got := paneTitles(s)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("panes\n got %v\nwant %v", got, want)
	}
}

func flagHeadings(n int) []FlagSpec {
	var fs []FlagSpec
	for i := 0; i < n; i++ {
		fs = append(fs, FlagSpec{
			Flag: fmt.Sprintf("--g%d", i), Kind: FlagBool, Label: fmt.Sprintf("G%d", i),
			Section: fmt.Sprintf("Heading %d", i),
		})
	}
	return fs
}

func TestArchetypePaneLayouts(t *testing.T) {
	cases := []struct {
		id   string
		spec ToolSpec
		want []string
	}{
		{"inspect", ToolSpec{Name: "file", Bin: "echo", ArchetypeID: "inspect"}, []string{"Report", "Help", "Details", "Actions"}},
		{"read", ToolSpec{Name: "man", Bin: "echo", ArchetypeID: "read"}, []string{"Page"}},
		{"probe", ToolSpec{Name: "nmap", Bin: "echo", ArchetypeID: "probe"}, []string{"Discovered targets", "Help"}},
		{"audit", ToolSpec{Name: "shellcheck", Bin: "echo", ArchetypeID: "audit", Findings: &FindingsSpec{Adapter: FindingsLines}}, []string{"Findings", "Help"}},
		{"convert", ToolSpec{Name: "ffmpeg", Bin: "echo", ArchetypeID: "convert"}, []string{"Help"}},
		{"toolkit", ToolSpec{Name: "docker", Bin: "echo", ArchetypeID: "toolkit", Subcommands: []SubcommandSpec{{Name: "ps", Label: "Ps", Desc: "List"}}}, []string{"Output", "Help"}},
		{"transfer", ToolSpec{Name: "curl", Bin: "echo", ArchetypeID: "transfer"}, []string{"Progress", "Help"}},
		{"mirror", ToolSpec{Name: "rsync", Bin: "echo", ArchetypeID: "mirror"}, []string{"What will change", "Help"}},
		{"measure", ToolSpec{Name: "fio", Bin: "echo", ArchetypeID: "measure"}, []string{"Options"}},
		{"control", ToolSpec{Name: "systemctl", Bin: "echo", ArchetypeID: "control"}, []string{"Details"}},
		{"history", ToolSpec{Name: "git", Bin: "echo", ArchetypeID: "history"}, []string{"Details"}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			s := newTestApp(t, tc.spec).MainScreen()
			assertPaneTitles(t, s, tc.want...)
		})
	}
}

func TestOptionHeadingsChunkAtTen(t *testing.T) {
	spec := ToolSpec{Name: "nmap", Bin: "echo", ArchetypeID: "probe", Flags: flagHeadings(25)}
	s := newTestApp(t, spec).MainScreen()
	assertPaneCap(t, s)
	got := paneTitles(s)
	// 25 headings do not fit with the chrome, so Options panes precede the results.
	if len(got) < 3 || got[0] != "Options" || got[len(got)-2] != "Discovered targets" || got[len(got)-1] != "Help" {
		t.Fatalf("chunked panes %v", got)
	}
	for _, title := range got[:len(got)-2] {
		if title != "Options" {
			t.Fatalf("unexpected pane %q in %v", title, got)
		}
	}
}

func TestReportBlocksChunkAtTen(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 25; i++ {
		fmt.Fprintf(&b, "SECTION %d\n    line\n", i)
	}
	a := newTestApp(t, ToolSpec{Name: "file", Bin: "echo", ArchetypeID: "inspect"})
	a.SetSnapshotText(b.String())
	s := a.MainScreen()
	assertPaneCap(t, s)
	var reports int
	for _, p := range s.Panes {
		if p.Title == "Report" {
			reports++
			if len(p.Sections) == 0 || len(p.Sections) > 10 {
				t.Fatalf("report pane %d sections", len(p.Sections))
			}
		}
	}
	if reports < 3 {
		t.Fatalf("25 heading blocks produced %d report panes", reports)
	}
}

func TestTransferAndMirrorPanesHoldOutput(t *testing.T) {
	for _, tc := range []struct {
		id, title, text string
	}{
		{"transfer", "Progress", "downloaded 10 files"},
		{"mirror", "What will change", "copied 3 paths"},
	} {
		a := newTestApp(t, ToolSpec{Name: tc.id, Bin: "echo", ArchetypeID: tc.id})
		a.SetSnapshotText(tc.text)
		s := a.MainScreen()
		var found bool
		for _, p := range s.Panes {
			if p.Title != tc.title {
				continue
			}
			found = true
			if len(p.Sections) == 0 {
				t.Fatalf("%s pane empty", tc.title)
			}
			blob := sectionBlob(p.Sections)
			if !strings.Contains(blob, tc.text) && !strings.Contains(blob, "Output") && !strings.Contains(blob, "Changes") && !strings.Contains(blob, "Progress") {
				t.Fatalf("%s pane did not attach output:\n%s", tc.title, blob)
			}
		}
		if !found {
			t.Fatalf("missing %s pane in %v", tc.title, paneTitles(s))
		}
	}
}

func TestEditFilesPreviewSplitsFiles(t *testing.T) {
	p, _ := KnownPreviewSpec("cp")
	a := newTestApp(t, ToolSpec{Name: "cp", Bin: "echo", ArchetypeID: "edit-files", Preview: &p})
	plan := fileplan.Plan{Tool: "cp", Effects: []fileplan.Effect{
		{Kind: fileplan.Create, Path: "/tmp/a"},
		{Kind: fileplan.Create, Path: "/tmp/b"},
	}}
	scr := a.PreviewScreen(plan, func(*mint.Ctx, fileplan.Plan) {})
	assertPaneTitles(t, scr, "Files")
	var names []string
	for _, s := range scr.Sections {
		names = append(names, s.Name)
	}
	if strings.Join(names, "|") != "Preview|Actions" {
		t.Fatalf("preview top %v", names)
	}
	if len(scr.Panes[0].Sections) == 0 || !strings.Contains(scr.Panes[0].Sections[0].Name, "Created") {
		t.Fatalf("files pane %+v", scr.Panes[0].Sections)
	}
}

func sectionBlob(sections []mint.Section) string {
	var b strings.Builder
	for _, s := range sections {
		b.WriteString(s.Name)
		b.WriteByte('\n')
		for _, r := range s.Rows {
			b.WriteString(rowField(r, "label"))
			b.WriteByte('\n')
			b.WriteString(rowField(r, "name"))
			b.WriteByte('\n')
		}
	}
	return b.String()
}
