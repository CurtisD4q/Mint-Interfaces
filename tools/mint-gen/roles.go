package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Evidence is one reason for (or against) a role, with its weight.
type Evidence struct {
	Role   appkit.ArgRole
	Weight int
	Reason string
}

func (e Evidence) String() string {
	return fmt.Sprintf("%+d %s: %s", e.Weight, roleName(e.Role), e.Reason)
}

// argCase is what the rules look at for one option.
type argCase struct {
	Tool        string
	Flag        string
	Kind        appkit.FlagKind
	Placeholder Placeholder
	Desc        string // the description, without mint-gen's " (-x)" suffix
	Pathish     bool   // the value is a file or folder (and not clearly something else)
	Dirish      bool   // …a folder
}

// roleRule adds evidence for an option. Rules are small and independent: a
// new case is a new rule, not a change to the others.
type roleRule struct {
	name  string
	apply func(c argCase) []Evidence
}

// Scores at or above this make a role certain enough to act on. Below it,
// a path option is left as RoleFile/RoleDir: shown as a path, never
// written to by the app.
const roleThreshold = 3

var (
	reInputWords  = regexp.MustCompile(`(?i)\b(?:read|reads|reading|load|loads|input|from|source|use|uses|search|searches|include|includes|import)\b`)
	reLogWords    = regexp.MustCompile(`(?i)\b(?:log|logs|logging|messages|trace|debug)\b`)
	reConfigWords = regexp.MustCompile(`(?i)\b(?:config|configuration|settings|rc\s+file|profile\s+file)\b`)
	reBaseWords   = regexp.MustCompile(`(?i)\b(?:basename|base\s+name|filename\s+prefix|file\s+prefix|all\s+(?:three\s+)?(?:major\s+)?formats?)\b`)
	reDirOutWords = regexp.MustCompile(`(?i)\b(?:save|saves|saved|write|writes|written|download|downloads|downloaded|store|stores|extract|extracts|put|place|output|create)\b`)
	reTempWords   = regexp.MustCompile(`(?i)\b(?:temporar(?:y|ies)|temp|tmp|tempdir|scratch|cache)\b|tmpdir|tempdir`)
	reFormatDesc  = regexp.MustCompile(`(?i)\b(?:output|result|results|print|display)\s+(?:format|type|style)\b|\bformat\s+(?:of|for)\s+(?:the\s+)?(?:output|result|results)\b`)
	reFormatWords = regexp.MustCompile(`(?i)^(?:format|fmt|type|style|output[-_]?format|outfmt)$`)
	knownFormats  = map[string]bool{"json": true, "xml": true, "csv": true, "tsv": true, "yaml": true, "yml": true,
		"html": true, "text": true, "txt": true, "markdown": true, "md": true, "table": true, "plain": true, "raw": true}
)

