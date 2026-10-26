package main

import (
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Item 2: when the placeholder says nothing, the description decides, by
// whole words only.
func TestClassifyFlagWholeWords(t *testing.T) {
	cases := []struct {
		arg, desc string
		want      appkit.FlagKind
	}{
		{"x", "sort direction: asc or desc", appkit.FlagText},
		{"x", "redirect output to the given host", appkit.FlagText},
		{"x", "use the named profile", appkit.FlagText},
		{"x", "filesystem type to create", appkit.FlagText},
		{"x", "the account to use", appkit.FlagText},
		{"x", "the directory to search", appkit.FlagDir},
		{"x", "read rules from this file", appkit.FlagFile},
		{"x", "use this configuration file", appkit.FlagFile},
		{"x", "number of retries", appkit.FlagInt},
		{"x", "wait this many seconds", appkit.FlagInt},
	}
	for _, c := range cases {
		if k, _, _, _ := classifyFlag("--opt", c.arg, c.desc); k != c.want {
			t.Errorf("%q: %q, want %q", c.desc, k, c.want)
		}
	}
}

func TestChooseExtension(t *testing.T) {
	withToolDirs(t)
	cases := []struct {
		tool, help string
		want       string
	}{
		{"ffmpeg", "usage: ffmpeg [options] {[outfile options] outfile}...", "mkv"},                              // known-tools list
		{"jq", "Usage: jq [options] <jq filter> [file...]", "json"},                                              // what it prints
		{"reporter", "Usage: reporter [OPTION]...\n  -o, --output=FILE   write the HTML report to FILE", "html"}, // named in the description
		{"gzip", "Usage: gzip [OPTION]... [FILE]...", "log"},                                                     // messages only
		{"pandoc", "Usage: pandoc [OPTIONS] [FILES]\n  -o FILE  --output=FILE   Write output to FILE", "html"},   // pandoc's default format
		{"plain", "Usage: plain [FILE]...", "txt"},
	}
	for _, c := range cases {
		spec, m := ParseManAndHelpIO(c.tool, "", c.help)
		if spec.DefaultExt != c.want {
			t.Errorf("%s: %q (%s), want %q", c.tool, spec.DefaultExt, m.ExtWhy, c.want)
		}
	}
}

// Mint saving printed text never names it after a binary template format.
func TestTextCaptureNeverGetsBinaryExtension(t *testing.T) {
	withToolDirs(t)
	for _, tpl := range []string{"T13", "T08", "T06", "T07"} {
		spec := appkit.ToolSpec{Name: "unknowntool", Template: tpl}
		m := IOModel{Save: SaveByCapture}
		ext, _ := chooseExtension(spec, m, "", map[string]string{"T13": "pcap", "T08": "tar.gz", "T06": "out", "T07": "dat"}[tpl])
		if !isTextExtension(ext) {
			t.Errorf("%s: captured text saved as .%s", tpl, ext)
		}
	}
}

func TestKnownToolStdoutExtValidated(t *testing.T) {
	if err := validKnownTool(KnownTool{Name: "x", StdoutExt: "../../evil"}); err == nil {
		t.Fatal("bad stdout extension accepted")
	}
	if err := validKnownTool(KnownTool{Name: "x", StdoutExt: "json"}); err != nil {
		t.Fatal(err)
	}
}
