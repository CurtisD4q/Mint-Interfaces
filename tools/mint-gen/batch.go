package main

// Batch modes: many programs that are full-screen by default can print
// plain text and exit instead (top -b -n 1, lynx -dump). Mint runs them
// that way: a monitor that prints one round is shown as a live view
// refreshed on a timer; a viewer that dumps a page runs once.
//
// A batch mode comes from the known list (checked by hand), or from the
// program's own options: an option for batch, plain or dump output. Only
// a mode that also stops after a set number of rounds is refreshed on a
// timer: Mint never keeps starting a program that does not exit.

import (
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// batchMode is how to run a program so it prints plain text and exits.
type batchMode struct {
	Args []string // the words every run starts with
	Poll bool     // it prints one round and exits: refresh it on a timer
	Why  string
}

// knownBatchModes are checked by hand.
var knownBatchModes = map[string]batchMode{
	"top":     {Args: []string{"-b", "-n", "1"}, Poll: true, Why: "batch mode, one round"},
	"iotop":   {Args: []string{"-b", "-n", "1"}, Poll: true, Why: "batch mode, one round"},
	"iftop":   {Args: []string{"-t", "-s", "1"}, Poll: true, Why: "text mode, one report after a second"},
	"nethogs": {Args: []string{"-t", "-c", "2"}, Poll: true, Why: "trace mode, two rounds"},
	"lynx":    {Args: []string{"-dump"}, Why: "dumps the page as text"},
	"links":   {Args: []string{"-dump"}, Why: "dumps the page as text"},
	"elinks":  {Args: []string{"-dump"}, Why: "dumps the page as text"},
	"w3m":     {Args: []string{"-dump"}, Why: "dumps the page as text"},
	"gdu":     {Args: []string{"-n"}, Why: "non-interactive mode"},
}

// noBatchMode are programs whose batch mode does something else: a
// mailer's sends mail from its input.
var noBatchMode = map[string]bool{"mutt": true, "neomutt": true}

var (
	// An option that turns plain output on.
	reBatchOptName = regexp.MustCompile(`^(batch|batch-mode|pipe|stdout|dump|once|one-shot|oneshot|non-interactive|noninteractive|plain|no-tui|text)$`)
	reBatchOptDesc = regexp.MustCompile(`(?i)\b(batch|non-?interactive|plain text|to (standard output|stdout)|pipe|dump|print once|once and exit|without the (tui|interface))\b`)
	// A description that says it outright, whatever the option is called
	// (the help may only keep its short name: -b).
	reBatchOptPlain = regexp.MustCompile(`(?i)\b(batch mode|non-?interactive mode|dumps? (the )?(\w+ )?(as|to) (plain )?text)\b`)
	// Never a batch mode: version and help options ("print version and
	// exit"), and a mailer's batch mode, which sends mail.
	reBatchNot = regexp.MustCompile(`(?i)\b(version|help|usage|send|mail)\b|\bin (batch|non-?interactive) mode\b`)
	// An option that sets how many rounds it prints.
	reRoundsOptDesc = regexp.MustCompile(`(?i)\b(iterations?|number of (updates|refreshes|frames|rounds|times|samples)|(update|refresh|sample) count|maximum number of iterations|exit after)\b`)
)

// findBatchMode is the batch mode for a program: the known list, else its
// own options.
func findBatchMode(name string, spec appkit.ToolSpec) (batchMode, bool) {
	if b, ok := knownBatchModes[name]; ok {
		return b, true
	}
	if noBatchMode[name] {
		return batchMode{}, false
	}
	batchFlag, roundsFlag := "", ""
	for _, f := range spec.Flags {
		n := strings.ToLower(strings.TrimLeft(flagName(f.Flag), "-"))
		desc := f.Label + " " + f.Hint
		switch {
		case batchFlag == "" && f.Kind == appkit.FlagBool && !reBatchNot.MatchString(desc) &&
			(reBatchOptName.MatchString(n) && reBatchOptDesc.MatchString(desc+" "+n) || reBatchOptPlain.MatchString(desc)):
			batchFlag = f.Flag
		case roundsFlag == "" && f.Kind == appkit.FlagInt && reRoundsOptDesc.MatchString(desc):
			roundsFlag = f.Flag
		}
	}
	if batchFlag == "" {
		return batchMode{}, false
	}
	if roundsFlag != "" {
		return batchMode{Args: []string{batchFlag, roundsFlag, "1"}, Poll: true, Why: "its " + batchFlag + " option, one round with " + roundsFlag}, true
	}
	return batchMode{Args: []string{batchFlag}, Why: "its " + batchFlag + " option"}, true
}

// applyBatchMode sets spec up to run in batch mode: the batch words start
// every run, their options leave the form, and it is not full-screen any
// more. A mode that prints one round is a live view (Watch) and, unless
// the program changes things, read-only, so it can be refreshed.
func applyBatchMode(spec *appkit.ToolSpec, b batchMode, listed bool, changes bool, help string) {
	spec.BatchArgs = append([]string(nil), b.Args...)
	spec.FullScreen = false
	inBatch := map[string]bool{}
	for _, w := range b.Args {
		if strings.HasPrefix(w, "-") {
			inBatch[w] = true
		}
	}
	kept := spec.Flags[:0:0]
	for _, f := range spec.Flags {
		if inBatch[f.Flag] {
			continue
		}
		kept = append(kept, f)
	}
	spec.Flags = kept
	if !b.Poll {
		return
	}
	if !listed && spec.ArchetypeID != "watch" {
		if watch, ok := appkit.LookupArchetype("watch"); ok {
			applyChosenArchetype(spec, watch, help)
		}
	}
	if !changes {
		spec.ReadOnly = true
	}
}

// batchNote says how a program runs in batch mode.
func batchNote(name string, b batchMode) string {
	cmd := name + " " + strings.Join(b.Args, " ")
	if b.Poll {
		return "it is full-screen by default; Mint runs it in batch mode (" + cmd + ": " + b.Why + ") and refreshes it as a Watch live view"
	}
	return "it is full-screen by default; Mint runs it in batch mode (" + cmd + ": " + b.Why + ")"
}

// knownColourArgs ask a program for its colours when it leaves them out
// without a terminal. Mint keeps only the colours, safely.
var knownColourArgs = map[string][]string{
	"fastfetch": {"--pipe", "false"},
}