// roleRules are tried in order; all of them add their evidence.
var roleRules = []roleRule{
	{"known tool", func(c argCase) []Evidence {
		kt, ok := KnownToolIO(c.Tool)
		if !ok {
			return nil
		}
		if o, listed := kt.option(c.Flag); listed {
			return []Evidence{{appkit.ArgRole(o.Role), 10, "listed for " + c.Tool + " in the known-tools list"}}
		}
		if _, hasMain := kt.mainOutput(); hasMain {
			// The list names this tool's output file, and this is not it.
			return []Evidence{{appkit.RoleOutputFile, -10, "not " + c.Tool + "'s listed output option"}}
		}
		return nil
	}},
	{"main output", func(c argCase) []Evidence {
		if reMainOutput.MatchString(c.Desc) {
			return []Evidence{{appkit.RoleOutputFile, 6, "description: writes the output or results to it"}}
		}
		return nil
	}},
	{"output name", func(c argCase) []Evidence {
		if !outputFlagNames[c.Flag] {
			return nil
		}
		switch {
		case c.Desc == "":
			return []Evidence{{appkit.RoleOutputFile, 3, "usual output option name " + c.Flag}}
		case reWritesOutput.MatchString(c.Desc):
			return []Evidence{{appkit.RoleOutputFile, 4, "usual output option name " + c.Flag + ", and the description writes"}}
		}
		return nil
	}},
	{"log", func(c argCase) []Evidence {
		if reLogWords.MatchString(c.Desc) {
			return []Evidence{{appkit.RoleLogFile, 5, "description: messages or a log"}}
		}
		return nil
	}},
	{"config", func(c argCase) []Evidence {
		if reConfigWords.MatchString(c.Desc) || reConfigWords.MatchString(c.Flag) {
			// A settings file is also read, so this outweighs "input".
			return []Evidence{{appkit.RoleConfigFile, 6, "description or name: a settings file"}}
		}
		return nil
	}},
	{"input", func(c argCase) []Evidence {
		if reInputWords.MatchString(c.Desc) {
			return []Evidence{{appkit.RoleInputFile, 5, "description: reads, loads or uses it"}}
		}
		return nil
	}},
	{"output base", func(c argCase) []Evidence {
		if reBaseWords.MatchString(c.Desc) || strings.Contains(c.Placeholder.Word, "basename") {
			return []Evidence{{appkit.RoleOutputBase, 7, "a base name the tool adds extensions to"}}
		}
		return nil
	}},
	{"temporary folder", func(c argCase) []Evidence {
		if c.Dirish && reTempWords.MatchString(c.Desc+" "+c.Flag) {
			return []Evidence{{appkit.RoleDir, 8, "a folder for temporary files"}}
		}
		return nil
	}},
	{"folder direction", func(c argCase) []Evidence {
		if !c.Dirish {
			return nil
		}
		if reDirOutWords.MatchString(c.Desc) || strings.Contains(c.Placeholder.Word, "prefix") {
			return []Evidence{{appkit.RoleOutputDir, 5, "a folder the tool saves or writes into"}}
		}
		if reInputWords.MatchString(c.Desc) {
			return []Evidence{{appkit.RoleInputDir, 5, "a folder the tool reads or searches"}}
		}
		return nil
	}},
}

