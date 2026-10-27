// Package diff reads the output of diff and cmp into structures a
// side-by-side view can draw: files, hunks, and rows that pair a line on
// the left with a line on the right. It parses text only; it never runs a
// program and never reads the files being compared.
//
// Input lines are expected to have passed through mint.CleanLine, as the
// runner's output does. That removes carriage returns and expands tabs, so
// two lines that differ only in those can arrive looking the same; such a
// pair is kept as a change and flagged Hidden rather than dropped.
package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RowKind is what a row shows.
type RowKind int

const (
	Same   RowKind = iota // the line is on both sides, unchanged
	Delete                // only on the left
	Insert                // only on the right
	Change                // a line on each side, different
)

func (k RowKind) String() string {
	switch k {
	case Same:
		return "same"
	case Delete:
		return "delete"
	case Insert:
		return "insert"
	case Change:
		return "change"
	}
	return "?"
}

// Side is one side of a row.
type Side struct {
	No        int    // line number in that file, from 1
	Text      string // without diff's leading marker
	NoNewline bool   // the file ends here without a newline
}

// Row pairs a line from the left file with one from the right. Left is nil
// for an Insert, Right is nil for a Delete.
type Row struct {
	Kind        RowKind
	Left, Right *Side
	// Hidden is set on a Change whose two sides look the same once
	// cleaned: they differ in a line ending, a tab, or a character that is
	// not shown.
	Hidden bool
}

// Hunk is one "@@ -a,b +c,d @@" block.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Section            string // text after the closing "@@" (often a function name)
	Rows               []Row
}

// Added and Removed count the lines inserted and deleted in h (a Change
// counts as one of each).
func (h Hunk) Added() int   { return h.count(Insert) }
func (h Hunk) Removed() int { return h.count(Delete) }

func (h Hunk) count(k RowKind) int {
	n := 0
	for _, r := range h.Rows {
		if r.Kind == k || r.Kind == Change {
			n++
		}
	}
	return n
}

// FileStatus says what diff reported about a file.
type FileStatus int

const (
	Modified FileStatus = iota // hunks follow
	OnlyIn                     // "Only in DIR: NAME"
	Binary                     // "Binary files A and B differ"
	Differ                     // "Files A and B differ" (diff -q)
)

// File is one compared file.
type File struct {
	Status           FileStatus
	OldName, NewName string // for OnlyIn, OldName is the directory and NewName the entry
	Hunks            []Hunk
}

// Added and Removed total the lines inserted and deleted in f.
func (f File) Added() int {
	n := 0
	for _, h := range f.Hunks {
		n += h.Added()
	}
	return n
}

func (f File) Removed() int {
	n := 0
	for _, h := range f.Hunks {
		n += h.Removed()
	}
	return n
}

var (
	hunkRe   = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)
	onlyRe   = regexp.MustCompile(`^Only in (.+): (.+)$`)
	binaryRe = regexp.MustCompile(`^Binary files (.+) and (.+) differ$`)
	differRe = regexp.MustCompile(`^Files (.+) and (.+) differ$`)
	// The timestamp diff -u writes after a file name, once its tab has
	// been expanded to spaces.
	stampRe = regexp.MustCompile(`\s+\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? [+-]\d{4}$`)
)

// noNewline is the marker line diff writes after a line that ends its file
// without a newline.
const noNewline = `\ No newline at end of file`

// ParseUnified reads unified diff output (diff -u, diff -ru, git diff).
// Lines it does not recognise, such as the "diff -ru a b" line diff -r
// prints before each file, are skipped. A hunk cut short (output that
// stopped early) keeps the rows read so far.
func ParseUnified(lines []string) []File {
	p := &unifiedParser{lastSame: -1}
	for _, l := range lines {
		p.line(l)
	}
	p.endFile()
	return p.files
}

type unifiedParser struct {
	files []File
	cur   *File
	hunk  *Hunk
	// lines of the current change block, waiting to be paired
	dels, ins []Side
	// next line numbers in the current hunk, and lines still expected
	oldNo, newNo     int
	oldLeft, newLeft int
	// where "\ No newline at end of file" applies: the last pending side,
	// or (lastSame >= 0) both sides of a context row
	last     *Side
	lastSame int
}

