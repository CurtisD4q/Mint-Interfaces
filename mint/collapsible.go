package mint

import (
	"fmt"
	"strings"
)

// CollapsibleOpts configures a foldable inline section or large-text block.
// A Collapsible Row can be placed directly inside a Section's Rows slice and
// expands inline when opened with Enter, Space, or →.
type CollapsibleOpts struct {
	ID        string          // optional state key; defaults to Title
	Title     string          // header label shown beside ▸ / ▾
	Badge     string          // optional right-side summary (defaults to "N hidden" or line count)
	Open      *bool           // optional pointer to caller-managed open state
	Collapsed bool            // initial state when Open is nil (true = start folded)
	Rows      []Row           // child rows shown when unfolded
	Lines     func() []string // optional dynamic large-text line source
	Text      string          // optional static multi-line text
	ChunkSize int             // when > 0, splits large text into nested collapsible chunks of ChunkSize lines
	Empty     string          // shown when Lines/Text is empty (default "No text")
}

// Collapsible creates a foldable header row that expands its child Rows or
// large-text Lines inline when toggled.
func Collapsible(o CollapsibleOpts) Row {
	empty := o.Empty
	if empty == "" {
		empty = "No text"
	}
	linesFn := o.Lines
	if linesFn == nil && o.Text != "" {
		raw := strings.TrimRight(o.Text, "\n")
		split := strings.Split(raw, "\n")
		linesFn = func() []string { return split }
	}
	collapsed := o.Collapsed
	if o.Open != nil {
		collapsed = !*o.Open
	}
	return Row{
		kind:        "row",
		id:          o.ID,
		name:        o.Title,
		label:       o.Title,
		badge:       o.Badge,
		wtype:       "collapsible",
		collapsible: true,
		collapsed:   collapsed,
		openPtr:     o.Open,
		children:    o.Rows,
		textLines:   linesFn,
		chunkSize:   o.ChunkSize,
		placeholder: empty,
	}
}

// CollapsibleSection is a shorthand for building a foldable Section.
func CollapsibleSection(name string, collapsed bool, rows ...Row) Section {
	return Section{
		Name:        name,
		Collapsible: true,
		Collapsed:   collapsed,
		Rows:        rows,
	}
}

// CollapsibleText wraps a potentially large multi-line string into a
// foldable Row. When chunkSize > 0 and text exceeds chunkSize lines, it is
// automatically divided into nested collapsible chunk headers.
func CollapsibleText(title, text string, chunkSize int) Row {
	return Collapsible(CollapsibleOpts{
		Title:     title,
		Text:      text,
		Collapsed: true,
		ChunkSize: chunkSize,
	})
}

// CollapsibleLines wraps a dynamic slice of text lines into a foldable Row,
// splitting into chunkSize-line collapsible sections when chunkSize > 0.
func CollapsibleLines(title string, lines func() []string, chunkSize int) Row {
	return Collapsible(CollapsibleOpts{
		Title:     title,
		Lines:     lines,
		Collapsed: true,
		ChunkSize: chunkSize,
	})
}

func buildCollapsibleChildren(r Row) ([]Row, string) {
	var out []Row
	out = append(out, r.children...)
	badge := r.badge

	if r.textLines != nil {
		var raw []string
		protect(func() { raw = r.textLines() })
		cleaned := make([]string, 0, len(raw))
		for _, l := range raw {
			cleaned = append(cleaned, CleanLine(l))
		}
		if badge == "" {
			badge = fmt.Sprintf("%d lines", len(cleaned))
		}
		if len(cleaned) == 0 {
			out = append(out, Note(r.placeholder))
			return out, badge
		}
		title := r.name
		if title == "" {
			title = "Output"
		}
		logLinesFn := func() []LogLine {
			ll := make([]LogLine, len(cleaned))
			for i, s := range cleaned {
				ll[i] = LogLine{Text: s}
			}
			return ll
		}
		if r.chunkSize > 0 && len(cleaned) > r.chunkSize {
			out = append(out, Action(ActionOpts{
				Label: "Open full text in viewer",
				Hint:  fmt.Sprintf("view (%d lines)", len(cleaned)),
				OnActivate: func(c *Ctx) {
					c.Push(ViewLog(title, logLinesFn))
				},
			}))
			baseKey := r.id
			if baseKey == "" {
				baseKey = r.name
			}
			for start := 0; start < len(cleaned); start += r.chunkSize {
				end := min(len(cleaned), start+r.chunkSize)
				chunk := cleaned[start:end]
				chunkRows := make([]Row, len(chunk))
				for i, line := range chunk {
					chunkRows[i] = Row{kind: "row", wtype: "logline", logLine: LogLine{Text: line}}
				}
				chunkTitle := fmt.Sprintf("Lines %d–%d", start+1, end)
				out = append(out, Collapsible(CollapsibleOpts{
					ID:        fmt.Sprintf("%s:chunk:%d", baseKey, start),
					Title:     chunkTitle,
					Badge:     fmt.Sprintf("%d lines", len(chunk)),
					Collapsed: true,
					Rows:      chunkRows,
				}))
			}
		} else {
			for _, line := range cleaned {
				out = append(out, Row{kind: "row", wtype: "logline", logLine: LogLine{Text: line}})
			}
			if len(cleaned) > 10 {
				out = append(out, Action(ActionOpts{
					Label: "Open full text in viewer",
					Hint:  fmt.Sprintf("view (%d lines)", len(cleaned)),
					OnActivate: func(c *Ctx) {
						c.Push(ViewLog(title, logLinesFn))
					},
				}))
			}
		}
	}
	return out, badge
}
