package mint

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// Section is a heading followed by rows. An empty Name draws no heading.
//
// A Collapsible section draws its heading as ▾ Name, can be selected, and
// folds away its rows with Enter, Space, ← or a click on the arrow
// (▸ Name, "3 hidden"). Collapsed sets its starting state; after that the
// screen remembers what the user chose, even across Refresh.
type Section struct {
	Name        string
	Rows        []Row
	Collapsible bool
	Collapsed   bool
}

// Screen is one page of rows. Returning a *Screen from a home item's Open
// (or passing one to ctx.Push) shows it; b pops it and q quits.
type Screen struct {
	Icon       string
	Title      string
	Breadcrumb string // defaults to the app's Crumb
	Banner     string // amber line under the title
	Hints      string // footer override
	Sections   []Section

	// Weight is the top pane's share of the content height when Panes is
	// set and any pane sets a weight above zero. With every weight left at
	// zero, the top pane takes half the content rows and the other panes
	// split what remains.
	Weight float64

	// Panes are stacked under Sections, top to bottom. A screen with none
	// is one column and draws exactly as before. There is no side-by-side
	// layout: every extra pane sits under the one above it, with a one-line
	// divider that shows its title.
	Panes []*Pane

	// SectionSpacing puts that many blank lines before every named section
	// except the first. 0 uses the app's setting; negative means none.
	SectionSpacing int

	// Status returns the badges shown on the left of the footer, such as
	// {"running", BadgeOK}. It overrides the app's Status. It is called
	// about ten times a second so the footer can update; keep it cheap.
	Status func() []Badge

	// Refresh runs before each draw and may replace Sections. A panic sets
	// Banner to the error.
	Refresh func(*Screen)
	// Poll runs on idle (about ten times a second) and returns true when the
	// next frame must be painted. Keep it non-blocking.
	Poll func(*Screen) bool
	// Enter runs once each time the screen is shown. If it takes a while,
	// the footer shows a loading spinner.
	Enter func(*Screen)

	appStatus func() []Badge
	ui        *uiState
	customRun func() screenResult

	// Split-view state. Zero values keep a screen with no Panes on the
	// single-column path.
	focus     int
	bias      map[int]int
	topHidden bool
	bands     []paneBand
	viewTop   int
	viewAvail int
	viewWidth int
	selRow    Row
	selOK     bool

	redraw   func(int, int)
	onKey    func(*Key, *int, *int, *int) (*screenResult, bool)
	frameBuf *strings.Builder

	// viewerLines is the text a full-screen log viewer shows. ViewLog sets
	// it; every other screen leaves it nil.
	viewerLines func() []LogLine
}

// Pane is one band stacked under a screen's own sections. Each pane has its
// own sections, cursor, scroll and collapsed state, and a weight for how
// much of the height it takes when any pane sets a weight above zero.
// With every weight left at zero, the top pane takes half the content
// rows and the other panes share what remains.
type Pane struct {
	Title    string
	Sections []Section
	Weight   float64
	Refresh  func(*Pane)

	ui     *uiState
	hidden bool
}

type editState struct {
	idx     int
	chars   []string
	buf     *lineBuffer
	numeric bool
	commit  func(string)
}

type hitEntry struct {
	idx      int
	slider   bool
	sx0, sw  int
	stepper  bool
	minusCol int
	plusCol  int
	mark     bool // check / radio: clicking the box or circle acts at once
	header   bool
}

type scrollInfo struct {
	col, top, rows, maxoff, vis int
}

type uiState struct {
	row, scroll int
	lastGood    map[string]any
	editing     *editState
	hit         map[int]hitEntry
	sb          *scrollInfo
	items       []Row
	collapsed   map[string]bool
	footSig     string
	page        int
}

// Ctx is handed to row callbacks. It can open dialogs, show toasts, run
// slow work behind a spinner, and change screens.
type Ctx struct {
	nav *navReq
}

type navReq struct {
	op     string
	screen *Screen
}

// Confirm asks a Yes/No question (No is preselected).
func (c *Ctx) Confirm(title, body string) bool { return Confirm(title, body) }

// Message shows text until it is dismissed.
func (c *Ctx) Message(title, body string) { Message(title, body) }

// Prompt asks for a line of text. ok is false when cancelled.
func (c *Ctx) Prompt(title, subtitle, initial string) (string, bool) {
	return Prompt(title, subtitle, initial)
}

// Choose offers a filterable list. ok is false when cancelled.
func (c *Ctx) Choose(title string, items []string, subtitle string) (string, bool) {
	return Choose(title, items, subtitle)
}

