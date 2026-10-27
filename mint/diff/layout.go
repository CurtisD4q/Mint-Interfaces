package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// Mode is how differences are laid out.
type Mode int

const (
	Auto       Mode = iota // side by side when there is room, else unified
	SideBySide             // two columns
	Unified                // one column, - and + lines
)

// SideBySideMinWidth is the narrowest width Auto shows two columns at.
const SideBySideMinWidth = 100

// Options tunes Layout.
type Options struct {
	Mode       Mode
	LeftLabel  string // shown over the left column ("" = the file's own name)
	RightLabel string
	// ShowSpace shows spaces and tabs on changed lines as · and →, so a
	// whitespace-only change is visible.
	ShowSpace bool
}

// Layout turns parsed diff output into styled lines for a view width
// cells wide. Every file and hunk header is an anchor. Text is cleaned by
// the view when drawn.
func Layout(files []File, width int, o Options) []mint.StyledLine {
	mode := o.Mode
	if mode == Auto {
		mode = Unified
		if width >= SideBySideMinWidth {
			mode = SideBySide
		}
	}
	var out []mint.StyledLine
	if len(files) == 0 {
		return []mint.StyledLine{{Segs: []mint.Seg{{Text: "No differences.", Role: mint.RoleMuted}}}}
	}
	for fi, f := range files {
		if fi > 0 {
			out = append(out, mint.StyledLine{})
		}
		out = append(out, fileHeader(f, o))
		switch f.Status {
		case OnlyIn:
			continue
		case Binary:
			out = append(out, note("Binary files: the contents differ; there are no lines to compare. Use cmp to see which bytes.", mint.RoleWarn))
			continue
		case Differ:
			out = append(out, note("The files differ. Run without -q (brief) to see how.", mint.RoleMuted))
			continue
		}
		numW := numberWidth(f)
		for _, h := range f.Hunks {
			out = append(out, hunkHeader(h))
			if mode == SideBySide {
				out = append(out, sideBySide(h, width, numW, o)...)
			} else {
				out = append(out, unified(h, width, numW, o)...)
			}
		}
	}
	return out
}

func note(s string, r mint.Role) mint.StyledLine {
	return mint.StyledLine{Segs: []mint.Seg{{Text: "  " + s, Role: r}}}
}

func fileHeader(f File, o Options) mint.StyledLine {
	l, r := f.OldName, f.NewName
	if o.LeftLabel != "" {
		l = o.LeftLabel
	}
	if o.RightLabel != "" {
		r = o.RightLabel
	}
	var segs []mint.Seg
	switch f.Status {
	case OnlyIn:
		segs = []mint.Seg{{Text: "Only in " + f.OldName + ": ", Role: mint.RoleAccent}, {Text: f.NewName, Role: mint.RoleWarn}}
	default:
		segs = []mint.Seg{{Text: l, Role: mint.RoleAccent}, {Text: "  ↔  ", Role: mint.RoleMuted}, {Text: r, Role: mint.RoleAccent}}
		if f.Status == Modified {
			segs = append(segs,
				mint.Seg{Text: fmt.Sprintf("   +%d", f.Added()), Role: mint.RoleAdded},
				mint.Seg{Text: fmt.Sprintf(" −%d", f.Removed()), Role: mint.RoleRemoved})
		}
	}
	return mint.StyledLine{Segs: segs, Anchor: true}
}

func hunkHeader(h Hunk) mint.StyledLine {
	text := fmt.Sprintf("@@ lines %d–%d → %d–%d", h.OldStart, h.OldStart+max(h.OldLines, 1)-1, h.NewStart, h.NewStart+max(h.NewLines, 1)-1)
	segs := []mint.Seg{{Text: text, Role: mint.RoleMuted}}
	if h.Section != "" {
		segs = append(segs, mint.Seg{Text: "  " + h.Section, Role: mint.RoleText})
	}
	return mint.StyledLine{Segs: segs, Anchor: true}
}

// numberWidth is the width of the largest line number in f.
func numberWidth(f File) int {
	n := 1
	for _, h := range f.Hunks {
		n = max(n, h.OldStart+h.OldLines, h.NewStart+h.NewLines)
	}
	return len(strconv.Itoa(n))
}

