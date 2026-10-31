package appkit

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// pathMissingLabel is the warning appended to a box's own label. The text
// is the warning: a monochrome terminal (NO_COLOR) has no red to show.
const pathMissingLabel = " - File path does not exist!"

// boxKind says which box was committed, so the path check knows whether
// the value is an input, an output, or free text.
type boxKind int

const (
	boxPlain boxKind = iota
	boxTarget
	boxTarget2
	boxExtra
	boxOutDir
	boxOutFile
	boxFlag
	boxPathInput // a folder the tool reads (the served directory)
)

type pathMode int

const (
	pathInput pathMode = iota
	pathOutput
)

// expWord is one word after quote removal and expansion.
type expWord struct {
	Text    string
	FromVar bool // a $VAR or ${VAR} in this word was expanded, or left as typed
}

// expandInput rewrites s the way Enter does. Quoting and word splitting
// follow splitShellWords: quotes are removed, and a word that contains a
// space is quoted again. A leading unquoted ~ or ~/ becomes the home
// directory, including after "=" (--output=~/x). $VAR and ${VAR} expand
// from the environment in unquoted text and in double quotes; nothing
// expands inside single quotes, and ~ is not expanded inside quotes.
// An unset variable is left as typed and named in unset. | ; && $(...)
// backticks and globs stay literal text. Nothing is executed.
func expandInput(s string) (display string, words []expWord, unset []string) {
	words, unset = scanExpand(s)
	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.Text
	}
	return joinShellWords(texts), words, unset
}

// expandArgWords is the run-time form of expandInput: the words that
// actually become arguments. Unset variables stay as typed.
func expandArgWords(s string) []string {
	words, _ := scanExpand(s)
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = w.Text
	}
	return out
}

// expandArgValue expands one flag value. The value stays a single argument
// even when it contains spaces; only ~ and $VAR change.
func expandArgValue(s string) string {
	return strings.Join(expandArgWords(s), " ")
}

func scanExpand(s string) (words []expWord, unset []string) {
	var cur strings.Builder
	fromVar := false
	quote := rune(0)
	atStart := true
	afterEq := false

	flush := func() {
		if cur.Len() > 0 {
			words = append(words, expWord{Text: cur.String(), FromVar: fromVar})
		}
		cur.Reset()
		fromVar = false
		atStart = true
		afterEq = false
	}

	for i := 0; i < len(s); {
		c := s[i]
		if quote != 0 {
			if c < utf8.RuneSelf && rune(c) == quote {
				quote = 0
				i++
				continue
			}
			if quote == '"' && c == '$' {
				if n, name, raw, ok := takeVar(s[i:]); ok {
					writeVar(&cur, &fromVar, &unset, name, raw)
					atStart = false
					afterEq = false
					i += n
					continue
				}
			}
			r, size := utf8.DecodeRuneInString(s[i:])
			cur.WriteRune(r)
			atStart = false
			afterEq = false
			i += size
			continue
		}
		switch c {
		case '\'', '"':
			quote = rune(c)
			i++
		case ' ', '\t':
			flush()
			i++
		case '~':
			if (atStart && cur.Len() == 0) || afterEq {
				if rep, n, ok := takeTilde(s[i:]); ok {
					cur.WriteString(rep)
					atStart = false
					afterEq = false
					i += n
					continue
				}
			}
			cur.WriteByte(c)
			atStart = false
			afterEq = false
			i++
		case '$':
			if n, name, raw, ok := takeVar(s[i:]); ok {
				writeVar(&cur, &fromVar, &unset, name, raw)
				atStart = false
				afterEq = false
				i += n
				continue
			}
			cur.WriteByte(c)
			atStart = false
			afterEq = false
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			cur.WriteRune(r)
			atStart = false
			afterEq = r == '='
			i += size
		}
	}
	flush()
	return words, unset
}

func writeVar(cur *strings.Builder, fromVar *bool, unset *[]string, name, raw string) {
	*fromVar = true
	if val, ok := os.LookupEnv(name); ok {
		cur.WriteString(val)
		return
	}
	cur.WriteString(raw)
	*unset = append(*unset, name)
}

