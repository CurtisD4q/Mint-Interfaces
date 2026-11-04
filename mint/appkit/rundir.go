package appkit

import (
	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"os"
	"path/filepath"
	"strings"
)

// Where a tool runs, and what its relative paths mean.
//
// Tools that write files of their own into the folder they run in, and
// never look at that folder otherwise (nmap -oA, tshark -w, curl -O, wget,
// ffmpeg, a benchmark's scratch files), run in the app's results folder
// (apps/<tool>, or the in-memory folder), so what they write lands next to
// the app's other results rather than wherever the app was started.
//
// What you type keeps its meaning: a relative path that names something in
// the folder the app was started in (or is written ./x or ../x), and
// anything in a box for files the tool reads, is made absolute from that
// folder before the tool runs. Names in boxes for files the tool writes
// stay relative, so they land in the results folder.
//
// Every other tool keeps running in the folder the app was started in:
// tools that work on that folder when given nothing else (ls, find, rg,
// git, make, cargo, tar, rsync, mv), and tools that change the files they
// are given, where they are.

// resultsFolderArchetypes run in the results folder.
var resultsFolderArchetypes = map[string]bool{
	"ask": true, "look-up": true, "watch": true,
	"record": true, "record-sound": true,
	"probe": true, "measure": true,
	"convert": true, "generate": true, "transfer": true,
	"signal": true, "control": true, "device": true,
}

// rebase is how a relative word is treated before the tool runs.
type rebase int

const (
	rebaseMaybe    rebase = iota // the start folder's when it is there (or written ./x, ../x)
	rebaseInput                  // a path the tool reads: always the start folder's
	rebaseOutput                 // a path the tool writes: left relative (the results folder)
	rebaseExplicit               // not a path box: only a word written ./x or ../x
)

// runsInPlace reports whether the tool keeps running in the folder the app
// was started in.
func (a *ToolApp) runsInPlace() bool {
	return !resultsFolderArchetypes[a.Spec.ArchetypeID] || a.Spec.Preview != nil
}

// runDir is the folder the tool runs in: the results folder, or "" (the
// folder the app was started in) for tools that keep running there, or
// when the results folder cannot be made.
func (a *ToolApp) runDir() string {
	if a.runsInPlace() {
		return ""
	}
	dir := expandArgValue(strings.TrimSpace(a.ResultsDir))
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if err := a.makeResultsDir(dir); err != nil {
		return ""
	}
	a.giveBack(dir)
	return dir
}

// rebased is word, made absolute from the folder the app was started in
// when mode says it is the user's path. Nothing changes when the tool runs
// there anyway.
func (a *ToolApp) rebased(word string, mode rebase) string {
	return a.rebasedWord(word, mode, false)
}

// rebasedWord is rebased; operand says the word cannot be an option (it
// comes after the tool's "--"), so a name starting with "-" is a path too.
// An option's value (--in=x, if=x) is treated like a word of its own.
func (a *ToolApp) rebasedWord(word string, mode rebase, operand bool) string {
	if word == "" || mode == rebaseOutput || a.runsInPlace() {
		return word
	}
	launch := a.startDir()
	if launch == "" {
		return word
	}
	if !operand {
		if strings.HasPrefix(word, "-") || (mode != rebaseInput && strings.Contains(word, "=")) {
			if i := strings.IndexByte(word, '='); i > 0 {
				m := rebaseMaybe
				if mode == rebaseExplicit {
					m = rebaseExplicit
				}
				return word[:i+1] + a.rebasePath(launch, word[i+1:], m)
			}
			if strings.HasPrefix(word, "-") {
				return word
			}
		}
	}
	return a.rebasePath(launch, word, mode)
}

// rebasePath is one relative path, made absolute from launch by mode.
func (a *ToolApp) rebasePath(launch, word string, mode rebase) string {
	if word == "" || filepath.IsAbs(word) || strings.HasPrefix(word, "~") || strings.Contains(word, "://") {
		return word
	}
	full := filepath.Join(launch, word)
	if mode == rebaseInput || word == "." || word == ".." ||
		strings.HasPrefix(word, "./") || strings.HasPrefix(word, "../") {
		return full
	}
	if mode == rebaseExplicit {
		return word
	}
	if _, err := os.Lstat(full); err == nil {
		return full
	}
	return word
}

