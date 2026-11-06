package main

import (
	"fmt"
	"strings"
	"testing"
)

// git's help introduces its commands with a sentence and groups them under
// small headings of its own.
func TestCommandsUnderASentenceHeading(t *testing.T) {
	help := `usage: git [-v | --version] [-C <path>] <command> [<args>]

These are common Git commands used in various situations:

start a working area (see also: git help tutorial)
   clone     Clone a repository into a new directory
   init      Create an empty Git repository or reinitialize an existing one

work on the current change (see also: git help everyday)
   add       Add file contents to the index
   status    Show the working tree status

collaborate (see also: git help workflows)
   push      Update remote refs along with associated objects

'git help -a' and 'git help -g' list available subcommands.
`
	subs := extractSubcommands(nil, strings.Split(help, "\n"))
	var names []string
	for _, s := range subs {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, " "); got != "clone init add status push" {
		t.Fatalf("commands %q", got)
	}
	if subs[3].Desc != "Show the working tree status" {
		t.Errorf("desc %q", subs[3].Desc)
	}
}

func TestCommandsHeading(t *testing.T) {
	for _, yes := range []string{"these are common git commands used in various situations:", "available commands:", "management commands:"} {
		if !commandsHeading(yes) {
			t.Errorf("%q not a heading", yes)
		}
	}
	for _, no := range []string{"run 'docker command --help' for more information on a command.", "-c, --commands: run these", "options:", strings.Repeat("commands ", 20) + ":"} {
		if commandsHeading(no) {
			t.Errorf("%q taken for a heading", no)
		}
	}
}

func TestSuiteKeepsUpToTheLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("Commands:\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "  cmd%02d     Does thing %d\n", i, i)
	}
	if n := len(extractSubcommands(nil, strings.Split(b.String(), "\n"))); n != maxSuiteCommands {
		t.Fatalf("%d commands, want %d", n, maxSuiteCommands)
	}
}
