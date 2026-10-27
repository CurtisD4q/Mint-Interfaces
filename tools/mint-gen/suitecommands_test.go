package main

import (
	"strings"
	"testing"
)

// The subcommand reader finds names in the ways programs list them, and
// not in option descriptions or prose.
func TestSuiteCommandNames(t *testing.T) {
	for _, c := range []struct {
		name, help, want string
	}{
		{"two columns", "Commands:\n  start   Start it\n  stop    Stop it\n  status  Show it\n", "start stop status"},
		{"git sections", "These are common Git commands used in various situations:\n\nstart a working area\n   clone     Clone a repository\n   init      Create one\n\nwork on the current change\n   add       Add file contents\n", "clone init add"},
		{"aliases", "Commands:\n    build, b    Compile the package\n    check, c    Analyze it\n    clean       Remove the target\n", "build check clean"},
		{"dash", "Most used commands:\n  list - list packages based on names\n  search - search in package descriptions\n", "list search"},
		{"colon", "CORE COMMANDS\n  auth:          Authenticate gh\n  browse:        Open in the browser\n", "auth browse"},
		{"alone", "Available subcommands:\n   add\n   blame (praise, annotate, ann)\n   cat\n", "add blame cat"},
		{"columns", "Standard commands\nasn1parse         ca                ciphers\ncms               crl               dgst\n", "asn1parse ca ciphers cms crl dgst"},
		{"braces", "Usage: ip [ OPTIONS ] OBJECT { COMMAND | help }\nwhere  OBJECT := { address | link |\n                   route | rule }\n", "address link route rule"},
		{"modes", "Commands:\n -s, --sign       make a signature\n     --encrypt    encrypt data\n -d, --decrypt    decrypt data\n\nOptions:\n  --armor   create ascii output\n", "sign encrypt decrypt"},
		// Braces in an option's description are its values.
		{"option values", "Usage: x [options]\n  --color { auto | always | never }   when to colour\n", ""},
		// The options heading ends the list.
		{"ends", "Commands:\n  run    Run it\n  test   Test it\nOptions:\n  verbose   Say more\n", "run test"},
		// Filler words at the start of wrapped lines are not names.
		{"filler", "Commands:\n  run    Run it and wait\n  the    end of a wrapped line\n  test   Test it\n", "run test"},
	} {
		got := strings.Join(suiteCommandNames(nil, cleanLines(c.help)), " ")
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// The man page is read when the help has none.
	if got := suiteCommandNames(cleanLines("COMMANDS\n       start  Start it\n       stop   Stop it\n"), cleanLines("Usage: x [options]\n")); strings.Join(got, " ") != "start stop" {
		t.Errorf("man: %v", got)
	}
}