// startDir is the folder the app was started in.
func (a *ToolApp) startDir() string {
	if a.launchDir == "" {
		a.launchDir, _ = os.Getwd()
	}
	return a.launchDir
}

// flagRebase is how a flag's value is treated: by its role, as the path
// check on its box does.
func flagRebase(f *FlagSpec) rebase {
	switch f.Role {
	case RoleOutputFile, RoleOutputDir, RoleOutputBase, RoleLogFile:
		return rebaseOutput
	case RoleInputFile, RoleInputDir, RoleConfigFile:
		return rebaseInput
	case RoleFile, RoleDir:
		return rebaseMaybe
	}
	if f.Kind == FlagFile || f.Kind == FlagDir {
		return rebaseMaybe
	}
	if mode, dedicated := heuristicPath(f.Label, f.Hint); dedicated {
		if mode == pathOutput {
			return rebaseOutput
		}
		return rebaseMaybe
	}
	// A port, a count, a pattern: only a path written as one.
	return rebaseExplicit
}

// targetRebased rebases the words of the Target box (or the second input),
// by the positional roles, as the path check on that box does.
func (a *ToolApp) targetRebased(words []string, second bool) []string {
	roles := a.Spec.PositionalRoles
	label, hint := a.Spec.TargetLabel, a.Spec.TargetPlaceholder
	if second {
		label, hint = a.Spec.SecondTargetLabel, a.Spec.SecondTargetPlaceholder
	}
	modeFor := func(r ArgRole) rebase {
		switch mode, dedicated := rolePath(r); {
		case !dedicated:
			return rebaseMaybe
		case mode == pathOutput:
			return rebaseOutput
		case r == RoleFile || r == RoleDir:
			return rebaseMaybe
		default:
			return rebaseInput
		}
	}
	single := rebaseMaybe
	mixed := false
	switch {
	case second && len(roles) >= 2:
		single = modeFor(roles[1])
	case second && a.Spec.OutputPositional:
		single = rebaseOutput
	case !second && len(roles) == 1:
		single = modeFor(roles[0])
	case !second && len(roles) > 1:
		mixed = true
	default:
		if mode, dedicated := heuristicPath(label, hint); dedicated && mode == pathOutput {
			single = rebaseOutput
		}
	}
	operand := a.Spec.OperandSeparator
	out := make([]string, len(words))
	if !mixed {
		for i, w := range words {
			out[i] = a.rebasedWord(w, single, operand)
		}
		return out
	}
	// Several positionals in one box ("pattern file… outfile"): the last
	// word follows the last role; the rest are the user's paths only when
	// they are written as paths or are there.
	last := -1
	for i, w := range words {
		if w != "" && (operand || !strings.HasPrefix(w, "-")) {
			last = i
		}
	}
	lastMode := modeFor(roles[len(roles)-1])
	for i, w := range words {
		if i == last && lastMode == rebaseOutput {
			out[i] = w
			continue
		}
		out[i] = a.rebasedWord(w, rebaseMaybe, operand)
	}
	return out
}

// viewLog is the full-screen output viewer for this app. Its save key (s)
// offers the results folder, and saves as privately as the app's other
// saves: owner-only when the results are sensitive, then handed back to
// the user who ran sudo. noSave is for text that must stay off the disk
// (a decrypted file).
func (a *ToolApp) viewLog(title string, lines func() []mint.LogLine, noSave bool) *mint.Screen {
	level, _ := a.DataLevel()
	dir := expandArgValue(strings.TrimSpace(a.ResultsDir))
	return mint.ViewLogWith(mint.ViewLogOpts{
		Title:       title,
		Lines:       lines,
		SaveDir:     dir,
		SaveName:    DefaultOutputFilename(a.Spec.Name, "txt"),
		SavePrivate: level != DataNormal,
		NoSave:      noSave,
		OnSave: func(path string) {
			a.protectFile(level, path)
			a.giveBack(path)
		},
	})
}
