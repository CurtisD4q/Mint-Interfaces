package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestIOModel(t *testing.T) {
	withToolDirs(t) // built-in list only
	cases := []struct {
		tool, help string
		save       string
		positional bool
		stdout     string
		ext        string
		flag       string
	}{
		{"ffmpeg", "usage: ffmpeg [options] [[infile options] -i infile]... {[outfile options] outfile}...",
			SaveByPositional, true, StdoutLog, "mkv", ""},
		{"wget", "Usage: wget [OPTION]... [URL]...\n  -O,  --output-document=FILE      write documents to FILE\n  -o,  --output-file=FILE          log messages to FILE",
			SaveByOption, false, StdoutLog, "txt", "-O"},
		{"gzip", "Usage: gzip [OPTION]... [FILE]...", SaveLog, false, StdoutLog, "", ""},
		{"mycopy", "Usage: mycopy [OPTION]... SOURCE DEST", SaveLog, false, StdoutLog, "", ""},
		{"mylist", "Usage: mylist [OPTION]... [FILE]...", SaveByCapture, false, "", "", ""},
	}
	for _, c := range cases {
		spec := ParseManAndHelp(c.tool, "", c.help)
		m := BuildIOModel(spec, ParseSynopsis(c.tool, nil, cleanLines(c.help)))
		if m.Save != c.save || spec.OutputPositional != c.positional || spec.StdoutKind != c.stdout || spec.OutputFlag != c.flag {
			t.Errorf("%s: save=%s positional=%v stdout=%q flag=%q\n  model: %+v", c.tool, m.Save, spec.OutputPositional, spec.StdoutKind, spec.OutputFlag, m)
		}
		if c.ext != "" && spec.DefaultExt != c.ext {
			t.Errorf("%s: extension %q, want %q", c.tool, spec.DefaultExt, c.ext)
		}
	}
}

// An output argument found only in a usage line is reported, never written
// to: only the known-tools list can confirm one.
func TestUsageLineAloneNeverMakesTheAppWrite(t *testing.T) {
	withToolDirs(t)
	for _, help := range []string{
		"Usage: tool [OPTION]... SOURCE DEST",
		"Usage: tool infile outfile",
		"Usage: tool [options] {[outfile options] outfile}...",
		"Usage: tool INPUT OUTPUT",
	} {
		spec := ParseManAndHelp("tool", "", help)
		if spec.OutputPositional {
			t.Errorf("%q: the app would write to the last argument", help)
		}
		m := BuildIOModel(spec, ParseSynopsis("tool", nil, cleanLines(help)))
		if m.Save == SaveByPositional || m.Main != nil {
			t.Errorf("%q: model writes: %+v", help, m)
		}
		if len(m.Outputs) == 0 {
			t.Errorf("%q: the output argument should still be reported", help)
		}
	}
}

// A known-tools file can say a tool prints binary data; then nothing is
// captured as text.
func TestBinaryStdout(t *testing.T) {
	dir := privateToolDir(t)
	writeToolFile(t, dir, "b.json", `{"tools":[{"name":"blob","stdout":"binary"}]}`, 0o644)
	withToolDirs(t, dir)
	spec := ParseManAndHelp("blob", "", "Usage: blob [FILE]")
	if spec.OutputsFile || spec.StdoutKind != StdoutBinary {
		t.Fatalf("binary tool: OutputsFile=%v StdoutKind=%q", spec.OutputsFile, spec.StdoutKind)
	}
}

// The new fields reach generated code only as fixed identifiers.
func TestStdoutKindRendering(t *testing.T) {
	spec := appkit.ToolSpec{Name: "x", StdoutKind: "log\"); panic(1); //", OutputPositional: true}
	src, err := renderSpecGo(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "panic") || strings.Contains(string(src), "StdoutKind") {
		t.Fatalf("unknown stdout kind written:\n%s", src)
	}
	spec.StdoutKind = StdoutLog
	src, _ = renderSpecGo(spec)
	if !strings.Contains(string(src), "StdoutKind:        appkit.StdoutLog") || !strings.Contains(string(src), "OutputPositional:  true") {
		t.Fatalf("fields missing:\n%s", src)
	}
}

// --explain prints help text only after removing control characters.
func TestExplainIsTerminalSafe(t *testing.T) {
	withToolDirs(t)
	help := "Usage: evil\x1b]0;pwned\x07 [OPTION]... FILE\n  -o, --output=FILE   write result to FILE\x1b[2J"
	spec := ParseManAndHelp("evil", "", help)
	m := BuildIOModel(spec, ParseSynopsis("evil", nil, cleanLines(help)))
	// Also straight into the model, past the help reader's own cleaning.
	m.Usage = "evil \x1b]0;pwned\x07 FILE"
	m.Inputs = append(m.Inputs, IOEndpoint{Name: "in\x1b[2Jput", Role: appkit.RoleInputFile, Why: "x\x9b31m"})
	spec.Flags = append(spec.Flags, appkit.FlagSpec{Flag: "--x\x1b[1m", Kind: appkit.FlagText, Hint: "file", Label: "read \x1b[31mFILE"})
	var b bytes.Buffer
	printIOModel(&b, m, spec)
	explainOptions(&b, spec)
	if strings.ContainsAny(b.String(), "\x1b\x07\u009b") {
		t.Fatalf("control characters printed: %q", b.String())
	}
}
