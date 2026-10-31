package mint

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// TableOpts configures an aligned, navigable multi-column table row.
type TableOpts struct {
	ID        string
	Headers   []string
	HeadersFn func() []string
	Rows      [][]string
	Get       func() [][]string
	OnSelect  func(ctx *Ctx, rowIndex int, row []string)
	MaxHeight int    // maximum rows shown inline (0 = show all)
	Empty     string // shown when there are no data rows (default "No entries")
	Separator string // column separator (default "  ")

	// Viewer keeps every column at its natural width and lets the whole
	// table scroll sideways with ← →, instead of shrinking the widest column
	// to fit the screen. The header stays visible while the body scrolls.
	// Columns can be resized with H and L.
	Viewer bool
}

// Table formats multi-column data into aligned, selectable Mint rows with a
// column header bar and automatic terminal-width fitting.
func Table(o TableOpts) Row {
	empty := o.Empty
	if empty == "" {
		empty = "No entries"
	}
	sep := o.Separator
	if sep == "" {
		sep = "  "
	}
	return Row{
		id:          o.ID,
		wtype:       "table",
		headers:     o.Headers,
		headersFn:   o.HeadersFn,
		tableRows:   o.Rows,
		tableGet:    o.Get,
		tableSelect: o.OnSelect,
		max:         o.MaxHeight,
		placeholder: empty,
		separator:   sep,
		viewer:      o.Viewer,
	}
}

func expandTable(r Row, width int) []Row {
	headers := r.headers
	if r.headersFn != nil {
		protect(func() { headers = r.headersFn() })
	}
	rows := r.tableRows
	if r.tableGet != nil {
		protect(func() { rows = r.tableGet() })
	}

	numCols := len(headers)
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	if numCols == 0 {
		return []Row{Note(r.placeholder)}
	}

	sep := r.separator
	if sep == "" {
		sep = "  "
	}
	avail := max(16, width-8)
	var colW []int
	if r.viewer {
		// Natural widths, resized where the reader asked, then scrolled.
		colW = naturalColumnWidths(headers, rows, numCols, r.colWidths)
	} else {
		colW = computeColumnWidths(headers, rows, numCols, avail, sep)
	}
	// The cells a row may show: the table scrolls inside this, so a row is
	// never wider than the screen.
	window := max(16, width-8)

	var out []Row
	if len(headers) > 0 {
		hdrLine := tableWindow(formatTableRow(headers, colW, sep), r.hscroll, window)
		ruleLine := tableWindow(formatTableRule(colW, sep), r.hscroll, window)
		out = append(out,
			Row{kind: "row", wtype: "label", style: StyleHeading, label: hdrLine, indent: 2, pin: r.viewer},
			Note("  "+ruleLine),
		)
		if r.viewer {
			out[len(out)-1].pin = true
		}
	}
	if len(rows) == 0 {
		out = append(out, Note("  "+r.placeholder))
		return out
	}

	limit := len(rows)
	if r.max > 0 && limit > r.max {
		limit = r.max
	}

	hdrCopy := append([]string(nil), headers...)
	for i := 0; i < limit; i++ {
		row := rows[i]
		line := tableWindow(formatTableRow(row, colW, sep), r.hscroll, window)
		idx := i
		rowCopy := append([]string(nil), row...)
		var act func(*Ctx)
		if r.tableSelect != nil {
			selFn := r.tableSelect
			act = func(c *Ctx) { selFn(c, idx, rowCopy) }
		} else {
			act = func(*Ctx) {
				var parts []string
				for cIdx, val := range rowCopy {
					if cIdx < len(hdrCopy) && hdrCopy[cIdx] != "" {
						parts = append(parts, hdrCopy[cIdx]+": "+val)
					} else {
						parts = append(parts, val)
					}
				}
				Message(fmt.Sprintf("Row %d", idx+1), strings.Join(parts, " · "))
			}
		}
		out = append(out, Row{
			kind:       "row",
			wtype:      "tablerow",
			label:      line,
			onActivate: act,
			// the row remembers the table it came from, so ← → can scroll
			// it and H L resize its columns.
			table:    &r,
			cells:    rowCopy,
			cellHead: append([]string(nil), hdrCopy...),
			cellIdx:  idx,
		})
	}

	if len(rows) > limit {
		allRows := rows
		allHeaders := headers
		fullW := computeColumnWidths(allHeaders, allRows, numCols, 240, sep)
		out = append(out, Action(ActionOpts{
			Label: fmt.Sprintf("View all %d rows…", len(allRows)),
			Hint:  "table",
			OnActivate: func(c *Ctx) {
				c.Push(ViewLog("Table", func() []LogLine {
					var ll []LogLine
					if len(allHeaders) > 0 {
						ll = append(ll,
							LogLine{Text: formatTableRow(allHeaders, fullW, sep)},
							LogLine{Text: formatTableRule(fullW, sep)},
						)
					}
					for _, rw := range allRows {
						ll = append(ll, LogLine{Text: formatTableRow(rw, fullW, sep)})
					}
					return ll
				}))
			},
		}))
	}
	return out
}

