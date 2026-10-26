package appkit

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatExtension(t *testing.T) {
	cases := map[string]string{"html5": "html", "HTML": "html", "gfm+smart": "md", "markdown-raw_html": "md",
		"docx": "docx", "json": "json", "yml": "yaml", "../../etc/passwd": "", "json; rm -rf ~": "", "": "", "exe": ""}
	for in, want := range cases {
		got, ok := FormatExtension(in)
		if got != want || ok != (want != "") {
			t.Errorf("%q: %q %v, want %q", in, got, ok, want)
		}
	}
}

// The automatic file name follows the chosen output format; a name the
// user typed never changes; unknown or hostile format values change nothing.
func TestOutputNameFollowsFormat(t *testing.T) {
	bin, _ := fakeTool(t, "pandoc", "")
	a := newTestApp(t, ToolSpec{Name: "pandoc", Bin: bin, OutputsFile: true, OutputFlag: "-o", DefaultExt: "html",
		Flags: []FlagSpec{{Flag: "-t", Kind: FlagText, Role: RoleFormat}, {Flag: "--json", Kind: FlagBool}}})
	a.SaveToFile = true
	ext := func() string { return strings.TrimPrefix(filepath.Ext(a.ResolvedOutputPath()), ".") }
	if ext() != "html" {
		t.Fatalf("default: %s", a.ResolvedOutputPath())
	}
	a.Values["-t"] = "docx"
	if ext() != "docx" {
		t.Fatalf("-t docx: %s", a.ResolvedOutputPath())
	}
	a.Values["-t"] = "gfm+smart"
	if ext() != "md" {
		t.Fatalf("-t gfm+smart: %s", a.ResolvedOutputPath())
	}
	for _, bad := range []string{"../../etc/passwd", "x/y", "exe"} {
		a.Values["-t"] = bad
		if p := a.ResolvedOutputPath(); filepath.Ext(p) != ".html" || strings.Contains(p, "etc/passwd") || filepath.Dir(p) != a.ResultsDir {
			t.Fatalf("-t %q: %s", bad, p)
		}
	}
	a.Values["-t"] = ""
	a.Values["--json"] = "true"
	if ext() != "json" {
		t.Fatalf("--json: %s", a.ResolvedOutputPath())
	}
	// A name the user typed stays exactly as typed.
	a.OutputFile = "report.md"
	a.Values["-t"] = "docx"
	if filepath.Base(a.ResolvedOutputPath()) != "report.md" {
		t.Fatalf("typed name changed: %s", a.ResolvedOutputPath())
	}
}
