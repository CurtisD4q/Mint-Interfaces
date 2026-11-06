package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Each command's own options.
//
// A program made of commands lists them in its help, but each command's
// options are in that command's help: docker run --help. mint-gen reads it
// only when the program's own help says how ("Run 'docker COMMAND --help'
// for more information on a command", "See 'git help <command>'"), the same
// rule as for every other help it reads: it never runs a program with
// arguments the help did not ask for. The help is read with the same
// runner, so the same empty folder, cleaned environment and limits apply.

// The command word in such a sentence: COMMAND, <command>, [command],
// <subcommand> and so on.
const commandWord = `(?:<|\[)?(?:sub)?command(?:>|\])?`

// maxCommandHelps is the most command helps read for one program.
const maxCommandHelps = 24

// commandHelpForm reads how the program's help says to get one command's
// help: as "<program> <command> --help" (or -h), or as "<program> help
// <command>". It returns the arguments for a command, and false when the
// help says neither.
func commandHelpForm(name, text string) (func(cmd string) []string, bool) {
	if name == "" || text == "" {
		return nil, false
	}
	q := regexp.QuoteMeta(name)
	dash := regexp.MustCompile(`(?i)\b` + q + `\s+` + commandWord + `\s+(--help|-h)\b`)
	if m := dash.FindStringSubmatch(text); m != nil {
		flag := strings.ToLower(m[1])
		return func(cmd string) []string { return []string{cmd, flag} }, true
	}
	word := regexp.MustCompile(`(?i)\b` + q + `\s+help\s+` + commandWord)
	if word.MatchString(text) {
		return func(cmd string) []string { return []string{"help", cmd} }, true
	}
	return nil, false
}

// genericSection reports whether a section name says nothing about which
// command it belongs to, so the command's name should be put on it.
func genericSection(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "options", "flags", "general", "general options":
		return true
	}
	return false
}

// commandFlagsFrom parses one command's help and keeps the options that are
// its own: not the program's own options (those are shown for every
// command), and never the output-file option, which is the program's.
func commandFlagsFrom(name string, sub appkit.SubcommandSpec, help string, global []appkit.FlagSpec) []appkit.FlagSpec {
	if strings.TrimSpace(help) == "" || crashReport(help) != "" {
		return nil
	}
	parsed := ParseManAndHelp(name, "", help)
	have := map[string]bool{}
	for _, f := range global {
		have[f.Flag] = true
	}
	label := sub.Label
	if label == "" {
		label = titleWords(sub.Name)
	}
	var out []appkit.FlagSpec
	for _, f := range parsed.Flags {
		if f.Flag == "" || have[f.Flag] {
			continue
		}
		have[f.Flag] = true
		if genericSection(f.Section) {
			f.Section = label + " options"
		}
		out = append(out, f)
	}
	return out
}

// attachCommandFlags reads each command's help, as the program's help says
// to, and gives each command its own options. It returns notes for the
// build output.
func attachCommandFlags(bin string, spec *appkit.ToolSpec, helpText string) []string {
	if spec == nil || len(spec.Subcommands) == 0 {
		return nil
	}
	form, ok := commandHelpForm(spec.Name, helpText)
	if !ok {
		return nil
	}
	path, err := resolveProgram(bin)
	if err != nil {
		return nil
	}
	r, cleanup, err := newHelpRunner()
	if err != nil {
		return []string{"could not prepare a safe folder to read each command's help in: " + err.Error()}
	}
	defer cleanup()
	read := 0
	for i := range spec.Subcommands {
		if i >= maxCommandHelps {
			break
		}
		sub := &spec.Subcommands[i]
		if len(sub.Flags) > 0 || !safeCommandWord(sub.Name) {
			continue
		}
		help, _ := r.run(path, form(sub.Name)...)
		if flags := commandFlagsFrom(spec.Name, *sub, help, spec.Flags); len(flags) > 0 {
			markSecretFlags(flags)
			sub.Flags = flags
			read++
		}
	}
	if read == 0 {
		return nil
	}
	return []string{"read the options of " + plural(read, "command") + " from each command's help, as the help says to"}
}

// safeCommandWord reports whether a command name is a plain word, so it can
// be passed to the program as one argument for its help: no option, path or
// odd characters.
var reCommandWord = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

func safeCommandWord(s string) bool { return len(s) <= 40 && reCommandWord.MatchString(s) }

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