// TableLines returns the lines row draws at width. hscroll moves a viewer
// table sideways, in cells; a table fitted to the width ignores it.
func TableLines(row Row, width, hscroll int) []string {
	if row.viewer {
		row.hscroll = hscroll
	}
	expanded := expandTable(row, width)
	lines := make([]string, len(expanded))
	for i, r := range expanded {
		lines[i] = r.label
	}
	return lines
}

func computeColumnWidths(headers []string, rows [][]string, numCols, avail int, sep string) []int {
	if numCols <= 0 {
		return nil
	}
	colW := make([]int, numCols)
	for c := 0; c < numCols; c++ {
		colW[c] = 1
		if c < len(headers) {
			if w := util.VLen(CleanLine(headers[c])); w > colW[c] {
				colW[c] = w
			}
		}
		for _, r := range rows {
			if c < len(r) {
				if w := util.VLen(CleanLine(r[c])); w > colW[c] {
					colW[c] = w
				}
			}
		}
	}

	sepTotal := (numCols - 1) * util.VLen(sep)
	budget := max(numCols*3, avail-sepTotal)
	total := 0
	for _, w := range colW {
		total += w
	}
	minCol := max(3, min(8, budget/numCols))
	for total > budget {
		// Shrink the widest column by 1 cell per step so narrow ID/status
		// columns remain intact while wide description columns truncate cleanly.
		maxIdx := -1
		maxVal := minCol
		for c, w := range colW {
			if w > maxVal {
				maxVal = w
				maxIdx = c
			}
		}
		if maxIdx < 0 {
			break
		}
		colW[maxIdx]--
		total--
	}
	return colW
}