// takeTilde expands a leading "~" or "~/". "~user" is left alone.
func takeTilde(s string) (rep string, n int, ok bool) {
	if s == "" || s[0] != '~' {
		return "", 0, false
	}
	if len(s) >= 2 && s[1] == '/' {
		return util.ExpandUser("~/"), 2, true
	}
	if len(s) == 1 || s[1] == ' ' || s[1] == '\t' || s[1] == '\'' || s[1] == '"' || s[1] == '=' {
		return util.ExpandUser("~"), 1, true
	}
	return "", 0, false
}

// takeVar reads $NAME or ${NAME}. $(...) is not a variable: it stays
// literal, so command substitution never runs.
func takeVar(s string) (n int, name, raw string, ok bool) {
	if len(s) < 2 || s[0] != '$' {
		return 0, "", "", false
	}
	if s[1] == '{' {
		end := strings.IndexByte(s, '}')
		if end < 2 {
			return 0, "", "", false
		}
		name = s[2:end]
		if !validVar(name) {
			return 0, "", "", false
		}
		return end + 1, name, s[:end+1], true
	}
	if !varStart(s[1]) {
		return 0, "", "", false
	}
	j := 2
	for j < len(s) && varCont(s[j]) {
		j++
	}
	return j, s[1:j], s[:j], true
}

func varStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func varCont(c byte) bool {
	return varStart(c) || (c >= '0' && c <= '9')
}

func validVar(name string) bool {
	if name == "" || !varStart(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !varCont(name[i]) {
			return false
		}
	}
	return true
}

func (a *ToolApp) boxLabel(id, label string) string {
	if a.fieldErr[id] {
		return label + pathMissingLabel
	}
	return label
}

func (a *ToolApp) boxBad(id string) bool {
	return a.fieldErr[id]
}

// textInput is the shared text box: Enter expands ~ and $VAR in place and
// marks the row when a variable is unset or a path is missing.
func (a *ToolApp) textInput(id, label, placeholder string, get func() string, set func(string), kind boxKind, flag *FlagSpec) mint.Row {
	var spec *FlagSpec
	if flag != nil {
		copied := *flag
		spec = &copied
	}
	return mint.Text(mint.TextOpts{
		ID:          id,
		Label:       a.boxLabel(id, label),
		Placeholder: placeholder,
		Alert:       a.boxBad(id),
		Get:         get,
		Set: func(v string) {
			set(a.acceptText(id, v, kind, spec))
		},
	})
}

// acceptText expands raw, records an error on id, and returns the text to
// store. The error clears when a later commit has no unset variable and no
// missing path.
func (a *ToolApp) acceptText(id, raw string, kind boxKind, flag *FlagSpec) string {
	if a.fieldErr == nil {
		a.fieldErr = map[string]bool{}
	}
	display, words, unset := expandInput(strings.TrimSpace(raw))
	if len(unset) > 0 || a.pathsFail(kind, flag, words) {
		a.fieldErr[id] = true
	} else {
		delete(a.fieldErr, id)
	}
	return display
}

func (a *ToolApp) pathsFail(kind boxKind, flag *FlagSpec, words []expWord) bool {
	switch kind {
	case boxExtra:
		return a.extraPathsFail(words)
	case boxPlain:
		return false
	case boxOutDir:
		return wordsMissing(words, pathOutput, true)
	case boxOutFile:
		// A bare file name is created in the save directory. Only a path
		// (or a value that came from $VAR) is checked, and only its parent.
		return wordsMissing(words, pathOutput, false)
	case boxFlag:
		mode, dedicated := flagPathMode(flag)
		if !dedicated {
			return false
		}
		return wordsMissing(words, mode, true)
	case boxPathInput:
		return wordsMissing(words, pathInput, true)
	case boxTarget:
		return a.targetPathsFail(words, false)
	case boxTarget2:
		return a.targetPathsFail(words, true)
	default:
		return false
	}
}

