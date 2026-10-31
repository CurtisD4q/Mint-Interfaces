package mint

import (
	"fmt"
	"strings"
)

// paneBand is one horizontal slice of the content area. index 0 is the
// screen's own sections; 1 and up are Panes. A divider row of 0 means this
// band has no divider (the top pane, while it is open).
type paneBand struct {
	index     int
	divRow    int
	lead      int // blank rows reserved immediately above divRow
	top       int
	rows      int
	collapsed bool
	hidden    bool
}

// SelectedTableRow reports the table row under the cursor in the top pane,
// once the screen has been drawn. ok is false when the cursor is not on a
// table row.
func (s *Screen) SelectedTableRow() (headers, cells []string, index int, ok bool) {
	if s == nil || !s.selOK {
		return nil, nil, 0, false
	}
	return s.selRow.TableCells()
}

func (s *Screen) paneCount() int {
	n := 1
	for _, p := range s.Panes {
		if p != nil {
			n++
		}
	}
	return n
}

func (s *Screen) paneHidden(index int) bool {
	if index <= 0 {
		return s.topHidden
	}
	p := s.paneAt(index)
	return p != nil && p.hidden
}

func (s *Screen) setPaneHidden(index int, hidden bool) {
	if index <= 0 {
		s.topHidden = hidden
		return
	}
	if p := s.paneAt(index); p != nil {
		p.hidden = hidden
	}
}

// paneAt returns Panes entry for a focus index (1-based among non-nil panes).
func (s *Screen) paneAt(index int) *Pane {
	if index <= 0 {
		return nil
	}
	n := 0
	for _, p := range s.Panes {
		if p == nil {
			continue
		}
		n++
		if n == index {
			return p
		}
	}
	return nil
}

// SetPanes replaces the stacked panes. A pane that keeps the same title
// and position keeps its cursor and scroll.
func (s *Screen) SetPanes(next []*Pane) {
	if s == nil {
		return
	}
	for i, p := range next {
		if p == nil || i >= len(s.Panes) || s.Panes[i] == nil {
			continue
		}
		if s.Panes[i].Title == p.Title {
			p.ui = s.Panes[i].ui
			p.hidden = s.Panes[i].hidden
		}
	}
	s.Panes = next
}

func (s *Screen) paneTitle(index int) string {
	if index <= 0 {
		if s.Title != "" {
			return s.Title
		}
		return "Screen"
	}
	if p := s.paneAt(index); p != nil && p.Title != "" {
		return p.Title
	}
	return fmt.Sprintf("Pane %d", index)
}

func (s *Screen) paneWeight(index int) float64 {
	if index <= 0 {
		return s.Weight
	}
	if p := s.paneAt(index); p != nil {
		return p.Weight
	}
	return 0
}

func (s *Screen) uiAt(index int) *uiState {
	if index <= 0 {
		if s.ui == nil {
			s.ui = &uiState{row: 1, lastGood: map[string]any{}, collapsed: map[string]bool{}}
		}
		if s.ui.collapsed == nil {
			s.ui.collapsed = map[string]bool{}
		}
		if s.ui.lastGood == nil {
			s.ui.lastGood = map[string]any{}
		}
		return s.ui
	}
	p := s.paneAt(index)
	if p == nil {
		return s.uiAt(0)
	}
	if p.ui == nil {
		p.ui = &uiState{row: 1, lastGood: map[string]any{}, collapsed: map[string]bool{}}
	}
	if p.ui.collapsed == nil {
		p.ui.collapsed = map[string]bool{}
	}
	if p.ui.lastGood == nil {
		p.ui.lastGood = map[string]any{}
	}
	return p.ui
}

func (s *Screen) sectionsAt(index int) []Section {
	if index <= 0 {
		return s.Sections
	}
	if p := s.paneAt(index); p != nil {
		return p.Sections
	}
	return nil
}

// cycleFocus moves focus by dir (+1 or -1) through every pane, including
// ones that are hidden or collapsed to a divider. Tab goes forward,
// Shift-Tab (btab) goes backward.
func (s *Screen) cycleFocus(dir int) {
	n := s.paneCount()
	if n <= 1 || dir == 0 {
		return
	}
	if s.focus < 0 || s.focus >= n {
		s.focus = 0
	}
	s.focus = (s.focus + dir%n + n) % n
}

