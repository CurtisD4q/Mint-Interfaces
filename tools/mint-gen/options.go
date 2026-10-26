package main

import (
	"regexp"
	"strings"
)

// Placeholder is the value part of an option as the help text writes it.
type Placeholder struct {
	Raw      string   // as written: "=FILE", "<file>", "[=WHEN]", "{json|xml}", "identity_file"
	Word     string   // the core word, lower case: "file", "when", "identity_file"; "" if none
	Optional bool     // [=WHEN], [file]
	Repeat   bool     // FILE..., <file>...
	Choices  []string // {json|xml}, (a|b)
}

// Words that say nothing about what the value is.
var vaguePlaceholderWords = map[string]bool{"value": true, "val": true, "arg": true, "args": true, "argument": true, "x": true, "param": true}

// ParsePlaceholder reads a placeholder such as "=FILE", "<file>...",
// "[=WHEN]", "{json|xml}" or "identity_file".
func ParsePlaceholder(raw string) Placeholder {
	p := Placeholder{Raw: raw}
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		p.Optional = true
	}
	s = strings.Trim(s, "[]")
	s = strings.TrimPrefix(s, "=")
	if strings.HasSuffix(s, "...") || strings.HasSuffix(s, "…") {
		p.Repeat = true
		s = strings.TrimSuffix(strings.TrimSuffix(s, "..."), "…")
	}
	s = strings.Trim(s, "<>(){}[]")
	if strings.Contains(s, "|") && !strings.ContainsAny(s, " \t") {
		for _, c := range strings.Split(s, "|") {
			if c = strings.TrimSpace(c); c != "" {
				p.Choices = append(p.Choices, c)
			}
		}
		return p
	}
	p.Word = strings.ToLower(s)
	if vaguePlaceholderWords[p.Word] {
		p.Word = ""
	}
	return p
}

// lastWordPart is the last part of a compound word: "identity_file" → "file",
// "max-count" → "count".
func lastWordPart(w string) string {
	if i := strings.LastIndexAny(w, "_-."); i >= 0 && i < len(w)-1 {
		return w[i+1:]
	}
	return w
}

// IsFile reports whether the placeholder names a file or folder: a path
// word (FILE, PATH, DIR…), as a whole, as the last part (identity_file), or
// as an ending (keyfile, scriptpath, outdir).
func (p Placeholder) IsFile() bool {
	if p.Word == "" {
		return false
	}
	// Alternatives: FILE/HASHES, DATE-OR-FILE: a path if any one is.
	if alts := splitAlternatives(p.Word); len(alts) > 1 {
		for _, a := range alts {
			if (Placeholder{Word: a}).IsFile() {
				return true
			}
		}
		return false
	}
	if reFileArg.MatchString(p.Word) || reFileArg.MatchString(lastWordPart(p.Word)) {
		return true
	}
	for _, end := range []string{"file", "files", "path", "dir", "directory", "folder"} {
		if len(p.Word) > len(end) && strings.HasSuffix(p.Word, end) {
			return true
		}
	}
	return false
}

// Specific reports whether the placeholder is a word that says what the
// value is, so it, rather than the description, decides. Missing, vague
// (VALUE, ARG), one-letter (F, N) and two-sided words (PREFIX, which may be
// a folder or a text prefix) are not specific.
func (p Placeholder) Specific() bool {
	w := p.Word
	return len(w) > 1 && !ambiguousPlaceholderWords[w]
}

// Words that may or may not be paths, depending on the option.
var ambiguousPlaceholderWords = map[string]bool{"prefix": true, "location": true, "base": true, "target": true, "dest": true, "where": true}