func (p *unifiedParser) line(l string) {
	if p.hunk != nil && (p.oldLeft > 0 || p.newLeft > 0) && p.body(l) {
		return
	}
	if p.hunk != nil && l == noNewline {
		p.markNoNewline()
		return
	}
	p.endHunk()
	switch {
	case strings.HasPrefix(l, "--- "):
		p.endFile()
		p.cur = &File{Status: Modified, OldName: fileName(l[4:])}
	case strings.HasPrefix(l, "+++ ") && p.cur != nil && p.cur.NewName == "":
		p.cur.NewName = fileName(l[4:])
	case hunkRe.MatchString(l):
		if p.cur == nil {
			p.cur = &File{Status: Modified}
		}
		m := hunkRe.FindStringSubmatch(l)
		h := Hunk{OldStart: atoi(m[1]), OldLines: count(m[2]), NewStart: atoi(m[3]), NewLines: count(m[4]), Section: m[5]}
		p.cur.Hunks = append(p.cur.Hunks, h)
		p.hunk = &p.cur.Hunks[len(p.cur.Hunks)-1]
		p.oldNo, p.newNo = h.OldStart, h.NewStart
		p.oldLeft, p.newLeft = h.OldLines, h.NewLines
		// A range of 0 lines names the line before it ("-0,0" is an
		// empty file), so the first line is one further on.
		if h.OldLines == 0 {
			p.oldNo++
		}
		if h.NewLines == 0 {
			p.newNo++
		}
		// Real diff never starts a non-empty range at 0; malformed input
		// must not produce impossible line numbers.
		p.oldNo, p.newNo = max(p.oldNo, 1), max(p.newNo, 1)
	case onlyRe.MatchString(l):
		p.endFile()
		m := onlyRe.FindStringSubmatch(l)
		p.files = append(p.files, File{Status: OnlyIn, OldName: m[1], NewName: m[2]})
	case binaryRe.MatchString(l):
		p.endFile()
		m := binaryRe.FindStringSubmatch(l)
		p.files = append(p.files, File{Status: Binary, OldName: m[1], NewName: m[2]})
	case differRe.MatchString(l):
		p.endFile()
		m := differRe.FindStringSubmatch(l)
		p.files = append(p.files, File{Status: Differ, OldName: m[1], NewName: m[2]})
	}
}

// body handles a line inside a hunk. It returns false for a line that is
// not part of the hunk body.
func (p *unifiedParser) body(l string) bool {
	if l == noNewline {
		p.markNoNewline()
		return true
	}
	var marker byte = ' '
	text := ""
	if l != "" {
		marker, text = l[0], l[1:]
	}
	// An empty line is a context line whose leading space was lost (some
	// tools and editors trim trailing whitespace).
	switch marker {
	case ' ':
		if p.oldLeft == 0 || p.newLeft == 0 {
			return false
		}
		p.flush()
		p.hunk.Rows = append(p.hunk.Rows, Row{Kind: Same,
			Left:  &Side{No: p.oldNo, Text: text},
			Right: &Side{No: p.newNo, Text: text}})
		p.last, p.lastSame = nil, len(p.hunk.Rows)-1
		p.oldNo++
		p.newNo++
		p.oldLeft--
		p.newLeft--
	case '-':
		if p.oldLeft == 0 {
			return false
		}
		if len(p.ins) > 0 { // "-" after "+": a new change block
			p.flush()
		}
		p.dels = append(p.dels, Side{No: p.oldNo, Text: text})
		p.last, p.lastSame = &p.dels[len(p.dels)-1], -1
		p.oldNo++
		p.oldLeft--
	case '+':
		if p.newLeft == 0 {
			return false
		}
		p.ins = append(p.ins, Side{No: p.newNo, Text: text})
		p.last, p.lastSame = &p.ins[len(p.ins)-1], -1
		p.newNo++
		p.newLeft--
	default:
		return false
	}
	return true
}

// markNoNewline applies "\ No newline at end of file" to the line before
// it. After a context line, both files end there.
func (p *unifiedParser) markNoNewline() {
	switch {
	case p.last != nil:
		p.last.NoNewline = true
	case p.lastSame >= 0 && p.hunk != nil && p.lastSame < len(p.hunk.Rows):
		p.hunk.Rows[p.lastSame].Left.NoNewline = true
		p.hunk.Rows[p.lastSame].Right.NoNewline = true
	}
}