// toggleFocused hides or shows the focused pane. A hidden pane keeps its
// divider so it can be focused again. The last pane that is still shown
// cannot be hidden.
func (s *Screen) toggleFocused() {
	n := s.paneCount()
	if n <= 1 {
		return
	}
	if s.focus < 0 || s.focus >= n {
		s.focus = 0
	}
	shown := 0
	for i := 0; i < n; i++ {
		if !s.paneHidden(i) {
			shown++
		}
	}
	if !s.paneHidden(s.focus) && shown <= 1 {
		return
	}
	s.setPaneHidden(s.focus, !s.paneHidden(s.focus))
}

// nudgeFocus grows (dir > 0) or shrinks (dir < 0) the focused pane by one
// row. The row comes from a neighbour when that neighbour has one to spare,
// and otherwise from the blank rows under the last pane. Every pane that
// is actually showing content keeps at least 3 rows. A collapsed or hidden
// pane is not resized.
func (s *Screen) nudgeFocus(dir int) {
	if dir == 0 || s.paneCount() <= 1 || s.viewAvail < 1 {
		return
	}
	bands := s.layoutBands(s.viewTop, s.viewAvail)
	var content []paneBand
	for _, b := range bands {
		if b.rows > 0 && !b.hidden && !b.collapsed {
			content = append(content, b)
		}
	}
	if len(content) < 2 {
		return
	}
	pos := -1
	for i, b := range content {
		if b.index == s.focus {
			pos = i
			break
		}
	}
	if pos < 0 {
		return
	}
	nb := pos + 1
	if nb >= len(content) {
		nb = pos - 1
	}
	focused, neighbour := content[pos], content[nb]
	slack := bandSlack(bands, s.viewTop, s.viewAvail)
	if dir < 0 && focused.rows-1 < paneMinRows {
		return
	}
	if dir > 0 && neighbour.rows-1 < paneMinRows && slack < 1 {
		return
	}
	if s.bias == nil {
		s.bias = map[int]int{}
	}
	s.bias[focused.index] += dir
	// A neighbour already at the minimum cannot give or take a row. The
	// blank space under the last pane absorbs it instead.
	if dir > 0 && neighbour.rows-1 < paneMinRows {
		return
	}
	if dir < 0 && neighbour.rows <= paneMinRows {
		return
	}
	s.bias[neighbour.index] -= dir
}

func (s *Screen) noteSelection(st *uiState) {
	s.selOK = false
	if st == nil {
		return
	}
	sel := selectable(st.items)
	if st.row < 1 || st.row > len(sel) {
		return
	}
	row := st.items[sel[st.row-1]-1]
	if row.wtype != "tablerow" {
		return
	}
	s.selRow = row
	s.selOK = true
}

func (s *Screen) bandIndex(row int) (int, bool) {
	for _, b := range s.bands {
		if b.divRow > 0 && row == b.divRow {
			return b.index, true
		}
		if b.rows > 0 && row >= b.top && row < b.top+b.rows {
			return b.index, true
		}
	}
	return 0, false
}

// layoutBands splits the content rows [contentTop, contentTop+avail) across
// the top pane and Panes. Extra panes each take a one-line divider, with
// blank lines before it so the gap above the title stays at two lines.
// When the area is too short for every visible pane to keep 3 rows,
// unfocused panes collapse to that divider and the focused pane takes
// what is left.
func (s *Screen) layoutBands(contentTop, avail int) []paneBand {
	n := s.paneCount()
	if s.focus < 0 || s.focus >= n {
		s.focus = 0
	}
	plans := make([]panePlan, n)
	for i := 0; i < n; i++ {
		plans[i] = panePlan{index: i, weight: s.paneWeight(i), hidden: s.paneHidden(i)}
	}
	shown := 0
	for i := range plans {
		if !plans[i].hidden {
			shown++
		}
	}
	dividers := 0
	for i := range plans {
		// Every extra pane has a divider. The top pane has one only when
		// it is hidden or about to be collapsed.
		if plans[i].index > 0 {
			dividers++
			plans[i].div = true
		}
	}
	need := shown*paneMinRows + dividers
	// A hidden top still needs a divider of its own, which was not counted.
	if plans[0].hidden {
		need++
	}
	short := avail < need

	for i := range plans {
		switch {
		case plans[i].hidden:
			plans[i].content = false
			plans[i].div = true
		case short && plans[i].index != s.focus:
			plans[i].content = false
			plans[i].div = true
			plans[i].hidden = false
		default:
			plans[i].content = true
		}
	}
	rows, content, leads := s.assignPaneRows(plans, short, avail)
	return placeBands(s, plans, rows, content, leads, contentTop, avail)
}