func num(s *Side, w int) string {
	if s == nil {
		return strings.Repeat(" ", w)
	}
	return fmt.Sprintf("%*d", w, s.No)
}

// rowRoles gives the base role and gutter marker of each side of a row.
func rowRoles(k RowKind) (left, right mint.Role, lm, rm string) {
	switch k {
	case Delete:
		return mint.RoleRemoved, mint.RoleMuted, "-", " "
	case Insert:
		return mint.RoleMuted, mint.RoleAdded, " ", "+"
	case Change:
		return mint.RoleRemoved, mint.RoleAdded, "-", "+"
	}
	return mint.RoleText, mint.RoleText, " ", " "
}

func sideBySide(h Hunk, width, numW int, o Options) []mint.StyledLine {
	// "NNN - text │ NNN + text": two numbers, two markers, separator.
	textW := width - 2*(numW+3) - 3
	colW, rightW := textW/2, textW-textW/2
	if colW < 8 {
		return unified(h, width, numW, o)
	}
	var out []mint.StyledLine
	for _, r := range h.Rows {
		lr, rr, lm, rm := rowRoles(r.Kind)
		var ls, rs []Span
		if r.Kind == Change && !r.Hidden {
			ls, rs = InlineChanges(r.Left.Text, r.Right.Text)
		}
		var segs []mint.Seg
		segs = append(segs, mint.Seg{Text: num(r.Left, numW) + " " + lm + " ", Role: mint.RoleMuted})
		segs = append(segs, cell(r.Left, ls, lr, colW, r.Kind != Same && o.ShowSpace)...)
		segs = append(segs, mint.Seg{Text: " │ ", Role: mint.RoleMuted})
		segs = append(segs, mint.Seg{Text: num(r.Right, numW) + " " + rm + " ", Role: mint.RoleMuted})
		segs = append(segs, cell(r.Right, rs, rr, rightW, r.Kind != Same && o.ShowSpace)...)
		out = append(out, mint.StyledLine{Segs: segs})
		if r.Hidden {
			out = append(out, note(strings.Repeat(" ", numW)+"  ⚠ these lines differ only in characters not shown (line ending, tab, or invisible text)", mint.RoleWarn))
		}
	}
	return out
}

func unified(h Hunk, width, numW int, o Options) []mint.StyledLine {
	textW := max(8, width-2*numW-4)
	var out []mint.StyledLine
	line := func(s *Side, spans []Span, marker string, role mint.Role, show bool) {
		l, r := num(s, numW), strings.Repeat(" ", numW)
		if marker == "+" {
			l, r = strings.Repeat(" ", numW), num(s, numW)
		} else if marker == " " {
			r = l
		}
		segs := []mint.Seg{{Text: l + " " + r + " " + marker + " ", Role: mint.RoleMuted}}
		segs = append(segs, cell(s, spans, role, textW, show)...)
		out = append(out, mint.StyledLine{Segs: segs})
	}
	for _, r := range h.Rows {
		switch r.Kind {
		case Same:
			// Show the right-hand number too: the two differ after a change.
			segs := []mint.Seg{{Text: num(r.Left, numW) + " " + num(r.Right, numW) + "   ", Role: mint.RoleMuted}}
			segs = append(segs, cell(r.Left, nil, mint.RoleText, textW, false)...)
			out = append(out, mint.StyledLine{Segs: segs})
		case Delete:
			line(r.Left, nil, "-", mint.RoleRemoved, o.ShowSpace)
		case Insert:
			line(r.Right, nil, "+", mint.RoleAdded, o.ShowSpace)
		case Change:
			var ls, rs []Span
			if !r.Hidden {
				ls, rs = InlineChanges(r.Left.Text, r.Right.Text)
			}
			line(r.Left, ls, "-", mint.RoleRemoved, o.ShowSpace)
			line(r.Right, rs, "+", mint.RoleAdded, o.ShowSpace)
			if r.Hidden {
				out = append(out, note(strings.Repeat(" ", 2*numW)+"  ⚠ these lines differ only in characters not shown (line ending, tab, or invisible text)", mint.RoleWarn))
			}
		}
	}
	return out
}