func (a *ToolApp) targetPathsFail(words []expWord, second bool) bool {
	roles := a.Spec.PositionalRoles
	label, hint := a.Spec.TargetLabel, a.Spec.TargetPlaceholder
	if second {
		label, hint = a.Spec.SecondTargetLabel, a.Spec.SecondTargetPlaceholder
		if len(roles) >= 2 {
			return roleWordsMissing(words, roles[1])
		}
		if a.Spec.OutputPositional {
			return wordsMissing(words, pathOutput, true)
		}
		mode, dedicated := heuristicPath(label, hint)
		if dedicated {
			return wordsMissing(words, mode, true)
		}
		return wordsMissing(words, pathInput, false)
	}
	switch len(roles) {
	case 0:
		mode, dedicated := heuristicPath(label, hint)
		if dedicated {
			return wordsMissing(words, mode, true)
		}
		return wordsMissing(words, pathInput, false)
	case 1:
		return roleWordsMissing(words, roles[0])
	default:
		return mixedTargetFail(words, roles)
	}
}

func roleWordsMissing(words []expWord, role ArgRole) bool {
	mode, dedicated := rolePath(role)
	if dedicated {
		return wordsMissing(words, mode, true)
	}
	return wordsMissing(words, pathInput, false)
}

// mixedTargetFail handles one box that holds several positionals
// ("pattern file… outfile"). Plain words are not paths. The last word
// follows the last role, so an output only needs its parent directory.
func mixedTargetFail(words []expWord, roles []ArgRole) bool {
	lastOut := false
	if len(roles) > 0 {
		if mode, dedicated := rolePath(roles[len(roles)-1]); dedicated && mode == pathOutput {
			lastOut = true
		}
	}
	last := -1
	for i, w := range words {
		if w.Text != "" && !strings.HasPrefix(w.Text, "-") {
			last = i
		}
	}
	for i, w := range words {
		if w.Text == "" || strings.HasPrefix(w.Text, "-") {
			continue
		}
		if i == last && lastOut {
			if parentMissing(w.Text) {
				return true
			}
			continue
		}
		if !w.FromVar && !looksLikePath(w.Text) {
			continue
		}
		if !pathExists(w.Text) {
			return true
		}
	}
	return false
}

func rolePath(r ArgRole) (pathMode, bool) {
	switch r {
	case RoleOutputFile, RoleOutputDir, RoleOutputBase, RoleLogFile:
		return pathOutput, true
	case RoleInputFile, RoleInputDir, RoleConfigFile, RoleFile, RoleDir:
		return pathInput, true
	default:
		return pathInput, false
	}
}

func flagPathMode(f *FlagSpec) (pathMode, bool) {
	if f == nil {
		return pathInput, false
	}
	switch f.Role {
	case RoleOutputFile, RoleOutputDir, RoleOutputBase, RoleLogFile:
		return pathOutput, true
	case RoleInputFile, RoleInputDir, RoleConfigFile, RoleFile, RoleDir:
		return pathInput, true
	case RoleFormat:
		return pathInput, false
	default:
		// No role was recorded. The metavar and the description decide.
		return heuristicPath(f.Label, f.Hint)
	}
}

var (
	rePathWord   = regexp.MustCompile(`(?i)\b(?:file|files|dir|dirs|directory|directories|folder|folders|path|paths|outfile|outdir)\b`)
	reOutputWord = regexp.MustCompile(`(?i)\b(?:output|outfile|outdir|dest|destination|save|saved|write|writes|create)\b`)
)

// heuristicPath is the fallback when a box has no argument role. The
// label counts when it names a file, directory or path. The hint counts
// only when it is a metavar (FILE, DIR, OUTFILE), not a sentence that
// happens to mention a file ("arguments, file, or host"). An output name
// only needs its parent directory.
func heuristicPath(label, hint string) (pathMode, bool) {
	path := rePathWord.MatchString(label)
	output := reOutputWord.MatchString(label)
	if p, o := metavarPath(hint); p {
		path = true
		output = output || o
	}
	if !path {
		return pathInput, false
	}
	if output {
		return pathOutput, true
	}
	return pathInput, true
}

func metavarPath(hint string) (path, output bool) {
	h := strings.Trim(strings.TrimSpace(hint), "<>[]")
	if h == "" || strings.ContainsAny(h, " \t,;") || strings.Contains(strings.ToLower(h), "e.g.") {
		return false, false
	}
	return rePathWord.MatchString(h), reOutputWord.MatchString(h)
}

