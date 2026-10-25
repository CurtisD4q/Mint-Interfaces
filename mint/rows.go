package mint

import (
	"fmt"
	"strconv"
)

// Style sets the visual emphasis of a Label row.
type Style int

// TextStyle is an alias for Style.
type TextStyle = Style

const (
	StyleBody Style = iota
	StyleTitle
	StyleHeading
	StyleCaption
	StyleItalic
	StyleUnderline
)

// Context is an alias for Ctx so callbacks may refer to either name.
type Context = Ctx

// Row is one item in a Section. Construct rows with Value, Action, Toggle,
// Checkbox, Radio, Picker (or Select), Slider, Progress, Stepper, Text,
// Note, Label, Heading, Divider, Spacer, Log, Collapsible, or Table.
type Row struct {
	kind        string // "row" or "section"
	name        string
	collapsible bool
	collapsed   bool
	hidden      int
	openPtr     *bool
	badge       string

	wtype       string
	id          string
	label       string
	value       string
	hasValue    bool
	hint        string
	placeholder string
	unit        string
	divChar     string
	radioOpt    string
	indent      int
	style       Style

	min, max, stepBy int
	hasBounds        bool
	snap             bool

	options     []string
	optionsFunc func() []string

	getStr     func() string
	setStr     func(string)
	getBool    func() bool
	setBool    func(bool)
	getInt     func() int
	setInt     func(int)
	step       func(int)
	format     func(int) string
	onActivate func(*Ctx)

	logHeight int
	logLines  func() []LogLine
	logLine   LogLine
	logEmpty  bool

	children  []Row
	textLines func() []string
	chunkSize int

	headers     []string
	headersFn   func() []string
	tableRows   [][]string
	tableGet    func() [][]string
	tableSelect func(*Ctx, int, []string)
	separator   string
}

func (r Row) normalized() Row {
	if r.kind == "" {
		r.kind = "row"
	}
	if r.wtype == "" {
		r.wtype = "value"
	}
	if r.wtype == "slider" {
		if r.min == 0 && r.max == 0 {
			r.min, r.max = 0, 100
		}
		if r.stepBy <= 0 {
			r.stepBy = 5
		}
		r.hasBounds = true
	}
	if r.wtype == "stepper" {
		if r.stepBy <= 0 {
			r.stepBy = 1
		}
		if r.min != 0 || r.max != 0 {
			r.hasBounds = true
		}
	}
	if r.wtype == "divider" && r.divChar == "" {
		r.divChar = "─"
	}
	if r.wtype == "log" && r.logHeight <= 0 {
		r.logHeight = 6
	}
	return r
}

func (r *Row) isSelectable() bool {
	if r.kind == "section" {
		return r.collapsible
	}
	switch r.wtype {
	case "note", "divider", "spacer", "label", "logline", "log", "progress":
		return false
	}
	return true
}

func (r *Row) hasGet() bool {
	return r.getStr != nil || r.getBool != nil || r.getInt != nil
}

func (r *Row) callGet() (val any, msg string, failed bool) {
	defer func() {
		if p := recover(); p != nil {
			val = nil
			msg = fmt.Sprint(p)
			failed = true
		}
	}()
	switch {
	case r.getStr != nil:
		return r.getStr(), "", false
	case r.getBool != nil:
		return r.getBool(), "", false
	case r.getInt != nil:
		return r.getInt(), "", false
	}
	return nil, "", false
}

func (r *Row) numberText(v int) string {
	if r.format != nil {
		var out string
		func() {
			defer func() {
				if recover() != nil {
					out = strconv.Itoa(v)
				}
			}()
			out = r.format(v)
		}()
		return out
	}
	if r.unit != "" {
		return fmt.Sprintf("%d %s", v, r.unit)
	}
	return strconv.Itoa(v)
}

func (r *Row) limited() bool {
	return r.hasBounds
}

func (r *Row) resolveOptions() []string {
	if r.optionsFunc != nil {
		var out []string
		func() {
			defer func() { _ = recover() }()
			out = r.optionsFunc()
		}()
		return out
	}
	return r.options
}

func protect(fn func()) {
	defer func() {
		if p := recover(); p != nil {
			ToastError(fmt.Sprint(p))
		}
	}()
	fn()
}

func tostr(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(x)
	}
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case string:
		return x != "" && x != "false" && x != "0"
	}
	return false
}