// splitAlternatives splits "file/hashes" or "date-or-file" into its choices.
func splitAlternatives(w string) []string {
	w = strings.ReplaceAll(w, "-or-", "/")
	w = strings.ReplaceAll(w, "_or_", "/")
	var out []string
	for _, a := range strings.Split(w, "/") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// IsNotFile reports whether the placeholder names something that is not a
// path: a number, time, width, format, one of a fixed set of choices…
func (p Placeholder) IsNotFile() bool {
	if len(p.Choices) > 0 {
		return true
	}
	if p.Word == "" || p.IsFile() {
		return false
	}
	if alts := splitAlternatives(p.Word); len(alts) > 1 {
		for _, a := range alts {
			if !(Placeholder{Word: a}).IsNotFile() {
				return false
			}
		}
		return true
	}
	return reNonFileArg.MatchString(p.Word) || reNonFileArg.MatchString(lastWordPart(p.Word))
}

// OptionText is what one option line of a help text says, before mint-gen
// decides how to show the option.
type OptionText struct {
	Names       []string // every spelling on the line: "-o", "--output"
	Placeholder Placeholder
	Desc        string
	Source      string // "man", "--help"…
	Line        int    // 1-based line number in that source
}

const (
	maxOptionLine = 512   // longer lines are not option lines
	maxOptions    = 10000 // per source
)

var (
	// -i identity_file / -o file  Write output / --log-file=path
	reLowerArgLine = regexp.MustCompile(`^\s{1,12}(-{1,2}[A-Za-z0-9][-A-Za-z0-9_.]*)(?:,\s*(-{1,2}[A-Za-z0-9][-A-Za-z0-9_.]*))?[ =]([a-z][a-z0-9_.-]{0,31}(?:\.\.\.)?)(?:\s{2,}(.*)|\s*)$`)
	// -oN/-oX/-oG <file>: Output scan… / -PS/PA/PU[portlist]: TCP SYN…
	// --files[=FILE]  read filenames… (an optional value written with brackets)
	reOptionalArgLine = regexp.MustCompile(`^\s{1,12}(-{1,2}[A-Za-z0-9][-A-Za-z0-9_.]*)(?:,\s*(-{1,2}[A-Za-z0-9][-A-Za-z0-9_.]*))?(\[=?[^\]\s]{1,32}\])(?:\s{2,}(.*)|\s*)$`)
	// -t FORMAT, -w FORMAT  --to=FORMAT, --write=FORMAT (each name with its
	// own value; long names may follow instead of a description)
	reMultiNameLine = regexp.MustCompile(`^\s{1,12}((?:-{1,2}[A-Za-z0-9][-A-Za-z0-9_.]*[ =](?:[A-Z][A-Z0-9_-]{0,31}|<[^>\s]{1,32}>)(?:,\s*)?){2,})(?:\s{2,}(.*?))?\s*$`)
	reNameWithValue = regexp.MustCompile(`(-{1,2}[A-Za-z0-9][-A-Za-z0-9_.]*)[ =]([A-Z][A-Z0-9_-]{0,31}|<[^>\s]{1,32}>)`)
	// A folder rather than a file.
	reDirWord = regexp.MustCompile(`(?i)^(?:dir|dirs|directory|directories|folder|folders|dirname)$`)
	// Any line that starts like an option: "-x", "--long", "-oN/-oX".
	reOptionStart    = regexp.MustCompile(`^\s{1,12}--?[A-Za-z0-9]`)
	reSlashGroupLine = regexp.MustCompile(`^\s{1,12}(-[A-Za-z0-9]{1,4}(?:/-?[A-Za-z0-9]{1,4})+)(\[[^\]\s]{1,32}\]|<[^>\s]{1,32}>)?(?:\s+(<[^>\s]{1,32}>|[A-Z][A-Z0-9_-]{1,15}|[a-z][a-z0-9_-]{0,31}))?:?(?:\s+(.*))?$`)
)

// scanOptionLine reads option lines that the main pattern (reOptLine) does
// not: lower-case placeholders and slash-separated groups. A group gives
// one OptionText per option.
func scanOptionLine(line string) []OptionText {
	if len(line) > maxOptionLine {
		return nil
	}
	if m := reSlashGroupLine.FindStringSubmatch(line); m != nil {
		ph := m[2]
		if ph == "" {
			ph = m[3]
		}
		var out []OptionText
		for _, n := range strings.Split(m[1], "/") {
			if !strings.HasPrefix(n, "-") {
				n = "-" + n
			}
			out = append(out, OptionText{Names: []string{n}, Placeholder: ParsePlaceholder(ph), Desc: strings.TrimSpace(m[4])})
		}
		return out
	}
	if m := reMultiNameLine.FindStringSubmatch(line); m != nil {
		var names []string
		ph := ""
		for _, nv := range reNameWithValue.FindAllStringSubmatch(m[1], -1) {
			names = append(names, nv[1])
			if ph == "" {
				ph = nv[2]
			}
		}
		desc := strings.TrimSpace(m[2])
		if strings.HasPrefix(desc, "-") { // more names, not a description
			for _, nv := range reNameWithValue.FindAllStringSubmatch(desc, -1) {
				names = append(names, nv[1])
			}
			desc = ""
		}
		// Short name first, then the first long name, as elsewhere.
		out := OptionText{Names: names[:1], Placeholder: ParsePlaceholder(ph), Desc: desc}
		for _, n := range names[1:] {
			if strings.HasPrefix(n, "--") {
				out.Names = append(out.Names, n)
				break
			}
		}
		return []OptionText{out}
	}
	if m := reOptionalArgLine.FindStringSubmatch(line); m != nil {
		names := []string{m[1]}
		if m[2] != "" {
			names = append(names, m[2])
		}
		return []OptionText{{Names: names, Placeholder: ParsePlaceholder(m[3]), Desc: strings.TrimSpace(m[4])}}
	}
	if m := reLowerArgLine.FindStringSubmatch(line); m != nil {
		names := []string{m[1]}
		if m[2] != "" {
			names = append(names, m[2])
		}
		return []OptionText{{Names: names, Placeholder: ParsePlaceholder(m[3]), Desc: strings.TrimSpace(m[4])}}
	}
	return nil
}

// ScanOptions reads every option line of a help text, with both the main
// pattern and scanOptionLine, and records where each came from.
func ScanOptions(lines []string, source string) []OptionText {
	var out []OptionText
	for i, line := range lines {
		if len(out) >= maxOptions {
			break
		}
		if len(line) > maxOptionLine {
			continue
		}
		if m := reOptLine.FindStringSubmatch(line); m != nil {
			names := []string{strings.TrimSpace(m[1])}
			if s := strings.TrimSpace(m[2]); s != "" {
				names = append(names, s)
			}
			out = append(out, OptionText{Names: names, Placeholder: ParsePlaceholder(strings.TrimSpace(m[3])),
				Desc: strings.TrimSpace(m[4]), Source: source, Line: i + 1})
			continue
		}
		for _, o := range scanOptionLine(line) {
			o.Source, o.Line = source, i+1
			out = append(out, o)
		}
	}
	return out
}

// looksLikeOptionLine reports whether a line starts like an option, so a
// description that continues onto the next lines stops there.
func looksLikeOptionLine(line string) bool {
	return len(line) <= maxOptionLine && (reOptLine.MatchString(line) || reOptionStart.MatchString(line))
}