func formatTableRow(cells []string, colW []int, sep string) string {
	var parts []string
	for c, w := range colW {
		val := ""
		if c < len(cells) {
			val = CleanLine(strings.TrimSpace(cells[c]))
		}
		t := Truncate(val, w)
		pad := w - util.VLen(t)
		if pad > 0 && c < len(colW)-1 {
			t += strings.Repeat(" ", pad)
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, sep)
}

func formatTableRule(colW []int, sep string) string {
	var parts []string
	for _, w := range colW {
		parts = append(parts, rt.profile.repeatTo("─", max(1, w)))
	}
	return strings.Join(parts, strings.Repeat(" ", util.VLen(sep)))
}

var multiSpaceRe = regexp.MustCompile(`\s{2,}`)

// ParseTable parses CLI tabular output (pipe/Unicode box tables, tab-aligned
// tables, or whitespace-aligned tables like ps, df, ss, ls -l, and nmap) into
// column headers and rows suitable for mint.Table.
func ParseTable(lines []string) (headers []string, rows [][]string) {
	return ParseColumns(lines)
}

// ParseColumns splits tabular CLI output lines into headers and rows. It
// supports pipe/box-drawing tables (| or │), tab-separated tables, and
// space-aligned tables (including tables embedded inside command output).
func ParseColumns(lines []string) (headers []string, rows [][]string) {
	cleaned := make([]string, 0, len(lines))
	for _, raw := range lines {
		c := strings.TrimRight(CleanLine(raw), " ")
		if strings.TrimSpace(c) == "" {
			cleaned = append(cleaned, "")
			continue
		}
		cleaned = append(cleaned, c)
	}

	// 0. tree(1) output: a single indented column where │, ├── and └── are
	// the tree's own drawing, not column separators.
	if isTreeOutput(cleaned) {
		return nil, treeRows(cleaned)
	}

	// 1. Check for pipe or Unicode vertical-bar tables (| or │).
	if h, r := parsePipeTable(cleaned); len(h) > 0 || len(r) > 0 {
		return h, r
	}

	// 2. Check for tab-separated tables (\t in raw input before tab expansion).
	if h, r := parseRawTabTable(lines); len(h) > 0 || len(r) > 0 {
		return h, r
	}

	// 3. Space-aligned CLI tables.
	return parseSpaceTable(cleaned)
}

func isBorderOnlyLine(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	hasRune := false
	for _, r := range s {
		switch r {
		case '-', '=', '+', '|', ':', ' ',
			'─', '━', '│', '┃', '┌', '┐', '└', '┘',
			'├', '┤', '┬', '┴', '┼', '╭', '╮', '╰', '╯',
			'═', '║', '╔', '╗', '╚', '╝', '╠', '╣', '╦', '╩', '╬':
			hasRune = true
		default:
			return false
		}
	}
	return hasRune
}

func parsePipeTable(lines []string) ([]string, [][]string) {
	var pipeLines []string
	for _, l := range lines {
		if isBorderOnlyLine(l) {
			continue
		}
		if strings.ContainsRune(l, '|') || strings.ContainsRune(l, '│') {
			pipeLines = append(pipeLines, l)
		}
	}
	if len(pipeLines) < 2 {
		return nil, nil
	}
	var all [][]string
	for _, l := range pipeLines {
		norm := strings.ReplaceAll(l, "│", "|")
		norm = strings.TrimSpace(norm)
		norm = strings.TrimPrefix(norm, "|")
		norm = strings.TrimSuffix(norm, "|")
		parts := strings.Split(norm, "|")
		if len(parts) < 2 {
			continue
		}
		row := make([]string, len(parts))
		for i, p := range parts {
			row[i] = strings.TrimSpace(p)
		}
		all = append(all, row)
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all[0], all[1:]
}

func parseRawTabTable(lines []string) ([]string, [][]string) {
	var tabLines []string
	for _, l := range lines {
		if strings.ContainsRune(l, '\t') && strings.TrimSpace(l) != "" {
			tabLines = append(tabLines, l)
		}
	}
	if len(tabLines) < 2 {
		return nil, nil
	}
	var all [][]string
	for _, l := range tabLines {
		parts := strings.Split(strings.TrimSpace(l), "\t")
		var row []string
		for _, p := range parts {
			t := strings.TrimSpace(CleanLine(p))
			if t != "" || len(parts) > 1 {
				row = append(row, t)
			}
		}
		if len(row) >= 2 {
			all = append(all, row)
		}
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all[0], all[1:]
}

func parseSpaceTable(lines []string) ([]string, [][]string) {
	var nonEmpty []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || isBorderOnlyLine(t) {
			if len(nonEmpty) >= 2 {
				// If we already collected a table block and hit a blank line, stop if valid.
				if h, r := tryParseBlock(nonEmpty); len(h) >= 2 && len(r) >= 1 {
					return h, r
				}
				nonEmpty = nil
			}
			continue
		}
		nonEmpty = append(nonEmpty, t)
	}
	return tryParseBlock(nonEmpty)
}

func tryParseBlock(lines []string) ([]string, [][]string) {
	if len(lines) == 0 {
		return nil, nil
	}

	// Find the best header line in lines (often index 0, or a line like
	// "PORT STATE SERVICE" after a banner line).
	startIdx := 0
	for i, l := range lines {
		if looksLikeTableHeader(l) && i+1 < len(lines) {
			startIdx = i
			break
		}
	}
	block := lines[startIdx:]
	if len(block) == 0 {
		return nil, nil
	}

	hdrFields := splitHeaderFields(block[0])
	multiHdr := splitMultiSpace(block[0])

	// Strategy A: split by 2+ spaces when the header itself uses 2+ spaces
	// for every column (len(multiHdr) == len(hdrFields)).
	if len(multiHdr) >= 2 && len(multiHdr) == len(hdrFields) {
		var rows [][]string
		for _, l := range block[1:] {
			if looksLikeProseFooter(l) {
				break
			}
			cols := splitMultiSpace(l)
			if len(cols) < len(multiHdr) {
				cols = splitFieldsN(l, len(multiHdr))
			}
			if len(cols) >= 2 {
				rows = append(rows, cols)
			}
		}
		if len(rows) > 0 {
			return multiHdr, rows
		}
	}

	// Strategy B: split by whitespace capped at len(headers) columns (covers
	// ps, df, ls -l, ss, nmap where columns are single- or multi-space aligned
	// and the final column may contain spaces).
	hdr := splitHeaderFields(block[0])
	if len(hdr) < 2 {
		return nil, nil
	}
	var rows [][]string
	for _, l := range block[1:] {
		if looksLikeProseFooter(l) {
			break
		}
		cols := splitFieldsN(l, len(hdr))
		if len(cols) >= 2 {
			rows = append(rows, cols)
		}
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return hdr, rows
}

func splitMultiSpace(s string) []string {
	parts := multiSpaceRe.Split(strings.TrimSpace(s), -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitHeaderFields(s string) []string {
	// Keep common two-word header "Mounted on" together when present.
	re := strings.ReplaceAll(s, "Mounted on", "Mounted_on")
	re = strings.ReplaceAll(re, "Local Address", "Local_Address")
	re = strings.ReplaceAll(re, "Peer Address", "Peer_Address")
	fields := strings.Fields(re)
	for i, f := range fields {
		fields[i] = strings.ReplaceAll(f, "_", " ")
	}
	return fields
}

func splitFieldsN(s string, maxCols int) []string {
	s = strings.TrimSpace(s)
	if s == "" || maxCols <= 0 {
		return nil
	}
	var out []string
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		if len(out) == maxCols-1 {
			out = append(out, strings.TrimSpace(s[i:]))
			break
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

func looksLikeTableHeader(s string) bool {
	fields := strings.Fields(s)
	if len(fields) < 2 || len(fields) > 14 {
		return false
	}
	upperWords := 0
	for _, f := range fields {
		hasLetter := false
		allUpper := true
		for _, r := range f {
			if unicode.IsLetter(r) {
				hasLetter = true
				if !unicode.IsUpper(r) {
					allUpper = false
				}
			}
		}
		if hasLetter && allUpper {
			upperWords++
		}
	}
	if upperWords >= 2 && upperWords*2 >= len(fields) {
		return true
	}
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "filesystem ") ||
		strings.HasPrefix(lower, "proto ") ||
		strings.HasPrefix(lower, "netid ") ||
		strings.HasPrefix(lower, "unit ")
}

func looksLikeProseFooter(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lower, "nmap done:") ||
		strings.HasPrefix(lower, "service detection performed") ||
		strings.HasPrefix(lower, "total ")
}

// naturalColumnWidths sizes every column to its widest cell, then applies
// any width the reader has set. Nothing is shrunk to fit: the table scrolls
// instead.
func naturalColumnWidths(headers []string, rows [][]string, numCols int, set map[int]int) []int {
	colW := make([]int, numCols)
	for c := 0; c < numCols; c++ {
		colW[c] = 1
		if c < len(headers) {
			if w := util.VLen(CleanLine(headers[c])); w > colW[c] {
				colW[c] = w
			}
		}
		for _, r := range rows {
			if c < len(r) {
				if w := util.VLen(CleanLine(r[c])); w > colW[c] {
					colW[c] = w
				}
			}
		}
		colW[c] = min(colW[c], maxColumnWidth)
	}
	for c, w := range set {
		if c >= 0 && c < len(colW) && w > 0 {
			colW[c] = min(w, maxColumnWidth)
		}
	}
	return colW
}

// maxColumnWidth is the widest a viewer column may be, so one long
// description cannot make the table unreadable.
const maxColumnWidth = 120

// tableWindow returns the part of a formatted line that is visible after
// scrolling off skip cells, cut to at most width cells.
func tableWindow(line string, skip, width int) string {
	if skip <= 0 {
		return Truncate(line, width)
	}
	rest := line
	for skip > 0 && rest != "" {
		_, tail := wrapHead(rest, 1)
		if tail == rest {
			break
		}
		rest = tail
		skip--
	}
	return Truncate(rest, width)
}

// tableScrollStep is how many cells ← → move a viewer table.
const tableScrollStep = 4

// scrollTable moves t sideways by delta cells, or vertically by rows.
func scrollTable(t *Row, delta, rows int) {
	if delta != 0 {
		t.hscroll = max(0, t.hscroll+delta)
	}
	if rows != 0 {
		t.scrollRows = max(0, t.scrollRows+rows)
	}
}

// resizeTableColumn widens (d > 0) or narrows the column the reader is
// looking at: the first column whose right edge is past hscroll.
func resizeTableColumn(t *Row, hscroll, d int) {
	if t.colWidths == nil {
		t.colWidths = map[int]int{}
	}
	col := 0
	seen := 0
	for c := 0; c < len(t.headers); c++ {
		w := t.colWidths[c]
		if w <= 0 {
			w = util.VLen(t.headers[c])
		}
		if seen+w > hscroll {
			col = c
			break
		}
		seen += w + util.VLen(t.separator)
	}
	w := t.colWidths[col]
	if w <= 0 {
		w = util.VLen(t.headers[col])
	}
	t.colWidths[col] = max(4, min(w+d*2, maxColumnWidth))
}

// treeGlyphs are the characters tree(1) draws its structure with. tree's
// default output has no colour; when it is coloured the escape sequences are
// already gone by the time these lines arrive (CleanLine).
const treeGlyphs = "│├└─┌┐┬┴┼ "

// isTreeOutput reports whether the lines look like tree(1): every line either
// begins with the tree's drawing characters or is a plain name, and at least
// one line is a ├──, └── or │ branch. A pipe table has a header row of plain
// words and no branch glyphs, so it does not match.
func isTreeOutput(lines []string) bool {
	branches := 0
	named := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if strings.ContainsAny(l, "├└") {
			branches++
		}
		if strings.ContainsAny(l, "│├└─") {
			// The run before the first name must be drawing characters and
			// spaces only: a column separator with two spaces after it means
			// an aligned table, not a tree.
			if gap := treeIndent(l); gap > 0 && strings.Contains(l[gap:], "  ") {
				return false
			}
			continue
		}
		if strings.ContainsAny(l, "|┌┐┬┴┼") {
			return false // a box or pipe table
		}
		named++
	}
	// At least two branch lines, or one branch and nothing table-like.
	return branches >= 2 || (branches >= 1 && named > 0 && branches+named > 1)
}

// treeRows turns tree(1) output into one row per line, holding the line as it
// is. The indentation and the branch glyphs are the information, so they are
// kept, not split into columns.
func treeRows(lines []string) [][]string {
	var rows [][]string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		rows = append(rows, []string{strings.TrimRight(l, " ")})
	}
	return rows
}

// treeIndent returns the number of bytes of drawing characters and spaces at
// the start of a tree line: everything up to the name.
func treeIndent(l string) int {
	i := 0
	for i < len(l) {
		r, size := utf8.DecodeRuneInString(l[i:])
		if !strings.ContainsRune(treeGlyphs, r) {
			break
		}
		i += size
	}
	return i
}