// ChooseItem is Choose with separate labels and values.
func (c *Ctx) ChooseItem(title string, items []Choice, subtitle string) (Choice, bool) {
	return ChooseItem(title, items, subtitle)
}

// Password asks for masked text. ok is false when cancelled.
func (c *Ctx) Password(title, subtitle string) (string, bool) { return Password(title, subtitle) }

// TypeToConfirm requires the exact phrase to be typed.
func (c *Ctx) TypeToConfirm(title, body, phrase string) bool {
	return TypeToConfirm(title, body, phrase)
}

// Toast flashes "✓ msg" in the footer for a couple of seconds.
func (c *Ctx) Toast(msg string) { Toast(msg) }

// ToastWarn flashes "! msg" in amber.
func (c *Ctx) ToastWarn(msg string) { ToastWarn(msg) }

// ToastError flashes "✕ msg" in red.
func (c *Ctx) ToastError(msg string) { ToastError(msg) }

// Busy runs slow work (saving, applying) with a spinner and msg in the
// footer if it takes more than a moment, e.g. ctx.Busy("Saving…", save).
// fn runs on another goroutine and must not open dialogs. A panic inside it
// becomes an error toast and Busy returns false.
func (c *Ctx) Busy(msg string, fn func()) bool { return Busy(msg, fn) }

// Busy is ctx.Busy for code without a ctx, such as a row's Set. Call it
// from the interface's own callbacks, not from another goroutine.
func Busy(msg string, fn func()) bool {
	if p := withLoading(msg, fn); p != nil {
		ToastError(fmt.Sprint(p))
		return false
	}
	return true
}

// Open builds a screen with "Loading <title>…" in the footer while it
// works, then shows it. fn runs on another goroutine and must not open
// dialogs. A nil screen stays put.
func (c *Ctx) Open(title string, fn func() *Screen) {
	var s *Screen
	if p := withLoading("Loading "+title+"…", func() { s = fn() }); p != nil {
		ToastError(fmt.Sprint(p))
		return
	}
	if s != nil {
		c.Push(s)
	}
}

// Push shows another screen on top of this one.
func (c *Ctx) Push(s *Screen) { c.nav.op, c.nav.screen = "push", s }

// Pop returns to the previous screen.
func (c *Ctx) Pop() { c.nav.op = "pop" }

// Quit leaves the app.
func (c *Ctx) Quit() { c.nav.op = "quit" }

type screenResult struct {
	op     string // "back", "quit", "push"
	screen *Screen
}

// expand turns one Row into the lines it occupies at this width.
func expand(r Row, width int) []Row {
	switch r.wtype {
	case "radiogroup":
		var out []Row
		if r.label != "" {
			caption := Label(StyleCaption, r.label)
			caption.indent = 2
			out = append(out, expand(caption, width-2)...)
		}
		for _, opt := range r.options {
			o := r
			o.wtype, o.radioOpt, o.label, o.options = "radio", opt, opt, nil
			out = append(out, o)
		}
		return out
	case "log":
		return expandLog(r)
	case "table":
		return expandTable(r, width)
	case "label":
		lines := wrapLines(r.label, max(10, width-6))
		if len(lines) == 0 {
			lines = []string{""}
		}
		out := make([]Row, len(lines))
		for i, l := range lines {
			out[i] = r
			out[i].label = l
		}
		return out
	}
	return []Row{r}
}

func (st *uiState) expandRow(r Row, width int) []Row {
	r = r.normalized()
	if r.wtype == "collapsible" {
		key := r.id
		if key == "" {
			key = "col:" + r.name
		}
		var collapsed bool
		if r.openPtr != nil {
			collapsed = !*r.openPtr
			st.collapsed[key] = collapsed
		} else {
			var seen bool
			collapsed, seen = st.collapsed[key]
			if !seen {
				collapsed = r.collapsed
				st.collapsed[key] = collapsed
			}
		}
		childRows, badge := buildCollapsibleChildren(r)
		var expanded []Row
		for _, cr := range childRows {
			expanded = append(expanded, st.expandRow(cr, width)...)
		}
		hidden := 0
		for i := range expanded {
			if w := expanded[i].wtype; w != "spacer" && w != "divider" && w != "label" && w != "logline" && w != "note" {
				hidden++
			}
		}
		if hidden == 0 {
			hidden = len(expanded)
		}
		hdr := Row{
			kind:        "section",
			id:          key,
			name:        r.name,
			badge:       badge,
			collapsible: true,
			collapsed:   collapsed,
			hidden:      hidden,
			openPtr:     r.openPtr,
		}
		if collapsed {
			return []Row{hdr}
		}
		return append([]Row{hdr}, expanded...)
	}
	return expand(r, width)
}

