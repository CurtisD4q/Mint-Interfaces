package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestParsePlaceholder(t *testing.T) {
	cases := []struct {
		raw             string
		word            string
		opt, rep        bool
		choices         string
		isFile, notFile bool
	}{
		{"=FILE", "file", false, false, "", true, false},
		{"<file>...", "file", false, true, "", true, false},
		{"[=WHEN]", "when", true, false, "", false, true},
		{"{json|xml|csv}", "", false, false, "json,xml,csv", false, true},
		{"identity_file", "identity_file", false, false, "", true, false},
		{"max-count", "max-count", false, false, "", false, true},
		{"COLS", "cols", false, false, "", false, true},
		{"value", "", false, false, "", false, false},
		{"=URL", "url", false, false, "", false, true},
		{"[=DIR]", "dir", true, false, "", true, false},
	}
	for _, c := range cases {
		p := ParsePlaceholder(c.raw)
		got := fmt.Sprint(p.Word, p.Optional, p.Repeat, strings.Join(p.Choices, ","), p.IsFile(), p.IsNotFile())
		want := fmt.Sprint(c.word, c.opt, c.rep, c.choices, c.isFile, c.notFile)
		if got != want {
			t.Errorf("%q: got %s, want %s", c.raw, got, want)
		}
	}
}

func TestScanOptionLine(t *testing.T) {
	cases := []struct {
		line string
		want string // "names:placeholder|..." or "" for none
	}{
		{"     -i identity_file", "-i:identity_file"},
		{"     -o file      Write output to file", "-o:file"},
		{"       -S .suf, --suffix=.suf", ""}, // reOptLine handles this one in full lines; not a lower-case pattern
		{"  -oN/-oX/-oS/-oG <file>: Output scan in normal, XML, s|<rIpt kIddi3,", "-oN:<file>|-oX:<file>|-oS:<file>|-oG:<file>"},
		{"  -PS/PA/PU/PY[portlist]: TCP SYN/ACK, UDP or SCTP discovery", "-PS:[portlist]|-PA:[portlist]|-PU:[portlist]|-PY:[portlist]"},
		{"      --files[=FILE]  read filenames to process from FILE", "--files:[=FILE]"},
		// Not options, or a description on the same line.
		{"     -v verbose output", ""},
		{"Use -o to write a file", ""},
		{"     " + strings.Repeat("x", 600), ""},
	}
	for _, c := range cases {
		var parts []string
		for _, o := range scanOptionLine(c.line) {
			parts = append(parts, strings.Join(o.Names, ",")+":"+o.Placeholder.Raw)
		}
		if got := strings.Join(parts, "|"); got != c.want {
			t.Errorf("%q: got %q, want %q", c.line, got, c.want)
		}
	}
}

// Options the main pattern missed now reach the app, with the right kinds,
// and a slash group gives one option each.
func TestParserReadsMissedOptionLines(t *testing.T) {
	man := `NAME
       sshlike - remote login

OPTIONS
     -i identity_file
             Selects a file from which the identity (private key) is read.

     -p port
             Port to connect to on the remote host.

  -oN/-oX/-oG <file>: Output scan in normal, XML, and Grepable format
     -v      Verbose mode.
`
	spec := ParseManAndHelp("nmap", man, "")
	kinds := map[string]appkit.FlagKind{}
	for _, f := range spec.Flags {
		kinds[f.Flag] = f.Kind
	}
	want := map[string]appkit.FlagKind{"-i": appkit.FlagFile, "-p": appkit.FlagText, "-oX": appkit.FlagFile, "-oG": appkit.FlagFile, "-v": appkit.FlagBool}
	for flag, k := range want {
		if kinds[flag] != k {
			t.Errorf("%s: kind %q, want %q (all: %v)", flag, kinds[flag], k, kinds)
		}
	}
	// -oN is nmap's known output option, now that it is read at all.
	if spec.OutputFlag != "-oN" {
		t.Errorf("output option %q, want -oN", spec.OutputFlag)
	}
	if spec.DefaultExt != "nmap" {
		t.Errorf("extension %q, want nmap (from the known-tools list)", spec.DefaultExt)
	}
	for _, f := range spec.Flags {
		if f.Flag == "-oX" && f.Role != appkit.RoleOutputFile {
			t.Errorf("-oX role %q, want output-file", f.Role)
		}
	}
	// The -i description must not run into the next option.
	for _, f := range spec.Flags {
		if f.Flag == "-i" && strings.Contains(f.Label, "Port") {
			t.Errorf("-i description swallowed the next option: %q", f.Label)
		}
	}
}

