package main

import (
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Positional is one argument a program takes without an option name, as
// its usage line writes it: SOURCE, DEST, [FILE]..., <url>, outfile.
type Positional struct {
	Name     string         // the word as written, without brackets: "SOURCE", "outfile", "jq filter"
	Role     appkit.ArgRole // what it is for
	Optional bool           // [FILE]
	Repeat   bool           // FILE..., [FILE]...
	Why      string         // the reason for the role, for --explain
}

// UsageForm is one way of calling the program ("cp SOURCE DEST" and
// "cp SOURCE... DIRECTORY" are two forms).
type UsageForm struct {
	Line        string
	Positionals []Positional
}

// Synopsis is what the usage lines say about a program's arguments.
type Synopsis struct {
	Forms []UsageForm
}

const (
	maxUsageLines = 12
	maxUsageLen   = 400
	maxTokens     = 40
)

var reUsageStart = regexp.MustCompile(`(?i)^\s*(?:usage:|or:)\s*`)

// usageLines finds the usage lines: the man page's SYNOPSIS section, and
// "Usage:" / "or:" lines in the help, each starting with the program name.
func usageLines(name string, manLines, helpLines []string) []string {
	var out []string
	add := func(l string) {
		l = strings.TrimSpace(reUsageStart.ReplaceAllString(l, ""))
		if len(l) == 0 || len(l) > maxUsageLen || len(out) >= maxUsageLines {
			return
		}
		f := strings.Fields(l)
		if len(f) == 0 || toolBaseName(f[0]) != name {
			return
		}
		for _, o := range out {
			if o == l {
				return
			}
		}
		out = append(out, l)
	}
	inSyn := false
	for _, l := range manLines {
		t := strings.TrimSpace(l)
		if t == "SYNOPSIS" {
			inSyn = true
			continue
		}
		if inSyn {
			if isManSectionHeader(l) {
				break
			}
			add(l)
		}
	}
	for _, l := range helpLines {
		if reUsageStart.MatchString(l) {
			add(l)
		}
	}
	return out
}

// tokenizeUsage splits a usage line into words, keeping bracketed groups
// ([OPTION]..., <jq filter>, {[outfile options] outfile}...) whole.
func tokenizeUsage(line string) []string {
	var toks []string
	var cur strings.Builder
	depth := 0
	for _, r := range line {
		switch {
		case r == '[' || r == '<' || r == '{' || r == '(':
			depth++
			cur.WriteRune(r)
		case r == ']' || r == '>' || r == '}' || r == ')':
			if depth > 0 {
				depth--
			}
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && depth == 0:
			if cur.Len() > 0 {
				toks = append(toks, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
		if len(toks) >= maxTokens {
			break
		}
	}
	if cur.Len() > 0 && len(toks) < maxTokens {
		toks = append(toks, cur.String())
	}
	return toks
}

var (
	reBracketGroup = regexp.MustCompile(`\[[^\]]*\]`)
	reOptionsToken = regexp.MustCompile(`(?i)^(?:options?|option\.\.\.|options\.\.\.|global[_ ]options|flags?|switch(?:es)?|operands?|args?|arguments?|\.\.\.)$`)
	rePosInput     = regexp.MustCompile(`(?i)^(?:source|sources|src|input|inputs|infile|infiles|in|input[_ -]?file|input[_ -]?url|file|files|path|paths|archive|tarball|target)$`)
	rePosOutput    = regexp.MustCompile(`(?i)^(?:dest|destination|output|outfile|out|output[_ -]?file|output[_ -]?url|out[_ -]?file|outdir|link[_ -]?name)$`)
	rePosDir       = regexp.MustCompile(`(?i)^(?:dir|directory|folder|destdir|dest[_ -]?dir|outdir)$`)
	rePosValue     = regexp.MustCompile(`(?i)^(?:url|urls|uri|host|hosts|pattern|patterns|expr|expression|filter|command|cmd|string|text|name|number|n|count|regex|jq[_ ]filter|program|script)$`)
)

// parseUsageForm reads the positionals of one usage line.
func parseUsageForm(line string) UsageForm {
	form := UsageForm{Line: line}
	toks := tokenizeUsage(line)
	if len(toks) > 0 {
		toks = toks[1:] // the program name
	}
	for i, tok := range toks {
		// "-t DIRECTORY": the word after a bare option is that option's value.
		if i > 0 && strings.HasPrefix(toks[i-1], "-") && !strings.Contains(toks[i-1], "=") {
			continue
		}
		p, ok := positionalFromToken(tok)
		if ok {
			form.Positionals = append(form.Positionals, p)
		}
	}
	assignPositionalRoles(form.Positionals)
	return form
}

// positionalFromToken turns one usage token into a positional, or reports
// that it is an option ("-i infile", "[-T]", "[OPTION]...").
func positionalFromToken(tok string) (Positional, bool) {
	p := Positional{}
	t := tok
	if strings.HasSuffix(t, "...") || strings.HasSuffix(t, "…") {
		p.Repeat = true
		t = strings.TrimSuffix(strings.TrimSuffix(t, "..."), "…")
	}
	// {[outfile options] outfile}: the last word of a group is the argument.
	if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
		inner := strings.TrimSpace(t[1 : len(t)-1])
		inner = reBracketGroup.ReplaceAllString(inner, "")
		f := strings.Fields(inner)
		if len(f) == 0 {
			return p, false
		}
		t = f[len(f)-1]
	}
	if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
		p.Optional = true
		t = strings.TrimSpace(t[1 : len(t)-1])
		if strings.HasSuffix(t, "...") {
			p.Repeat = true
			t = strings.TrimSuffix(t, "...")
		}
	}
	t = strings.TrimSpace(strings.Trim(t, "<>"))
	switch {
	case t == "" || strings.HasPrefix(t, "-") || strings.HasPrefix(t, "[") || strings.ContainsAny(t, "|="):
		return p, false // an option, a nested optional group, a choice, or an operand like of=FILE
	case reOptionsToken.MatchString(t):
		return p, false
	}
	p.Name = t
	return p, true
}

// assignPositionalRoles decides each positional's role from its name and
// its place: in "SOURCE DEST" or "infile outfile" the last one is written.
func assignPositionalRoles(ps []Positional) {
	for i := range ps {
		w := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(ps[i].Name))
		last := lastWordPart(w)
		switch {
		case rePosOutput.MatchString(w):
			ps[i].Role, ps[i].Why = appkit.RoleOutputFile, "named as the output ("+ps[i].Name+")"
		case rePosDir.MatchString(w):
			ps[i].Role, ps[i].Why = appkit.RoleDir, "a folder"
		case rePosValue.MatchString(w) || rePosValue.MatchString(last):
			ps[i].Role, ps[i].Why = appkit.RoleValue, "named like a value ("+ps[i].Name+")"
		case rePosInput.MatchString(w) || rePosInput.MatchString(last):
			ps[i].Role, ps[i].Why = appkit.RoleInputFile, "named as an input ("+ps[i].Name+")"
		default:
			ps[i].Role, ps[i].Why = appkit.RoleValue, "not named as a path"
		}
	}
	// A folder after inputs is where they go: "SOURCE... DIRECTORY".
	for i := range ps {
		if ps[i].Role == appkit.RoleDir && i > 0 && ps[i-1].Role == appkit.RoleInputFile && i == len(ps)-1 {
			ps[i].Role, ps[i].Why = appkit.RoleOutputDir, "a folder after the inputs, where they go"
		}
	}
}

// ParseSynopsis reads every usage form of a program and the role of each
// positional argument.
func ParseSynopsis(name string, manLines, helpLines []string) Synopsis {
	var s Synopsis
	for _, l := range usageLines(name, manLines, helpLines) {
		s.Forms = append(s.Forms, parseUsageForm(l))
	}
	return s
}

// OutputPositional is the positional the program writes its results to in
// its first usage form (ffmpeg's outfile, cp's DEST), if there is one.
func (s Synopsis) OutputPositional() (Positional, int, bool) {
	if len(s.Forms) == 0 {
		return Positional{}, 0, false
	}
	for i, p := range s.Forms[0].Positionals {
		if p.Role == appkit.RoleOutputFile || p.Role == appkit.RoleOutputDir {
			return p, i, true
		}
	}
	return Positional{}, 0, false
}