func (st *uiState) flatten(sections []Section, width, spacing int) []Row {
	var items []Row
	for _, sec := range sections {
		var rows []Row
		for _, r := range sec.Rows {
			rows = append(rows, st.expandRow(r, width)...)
		}
		if sec.Name != "" {
			if spacing > 0 && len(items) > 0 {
				for i := 0; i < spacing; i++ {
					items = append(items, Spacer())
				}
			}
			if sec.Collapsible {
				collapsed, seen := st.collapsed[sec.Name]
				if !seen {
					collapsed = sec.Collapsed
					st.collapsed[sec.Name] = collapsed
				}
				hidden := 0
				for i := range rows {
					if w := rows[i].wtype; w != "spacer" && w != "divider" && w != "label" && w != "logline" {
						hidden++
					}
				}
				items = append(items, Row{kind: "section", name: sec.Name,
					collapsible: true, collapsed: collapsed, hidden: hidden})
				if collapsed {
					continue
				}
			} else {
				items = append(items, Row{kind: "section", name: sec.Name})
			}
		}
		items = append(items, rows...)
	}
	return items
}

// selectable returns the 1-based indexes of rows that can be selected.
func selectable(items []Row) []int {
	var idx []int
	for i := range items {
		if items[i].isSelectable() {
			idx = append(idx, i+1)
		}
	}
	return idx
}

var wsRun = regexp.MustCompile(`[ \t\n\v\f\r]+`)

