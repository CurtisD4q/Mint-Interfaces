package main

// Behaviour signals: a few things a program's help or package says about
// how it behaves, read only where the reading is reliable. They never
// change the archetype; they add caution and handling on top of it:
//
//   - a full-screen program is marked FullScreen (Mint shows its command
//     to run in a terminal, and never polls it);
//   - a program whose own summary says it changes or destroys things
//     always asks before running, and is not treated as read-only (unless
//     a person checked it is);
//   - --explain, --check and the build output say so, and note where the
//     archetype only reads but the summary says the program changes
//     things, or where a program may be full-screen.

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// behaviourSignals are what the help showed clearly. A field is set only
// on clear evidence; the Why fields say what it was. MaybeFullScreen is
// weaker evidence: it is reported, not acted on.
type behaviourSignals struct {
	FullScreen, MaybeFullScreen, Changes bool
	FullScreenWhy, ChangesWhy            string
}

var (
	// A summary that says the program changes the system or its settings.
	// Verbs that also describe plain file work (edit, copy, move, create)
	// are left out: they are not clear enough to ask before every run.
	sigChanges = regexp.MustCompile(`(?i)\b(configur(e|es|ing)|install(s|ing)?|uninstall(s|ing)?|(remove[sd]?|delete[sd]?) (packages?|users?|groups?|rules?|modules?|services?|devices?|partitions?|containers?|images?|keys?)|enable[sd]? (and|or) disable[sd]?|mount(s|ing)? (and|or)? ?(unmount)?|unmount(s|ing)?|kill(s)? (processes|a process|the)|format(s)? (a|the) (disk|partition|device|filesystem)|modif(y|ies) (the|a|system)|change[sd]? (the|a|system)|set(s)? (the|a) (system|kernel|device)|start(s)? (and|or) stop(s)?|manage[sd]? (system|services|packages|users|partitions|devices|containers|firewall))\b`)
	// It reads a setting and changes it: "query or change".
	sigReadAndSet = regexp.MustCompile(`(?i)\b(query|get|show|read|display|view)( and|,| or| and/or) (set|change|control|modify|adjust|configure)s?\b`)
	// It destroys things by purpose.
	sigDestroys = regexp.MustCompile(`(?i)\b(shred|wipe[sd]?|overwrite[sd]? (a |the )?(files?|disks?|devices?)|erase[sd]? (files?|disks?|devices?|data)|delete[sd]? (files?|directories)|remove[sd]? (files?|directories)|destroy(s|ed)?)\b`)
	// It says it is full-screen.
	sigFullScreen = regexp.MustCompile(`(?i)\b(n?curses|tui|terminal user interface|text[- ]based user interface|full[- ]screen)\b`)
	// A man page section that only a full-screen program has. (Not
	// "INTERACTIVE COMMANDS": prompt-style programs such as nslookup and
	// fdisk have one too, and run fine one-shot.)
	sigKeySection = regexp.MustCompile(`^(KEY ?BINDINGS?|KEYS|KEYBOARD (SHORTCUTS|COMMANDS|CONTROLS)|KEY COMMANDS)\s*$`)
	sigNcursesDep = regexp.MustCompile(`\blibncurses(w)?\d`)
	// With ncurses, a summary like these makes full-screen clear.
	sigInteractiveKind = regexp.MustCompile(`(?i)\b(interactive|text editor|editor|web browser|browser|music player|player|mixer|menu|file manager)\b`)
	// A "summary" that is really an instruction ("You need to install the
	// perl-doc package to use this program") says nothing about the program.
	sigInstruction = regexp.MustCompile(`(?i)\b(you (need|must|should|can|may)|to use this program|is not installed|run .* first)\b`)
)

// readSignals reads the signals from a program's help and package.
func readSignals(name, manText, helpText string) behaviourSignals {
	var s behaviourSignals
	p := programTextFrom(name, manText, helpText)
	// The program's own summary; its package's only when it has none (a
	// package's description can mention installing it).
	summary := p.Summary
	if strings.TrimSpace(summary) == "" || sigInstruction.MatchString(summary) {
		summary = p.PackageSummary
	}

	// Full-screen.
	for _, l := range cleanLines(manText) {
		if t := strings.TrimSpace(l); sigKeySection.MatchString(t) {
			s.FullScreen, s.FullScreenWhy = true, "its man page has a "+strings.ToLower(t)+" section"
			break
		}
	}
	if !s.FullScreen {
		if m := sigFullScreen.FindString(summary); m != "" {
			s.FullScreen, s.FullScreenWhy = true, "its summary says "+strings.ToLower(m)
		}
	}
	if !s.FullScreen {
		// A package of several programs uses ncurses for one of them
		// (procps for top, not ps): only a small package counts, and only
		// with an interactive kind of summary is it clear.
		if info, ok := lookupPackage(name); ok && info.Programs <= 3 && sigNcursesDep.MatchString(info.Depends) {
			if k := strings.ToLower(sigInteractiveKind.FindString(summary + "\n" + p.PackageSummary)); k != "" {
				article := "a "
				if strings.ContainsRune("aeiou", rune(k[0])) {
					article = "an "
				}
				s.FullScreen, s.FullScreenWhy = true, "it is "+article+k+" program whose package uses ncurses"
			} else {
				s.MaybeFullScreen, s.FullScreenWhy = true, "its package uses ncurses"
			}
		}
	}

	// Changes or destroys things.
	for _, re := range []*regexp.Regexp{sigChanges, sigReadAndSet, sigDestroys} {
		if m := re.FindString(summary); m != "" {
			s.Changes, s.ChangesWhy = true, "its summary says "+strings.ToLower(m)
			break
		}
	}
	return s
}

// applySignals adds the signals' caution and handling to spec. It only
// ever adds caution: it never removes a confirmation, and it leaves a
// read-only setting a person checked (appkit.IsReadOnlyTool) alone.
func applySignals(spec *appkit.ToolSpec, s behaviourSignals) {
	if s.FullScreen {
		spec.FullScreen = true
	}
	if s.Changes {
		spec.ConfirmRun = true
		if !appkit.IsReadOnlyTool(spec.BinaryName()) {
			spec.ReadOnly = false
		}
	}
}

// signalNotes say how the signals change Mint's handling, and where the
// archetype disagrees with them.
func signalNotes(archetype appkit.ArchetypeID, s behaviourSignals) []string {
	var notes []string
	if s.FullScreen {
		notes = append(notes, fmt.Sprintf("it draws its own full-screen interface (%s): Mint shows its command to run in a terminal instead of running it", s.FullScreenWhy))
	} else if s.MaybeFullScreen {
		notes = append(notes, fmt.Sprintf("it may draw a full-screen interface (%s): if it does, run it in a terminal", s.FullScreenWhy))
	}
	if s.Changes {
		def, ok := appkit.LookupArchetype(string(archetype))
		if ok && (def.Group == appkit.GroupAnswer || def.Group == appkit.GroupObserve) {
			notes = append(notes, fmt.Sprintf("the %s archetype only reads, but %s: Mint asks before running it; check the archetype", def.Name, s.ChangesWhy))
		} else {
			notes = append(notes, fmt.Sprintf("%s: Mint asks before running it", s.ChangesWhy))
		}
	}
	return notes
}

// printSignalNotes writes the notes, one per line.
func printSignalNotes(w io.Writer, notes []string) {
	for i, n := range notes {
		head := "  Note:      "
		if i > 0 {
			head = "             "
		}
		fmt.Fprint(w, safeTerminalText(head+n)+"\n")
	}
}
