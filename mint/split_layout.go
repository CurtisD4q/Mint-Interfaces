package mint

import (
	"math"
	"sort"
)

// paneGapLines is the blank lines between named sections on a pane screen,
// and the blank lines kept before the next pane's title.
const paneGapLines = 2

// paneMinRows is the smallest content height an open pane keeps, including
// while it is resized with [ and ].
const paneMinRows = 3

// panePlan is one pane while its rows are being decided. index 0 is the
// screen's own sections.
type panePlan struct {
	index   int
	weight  float64
	hidden  bool
	div     bool
	content bool
}

// splitSpacing is the blank lines before every named section but the first
// inside a pane. A negative SectionSpacing still means none. Otherwise a
// pane screen uses two, so sections sit the same distance apart as panes.
func (s *Screen) splitSpacing() int {
	if s == nil || s.SectionSpacing < 0 {
		return 0
	}
	return paneGapLines
}

// assignPaneRows decides how many content rows each open pane gets.
// short terminals keep the old rule: unfocused panes are already collapsed
// and the focused pane takes the rows that remain. Otherwise the top pane
// takes half and the other panes share the rest. A pane shorter than its
// share shrinks to its content and the spare rows go to panes that still
// have something to scroll, or stay blank under the last pane.
func (s *Screen) assignPaneRows(plans []panePlan, short bool, avail int) (rows []int, content []bool, leads []int) {
	n := len(plans)
	divCount := 0
	for _, p := range plans {
		if p.div {
			divCount++
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
		budget = avail - divCount
		short = true
	}

	rows = make([]int, n)
	content = make([]bool, n)
	if short {
		assignWeighted(plans, s.bias, budget, rows, content)
		return rows, content, nil
	}

	lens := make([]int, n)
	sp := s.splitSpacing()
	contentCount := 0
	for i, p := range plans {
		if !p.content {
			continue
		}
		contentCount++
		lens[i] = s.paneContentLen(p.index, sp)
	}
	leads = dividerLeads(plans, lens)
	room := budget - contentCount*paneMinRows
	if room < 0 {
		room = 0
	}
	trimLeads(leads, room)
	budget -= sumInts(leads)
	if budget < 0 {
		budget = 0
	}

	nums, den := s.shareNums(plans)
	got := apportionNums(budget, nums, den)
	for i, p := range plans {
		if !p.content {
			continue
		}
		content[i] = true
		rows[i] = got[i]
	}
	clampContentRows(rows, content, paneMinRows, budget)
	spare := budget - sumRows(rows, content)
	for i := range rows {
		if !content[i] || lens[i] >= rows[i] {
			continue
		}
		target := lens[i]
		if target < paneMinRows {
			target = paneMinRows
		}
		if target < 1 {
			target = 1
		}
		if target < rows[i] {
			spare += rows[i] - target
			rows[i] = target
		}
	}
	giveSpare(rows, content, lens, spare)
	for i := range rows {
		if content[i] {
			rows[i] += s.bias[plans[i].index]
		}
	}
	clampContentRows(rows, content, paneMinRows, budget)
	return rows, content, leads
}

// paneContentLen is how many rows the pane's sections occupy at the
// current width, including the blank lines between named sections.
func (s *Screen) paneContentLen(index, spacing int) int {
	if s == nil || s.viewWidth <= 0 {
		return math.MaxInt32 / 8
	}
	st := s.uiAt(index)
	return len(st.flatten(s.sectionsAt(index), s.viewWidth, spacing))
}

// shareNums is each content pane's portion of 100 (or of den, when a
// weight was set). The numbers may sum to less than den: that remainder
// is spare, not a share that gets padded back into a short pane.
func (s *Screen) shareNums(plans []panePlan) (nums []int, den int) {
	const den100 = 100
	den = den100
	nums = make([]int, len(plans))
	var idx []int
	explicit := false
	for i, p := range plans {
		if !p.content {
			continue
		}
		idx = append(idx, i)
		if p.weight > 0 {
			explicit = true
		}
	}
	if len(idx) == 0 {
		return nums, den
	}
	if len(idx) == 1 {
		nums[idx[0]] = den
		return nums, den
	}
	if explicit {
		den = 1000000
		sum := 0.0
		ws := make([]float64, len(plans))
		for _, i := range idx {
			w := plans[i].weight
			if w <= 0 {
				w = 1
			}
			ws[i] = w
			sum += w
		}
		used := 0
		last := idx[len(idx)-1]
		for _, i := range idx {
			if i == last {
				continue
			}
			nums[i] = int(math.Floor(ws[i]/sum*float64(den) + 1e-9))
			used += nums[i]
		}
		nums[last] = den - used
		if nums[last] < 0 {
			nums[last] = 0
		}
		return nums, den
	}

	var others []int
	top := false
	for _, i := range idx {
		if plans[i].index == 0 {
			top = true
			continue
		}
		others = append(others, i)
	}
	if top {
		nums[0] = 50
	}
	used := 0
	for _, n := range nums {
		used += n
	}
	if used > den {
		return scaleNums(nums, den), den
	}
	rem := den - used
	if len(others) == 0 || rem <= 0 {
		return nums, den
	}
	base := rem / len(others)
	extra := rem % len(others)
	for i, ix := range others {
		nums[ix] = base
		if i < extra {
			nums[ix]++
		}
	}
	return nums, den
}

// assignWeighted is the short-terminal split: equal shares (or explicit
// weights), with any leftover row given to the last open pane. It does not
// shrink a pane to its content, so the focused pane still fills the screen.
func assignWeighted(plans []panePlan, bias map[int]int, budget int, rows []int, content []bool) {
	n := len(plans)
	if budget <= 0 {
		return
	}
	sum := 0.0
	weights := make([]float64, n)
	open := 0
	for i, p := range plans {
		if !p.content {
			continue
		}
		open++
		w := p.weight
		if w <= 0 {
			w = 1
		}
		weights[i] = w
		sum += w
	}
	if open == 0 || sum <= 0 {
		return
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
		content[i] = true
	}
	if last >= 0 {
		rows[last] += budget - used
	}
	for i := range plans {
		if content[i] {
			rows[i] += bias[plans[i].index]
		}
	}
}

// apportionNums hands out budget rows in proportion to nums[i]/den using
// the largest remainder. Shares that do not add up to den leave the
// difference unallocated.
func apportionNums(budget int, nums []int, den int) []int {
	rows := make([]int, len(nums))
	if budget <= 0 || den <= 0 {
		return rows
	}
	sumN := 0
	for _, n := range nums {
		if n > 0 {
			sumN += n
		}
	}
	if sumN == 0 {
		return rows
	}
	target := budget * sumN / den
	type frac struct{ i, rem int }
	var fracs []frac
	used := 0
	for i, n := range nums {
		if n <= 0 {
			continue
		}
		prod := budget * n
		rows[i] = prod / den
		used += rows[i]
		fracs = append(fracs, frac{i, prod % den})
	}
	sort.SliceStable(fracs, func(a, b int) bool {
		if fracs[a].rem == fracs[b].rem {
			return fracs[a].i < fracs[b].i
		}
		return fracs[a].rem > fracs[b].rem
	})
	left := target - used
	for left > 0 && len(fracs) > 0 {
		for _, f := range fracs {
			if left == 0 {
				break
			}
			rows[f.i]++
			left--
		}
	}
	return rows
}

func scaleNums(nums []int, den int) []int {
	sum := 0
	for _, n := range nums {
		if n > 0 {
			sum += n
		}
	}
	out := make([]int, len(nums))
	if sum <= 0 {
		return out
	}
	type frac struct{ i, rem int }
	var fracs []frac
	used := 0
	for i, n := range nums {
		if n <= 0 {
			continue
		}
		prod := n * den
		out[i] = prod / sum
		used += out[i]
		fracs = append(fracs, frac{i, prod % sum})
	}
	sort.SliceStable(fracs, func(a, b int) bool {
		if fracs[a].rem == fracs[b].rem {
			return fracs[a].i < fracs[b].i
		}
		return fracs[a].rem > fracs[b].rem
	})
	left := den - used
	for left > 0 && len(fracs) > 0 {
		for _, f := range fracs {
			if left == 0 {
				break
			}
			out[f.i]++
			left--
		}
	}
	return out
}

// leadFor is the blank rows to leave before the next title so the gap
// after this pane reads as two lines. A pane shorter than the 3-row
// minimum already shows some of those blanks inside its own band.
func leadFor(contentLen int) int {
	if contentLen >= paneMinRows {
		return paneGapLines
	}
	if contentLen < 0 {
		contentLen = 0
	}
	internal := paneMinRows - contentLen
	if internal >= paneGapLines {
		return 0
	}
	return paneGapLines - internal
}

// dividerLeads is the blank rows drawn before each divider. The top pane
// has no divider of its own while it is open, so its lead is zero.
func dividerLeads(plans []panePlan, lens []int) []int {
	leads := make([]int, len(plans))
	const (
		none = iota
		wasContent
		wasDiv
	)
	prev := none
	prevLen := 0
	for i, p := range plans {
		if p.div {
			switch prev {
			case wasContent:
				leads[i] = leadFor(prevLen)
			case wasDiv:
				leads[i] = paneGapLines
			}
			if !p.content {
				prev = wasDiv
			}
		}
		if p.content {
			prev = wasContent
			if i < len(lens) {
				prevLen = lens[i]
			}
		}
	}
	return leads
}

// trimLeads drops blank rows, from the last gap backward, until the leads
// fit in maxSum. The 3-row minimum wins when the terminal is tight.
func trimLeads(leads []int, maxSum int) {
	if maxSum < 0 {
		maxSum = 0
	}
	for sumInts(leads) > maxSum {
		best, bestV := -1, 0
		for i, n := range leads {
			if n > bestV || (n == bestV && n > 0 && i > best) {
				best, bestV = i, n
			}
		}
		if best < 0 || bestV == 0 {
			return
		}
		leads[best]--
	}
}

// giveSpare hands leftover rows, one at a time and in pane order, to panes
// that still have content below the fold.
func giveSpare(rows []int, content []bool, lens []int, spare int) {
	for spare > 0 {
		gave := false
		for i := range rows {
			if spare == 0 {
				return
			}
			if !content[i] || i >= len(lens) || lens[i] <= rows[i] {
				continue
			}
			rows[i]++
			spare--
			gave = true
		}
		if !gave {
			return
		}
	}
}

// clampContentRows keeps every content pane at min rows or more, and the
// total at total or less. It does not pad the total back up: spare rows
// stay unallocated so a short pane is not filled with blank lines.
func clampContentRows(rows []int, content []bool, min, total int) {
	if min < 1 {
		min = 1
	}
	if total < 0 {
		total = 0
	}
	for guard := 0; guard < 8; guard++ {
		for i := range rows {
			if content[i] && rows[i] < min {
				rows[i] = min
			}
		}
		sum := 0
		for i := range rows {
			if content[i] {
				sum += rows[i]
			}
		}
		for sum > total {
			best, bestV := -1, min
			for i := range rows {
				if content[i] && rows[i] > bestV {
					best, bestV = i, rows[i]
				}
			}
			if best < 0 {
				return
			}
			rows[best]--
			sum--
		}
		under := false
		for i := range rows {
			if content[i] && rows[i] < min {
				under = true
				break
			}
		}
		if !under {
			return
		}
	}
}

func placeBands(s *Screen, plans []panePlan, rows []int, content []bool, leads []int, contentTop, avail int) []paneBand {
	row := contentTop
	limit := contentTop + avail
	out := make([]paneBand, 0, len(plans))
	for i, p := range plans {
		b := paneBand{index: p.index, hidden: s.paneHidden(p.index)}
		if p.div && row < limit {
			lead := 0
			if i < len(leads) {
				lead = leads[i]
			}
			if room := limit - row; lead > room {
				lead = room
			}
			b.lead = lead
			row += lead
			if row < limit {
				b.divRow = row
				row++
			}
		}
		if i < len(content) && content[i] && i < len(rows) && rows[i] > 0 && row < limit {
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

func bandRows(bands []paneBand, index int) int {
	for _, b := range bands {
		if b.index == index {
			return b.rows
		}
	}
	return 0
}

// bandSlack is the unused rows under the last pane, inside the content area.
func bandSlack(bands []paneBand, top, avail int) int {
	end := top
	for _, b := range bands {
		if b.divRow >= end {
			end = b.divRow + 1
		}
		if b.rows > 0 && b.top+b.rows > end {
			end = b.top + b.rows
		}
	}
	slack := top + avail - end
	if slack < 0 {
		return 0
	}
	return slack
}

func sumInts(ns []int) int {
	s := 0
	for _, n := range ns {
		s += n
	}
	return s
}

func sumRows(rows []int, content []bool) int {
	s := 0
	for i := range rows {
		if i < len(content) && content[i] {
			s += rows[i]
		}
	}
	return s
}