func TestClassifyFlagTrustsPlaceholder(t *testing.T) {
	cases := []struct {
		arg, desc string
		want      appkit.FlagKind
	}{
		{"FORMAT", "File format to encode or decode", appkit.FlagText},
		{"NAME", "use the named profile", appkit.FlagText},
		{"URL", "resolve links relative to URL", appkit.FlagText},
		{"PATTERN", "exclude files matching PATTERN", appkit.FlagText},
		{"FILE", "exclude contents of directories containing FILE", appkit.FlagFile},
		{"DIR", "create a top-level directory", appkit.FlagDir},
		{"SECONDS", "set the read timeout to SECONDS", appkit.FlagInt},
		{"NUMBER", "set number of retries to NUMBER", appkit.FlagInt},
		{"x", "the file to read", appkit.FlagFile}, // vague placeholder: the description decides
	}
	for _, c := range cases {
		if k, _, _, _ := classifyFlag("--x", c.arg, c.desc); k != c.want {
			t.Errorf("%s %q: %q, want %q", c.arg, c.desc, k, c.want)
		}
	}
}

// A short help table that leaves out an option's value does not hide the
// man page's definition with the value; an optional value stays a switch,
// and in a program with subcommands the first definition stays.
func TestDefinitionWithValueWins(t *testing.T) {
	help := "Usage: cronish [-s] [-t tab]\n\n -s  Serialize jobs\n -t  Use this table\n --json  Print JSON\n"
	man := "NAME\n       cronish - run jobs\n\nOPTIONS\n       -t table\n              Use this table.\n\n       --json[=false]\n              Print JSON.\n"
	kind := func(spec appkit.ToolSpec, flag string) appkit.FlagKind {
		for _, f := range spec.Flags {
			if f.Flag == flag {
				return f.Kind
			}
		}
		t.Fatalf("%s missing: %+v", flag, spec.Flags)
		return ""
	}
	spec := ParseManAndHelp("cronish", man, help)
	if k := kind(spec, "-t"); k == appkit.FlagBool {
		t.Fatalf("-t stayed a switch")
	}
	if k := kind(spec, "--json"); k != appkit.FlagBool {
		t.Fatalf("--json became %s", k)
	}
	if k := kind(spec, "-s"); k != appkit.FlagBool {
		t.Fatalf("-s became %s", k)
	}
	suite := ParseManAndHelp("cronish", man, help+"\nAvailable commands:\n  run   Run jobs\n  list  List jobs\n")
	if k := kind(suite, "-t"); k != appkit.FlagBool {
		t.Fatalf("suite: -t replaced (%s)", k)
	}
}

// Bulleted option lists and nmap-style names (masscan's man page):
// "o   -oL <filename>: sets the output format to a simple list".
func TestBulletedAndCompactOptions(t *testing.T) {
	man := "NAME\n       scan9 - scan things\n\nOPTIONS\n" +
		"       o   -oL <filename>: sets the output format to a simple list format and saves the output in\n           the given file.\n\n" +
		"       o   --rate <packets-per-second>: specifies the desired rate\n\n" +
		"       *  -iL <inputfilename>: reads a list of targets\n"
	flags := map[string]appkit.FlagSpec{}
	for _, f := range parseFlagsFromLines(cleanLines(man), true) {
		flags[f.Flag] = f
	}
	for _, want := range []string{"-oL", "--rate", "-iL"} {
		f, ok := flags[want]
		if !ok || f.Kind == appkit.FlagBool {
			t.Fatalf("%s: %+v (all: %v)", want, f, flags)
		}
	}
	if !strings.Contains(flags["-oL"].Label, "simple list") {
		t.Fatalf("-oL label: %q", flags["-oL"].Label)
	}
	// A plain sentence starting with "o" is not an option.
	for _, f := range parseFlagsFromLines(cleanLines("NAME\n       x - y\n\nDESCRIPTION\n       o   the output goes to standard output\n"), true) {
		t.Fatalf("parsed %+v", f)
	}
}