const splitKeyHints = "Tab/S-Tab pane   [ ] size   | hide"

// withSplitHints appends the stacked-pane keys. They sit at the right, so
// a narrow footer drops the general keys first and keeps these.
func withSplitHints(hints string) string {
	hints = strings.TrimRight(hints, " ")
	if hints == "" {
		return " " + splitKeyHints + " "
	}
	if !strings.HasPrefix(hints, " ") {
		hints = " " + hints
	}
	return hints + "   " + splitKeyHints + " "
}

// preparePane flattens sections into st and scrolls so the cursor stays
// inside a pane of avail rows. It is the per-pane form of the single-column
// preparation the screen has always done.
func preparePane(st *uiState, sections []Section, width, spacing, avail int) []Row {
	st.page = avail
	items := st.flatten(sections, width, spacing)
	st.items = items
	sel := selectable(items)
	if len(sel) < 1 {
		st.row = 1
	} else {
		st.row = clampInt(st.row, 1, len(sel))
	}
	selItem := 0
	if st.row >= 1 && st.row <= len(sel) {
		selItem = sel[st.row-1]
	}
	if selItem != 0 {
		if selItem-st.scroll > avail-1 {
			st.scroll = selItem - avail + 1
		}
		if selItem-st.scroll < 1 {
			st.scroll = selItem - 1
		}
	}
	if st.scroll < 0 {
		st.scroll = 0
	}
	if maxScroll := max(0, len(items)-avail); st.scroll > maxScroll {
		st.scroll = maxScroll
	}
	return items
}

