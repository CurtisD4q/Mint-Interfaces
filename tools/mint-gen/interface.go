package main

// Interface answers: mint-gen's answers to the ten questions about how a
// program behaves (appkit.Questions), each from the program's own help,
// never from its archetype. In interface mode (--interface answers) they
// place an unlisted program in one of the 22 interface groups and decide
// its screen; the archetype then only words it.

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Interface modes for --interface.
const (
	InterfaceArchetype = "archetype" // the screen comes from the chosen archetype (the default)
	InterfaceAnswers   = "answers"   // the screen comes from the program's answers
)

// interfaceMode is how ParseManAndHelpIO decides a screen.
var interfaceMode = InterfaceArchetype

var (
	ifKeepsRunning = regexp.MustCompile(`(?i)\b(until (it is )?(interrupted|killed|stopped)|until you (quit|press|exit)|press (q|ctrl-c|control-c)\b|ctrl-c|real[- ]time|refresh(es|ed)? (every|the display|rate|interval)|update interval|every \d+ seconds?|continuously|keeps? running|runs? in the foreground|as a daemon|daemoni[sz]e|listen(s|ing)? (on|for)\b|output appended data as the file grows|wait for (new|more) (data|input|events))`)
	ifMonitorWord  = regexp.MustCompile(`(?i)\b(monitor(s|ing)?|watch(es|ing)?|live|dashboard|top-like)\b`)
	ifRunOption    = regexp.MustCompile(`^(interval|delay|refresh|refresh-rate|rate|period|tick|tick-rate|update|update-interval|follow|watch|daemon|foreground|listen|bind|port|serve)$`)
	ifFullScreen   = regexp.MustCompile(`(?i)\b(n?curses|tui|terminal user interface|text[- ]based user interface|full[- ]screen|interactive (terminal|text|console) (ui|interface|application|program|browser|client|viewer|editor)|key ?bindings?|keyboard shortcuts|vi-like|vim-like|emacs-like|terminal ui|text user interface)\b`)
	ifChanges      = regexp.MustCompile(`(?i)\b(configure[sd]?|change[sd]?|modif(y|ies)|set(s)? (the|a|up|or)|enable[sd]?|disable[sd]?|install(s|ed)?|uninstall|remove[sd]?|add(s)? (a|an|or|new)|delete[sd]?|start(s)? (and|or|the)|stop(s)? (and|or|the)|mount(s)?|unmount|kill(s)?|manage[sd]?|creates? (a|an|new)|update[sd]? (the|a)|apply|applies|edit(s|or)?|rename[sd]?|move[sd]?|cop(y|ies)|format(s)? (a|the) (disk|partition|device))\b`)
	ifTable        = regexp.MustCompile(`(?i)\b(columns?|tabular|in a table|table (format|output|view)|no[- ]headers?|header (line|row)s?|omit (the )?header|fields? (to|separated)|--output[= ](wide|table)|output fields|csv|tsv)\b`)
	ifRoot         = regexp.MustCompile(`(?i)\b(must be root|must be run as root|run (it )?as root|requires? (root|superuser|administrator)|root (privileges?|permissions?|access)|superuser (privileges?|access)?|only (the )?root|cap_net_(raw|admin)|cap_sys_\w+|privileged (user|operation|mode)|needs? to be root|as the root user)\b`)
	ifDestroys     = regexp.MustCompile(`(?i)\b(delete[sd]?|remove[sd]? (files?|directories|the|all|packages?|entries)|erase[sd]?|wipe[sd]?|shred|destroy(s|ed)?|overwrite[sd]?|truncate[sd]?|kill(s)? (processes|a process|the)|format(s)? (a|the) (disk|partition|device|filesystem)|purge[sd]?)\b`)
	// Two inputs come in pairs: FILE1 FILE2, OLD NEW, LEFT RIGHT.
	ifPairNumbered = regexp.MustCompile(`(?i)^(.*?)[-_]?1$`)
	ifPairWords    = map[string]string{"old": "new", "left": "right", "first": "second", "a": "b", "this": "that", "base": "head"}
)

func ifMatch(re *regexp.Regexp, s string) string {
	return strings.ToLower(strings.TrimSpace(re.FindString(s)))
}