// flush pairs the pending deleted and inserted lines into rows: the i-th
// deletion with the i-th insertion as a Change, the rest on their own.
func (p *unifiedParser) flush() {
	if p.hunk == nil {
		return
	}
	n := len(p.dels)
	if len(p.ins) > n {
		n = len(p.ins)
	}
	for i := 0; i < n; i++ {
		var row Row
		switch {
		case i < len(p.dels) && i < len(p.ins):
			l, r := p.dels[i], p.ins[i]
			row = Row{Kind: Change, Left: &l, Right: &r, Hidden: l.Text == r.Text}
		case i < len(p.dels):
			l := p.dels[i]
			row = Row{Kind: Delete, Left: &l}
		default:
			r := p.ins[i]
			row = Row{Kind: Insert, Right: &r}
		}
		p.hunk.Rows = append(p.hunk.Rows, row)
	}
	// New slices: rows hold copies, and p.last must never point into a
	// reused array.
	p.dels, p.ins = nil, nil
	p.last, p.lastSame = nil, -1
}

func (p *unifiedParser) endHunk() {
	p.flush()
	p.hunk = nil
	p.last, p.lastSame = nil, -1
	p.oldLeft, p.newLeft = 0, 0
}

func (p *unifiedParser) endFile() {
	p.endHunk()
	if p.cur != nil {
		p.files = append(p.files, *p.cur)
		p.cur = nil
	}
}

// fileName takes the name from a "---"/"+++" line, dropping the timestamp.
func fileName(s string) string {
	return strings.TrimSpace(stampRe.ReplaceAllString(s, ""))
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// count reads a hunk range length; a missing one means 1.
func count(s string) int {
	if s == "" {
		return 1
	}
	return atoi(s)
}

// ByteDiff is one differing byte reported by cmp -l.
type ByteDiff struct {
	Offset      int64 // from 1, as cmp reports it
	Left, Right byte
}

// CmpResult is what cmp reported.
type CmpResult struct {
	Diffs []ByteDiff // cmp -l: every differing byte
	// First is the first difference from plain cmp ("a b differ: byte 3,
	// line 2"); 0 if not reported.
	First, FirstLine int64
	// ShorterFile and ShorterAt describe "cmp: EOF on NAME after byte N":
	// one file is a prefix of the other.
	ShorterFile string
	ShorterAt   int64
}

var (
	cmpLRe     = regexp.MustCompile(`^\s*(\d+)\s+([0-7]{1,3})\s+([0-7]{1,3})\s*$`)
	cmpEOFRe   = regexp.MustCompile(`^cmp: EOF on (.+?) after (?:byte|char) (\d+)`)
	cmpEmptyRe = regexp.MustCompile(`^cmp: EOF on (.+?) which is empty`)
	cmpFirstRe = regexp.MustCompile(`^.+ differ: (?:byte|char) (\d+), line (\d+)$`)
)

// ParseCmp reads cmp's output, with or without -l, from both stdout and
// stderr lines (the EOF message goes to stderr).
func ParseCmp(lines []string) CmpResult {
	var r CmpResult
	for _, l := range lines {
		switch {
		case cmpLRe.MatchString(l):
			m := cmpLRe.FindStringSubmatch(l)
			off, _ := strconv.ParseInt(m[1], 10, 64)
			a, _ := strconv.ParseUint(m[2], 8, 8)
			b, _ := strconv.ParseUint(m[3], 8, 8)
			r.Diffs = append(r.Diffs, ByteDiff{Offset: off, Left: byte(a), Right: byte(b)})
		case cmpEOFRe.MatchString(l):
			m := cmpEOFRe.FindStringSubmatch(l)
			r.ShorterFile = m[1]
			r.ShorterAt, _ = strconv.ParseInt(m[2], 10, 64)
		case cmpEmptyRe.MatchString(l):
			r.ShorterFile = cmpEmptyRe.FindStringSubmatch(l)[1]
		case cmpFirstRe.MatchString(l):
			m := cmpFirstRe.FindStringSubmatch(l)
			r.First, _ = strconv.ParseInt(m[1], 10, 64)
			r.FirstLine, _ = strconv.ParseInt(m[2], 10, 64)
		}
	}
	return r
}

// String describes a byte as cmp -l would show it in a table: hex and a
// printable ASCII character, or "." for anything else.
func (d ByteDiff) String() string {
	return fmt.Sprintf("%d: %02x %s → %02x %s", d.Offset, d.Left, printable(d.Left), d.Right, printable(d.Right))
}

func printable(b byte) string {
	if b >= 0x20 && b < 0x7f {
		return string(rune(b))
	}
	return "."
}
