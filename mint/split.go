package mint

import (
	"fmt"
	"math"
	"strings"
)

// paneBand is one horizontal slice of the content area. index 0 is the
// screen's own sections; 1 and up are Panes. A divider row of 0 means this
// band has no divider (the top pane, while it is open).
type paneBand struct {
	index     int
	divRow    int
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
// row, taking or giving that row to a neighbour. Every pane that is
// actually showing content keeps at least 3 rows. A collapsed or hidden
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
	if dir < 0 && focused.rows-1 < 3 {
		return
	}
	if dir > 0 && neighbour.rows-1 < 3 {
		return
	}
	if s.bias == nil {
		s.bias = map[int]int{}
	}
	s.bias[focused.index] += dir
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
// the top pane and Panes. Extra panes each take a one-line divider. When
// the area is too short for every visible pane to keep 3 rows, unfocused
// panes collapse to that divider and the focused pane takes what is left.
func (s *Screen) layoutBands(contentTop, avail int) []paneBand {
	n := s.paneCount()
	if s.focus < 0 || s.focus >= n {
		s.focus = 0
	}
	type plan struct {
		index   int
		weight  float64
		hidden  bool
		div     bool
		content bool
	}
	plans := make([]plan, n)
	for i := 0; i < n; i++ {
		plans[i] = plan{index: i, weight: s.paneWeight(i), hidden: s.paneHidden(i)}
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
	need := shown*3 + dividers
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
	divCount := 0
	contentCount := 0
	for _, p := range plans {
		if p.div {
			divCount++
		}
		if p.content {
			contentCount++
		}
	}
	budget := avail - divCount
	if budget < 0 {
		// Not enough rows for every divider. Keep the focused pane's
		// divider (if it has one) and as many others as fit, from the top.
		budget = 0
		keep := avail
		if keep < 0 {
			keep = 0
		}
		seen := 0
		for i := range plans {
			if !plans[i].div {
				continue
			}
			if seen < keep {
				seen++
				continue
			}
			plans[i].div = false
		}
		divCount = seen
		// Give any leftover row to the focused content, if it is showing.
		budget = avail - divCount
	}
	rows := make([]int, n)
	content := make([]bool, n)
	if contentCount > 0 && budget > 0 {
		sum := 0.0
		weights := make([]float64, n)
		for i, p := range plans {
			if !p.content {
				continue
			}
			w := p.weight
			if w <= 0 {
				w = 1
			}
			weights[i] = w
			sum += w
		}
		used := 0
		last := -1
		for i, p := range plans {
			if !p.content {
				continue
			}
			last = i
			rows[i] = int(math.Floor(float64(budget)*weights[i]/sum + 1e-9))
			used += rows[i]
		}
		if last >= 0 {
			rows[last] += budget - used
		}
		for i := range plans {
			if !plans[i].content {
				continue
			}
			rows[i] += s.bias[plans[i].index]
			content[i] = true
		}
		if !short {
			clampContentRows(rows, content, 3, budget)
		}
	}

	out := make([]paneBand, 0, n)
	row := contentTop
	limit := contentTop + avail
	for i, p := range plans {
		b := paneBand{index: p.index, hidden: s.paneHidden(p.index)}
		if p.div && row < limit {
			b.divRow = row
			row++
		}
		if content[i] && rows[i] > 0 && row < limit {
			b.top = row
			b.rows = rows[i]
			if b.top+b.rows > limit {
				b.rows = limit - b.top
			}
			row += b.rows
		}
		if !p.content && p.div && !s.paneHidden(p.index) && p.index != s.focus {
			b.collapsed = true
		}
		if p.div || b.rows > 0 {
			out = append(out, b)
		}
	}
	return out
}

// clampContentRows keeps every content pane at min rows or more, without
// changing the total, by moving rows from panes that have some to spare.
func clampContentRows(rows []int, content []bool, min, total int) {
	for guard := 0; guard < 64; guard++ {
		need := 0
		for i := range rows {
			if content[i] && rows[i] < min {
				need += min - rows[i]
				rows[i] = min
			}
		}
		if need == 0 {
			break
		}
		for need > 0 {
			best, bestV := -1, min
			for i := range rows {
				if content[i] && rows[i] > bestV {
					best, bestV = i, rows[i]
				}
			}
			if best < 0 {
				break
			}
			rows[best]--
			need--
		}
	}
	sum, last := 0, -1
	for i := range rows {
		if content[i] {
			sum += rows[i]
			last = i
		}
	}
	if last >= 0 && sum != total {
		rows[last] += total - sum
		if rows[last] < 1 {
			rows[last] = 1
		}
	}
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
	bands := s.layoutBands(contentTop, avail)
	s.bands = bands
	// Lay the top pane out at its real height before the lower panes
	// refresh, so a detail pane sees the row the cursor is on.
	for _, b := range bands {
		if b.index != 0 || b.rows <= 0 {
			continue
		}
		top := s.uiAt(0)
		use(top)
		preparePane(top, s.Sections, w, s.SectionSpacing, b.rows)
		s.noteSelection(top)
	}
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
	for _, b := range bands {
		if b.divRow > 0 {
			paintDivider(cv, b.divRow, w, s.paneTitle(b.index), b.index == s.focus)
		}
		if b.rows <= 0 {
			continue
		}
		pst := s.uiAt(b.index)
		use(pst)
		var items []Row
		if b.index == 0 {
			items = pst.items
			if len(items) == 0 {
				items = preparePane(pst, s.Sections, w, s.SectionSpacing, b.rows)
			}
		} else {
			items = preparePane(pst, s.sectionsAt(b.index), w, s.SectionSpacing, b.rows)
		}
		paintRows(cv, pst, items, b.top, b.rows, w, b.index == s.focus, widget, marked)
	}
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