// cell renders one side's text, w cells wide (padded or cut with …), with
// changed spans marked. A missing side is blank.
func cell(s *Side, spans []Span, role mint.Role, w int, showSpace bool) []mint.Seg {
	if s == nil {
		return []mint.Seg{{Text: strings.Repeat(" ", w), Role: mint.RoleMuted}}
	}
	// Clean first, so widths and spans count what will be drawn. Spans
	// refer to the raw text's runes; cleaning keeps runes one for one
	// except for marked invisible characters, so recompute on the
	// cleaned text when they differ.
	text := mint.CleanLineVisible(s.Text)
	if text != s.Text && len(spans) > 0 {
		spans = nil
		role = mint.RoleMark
	}
	rs := []rune(text)
	mark := make([]bool, len(rs))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < len(rs); i++ {
			mark[i] = true
		}
	}
	var segs []mint.Seg
	var cur strings.Builder
	curMark, used := false, 0
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		r := role
		if curMark {
			r = mint.RoleMark
		}
		segs = append(segs, mint.Seg{Text: cur.String(), Role: r})
		cur.Reset()
	}
	for i, r := range rs {
		ch := string(r)
		if showSpace {
			switch r {
			case ' ':
				ch = "·"
			case '\t':
				ch = "→"
			}
		}
		cw := util.VLen(ch)
		if used+cw > w-1 && i < len(rs)-1 || used+cw > w {
			flush()
			segs = append(segs, mint.Seg{Text: "…", Role: mint.RoleMuted})
			used++
			break
		}
		if mark[i] != curMark {
			flush()
			curMark = mark[i]
		}
		cur.WriteString(ch)
		used += cw
	}
	flush()
	if s.NoNewline && used+2 <= w {
		segs = append(segs, mint.Seg{Text: " ⊘", Role: mint.RoleWarn})
		used += 2
	}
	if used < w {
		segs = append(segs, mint.Seg{Text: strings.Repeat(" ", w-used), Role: mint.RoleText})
	}
	return segs
}

// LayoutCmp lays out cmp -l output as a table of differing bytes.
func LayoutCmp(r CmpResult, leftLabel, rightLabel string) []mint.StyledLine {
	var out []mint.StyledLine
	if len(r.Diffs) == 0 && r.First == 0 && r.ShorterFile == "" {
		return []mint.StyledLine{{Segs: []mint.Seg{{Text: "No differences.", Role: mint.RoleMuted}}}}
	}
	if r.First > 0 {
		out = append(out, mint.StyledLine{Anchor: true, Segs: []mint.Seg{
			{Text: fmt.Sprintf("First difference at byte %d, line %d", r.First, r.FirstLine), Role: mint.RoleAccent}}})
	}
	if len(r.Diffs) > 0 {
		out = append(out, mint.StyledLine{Anchor: true, Segs: []mint.Seg{
			{Text: fmt.Sprintf("%d bytes differ", len(r.Diffs)), Role: mint.RoleAccent}}})
		out = append(out, mint.StyledLine{Segs: []mint.Seg{
			{Text: fmt.Sprintf("%12s   %-14s   %-14s", "byte", trimLabel(leftLabel, "left"), trimLabel(rightLabel, "right")), Role: mint.RoleMuted}}})
		for _, d := range r.Diffs {
			out = append(out, mint.StyledLine{Segs: []mint.Seg{
				{Text: fmt.Sprintf("%12d   ", d.Offset), Role: mint.RoleMuted},
				{Text: fmt.Sprintf("%02x %-11s", d.Left, "'"+printable(d.Left)+"'"), Role: mint.RoleRemoved},
				{Text: "   ", Role: mint.RoleMuted},
				{Text: fmt.Sprintf("%02x %-11s", d.Right, "'"+printable(d.Right)+"'"), Role: mint.RoleAdded},
			}})
		}
	}
	if r.ShorterFile != "" {
		var msg string
		if r.ShorterAt > 0 {
			msg = fmt.Sprintf("%s ends after byte %d; the other file goes on", r.ShorterFile, r.ShorterAt)
		} else {
			msg = r.ShorterFile + " is empty"
		}
		out = append(out, mint.StyledLine{Anchor: true, Segs: []mint.Seg{{Text: msg, Role: mint.RoleWarn}}})
	}
	return out
}

func trimLabel(s, def string) string {
	if s == "" {
		return def
	}
	return util.Truncate(s, 14)
}