func toNumber(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

// ── Row constructors ─────────────────────────────────────────────────────────

// ValueOpts configures a read-only (or drill-down) key/value row.
type ValueOpts struct {
	ID         string
	Label      string
	Value      string
	Get        func() string
	OnActivate func(*Ctx)
}

// Value creates a row displaying a label on the left and a value on the right.
func Value(opts ValueOpts) Row {
	return Row{
		kind:       "row",
		wtype:      "value",
		id:         opts.ID,
		label:      opts.Label,
		value:      opts.Value,
		hasValue:   opts.Value != "",
		getStr:     opts.Get,
		onActivate: opts.OnActivate,
	}
}

// ActionOpts configures a clickable action row.
type ActionOpts struct {
	ID         string
	Label      string
	Hint       string
	Get        func() string
	OnActivate func(*Ctx)
}

// Action creates an interactive command/navigation row.
func Action(opts ActionOpts) Row {
	return Row{
		kind:       "row",
		wtype:      "action",
		id:         opts.ID,
		label:      opts.Label,
		hint:       opts.Hint,
		getStr:     opts.Get,
		onActivate: opts.OnActivate,
	}
}

// ToggleOpts configures an immediate ON/OFF switch row.
type ToggleOpts struct {
	ID    string
	Label string
	Get   func() bool
	Set   func(bool)
}

// Toggle creates an ON/OFF toggle row.
func Toggle(opts ToggleOpts) Row {
	return Row{
		kind:    "row",
		wtype:   "toggle",
		id:      opts.ID,
		label:   opts.Label,
		getBool: opts.Get,
		setBool: opts.Set,
	}
}

// CheckboxOpts configures a checkbox row (☐ / ☑).
type CheckboxOpts struct {
	ID    string
	Label string
	Get   func() bool
	Set   func(bool)
}

// Checkbox creates a checkbox row for boolean flags.
func Checkbox(opts CheckboxOpts) Row {
	return Row{
		kind:    "row",
		wtype:   "check",
		id:      opts.ID,
		label:   opts.Label,
		getBool: opts.Get,
		setBool: opts.Set,
	}
}

// RadioOpts configures a group of mutually exclusive radio options (○ / ◉).
type RadioOpts struct {
	ID      string
	Label   string
	Options []string
	Get     func() string
	Set     func(string)
}

// Radio creates a radio-button group row.
func Radio(opts RadioOpts) Row {
	return Row{
		kind:    "row",
		wtype:   "radiogroup",
		id:      opts.ID,
		label:   opts.Label,
		options: opts.Options,
		getStr:  opts.Get,
		setStr:  opts.Set,
	}
}

// PickerOpts configures a left/right cycling picker row (‹ value ›).
type PickerOpts struct {
	ID          string
	Label       string
	Options     []string
	OptionsFunc func() []string
	Get         func() string
	Set         func(string)
	Step        func(int)
}

// SelectOpts is an alias for PickerOpts.
type SelectOpts = PickerOpts

// Picker creates a cycling choice row.
func Picker(opts PickerOpts) Row {
	return Row{
		kind:        "row",
		wtype:       "picker",
		id:          opts.ID,
		label:       opts.Label,
		options:     opts.Options,
		optionsFunc: opts.OptionsFunc,
		getStr:      opts.Get,
		setStr:      opts.Set,
		step:        opts.Step,
	}
}

// Select is an alias for Picker.
func Select(opts SelectOpts) Row { return Picker(opts) }

// SliderOpts configures a horizontal bar slider row.
type SliderOpts struct {
	ID     string
	Label  string
	Min    int
	Max    int
	Step   int
	Unit   string
	Snap   bool
	Format func(int) string
	Get    func() int
	Set    func(int)
}

// Slider creates a slider row.
func Slider(opts SliderOpts) Row {
	minV, maxV := opts.Min, opts.Max
	if minV == 0 && maxV == 0 {
		maxV = 100
	}
	step := opts.Step
	if step <= 0 {
		step = 5
	}
	return Row{
		kind:      "row",
		wtype:     "slider",
		id:        opts.ID,
		label:     opts.Label,
		min:       minV,
		max:       maxV,
		stepBy:    step,
		hasBounds: true,
		unit:      opts.Unit,
		snap:      opts.Snap || opts.Step > 0,
		format:    opts.Format,
		getInt:    opts.Get,
		setInt:    opts.Set,
	}
}

// ProgressOpts configures a read-only percentage progress bar row.
type ProgressOpts struct {
	ID     string
	Label  string
	Format func(int) string
	Get    func() int
}

// Progress creates a progress bar row (0–100%).
func Progress(opts ProgressOpts) Row {
	return Row{
		kind:   "row",
		wtype:  "progress",
		id:     opts.ID,
		label:  opts.Label,
		min:    0,
		max:    100,
		format: opts.Format,
		getInt: opts.Get,
	}
}

// StepperOpts configures a numeric − / + stepper row.
type StepperOpts struct {
	ID     string
	Label  string
	Min    int
	Max    int
	Step   int
	Unit   string
	Format func(int) string
	Get    func() int
	Set    func(int)
}

// Stepper creates a numeric stepper row.
func Stepper(opts StepperOpts) Row {
	step := opts.Step
	if step <= 0 {
		step = 1
	}
	return Row{
		kind:      "row",
		wtype:     "stepper",
		id:        opts.ID,
		label:     opts.Label,
		min:       opts.Min,
		max:       opts.Max,
		stepBy:    step,
		hasBounds: opts.Min != 0 || opts.Max != 0,
		unit:      opts.Unit,
		format:    opts.Format,
		getInt:    opts.Get,
		setInt:    opts.Set,
	}
}

// TextOpts configures an inline-editable text field row.
type TextOpts struct {
	ID          string
	Label       string
	Placeholder string
	Value       string
	Get         func() string
	Set         func(string)
}

// Text creates an inline-editable text row.
func Text(opts TextOpts) Row {
	return Row{
		kind:        "row",
		wtype:       "text",
		id:          opts.ID,
		label:       opts.Label,
		placeholder: opts.Placeholder,
		value:       opts.Value,
		hasValue:    opts.Value != "",
		getStr:      opts.Get,
		setStr:      opts.Set,
	}
}

// Note creates a single-line dim note row.
func Note(text string) Row {
	return Row{kind: "row", wtype: "note", label: text}
}

// Label creates a styled text row that wraps across the screen width.
func Label(style Style, text string) Row {
	return Row{kind: "row", wtype: "label", style: style, label: text}
}

// Heading creates a bold sub-heading row.
func Heading(text string) Row {
	return Label(StyleHeading, text)
}

// Divider creates a horizontal rule row using the optional character (default "─").
func Divider(char ...string) Row {
	ch := "─"
	if len(char) > 0 && char[0] != "" {
		ch = char[0]
	}
	return Row{kind: "row", wtype: "divider", divChar: ch}
}

// Spacer creates a blank line row.
func Spacer() Row {
	return Row{kind: "row", wtype: "spacer"}
}