func wordsMissing(words []expWord, mode pathMode, dedicated bool) bool {
	for _, w := range words {
		if w.Text == "" || strings.HasPrefix(w.Text, "-") {
			continue
		}
		if !dedicated && !w.FromVar && !looksLikePath(w.Text) {
			continue
		}
		if mode == pathOutput {
			if parentMissing(w.Text) {
				return true
			}
			continue
		}
		if !pathExists(w.Text) {
			return true
		}
	}
	return false
}

func looksLikePath(w string) bool {
	return strings.HasPrefix(w, "/") ||
		strings.HasPrefix(w, "~") ||
		strings.HasPrefix(w, "./") ||
		strings.HasPrefix(w, "../")
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func parentMissing(p string) bool {
	if p == "" {
		return false
	}
	parent := filepath.Dir(p)
	if parent == "" {
		return false
	}
	info, err := os.Stat(parent)
	return err != nil || !info.IsDir()
}

// outputFlagSet is every flag whose value is a file or directory the tool
// creates: the main output flag, and any parsed flag with an output role
// or an output metavar.
func (a *ToolApp) outputFlagSet() map[string]struct{} {
	m := map[string]struct{}{}
	add := func(flag string) {
		flag = strings.TrimRight(strings.TrimSpace(flag), "=")
		if flag != "" {
			m[flag] = struct{}{}
		}
	}
	add(a.Spec.OutputFlag)
	for i := range a.Spec.Flags {
		mode, dedicated := flagPathMode(&a.Spec.Flags[i])
		if dedicated && mode == pathOutput {
			add(a.Spec.Flags[i].Flag)
		}
	}
	return m
}

func (a *ToolApp) extraPathsFail(words []expWord) bool {
	outs := a.outputFlagSet()
	for i := 0; i < len(words); i++ {
		w := words[i]
		if path, ok := assignedOutput(w.Text, outs); ok {
			if path != "" && (w.FromVar || looksLikePath(path)) && parentMissing(path) {
				return true
			}
			continue
		}
		if _, out := outs[w.Text]; out {
			if i+1 < len(words) {
				nxt := words[i+1]
				if nxt.Text != "" && !strings.HasPrefix(nxt.Text, "-") && (nxt.FromVar || looksLikePath(nxt.Text)) {
					if parentMissing(nxt.Text) {
						return true
					}
					i++
					continue
				}
			}
			continue
		}
		if w.Text == "" || strings.HasPrefix(w.Text, "-") {
			continue
		}
		if !w.FromVar && !looksLikePath(w.Text) {
			continue
		}
		if !pathExists(w.Text) {
			return true
		}
	}
	return false
}

// assignedOutput reports the path in "--output=/tmp/x" when "--output" is
// a known output flag. ok is false for a bare "-o" (the path is the next word).
func assignedOutput(word string, outs map[string]struct{}) (path string, ok bool) {
	for flag := range outs {
		pref := flag + "="
		if strings.HasPrefix(word, pref) {
			return word[len(pref):], true
		}
	}
	return "", false
}

func stdoutEmpty(lines []mint.LogLine) bool {
	for _, l := range lines {
		if !l.Err && strings.TrimSpace(l.Text) != "" {
			return false
		}
	}
	return true
}

// readToolOutputFile loads text the tool wrote itself, when nothing came
// back on standard output. A binary file (a NUL in the first chunk) is
// left on disk and not poured into the pane.
func readToolOutputFile(path string) []string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return nil
	}
	n := info.Size()
	const maxBytes int64 = 1 << 20
	if n > maxBytes {
		n = maxBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, int(n))
	got, err := io.ReadFull(f, buf)
	if got == 0 || (err != nil && err != io.ErrUnexpectedEOF && err != io.EOF) {
		return nil
	}
	buf = buf[:got]
	if bytes.IndexByte(buf, 0) >= 0 {
		return nil
	}
	text := strings.TrimRight(string(buf), "\n")
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	if len(parts) > 5000 {
		parts = parts[:5000]
	}
	out := make([]string, len(parts))
	for i, l := range parts {
		out[i] = mint.CleanLine(l)
	}
	return out
}