// detectAnswers answers the ten questions for a program from its help.
// spec is the program parsed so far (its options).
func detectAnswers(name, manText, helpText string, spec appkit.ToolSpec) appkit.Answers {
	text := manText + "\n" + helpText
	p := programTextFrom(name, manText, helpText)
	front := p.Summary + "\n" + p.PackageSummary
	manLines, helpLines := cleanLines(manText), cleanLines(helpText)
	out := appkit.Answers{}
	set := func(q appkit.Question, yes bool, sure, why string) {
		out[q] = appkit.Answer{Yes: yes, Sure: sure, Why: why}
	}

	// Keeps running.
	runOpt := ""
	for _, f := range spec.Flags {
		if ifRunOption.MatchString(strings.ToLower(strings.TrimLeft(flagName(f.Flag), "-"))) {
			runOpt = f.Flag
			break
		}
	}
	switch {
	case runOpt != "":
		set(appkit.QKeepsRunning, true, appkit.SureStrong, "option "+runOpt)
	case ifMatch(ifKeepsRunning, text) != "":
		set(appkit.QKeepsRunning, true, appkit.SureStrong, "help says "+ifMatch(ifKeepsRunning, text))
	case ifMatch(ifMonitorWord, front) != "":
		set(appkit.QKeepsRunning, true, appkit.SureWeak, "summary says "+ifMatch(ifMonitorWord, front))
	default:
		set(appkit.QKeepsRunning, false, appkit.SureWeak, "no interval, follow or listen option")
	}

	// Changes things (settings, the system, something outside itself).
	switch {
	case ifMatch(ifChanges, front) != "":
		set(appkit.QChangesThings, true, appkit.SureStrong, "summary says "+ifMatch(ifChanges, front))
	default:
		mut := ""
		for _, n := range discoveredSubcommands(append(append([]string{}, helpLines...), manLines...)) {
			if isMutatingSubcommand(n) {
				mut = n
				break
			}
		}
		if mut != "" {
			set(appkit.QChangesThings, true, appkit.SureStrong, "command "+mut)
		} else {
			set(appkit.QChangesThings, false, appkit.SureWeak, "the summary only reports or reads")
		}
	}

	// Takes a target; two inputs.
	syn := ParseSynopsis(name, manLines, helpLines)
	var inputs []Positional
	if len(syn.Forms) > 0 {
		for _, ps := range syn.Forms[0].Positionals {
			if ps.Role == appkit.RoleOutputFile || ps.Role == appkit.RoleOutputDir {
				continue
			}
			inputs = append(inputs, ps)
		}
	}
	required := 0
	for _, ps := range inputs {
		if !ps.Optional {
			required++
		}
	}
	switch {
	case required > 0:
		set(appkit.QTarget, true, appkit.SureStrong, "usage needs "+inputs[0].Name)
	case len(inputs) > 0:
		set(appkit.QTarget, true, appkit.SureWeak, "usage allows ["+inputs[0].Name+"]")
	case len(syn.Forms) > 0:
		set(appkit.QTarget, false, appkit.SureStrong, "usage takes no arguments")
	default:
		set(appkit.QTarget, false, appkit.SureNone, "no usage line found")
	}
	if a, b, ok := pairedInputs(inputs); ok {
		set(appkit.QTwoInputs, true, appkit.SureStrong, "usage pairs "+a+" and "+b)
	} else {
		set(appkit.QTwoInputs, false, appkit.SureWeak, "no paired inputs in the usage")
	}

	// Subcommands.
	if hasSubcommands(manText, helpText, manLines, helpLines) {
		set(appkit.QSubcommands, true, appkit.SureStrong, "help lists commands")
	} else {
		set(appkit.QSubcommands, false, appkit.SureStrong, "no command list")
	}

	// Writes a file.
	outOpt := spec.OutputFlag
	if outOpt == "" {
		for _, f := range spec.Flags {
			if f.Role == appkit.RoleOutputFile || f.Role == appkit.RoleOutputDir || f.Role == appkit.RoleOutputBase {
				outOpt = f.Flag
				break
			}
		}
	}
	_, _, outPos := syn.OutputPositional()
	switch {
	case outOpt != "":
		set(appkit.QWritesFile, true, appkit.SureStrong, "output option "+outOpt)
	case outPos:
		set(appkit.QWritesFile, true, appkit.SureStrong, "usage names an output")
	default:
		set(appkit.QWritesFile, false, appkit.SureWeak, "no output option or argument")
	}

	// Table output.
	if w := ifMatch(ifTable, text); w != "" {
		set(appkit.QTable, true, appkit.SureWeak, "help says "+w)
	} else {
		set(appkit.QTable, false, appkit.SureWeak, "")
	}

	// Needs root.
	if w := ifMatch(ifRoot, text); w != "" {
		set(appkit.QNeedsRoot, true, appkit.SureStrong, "help says "+w)
	} else {
		set(appkit.QNeedsRoot, false, appkit.SureWeak, "help never says root")
	}

	// Can destroy something.
	destroyOpt := ""
	for _, f := range spec.Flags {
		if appkit.LooksDestructive(f.Flag, strings.TrimSpace(f.Label+" "+f.Hint)) {
			destroyOpt = f.Flag
			break
		}
	}
	switch {
	case ifMatch(ifDestroys, front) != "":
		set(appkit.QCanDestroy, true, appkit.SureStrong, "summary says "+ifMatch(ifDestroys, front))
	case destroyOpt != "":
		set(appkit.QCanDestroy, true, appkit.SureWeak, "option "+destroyOpt)
	default:
		set(appkit.QCanDestroy, false, appkit.SureWeak, "no deleting or overwriting options")
	}

	// Full-screen.
	switch {
	case ifMatch(ifFullScreen, front) != "":
		set(appkit.QFullScreen, true, appkit.SureStrong, "summary says "+ifMatch(ifFullScreen, front))
	case ifMatch(ifFullScreen, text) != "":
		set(appkit.QFullScreen, true, appkit.SureWeak, "help says "+ifMatch(ifFullScreen, text))
	default:
		set(appkit.QFullScreen, false, appkit.SureWeak, "")
	}
	return out
}