func floorf(x float64) int { return int(math.Floor(x)) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// dividerLine repeats ch to exactly n cells.
func dividerLine(ch string, n int) string {
	cells := util.Chars(ch)
	if len(cells) == 0 {
		cells = []string{"─"}
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(cells[i%len(cells)])
	}
	return b.String()
}

// alertPrefix is red and bold. On a monochrome terminal Err is bold only;
// the row's label text still states the warning.
func alertPrefix(selected bool) string {
	col := C.Amber + C.Bold
	if C.Err != "" {
		col = C.Err + C.Bold
	}
	if selected {
		return C.SelBg + col
	}
	return col
}

func paintAlert(styled string, selected bool) string {
	return alertPrefix(selected) + sgrSeq.ReplaceAllString(styled, "") + Reset
}

func labelStyle(s Style) string {
	switch s {
	case StyleTitle:
		return C.Accent + C.Bold
	case StyleHeading:
		return C.Bright + C.Bold
	case StyleCaption:
		return C.Muted
	case StyleItalic:
		return C.Text + esc + "3m"
	case StyleUnderline:
		return C.Text + esc + "4m"
	}
	return C.Text
}

// contentLine is one row of the content viewport, in the order it is drawn.
type contentLine struct {
	index int // 0-based index into the flattened items
	row   int // terminal row
}

// layoutContent places items into the content viewport. scroll is how many
// items are above it. A row's line is index - scroll + contentTop, the place
// the screen has always used, except a row marked pin is left out of that
// scroll: while its table is still on screen the pin stays at the top of the
// table's visible area and the body keeps its own line underneath.
func layoutContent(items []Row, scroll, contentTop, avail int) []contentLine {
	if avail < 1 {
		return nil
	}
	contentBottom := contentTop + avail - 1
	natural := func(index int) int { return index - scroll + contentTop }
	inView := func(r int) bool { return r >= contentTop && r <= contentBottom }

	at := make([]int, contentBottom+1)
	for i := range at {
		at[i] = -1
	}
	// Pins drawn at the sticky line, not at the line scroll would give them.
	held := map[int]bool{}

	for i := 0; i < len(items); {
		if !items[i].pin {
			i++
			continue
		}
		start := i
		for i < len(items) && items[i].pin {
			i++
		}
		end := i
		if natural(start) >= contentTop {
			continue // still on its own line; the pass below draws it
		}
		// What scrolls under the pin: the body, or the empty-table note.
		body := end
		for body < len(items) && items[body].wtype == "tablerow" {
			body++
		}
		if body == end && body < len(items) && items[body].wtype == "note" {
			body++
		}
		first, last := -1, -1
		for j := start; j < body; j++ {
			if r := natural(j); inView(r) {
				if first < 0 {
					first = r
				}
				last = r
			}
		}
		if first < 0 {
			continue // the table has scrolled off; the pin goes with it
		}
		for k := start; k < end; k++ {
			r := first + (k - start)
			if !inView(r) || r > last {
				break
			}
			at[r] = k
			held[k] = true
		}
	}

	for i := range items {
		if held[i] {
			continue
		}
		r := natural(i)
		if !inView(r) {
			continue
		}
		if at[r] >= 0 {
			// A pin already occupies this line. The row stays on the line
			// scroll gave it, underneath the pin, so it is not drawn.
			continue
		}
		at[r] = i
	}

	var out []contentLine
	for r := contentTop; r <= contentBottom; r++ {
		if at[r] >= 0 {
			out = append(out, contentLine{index: at[r], row: r})
		}
	}
	return out
}

func (screen *Screen) run() screenResult {
	if screen.customRun != nil {
		return screen.customRun()
	}
	screen.bind()
	h, w := TermSize()
	screen.redraw(h, w)
	sc := 0
	for {
		editing := false
		if u := screen.uiAt(screen.focus); u != nil {
			editing = u.editing != nil
		}
		k := navKey(ReadKey(), editing)
		res, dirty := screen.onKey(k, &h, &w, &sc)
		if res != nil {
			return *res
		}
		if dirty {
			screen.redraw(h, w)
		}
	}
}

func (screen *Screen) bind() {
	if screen.redraw != nil {
		return
	}
	if screen.ui == nil {
		screen.ui = &uiState{row: 1, lastGood: map[string]any{}}
	}
	st := screen.ui
	if st.collapsed == nil {
		st.collapsed = map[string]bool{}
	}
	st.editing = nil
	st.hit = map[int]hitEntry{}
	st.sb = nil
	if screen.Enter != nil {
		title := screen.Title
		if title == "" {
			title = "Screen"
		}
		withLoading("Loading "+title+"…", func() { screen.Enter(screen) })
	}

	nav := &navReq{}
	ctx := &Ctx{nav: nav}

	status := func() []Badge {
		if screen.Status != nil {
			return callStatus(screen.Status)
		}
		return callStatus(screen.appStatus)
	}

	takeNav := func() *screenResult {
		if nav.op == "" {
			return nil
		}
		op, pushed := nav.op, nav.screen
		nav.op, nav.screen = "", nil
		switch op {
		case "push":
			return &screenResult{op: "push", screen: pushed}
		case "pop":
			return &screenResult{op: "back"}
		}
		return &screenResult{op: "quit"}
	}

	clamp := func() {
		n := len(selectable(st.items))
		if n < 1 {
			st.row = 1
			return
		}
		st.row = clampInt(st.row, 1, n)
	}

	readValue := func(it *Row, key string) (any, string, bool) {
		if !it.hasGet() {
			if it.hasValue {
				return it.value, "", false
			}
			return nil, "", false
		}
		val, msg, failed := it.callGet()
		if val == nil {
			if lg, ok := st.lastGood[key]; ok && lg != nil {
				val = lg
			}
		} else {
			st.lastGood[key] = val
		}
		return val, msg, failed
	}

	keyOf := func(it *Row, idx int) string {
		k := it.id
		if k == "" {
			k = it.wtype
		}
		return fmt.Sprintf("%s:%d", k, idx)
	}

	// marked reports whether a checkbox or radio option is on.
	marked := func(it *Row, idx int) bool {
		val, _, _ := readValue(it, keyOf(it, idx))
		if it.wtype == "radio" {
			return val != nil && tostr(val) == it.radioOpt
		}
		return truthy(val)
	}

	widget := func(it *Row, idx int) string {
		editingHere := st.editing != nil && st.editing.idx == idx
		if it.wtype == "text" && editingHere {
			if st.editing.buf == nil {
				st.editing.buf = newLineBuffer(strings.Join(st.editing.chars, ""))
			}
			s := st.editing.buf.String()
			if s == "" {
				ph := it.placeholder
				if ph == "" {
					ph = "…"
				}
				return C.Dim + ph + Reset + C.Accent + "▏" + Reset
			}
			return C.Bright + st.editing.buf.renderViewport(28, "") + Reset
		}
		if it.wtype == "stepper" && editingHere {
			return C.Dim + "− " + Reset + C.Bright + strings.Join(st.editing.chars, "") +
				C.Accent + "_" + Reset + C.Dim + " +" + Reset
		}
		if it.wtype == "check" || it.wtype == "radio" || it.wtype == "tablerow" {
			return ""
		}
		val, msg, failed := readValue(it, keyOf(it, idx))
		if val == nil && it.wtype != "action" && it.wtype != "text" {
			switch {
			case it.wtype == "toggle":
				val = false
			case it.wtype == "slider" || it.wtype == "progress":
				val = it.min
			case failed:
				return C.Amber + "  " + Truncate(msg, 18) + Reset
			case it.wtype == "stepper":
				val = 0
			}
		}
		switch it.wtype {
		case "toggle":
			if truthy(val) {
				return C.On + "● ON " + Reset
			}
			return C.Muted + "  OFF" + Reset
		case "slider":
			lo, hi := float64(it.min), float64(it.max)
			v := math.Max(lo, math.Min(hi, toNumber(val)))
			const bar = 12
			fill := floorf((v - lo) / (hi - lo) * bar)
			var num string
			switch {
			case it.format != nil || it.unit != "":
				num = " " + it.numberText(floorf(v))
			case it.min == 0 && it.max == 100:
				num = fmt.Sprintf(" %d%%", floorf(v))
			default:
				num = " " + strconv.Itoa(floorf(v))
			}
			return C.Accent + rt.profile.repeatTo("━", fill) + "●" +
				C.Dim + rt.profile.repeatTo("─", bar-fill) + Reset +
				C.Muted + num + Reset
		case "progress":
			v := math.Max(0, math.Min(100, toNumber(val)))
			const bar = 13
			fill := floorf(v / 100 * bar)
			num := fmt.Sprintf(" %d%%", floorf(v))
			if it.format != nil {
				num = " " + it.numberText(floorf(v))
			}
			return C.Accent + rt.profile.repeatTo("━", fill) +
				C.Dim + rt.profile.repeatTo("─", bar-fill) + Reset +
				C.Muted + num + Reset
		case "stepper":
			v := int(toNumber(val))
			minus, plus := C.Accent+"−"+Reset, C.Accent+"+"+Reset
			if it.limited() && v <= it.min {
				minus = C.Dim + "−" + Reset
			}
			if it.limited() && v >= it.max {
				plus = C.Dim + "+" + Reset
			}
			return minus + " " + C.Text + Truncate(it.numberText(v), 20) + Reset + " " + plus
		case "picker":
			shown := "—"
			if val != nil && tostr(val) != "" {
				shown = tostr(val)
			}
			return C.Text + "‹ " + Truncate(shown, 24) + " ›" + Reset
		case "action":
			right := it.hint
			if right == "" && val != nil {
				right = tostr(val)
			}
			if right == "" {
				right = "open"
			}
			return C.Muted + Truncate(right, 22) + Reset
		case "text":
			if val == nil || tostr(val) == "" {
				ph := it.placeholder
				if ph == "" {
					ph = "…"
				}
				return C.Dim + ph + Reset
			}
			return C.Text + Truncate(tostr(val), 28) + Reset
		}
		if val == nil || tostr(val) == "" {
			return C.Muted + "—" + Reset
		}
		return C.Muted + Truncate(tostr(val), 28) + Reset
	}

	current := func() (*Row, int) {
		sel := selectable(st.items)
		if st.row < 1 || st.row > len(sel) {
			return nil, 0
		}
		idx := sel[st.row-1]
		return &st.items[idx-1], idx
	}

	cyclePicker := func(it *Row, dir int) {
		opts := it.resolveOptions()
		if len(opts) == 0 {
			if it.step != nil {
				protect(func() { it.step(dir) })
			}
			return
		}
		var cur any
		if it.getStr != nil {
			if v, _, failed := it.callGet(); !failed {
				cur = v
			}
		}
		idx := 1
		for i, v := range opts {
			if cur != nil && v == cur.(string) {
				idx = i + 1
				break
			}
		}
		step := dir
		if dir == 0 {
			step = 1
		}
		n := len(opts)
		next := ((idx-1+step)%n+n)%n + 1
		if it.setStr != nil {
			protect(func() { it.setStr(opts[next-1]) })
		}
	}

	setToggle := func(it *Row, dir int) {
		cur := false
		if it.getBool != nil {
			if v, _, failed := it.callGet(); !failed {
				cur = truthy(v)
			}
		}
		newv := dir > 0
		if dir == 0 {
			newv = !cur
		}
		if it.setBool != nil {
			protect(func() { it.setBool(newv) })
		}
	}

	chooseRadio := func(it *Row) {
		if it.setStr != nil {
			opt := it.radioOpt
			protect(func() { it.setStr(opt) })
		}
	}

	numberValue := func(it *Row, idx int) float64 {
		val, _, _ := readValue(it, keyOf(it, idx))
		return toNumber(val)
	}

	setSlider := func(it *Row, v float64) {
		n := clampInt(floorf(v+0.5), it.min, it.max)
		if it.setInt != nil {
			protect(func() { it.setInt(n) })
		}
	}

	setStepper := func(it *Row, n int) {
		if it.limited() {
			n = clampInt(n, it.min, it.max)
		}
		if it.setInt != nil {
			protect(func() { it.setInt(n) })
		}
	}

	toggleSection := func(it *Row, dir int) {
		c := !it.collapsed
		if dir > 0 {
			c = false
		} else if dir < 0 {
			c = true
		}
		key := it.id
		if key == "" {
			key = it.name
		}
		st.collapsed[key] = c
		if it.openPtr != nil {
			*it.openPtr = !c
		}
	}

	startEdit := func(it *Row, idx int) {
		if it.wtype == "stepper" {
			cur := strconv.Itoa(int(numberValue(it, idx)))
			st.editing = &editState{idx: idx, chars: util.Chars(cur), numeric: true, commit: func(s string) {
				n, err := strconv.Atoi(strings.TrimSpace(s))
				if err != nil {
					ToastWarn("Not a whole number: " + s)
					return
				}
				setStepper(it, n)
			}}
			return
		}
		cur := ""
		if it.getStr != nil {
			if v, _, failed := it.callGet(); !failed && v != nil {
				cur = tostr(v)
			}
		} else if it.hasValue {
			cur = it.value
		}
		set := it.setStr
		st.editing = &editState{idx: idx, chars: util.Chars(cur), buf: newLineBuffer(cur), commit: func(s string) {
			if set != nil {
				set(s)
			}
		}}
	}

	commitEdit := func() {
		ed := st.editing
		if ed == nil {
			return
		}
		st.editing = nil
		val := strings.Join(ed.chars, "")
		if ed.buf != nil && !ed.numeric {
			val = ed.buf.String()
		}
		protect(func() { ed.commit(val) })
	}

	editKey := func(k *Key) bool {
		ed := st.editing
		if !ed.numeric && ed.buf == nil {
			ed.buf = newLineBuffer(strings.Join(ed.chars, ""))
		}
		switch {
		case k.Mouse != nil:
			commitEdit()
			return false
		case k.Name == "esc":
			st.editing = nil
		case k.Name == "\r" || k.Name == "\n":
			commitEdit()
		case k.Name == "up":
			commitEdit()
			st.row--
			clamp()
		case k.Name == "down":
			commitEdit()
			st.row++
			clamp()
		case ed.numeric:
			if k.Name == "\x7f" || k.Name == "\b" {
				if len(ed.chars) > 0 {
					ed.chars = ed.chars[:len(ed.chars)-1]
				}
				break
			}
			if k.Special {
				break
			}
			if (k.Name >= "0" && k.Name <= "9" && len(k.Name) == 1) || (k.Name == "-" && len(ed.chars) == 0) {
				ed.chars = append(ed.chars, k.Name)
			}
		default:
			if ed.buf.handleKey(k) {
				ed.chars = ed.buf.clusters
			}
		}
		return true
	}

	change := func(dir int) {
		it, idx := current()
		if it == nil {
			return
		}
		switch {
		case it.kind == "section":
			toggleSection(it, dir)
		case it.wtype == "toggle" || it.wtype == "check":
			setToggle(it, dir)
		case it.wtype == "radio":
			if dir >= 0 {
				chooseRadio(it)
			}
		case it.wtype == "slider":
			if dir != 0 {
				lo, hi := float64(it.min), float64(it.max)
				cur := math.Max(lo, math.Min(hi, numberValue(it, idx)))
				setSlider(it, cur+float64(dir*it.stepBy))
			}
		case it.wtype == "stepper":
			if dir == 0 {
				startEdit(it, idx)
			} else {
				setStepper(it, int(numberValue(it, idx))+dir*it.stepBy)
			}
		case it.wtype == "picker":
			cyclePicker(it, dir)
		case it.wtype == "text":
			if dir == 0 {
				startEdit(it, idx)
			}
		case it.wtype == "tablerow" && it.table != nil && it.table.viewer && dir != 0:
			// A viewer scrolls sideways: ← → move through the columns.
			scrollTable(it.table, dir*tableScrollStep, 0)
		case dir == 0 && it.onActivate != nil:
			protect(func() { it.onActivate(ctx) })
		}
	}

	selIndexOf := func(itemIdx int) int {
		n := 0
		for i := 1; i <= itemIdx && i <= len(st.items); i++ {
			if st.items[i-1].isSelectable() {
				n++
			}
		}
		return n
	}

	lastClickIdx, lastClickTime := 0, time.Time{}

	finish := func(cv *canvas) {
		if screen.frameBuf != nil {
			screen.frameBuf.WriteString(cv.b.String())
			return
		}
		cv.paint()
	}
	draw := func(h, w int) {
		if screen.Refresh != nil {
			func() {
				defer func() {
					if p := recover(); p != nil {
						screen.Banner = "Error: " + Truncate(wsRun.ReplaceAllString(fmt.Sprint(p), " "), 70)
					}
				}()
				screen.Refresh(screen)
			}()
		}
		cv := begin(h, w)
		if w < 48 || h < 12 {
			cv.frame(h, w)
			cv.put(2, 3, C.Amber+"Terminal too small"+Reset)
			cv.put(3, 3, C.Muted+fmt.Sprintf("need 48x12, have %dx%d", w, h)+Reset)
			finish(cv)
			return
		}
		cv.frame(h, w)
		title := screen.Title
		if title == "" {
			title = "Screen"
		}
		if screen.Icon != "" {
			title = screen.Icon + " " + title
		}
		cv.chromeTitle(w, title, screen.Breadcrumb)
		contentTop := 4
		if screen.Banner != "" {
			cv.put(3, 3, C.Amber+Truncate(screen.Banner, max(8, w-6))+Reset)
			contentTop = 5
		}
		contentBottom := h - 2
		avail := contentBottom - contentTop + 1
		if avail < 1 {
			finish(cv)
			return
		}
		screen.viewTop, screen.viewAvail, screen.viewWidth = contentTop, avail, w
		if len(screen.Panes) > 0 {
			screen.paintSplit(cv, w, contentTop, avail, func(u *uiState) { st = u }, widget, marked)
			st = screen.uiAt(screen.focus)
			it, _ := current()
			hints := withSplitHints(footerHints(st.editing != nil, screen.Hints, it))
			badges := status()
			cv.footer(h, w, hints, badges)
			st.footSig = footerSig(badges)
			finish(cv)
			return
		}
		screen.bands = nil
		st.page = avail
		items := st.flatten(screen.Sections, w, screen.SectionSpacing)
		st.items = items
		sel := selectable(items)
		clamp()
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
		st.hit = map[int]hitEntry{}
		const left = 3
		for _, pl := range layoutContent(items, st.scroll, contentTop, avail) {
			i := pl.index + 1
			it := &items[pl.index]
			r := pl.row
			selected := i == selItem
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
		if total := len(items); total > avail {
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
		it, _ := current()
		hints := footerHints(st.editing != nil, screen.Hints, it)
		badges := status()
		cv.footer(h, w, hints, badges)
		st.footSig = footerSig(badges)
		finish(cv)
	}

	screen.redraw = draw
	screen.onKey = func(k *Key, h, w, sc *int) (*screenResult, bool) {
		dirty := false
		consumed := false
		if len(screen.Panes) > 0 {
			st = screen.uiAt(screen.focus)
		}
		if len(screen.Panes) > 0 && k != nil && k.Mouse != nil {
			if idx, ok := screen.bandIndex(k.Mouse.Row); ok {
				screen.focus = idx
				st = screen.uiAt(idx)
				dirty = true
			}
		}
		if len(screen.Panes) > 0 && k != nil && (k.Name == "\t" || k.Name == "btab") {
			if st.editing != nil {
				commitEdit()
			}
			dir := 1
			if k.Name == "btab" {
				dir = -1
			}
			screen.cycleFocus(dir)
			st = screen.uiAt(screen.focus)
			return nil, true
		}
		if st.editing != nil && k != nil {
			dirty = true
			consumed = editKey(k)
		}
		switch {
		case k == nil:
			*sc = *sc + 1
			if rt.consumeResize() || *sc >= 2 {
				*sc = 0
				if nh, nw := TermSize(); nh != *h || nw != *w {
					*h, *w = nh, nw
					dirty = true
				}
			}
			if screen.Poll != nil {
				changed := false
				protect(func() { changed = screen.Poll(screen) })
				if changed {
					dirty = true
				}
			}
			if footerSig(status()) != st.footSig {
				dirty = true
			}
		case consumed:
		case k.Mouse != nil && (k.Mouse.WheelUp || k.Mouse.WheelDown):
			n := len(selectable(st.items))
			if k.Mouse.WheelUp && st.row > 1 {
				st.row--
				dirty = true
			}
			if k.Mouse.WheelDown && st.row < n {
				st.row++
				dirty = true
			}
		case k.Mouse != nil && k.Mouse.Click:
			m := k.Mouse
			if sb := st.sb; sb != nil && m.Col == sb.col && m.Row >= sb.top && m.Row <= sb.top+sb.rows-1 {
				frac := float64(m.Row-sb.top) / float64(max(1, sb.rows-1))
				st.scroll = clampInt(floorf(frac*float64(sb.maxoff)+0.5), 0, sb.maxoff)
				sel := selectable(st.items)
				firstVis, lastVis := st.scroll+1, st.scroll+sb.rows
				if st.row >= 1 && st.row <= len(sel) {
					if cur := sel[st.row-1]; cur < firstVis || cur > lastVis {
						for si, ii := range sel {
							if ii >= firstVis && ii <= lastVis {
								st.row = si + 1
								break
							}
						}
					}
				}
				dirty = true
			} else if entry, ok := st.hit[m.Row]; ok {
				if si := selIndexOf(entry.idx); si >= 1 {
					now := time.Now()
					double := si == st.row && lastClickIdx == entry.idx && now.Sub(lastClickTime) <= 450*time.Millisecond
					st.row = si
					lastClickIdx, lastClickTime = entry.idx, now
					dirty = true
					it := &st.items[entry.idx-1]
					onMark := m.Col == 5 || m.Col == 6
					switch {
					case entry.header:
						if double || m.Col == 3 || m.Col == 4 {
							toggleSection(it, 0)
							lastClickIdx = 0
						}
					case entry.slider:
						rel := clampInt(m.Col-entry.sx0, 0, entry.sw-1)
						lo, hi := float64(it.min), float64(it.max)
						v := float64(rel)/float64(max(1, entry.sw-1))*(hi-lo) + lo
						if it.snap {
							s := float64(it.stepBy)
							v = lo + math.Floor((v-lo)/s+0.5)*s
						}
						setSlider(it, math.Floor(v+0.5))
					case entry.stepper && m.Col == entry.minusCol:
						setStepper(it, int(numberValue(it, entry.idx))-it.stepBy)
						lastClickIdx = 0
					case entry.stepper && m.Col == entry.plusCol:
						setStepper(it, int(numberValue(it, entry.idx))+it.stepBy)
						lastClickIdx = 0
					case entry.mark && (onMark || double):
						if it.wtype == "radio" {
							chooseRadio(it)
						} else {
							setToggle(it, 0)
						}
						lastClickIdx = 0
					case double:
						switch {
						case it.wtype == "text" || it.wtype == "stepper":
							startEdit(it, entry.idx)
						case it.wtype == "toggle":
							setToggle(it, 0)
						case it.wtype == "picker":
							cyclePicker(it, 0)
						case it.onActivate != nil:
							protect(func() { it.onActivate(ctx) })
						}
					}
				}
			}
		case k.Name == "home" || k.Name == "end" || k.Name == "pgup" || k.Name == "pgdn":
			n := len(selectable(st.items))
			switch k.Name {
			case "home":
				st.row = 1
			case "end":
				st.row = n
			case "pgup":
				st.row -= max(1, st.page-1)
			case "pgdn":
				st.row += max(1, st.page-1)
			}
			clamp()
			dirty = true
		case k.Name == "up":
			st.row--
			clamp()
			dirty = true
		case k.Name == "down":
			st.row++
			clamp()
			dirty = true
		case k.Name == "right":
			change(1)
			dirty = true
		case k.Name == "left":
			change(-1)
			dirty = true
		case k.Name == "H" || k.Name == "L":
			// Resize the table column under the selection, if there is one.
			if it, _ := current(); it != nil && it.table != nil && it.table.viewer {
				d := 1
				if k.Name == "H" {
					d = -1
				}
				resizeTableColumn(it.table, it.table.hscroll, d)
				dirty = true
			}
		case k.Name == " " || k.Name == "\r" || k.Name == "\n":
			change(0)
			dirty = true
		case k.Name == "b" || k.Name == "esc":
			return &screenResult{op: "back"}, false
		case k.Name == "q" || k.Name == "\x03":
			return &screenResult{op: "quit"}, false
		case len(screen.Panes) > 0 && st.editing == nil && (k.Name == "[" || k.Name == "]" || k.Name == "|"):
			switch k.Name {
			case "[":
				screen.nudgeFocus(-1)
			case "]":
				screen.nudgeFocus(1)
			default:
				screen.toggleFocused()
			}
			dirty = true
		}
		if res := takeNav(); res != nil {
			return res, false
		}
		return nil, dirty
	}
}

func (screen *Screen) renderFrame(h, w int) string {
	screen.bind()
	var b strings.Builder
	screen.frameBuf = &b
	screen.redraw(h, w)
	screen.frameBuf = nil
	return b.String()
}

func (screen *Screen) pressKey(k *Key, h, w int) {
	screen.bind()
	if screen.viewAvail == 0 {
		screen.renderFrame(h, w)
	}
	editing := false
	if u := screen.uiAt(screen.focus); u != nil {
		editing = u.editing != nil
	}
	k = navKey(k, editing)
	sc := 0
	_, dirty := screen.onKey(k, &h, &w, &sc)
	if dirty {
		screen.renderFrame(h, w)
	}
}