// paintRows draws items into rows [contentTop, contentTop+avail), with the
// same row layout the single-column screen uses. showCursor is false for a
// pane that does not have focus, so only the focused pane shows the cursor.
func paintRows(cv *canvas, st *uiState, items []Row, contentTop, avail, w int, showCursor bool, widget func(*Row, int) string, marked func(*Row, int) bool) {
	sel := selectable(items)
	selItem := 0
	if st.row >= 1 && st.row <= len(sel) {
		selItem = sel[st.row-1]
	}
	st.hit = map[int]hitEntry{}
	const left = 3
	for _, pl := range layoutContent(items, st.scroll, contentTop, avail) {
		i := pl.index + 1
		it := &items[pl.index]
		r := pl.row
		selected := showCursor && i == selItem
		switch {
		case it.kind == "section" && it.collapsible:
			arrow := "▾"
			if it.collapsed {
				arrow = "▸"
			}
			right := it.badge
			if it.collapsed && right == "" {
				right = fmt.Sprintf("%d hidden", it.hidden)
			}
			name := Truncate(it.name, max(4, w-8-VLen(right)))
			rx := (w - 2) - VLen(right) + 1
			st.hit[r] = hitEntry{idx: i, header: true}
			if selected {
				cv.selBar(r, w)
				cv.put(r, left, C.SelBg+C.Accent+C.Bold+arrow+" "+C.Bright+name+Reset)
				if right != "" {
					cv.put(r, rx, C.SelBg+C.Muted+right+Reset)
				}
			} else {
				cv.put(r, left, C.Accent+C.Bold+arrow+" "+name+Reset)
				if right != "" {
					cv.put(r, rx, C.Muted+right+Reset)
				}
			}
		case it.kind == "section":
			cv.put(r, left, C.Accent+C.Bold+"┄ "+Truncate(it.name, w-8)+Reset)
		case it.wtype == "note":
			cv.put(r, left, C.Dim+Truncate(it.label, w-8)+Reset)
		case it.wtype == "divider":
			cv.put(r, left, C.Dim+dividerLine(it.divChar, max(0, w-4))+Reset)
		case it.wtype == "rule":
			paintRule(cv, r, w)
		case it.wtype == "logline":
			cv.logLine(r, w, it)
		case it.wtype == "spacer":
		case it.wtype == "label":
			cv.put(r, left+it.indent, labelStyle(it.style)+Truncate(it.label, w-6-it.indent)+Reset)
		default:
			wt := widget(it, i)
			wv := plainLen(wt)
			wx := (w - 2) - wv + 1
			if wx < left+16 {
				wx = left + 16
			}
			room := max(4, wx-left-4)
			if wv == 0 {
				room = max(4, w-8)
			}
			mark, markCol := "", ""
			switch it.wtype {
			case "check":
				mark, markCol = "☐", C.Muted
				if marked(it, i) {
					mark, markCol = "☑", C.On
				}
			case "radio":
				mark, markCol = "○", C.Muted
				if marked(it, i) {
					mark, markCol = "◉", C.On
				}
			}
			if mark != "" {
				room = max(2, room-2)
			}
			label := Truncate(it.label, room)
			if it.isSelectable() {
				entry := hitEntry{idx: i, mark: mark != ""}
				switch it.wtype {
				case "slider":
					entry.slider, entry.sx0, entry.sw = true, wx, 13
				case "stepper":
					if st.editing == nil || st.editing.idx != i {
						entry.stepper, entry.minusCol, entry.plusCol = true, wx, wx+wv-1
					}
				}
				st.hit[r] = entry
			}
			switch {
			case it.alert && selected:
				cv.selBar(r, w)
				cv.put(r, left, alertPrefix(true)+"▸ "+label+Reset)
				if wv > 0 {
					cv.put(r, wx, paintAlert(wt, true))
				}
			case it.alert:
				cv.put(r, left+2, alertPrefix(false)+label+Reset)
				if wv > 0 {
					cv.put(r, wx, paintAlert(wt, false))
				}
			case selected && mark != "":
				cv.selBar(r, w)
				cv.put(r, left, C.SelBg+C.Accent+C.Bold+"▸ "+Reset+C.SelBg+markCol+mark+" "+
					Reset+C.SelBg+C.Bright+C.Bold+label+Reset)
			case selected:
				cv.selBar(r, w)
				cv.put(r, left, C.SelBg+C.Accent+C.Bold+"▸ "+C.Bright+label+Reset)
				if wv > 0 {
					cv.put(r, wx, C.SelBg+wt+Reset)
				}
			case mark != "":
				cv.put(r, left+2, markCol+mark+Reset+" "+C.Text+label+Reset)
			default:
				cv.put(r, left+2, C.Text+label+Reset)
				if wv > 0 {
					cv.put(r, wx, wt)
				}
			}
		}
	}
	st.sb = nil
	if total := len(items); total > avail && avail > 0 {
		sbx := w - 1
		thumb := max(1, floorf(float64(avail*avail)/float64(total)+0.5))
		maxoff := total - avail
		maxtop := avail - thumb
		tpos := 0
		if maxoff > 0 {
			tpos = floorf(float64(st.scroll)/float64(maxoff)*float64(maxtop) + 0.5)
		}
		st.sb = &scrollInfo{col: sbx, top: contentTop, rows: avail, maxoff: maxoff}
		cv.scrollbar(sbx, contentTop, avail, thumb, tpos)
	}
}