// pairedInputs finds two required inputs that form a pair: FILE1 FILE2,
// OLD NEW, LEFT RIGHT. Two unrelated arguments (SOURCE DEST, PATTERN FILE)
// are not two inputs.
func pairedInputs(ps []Positional) (string, string, bool) {
	var req []Positional
	for _, p := range ps {
		if !p.Optional && !p.Repeat {
			req = append(req, p)
		}
	}
	for i := 0; i+1 < len(req); i++ {
		a, b := strings.ToLower(req[i].Name), strings.ToLower(req[i+1].Name)
		if m := ifPairNumbered.FindStringSubmatch(a); m != nil && m[1] != "" {
			if strings.TrimRight(strings.TrimRight(b, "2"), "-_") == m[1] && strings.HasSuffix(b, "2") {
				return req[i].Name, req[i+1].Name, true
			}
		}
		if ifPairWords[a] == b {
			return req[i].Name, req[i+1].Name, true
		}
	}
	return "", "", false
}

// explainInterface prints the answers and the group they place a program
// in, for --explain.
func explainInterface(w io.Writer, a appkit.Answers, g appkit.InterfaceGroup) {
	fmt.Fprintf(w, "  Interface: %s (%s), from the program's answers\n", g.Name, g.Family)
	eff := a.Effective(g)
	for _, q := range appkit.Questions {
		ans := a[q]
		mark := "no "
		if eff[q] {
			mark = "yes"
		}
		note := ans.Sure
		if eff[q] != ans.Yes {
			note = "group's usual answer; the help " + map[bool]string{true: "hinted yes", false: "said nothing"}[ans.Yes]
		} else if _, inRule := g.Rule[q]; inRule {
			note += ", places the group"
		}
		why := ""
		if ans.Why != "" {
			why = ": " + safeTerminalText(ans.Why)
		}
		fmt.Fprintf(w, "    %-15s %s  (%s)%s\n", questionLabel[q], mark, note, why)
	}
}

var questionLabel = map[appkit.Question]string{
	appkit.QKeepsRunning: "Keeps running", appkit.QChangesThings: "Changes things", appkit.QTarget: "Takes a target",
	appkit.QTwoInputs: "Two inputs", appkit.QSubcommands: "Subcommands", appkit.QWritesFile: "Writes a file",
	appkit.QTable: "Table output", appkit.QNeedsRoot: "Needs root", appkit.QCanDestroy: "Can destroy", appkit.QFullScreen: "Full-screen",
}