// ClassifyArg decides what an option's value is for, and why. It never
// guesses towards writing: an option only gets a role that writes (output
// file, folder, base name, log) on clear evidence, and anything unclear is
// left as a plain path or value, which the app never writes to.
func ClassifyArg(tool string, f appkit.FlagSpec) (appkit.ArgRole, []Evidence) {
	if f.Kind == appkit.FlagBool || f.Kind == appkit.FlagSecret {
		return appkit.RoleValue, nil
	}
	// The known-tools list is the authority for the options it names.
	if kt, ok := KnownToolIO(tool); ok {
		if o, listed := kt.option(f.Flag); listed {
			return appkit.ArgRole(o.Role), []Evidence{{appkit.ArgRole(o.Role), 10, "listed for " + tool + " in the known-tools list"}}
		}
	}
	c := argCase{Tool: tool, Flag: f.Flag, Kind: f.Kind, Placeholder: ParsePlaceholder(f.Hint),
		Desc: stripFlagSuffix(f.Label, f.Flag)}

	// A format of the output (--format json, -t {html|pdf}).
	if ev, ok := formatEvidence(c); ok {
		return appkit.RoleFormat, ev
	}

	c.Dirish = f.Kind == appkit.FlagDir || reDirWord.MatchString(c.Placeholder.Word) || reDirWord.MatchString(lastWordPart(c.Placeholder.Word)) ||
		reJoinedDirWord.MatchString(c.Placeholder.Word) ||
		// "--prefix=PREFIX  install into PREFIX/bin": a folder prefix, but
		// not yt-dlp's "--default-search PREFIX  prefix for unqualified URLs".
		(strings.Contains(c.Placeholder.Word, "prefix") && (reFileWords.MatchString(c.Desc) || reInstallWords.MatchString(c.Desc)) && !reURLWords.MatchString(c.Desc)) ||
		// The description says it is a folder, and the placeholder names
		// nothing else ("--changed-since time" is a time, whatever the
		// description says about directories).
		(reDirDesc.MatchString(c.Desc) && (c.Placeholder.Word == "" || c.Placeholder.Word == "path" || vaguePlaceholderWords[c.Placeholder.Word]))
	c.Pathish = c.Dirish || f.Kind == appkit.FlagFile || (argIsFile(f.Hint, c.Desc) && !argIsNotFile(f.Hint, c.Desc))
	if !c.Pathish {
		return appkit.RoleValue, []Evidence{{appkit.RoleValue, 0, "the value is not a path"}}
	}

	var ev []Evidence
	for _, r := range roleRules {
		ev = append(ev, r.apply(c)...)
	}
	score := map[appkit.ArgRole]int{}
	for _, e := range ev {
		score[e.Role] += e.Weight
	}
	if c.Dirish {
		// Folder options are folders: file roles do not apply.
		delete(score, appkit.RoleOutputFile)
		delete(score, appkit.RoleInputFile)
		delete(score, appkit.RoleLogFile)
		delete(score, appkit.RoleConfigFile)
	} else {
		delete(score, appkit.RoleOutputDir)
		delete(score, appkit.RoleInputDir)
	}

	best, bestScore, tie := appkit.RoleValue, 0, false
	for _, r := range appkit.Roles() { // stable order, so ties are found the same way every time
		s, ok := score[r]
		if !ok {
			continue
		}
		switch {
		case s > bestScore:
			best, bestScore, tie = r, s, false
		case s == bestScore && s > 0:
			tie = true
		}
	}
	if bestScore < roleThreshold || tie {
		// Not sure. A path the app shows but never writes to.
		if c.Dirish {
			return appkit.RoleDir, ev
		}
		return appkit.RoleFile, ev
	}
	return best, ev
}

// formatEvidence recognises an option that sets the output format.
func formatEvidence(c argCase) ([]Evidence, bool) {
	known := 0
	for _, ch := range c.Placeholder.Choices {
		if knownFormats[strings.ToLower(ch)] {
			known++
		}
	}
	switch {
	case len(c.Placeholder.Choices) > 0 && known*2 >= len(c.Placeholder.Choices):
		return []Evidence{{appkit.RoleFormat, 5, "choices are output formats: " + strings.Join(c.Placeholder.Choices, ", ")}}, true
	case reFormatWords.MatchString(c.Placeholder.Word) && reFormatDesc.MatchString(c.Desc) && namesFormats(c.Desc):
		// "Output format: json or text" chooses a format; "Use output FORMAT
		// after completion" (curl -w) takes a free-form template instead.
		return []Evidence{{appkit.RoleFormat, 5, "sets the output format"}}, true
	}
	return nil, false
}

// roleName is a role as people read it.
func roleName(r appkit.ArgRole) string {
	switch r {
	case appkit.RoleValue:
		return "value"
	case appkit.RoleFile:
		return "file (read or written: unclear)"
	case appkit.RoleDir:
		return "folder (read or written: unclear)"
	}
	return strings.ReplaceAll(string(r), "-", " ")
}

// sortEvidence orders evidence strongest first, for reports.
func sortEvidence(ev []Evidence) []Evidence {
	out := append([]Evidence(nil), ev...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Weight > out[j].Weight })
	return out
}

var reWordsIn = regexp.MustCompile(`[A-Za-z]+`)

// Format names that are not also ordinary words ("text", "table" and
// "plain" are), for spotting formats in a description.
var distinctFormats = map[string]bool{"json": true, "xml": true, "csv": true, "tsv": true, "yaml": true, "yml": true, "html": true, "markdown": true}

// namesFormats reports whether text names at least one output format.
func namesFormats(text string) bool {
	for _, w := range reWordsIn.FindAllString(text, -1) {
		if distinctFormats[strings.ToLower(w)] {
			return true
		}
	}
	return false
}