// paintSplit draws the top pane and every extra pane into the content area.
// use points the screen's widget closures at the pane being drawn.
func (s *Screen) paintSplit(cv *canvas, w, contentTop, avail int, use func(*uiState), widget func(*Row, int) string, marked func(*Row, int) bool) {
	sp := s.splitSpacing()
	// Lay the top pane out before the lower panes refresh, so a detail
	// pane sees the row the cursor is on. Refresh can change how tall a
	// lower pane needs to be, so the bands are measured again afterwards.
	bands := s.layoutBands(contentTop, avail)
	if rows := bandRows(bands, 0); rows > 0 {
		top := s.uiAt(0)
		use(top)
		preparePane(top, s.Sections, w, sp, rows)
		s.noteSelection(top)
	}
	s.refreshSplitPanes()
	next := s.layoutBands(contentTop, avail)
	if bandRows(bands, 0) != bandRows(next, 0) {
		if rows := bandRows(next, 0); rows > 0 {
			top := s.uiAt(0)
			use(top)
			preparePane(top, s.Sections, w, sp, rows)
			before := s.selSig()
			s.noteSelection(top)
			if s.selSig() != before {
				s.refreshSplitPanes()
				next = s.layoutBands(contentTop, avail)
			}
		}
	}
	s.bands = next
	for _, b := range s.bands {
		if b.divRow > 0 {
			paintDivider(cv, b.divRow, w, s.paneTitle(b.index), b.index == s.focus)
		}
		if b.rows <= 0 {
			continue
		}
		pst := s.uiAt(b.index)
		use(pst)
		preparePane(pst, s.sectionsAt(b.index), w, sp, b.rows)
	}
	// Gap rules sit on blank rows. Paint them before pane bodies so a
	// scrollbar drawn with the body keeps the right-hand column.
	s.paintGapRules(cv, s.bands, w)
	for _, b := range s.bands {
		if b.rows <= 0 {
			continue
		}
		pst := s.uiAt(b.index)
		use(pst)
		paintRows(cv, pst, pst.items, b.top, b.rows, w, b.index == s.focus, widget, marked)
	}
}

func (s *Screen) refreshSplitPanes() {
	for _, p := range s.Panes {
		if p == nil || p.Refresh == nil {
			continue
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					p.Sections = []Section{{Rows: []Row{Note("  Error: " + Truncate(fmt.Sprint(rec), 60))}}}
				}
			}()
			p.Refresh(p)
		}()
	}
}

func (s *Screen) selSig() string {
	if s == nil || !s.selOK {
		return ""
	}
	_, cells, idx, ok := s.selRow.TableCells()
	if !ok {
		return s.selRow.label
	}
	return fmt.Sprintf("%d\x00%s", idx, strings.Join(cells, "\x00"))
}

// paintGapRules turns the first line of a two-line gap above a pane title
// into a dim rule. The second line stays blank, so the rule never sits on
// the title. When the gap is already between two titles (a collapsed pane,
// or two titles with only that gap between them), both lines stay blank:
// each one touches a title, and a rule there would stack on it.
func (s *Screen) paintGapRules(cv *canvas, bands []paneBand, w int) {
	divAt := map[int]bool{}
	for _, b := range bands {
		if b.divRow > 0 {
			divAt[b.divRow] = true
		}
	}
	var prev *paneBand
	for i := range bands {
		b := &bands[i]
		if b.divRow > 0 {
			trail := 0
			if prev != nil {
				trail = s.trailingPad(prev)
			}
			gap := b.lead + trail
			if gap > paneGapLines {
				gap = paneGapLines
			}
			if gap >= 2 {
				ruleRow := b.divRow - gap
				if ruleRow >= 1 && !divAt[ruleRow-1] && !s.slotIsRule(prev, ruleRow-1) {
					paintRule(cv, ruleRow, w)
				}
			}
		}
		if b.divRow > 0 || b.rows > 0 {
			cp := *b
			prev = &cp
		}
	}
}

// trailingPad is the blank rows at the bottom of a pane whose content is
// shorter than the band. They are the start of the gap above the next title.
func (s *Screen) trailingPad(b *paneBand) int {
	if s == nil || b == nil || b.rows <= 0 {
		return 0
	}
	st := s.uiAt(b.index)
	if st == nil {
		return 0
	}
	vis := len(st.items) - st.scroll
	if vis < 0 {
		vis = 0
	}
	if vis >= b.rows {
		return 0
	}
	return b.rows - vis
}

// slotIsRule reports whether row, inside b, is showing a section-gap rule.
func (s *Screen) slotIsRule(b *paneBand, row int) bool {
	if s == nil || b == nil || b.rows <= 0 || row < b.top || row >= b.top+b.rows {
		return false
	}
	st := s.uiAt(b.index)
	if st == nil {
		return false
	}
	idx := st.scroll + (row - b.top)
	if idx < 0 || idx >= len(st.items) {
		return false
	}
	return st.items[idx].wtype == "rule"
}

func paintDivider(cv *canvas, row, w int, title string, focused bool) {
	if title == "" {
		title = "Pane"
	}
	mark := "┄"
	if focused {
		mark = "▸"
	}
	cv.put(row, 3, C.Accent+C.Bold+Truncate(mark+" "+title, max(4, w-8))+Reset)
}
