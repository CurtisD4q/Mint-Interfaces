package appkit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/fileplan"
)

func assertNoEmpty(t *testing.T, s *mint.Screen) {
	t.Helper()
	if s.TopTitle != "" && len(dropEmpty(s.Sections)) == 0 {
		t.Fatalf("top pane %q has no sections", s.TopTitle)
	}
	check := func(where string, secs []mint.Section) {
		t.Helper()
		if len(secs) == 0 {
			t.Fatalf("%s has no sections", where)
		}
		for _, sec := range secs {
			if len(sec.Rows) == 0 {
				t.Fatalf("%s section %q has no rows", where, sec.Name)
			}
		}
	}
	if len(s.Sections) > 0 {
		check("top", s.Sections)
	}
	for _, p := range s.Panes {
		if p == nil {
			t.Fatal("nil pane")
		}
		check(p.Title, p.Sections)
	}
	if len(s.Dock) > 0 {
		check("command bar", s.Dock)
	}
}

func assertDistinctAdjacent(t *testing.T, s *mint.Screen) {
	t.Helper()
	titles := orderedPaneTitles(s)
	for i := 1; i < len(titles); i++ {
		if paneKind(titles[i]) == paneKind(titles[i-1]) {
			t.Fatalf("adjacent panes %q and %q are the same kind in %v", titles[i-1], titles[i], titles)
		}
	}
}

// paneKind groups titles that show the same kind of content. Basic options
// and advanced options are different kinds: that split is the reason the
// 13 have two panes. Two output panes, or two copies of one options pane,
// are the same kind.
func paneKind(title string) string {
	switch strings.ToLower(strings.TrimSpace(title)) {
	case "basic options", "options", "more options":
		return "basic"
	case "advanced + additional":
		return "advanced"
	case "output", "live output", "report", "page", "progress", "discovered targets", "discovered targets & ports":
		return "output"
	case "findings":
		return "findings"
	case "what will change", "files":
		return "preview"
	default:
		return strings.ToLower(title)
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
		{"inspect", ToolSpec{Name: "file", Bin: "echo", ArchetypeID: "inspect"}, []string{titleBasic, titleOutput}},
		{"read", ToolSpec{Name: "man", Bin: "echo", ArchetypeID: "read"}, []string{titleBasic, titleOutput}},
		{"probe", ToolSpec{Name: "nmap", Bin: "echo", ArchetypeID: "probe"}, []string{titleBasic, titleOutput}},
		{"audit", ToolSpec{Name: "shellcheck", Bin: "echo", ArchetypeID: "audit", Findings: &FindingsSpec{Adapter: FindingsLines}}, []string{titleBasic, "Findings", titleOutput}},
		{"convert", ToolSpec{Name: "ffmpeg", Bin: "echo", ArchetypeID: "convert"}, []string{titleBasic, titleOutput}},
		{"toolkit", ToolSpec{Name: "docker", Bin: "echo", ArchetypeID: "toolkit", Subcommands: []SubcommandSpec{{Name: "ps", Label: "Ps", Desc: "List"}}}, []string{titleBasic, titleOutput}},
		{"transfer", ToolSpec{Name: "curl", Bin: "echo", ArchetypeID: "transfer"}, []string{titleBasic, titleOutput}},
		{"mirror", ToolSpec{Name: "rsync", Bin: "echo", ArchetypeID: "mirror"}, []string{titleBasic, titleOutput}},
		{"measure", ToolSpec{Name: "fio", Bin: "echo", ArchetypeID: "measure"}, []string{titleBasic, titleOutput}},
		{"control", ToolSpec{Name: "systemctl", Bin: "echo", ArchetypeID: "control"}, []string{titleBasic, titleOutput}},
		{"history", ToolSpec{Name: "git", Bin: "echo", ArchetypeID: "history"}, []string{titleBasic, titleOutput}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			s := newTestApp(t, tc.spec).MainScreen()
			assertNoEmpty(t, s)
			got := orderedPaneTitles(s)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("panes\n got %v\nwant %v", got, tc.want)
			}
			if len(s.Dock) == 0 {
				t.Fatal("run command bar is not under the output")
			}
		})
	}
}

func TestManyHeadingsStayOnePane(t *testing.T) {
	spec := ToolSpec{Name: "nmap", Bin: "echo", ArchetypeID: "probe", Flags: flagHeadings(25)}
	s := newTestApp(t, spec).MainScreen()
	assertNoEmpty(t, s)
	got := orderedPaneTitles(s)
	if strings.Join(got, "|") != titleBasic+"|"+titleOutput {
		t.Fatalf("25 headings became %v; want one options pane and Output", got)
	}
	var headings int
	for _, sec := range s.Sections {
		if strings.HasPrefix(sec.Name, "Heading ") {
			headings++
		}
	}
	if headings != 25 {
		t.Fatalf("top pane holds %d headings", headings)
	}
}

func TestNoOptionsScreenHasAHelpPane(t *testing.T) {
	for _, def := range AllArchetypeDefs() {
		spec := ToolSpec{Name: string(def.ID), Bin: "echo", ArchetypeID: string(def.ID)}
		if def.ID == "toolkit" || def.ID == "records" {
			spec.Subcommands = []SubcommandSpec{{Name: "ps", Label: "Ps", Desc: "List"}}
		}
		s := newTestApp(t, spec).MainScreen()
		for _, title := range append(orderedPaneTitles(s), sectionNames(s)...) {
			if title == "Help" {
				t.Fatalf("%s still has a Help pane or section (%v / %v)", def.ID, orderedPaneTitles(s), sectionNames(s))
			}
		}
	}
}

func TestReportStaysOnePane(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 25; i++ {
		fmt.Fprintf(&b, "SECTION %d\n    line\n", i)
	}
	a := newTestApp(t, ToolSpec{Name: "file", Bin: "echo", ArchetypeID: "inspect"})
	a.SetSnapshotText(b.String())
	s := a.MainScreen()
	assertNoEmpty(t, s)
	var output int
	for _, title := range orderedPaneTitles(s) {
		if paneKind(title) == "output" {
			output++
		}
	}
	if output != 1 {
		t.Fatalf("output panes in %v, want one", orderedPaneTitles(s))
	}
	blob := sectionBlob(outputSections(s))
	if !strings.Contains(blob, "SECTION 0") && !strings.Contains(blob, "SECTION 24") && !strings.Contains(blob, "25") {
		t.Fatalf("output pane did not keep the report text:\n%s", blob)
	}
}

func TestTransferAndMirrorPanesHoldOutput(t *testing.T) {
	for _, tc := range []struct {
		id, text string
	}{
		{"transfer", "downloaded 10 files"},
		{"mirror", "copied 3 paths"},
	} {
		a := newTestApp(t, ToolSpec{Name: tc.id, Bin: "echo", ArchetypeID: tc.id})
		a.SetSnapshotText(tc.text)
		s := a.MainScreen()
		var found bool
		for _, p := range s.Panes {
			if p.Title != titleOutput {
				continue
			}
			found = true
			if len(p.Sections) == 0 {
				t.Fatalf("%s output pane empty", tc.id)
			}
			blob := sectionBlob(p.Sections)
			if !strings.Contains(blob, tc.text) && !strings.Contains(blob, "Output") && !strings.Contains(blob, "Changes") && !strings.Contains(blob, "Progress") {
				t.Fatalf("%s output pane did not attach output:\n%s", tc.id, blob)
			}
		}
		if !found {
			t.Fatalf("missing Output pane in %v", orderedPaneTitles(s))
		}
	}
}

func TestEditFilesPreviewIsOnePane(t *testing.T) {
	p, _ := KnownPreviewSpec("cp")
	a := newTestApp(t, ToolSpec{Name: "cp", Bin: "echo", ArchetypeID: "edit-files", Preview: &p})
	plan := fileplan.Plan{Tool: "cp", Effects: []fileplan.Effect{
		{Kind: fileplan.Create, Path: "/tmp/a"},
		{Kind: fileplan.Create, Path: "/tmp/b"},
	}}
	scr := a.PreviewScreen(plan, func(*mint.Ctx, fileplan.Plan) {})
	got := paneTitles(scr)
	if len(got) != 1 || got[0] != "Files" {
		t.Fatalf("preview panes %v", got)
	}
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

func outputSections(s *mint.Screen) []mint.Section {
	for _, p := range s.Panes {
		if p != nil && p.Title == titleOutput {
			return p.Sections
		}
	}
	return nil
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
