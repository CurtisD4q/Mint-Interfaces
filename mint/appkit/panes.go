package appkit

import (
	"fmt"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// maxPaneSections is how many sections one pane may hold. Further sections
// spill into another pane with the same title.
const maxPaneSections = 10

func takePane(sections []mint.Section) (head, rest []mint.Section) {
	if len(sections) <= maxPaneSections {
		return sections, nil
	}
	return append([]mint.Section(nil), sections[:maxPaneSections]...), append([]mint.Section(nil), sections[maxPaneSections:]...)
}

// chunkedPanes splits sections into panes of at most maxPaneSections.
// An empty list becomes one pane with a short note, so the title stays.
func chunkedPanes(title string, sections []mint.Section) []*mint.Pane {
	if len(sections) == 0 {
		sections = []mint.Section{{Name: title, Rows: []mint.Row{mint.Note("  No further options.")}}}
	}
	var out []*mint.Pane
	for len(sections) > 0 {
		var head []mint.Section
		head, sections = takePane(sections)
		out = append(out, &mint.Pane{Title: title, Sections: head})
	}
	return out
}

func splitTableArchetype(id string) bool {
	switch id {
	case "list", "records", "search":
		return true
	default:
		return false
	}
}

func (a *ToolApp) chromeSections(mode execMode, canPoll bool, interval time.Duration) []mint.Section {
	var sections []mint.Section
	sections = append(sections, a.inputZone())
	if a.Spec.OutputsFile {
		sections = append(sections, a.outputFileSection())
	}
	if root := a.rootAuthSection(); root != nil {
		sections = append(sections, *root)
	}
	sections = append(sections, a.executionZone(mode, canPoll, interval))
	return sections
}

// formSections is the single-column screen: chrome, flag headings, and
// inline output when this archetype shows it there.
func (a *ToolApp) formSections(mode execMode, canPoll bool, interval time.Duration, output bool) []mint.Section {
	var sections []mint.Section
	sections = append(sections, a.inputZone())
	sections = append(sections, a.flagSections()...)
	if a.Spec.OutputsFile {
		sections = append(sections, a.outputFileSection())
	}
	if root := a.rootAuthSection(); root != nil {
		sections = append(sections, *root)
	}
	sections = append(sections, a.executionZone(mode, canPoll, interval))
	if output {
		sections = append(sections, a.buildOutputDisplaySection(a.outputSectionName(a.outputFallback(mode))))
	}
	return sections
}

func (a *ToolApp) helpPrefix() []string {
	if a.Spec.ArchetypeID == "toolkit" && a.ActiveSubcommand != "" {
		return []string{a.ActiveSubcommand}
	}
	return nil
}

func (a *ToolApp) outputBlocks() []mint.Section {
	lines := a.ActiveLines()
	title := a.outputSectionName(a.outputFallback(a.executionMode()))
	if len(lines) == 0 {
		return []mint.Section{{Name: title, Rows: []mint.Row{mint.Note("  No output captured yet.")}}}
	}
	return splitBlockSections(lines, a.Spec.ChunkSize, title)
}

// splitBlockSections is SplitSections as one section per heading block.
func splitBlockSections(lines []string, chunkSize int, fallback string) []mint.Section {
	if chunkSize <= 0 {
		chunkSize = 100
	}
	if fallback == "" {
		fallback = "Output"
	}
	type secGroup struct {
		title string
		lines []string
	}
	var groups []secGroup
	var cur *secGroup
	for _, raw := range lines {
		cleaned := mint.CleanLine(raw)
		trimmed := strings.TrimSpace(cleaned)
		if len(cleaned) > 0 && cleaned[0] != ' ' && cleaned[0] != '\t' && sectionHeadingRe.MatchString(trimmed) && len(trimmed) >= 2 {
			groups = append(groups, secGroup{title: strings.TrimSuffix(trimmed, ":")})
			cur = &groups[len(groups)-1]
			continue
		}
		if cur == nil {
			groups = append(groups, secGroup{title: fallback})
			cur = &groups[len(groups)-1]
		}
		cur.lines = append(cur.lines, cleaned)
	}
	if len(groups) < 2 {
		copied := append([]string(nil), lines...)
		return []mint.Section{{
			Name: fallback,
			Rows: []mint.Row{mint.Collapsible(mint.CollapsibleOpts{
				ID: "output:main", Title: fallback, Badge: fmt.Sprintf("%d lines", len(copied)),
				Collapsed: false, Lines: func() []string { return copied }, ChunkSize: chunkSize,
			})},
		}}
	}
	var out []mint.Section
	for i, g := range groups {
		body := append([]string(nil), g.lines...)
		title := g.title
		out = append(out, mint.Section{
			Name: title,
			Rows: []mint.Row{mint.Collapsible(mint.CollapsibleOpts{
				ID: "sec:" + title, Title: title,
				Badge:     fmt.Sprintf("%d lines", len(body)),
				Collapsed: i != 0,
				Lines:     func() []string { return body },
				ChunkSize: chunkSize,
			})},
		})
	}
	return out
}

func (a *ToolApp) tableOnlySection(title string) (mint.Section, bool) {
	if !a.Spec.TableOutput {
		return mint.Section{}, false
	}
	lines := a.ActiveLines()
	if len(lines) == 0 {
		return mint.Section{}, false
	}
	headers, rows := mint.ParseTable(lines)
	if len(a.Spec.TableColumns) > 0 && len(headers) == 0 {
		headers = a.Spec.TableColumns
	}
	if len(headers) == 0 || len(rows) == 0 {
		return mint.Section{}, false
	}
	hdr, body := append([]string(nil), headers...), append([][]string(nil), rows...)
	return mint.Section{Name: title, Rows: []mint.Row{
		mint.Table(mint.TableOpts{
			ID: "tbl:" + a.Spec.Name, Headers: hdr, Rows: body, MaxHeight: 20,
			Viewer: wideTableViewer(a.Spec.ArchetypeID),
			OnSelect: func(c *mint.Ctx, rowIdx int, row []string) {
				a.handleTableRowSelect(c, rowIdx, hdr, row)
			},
		}),
	}}, true
}

// layoutMain chooses the stacked panes for this archetype. Screens that
// are not listed keep the single column, or the panes they already had.
func (a *ToolApp) layoutMain(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	switch a.Spec.ArchetypeID {
	case "inspect":
		a.layoutInspect(scr, mode, canPoll, interval)
	case "read":
		a.layoutRead(scr, mode, canPoll, interval)
	case "probe":
		a.layoutProbe(scr, mode, canPoll, interval)
	case "audit":
		a.layoutAudit(scr, mode, canPoll, interval)
	case "convert":
		a.layoutConvert(scr, mode, canPoll, interval)
	case "toolkit":
		a.layoutToolkit(scr, mode, canPoll, interval)
	case "transfer":
		a.layoutTransfer(scr, mode, canPoll, interval)
	case "mirror":
		a.layoutMirror(scr, mode, canPoll, interval)
	case "measure":
		a.layoutMeasure(scr, mode, canPoll, interval)
	case "control", "history":
		a.layoutDetailsOnly(scr, mode, canPoll, interval)
	default:
		a.layoutDefault(scr, mode, canPoll, interval)
	}
}

func (a *ToolApp) layoutDefault(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	splitTable := splitTableArchetype(a.Spec.ArchetypeID)
	splitDiff := (a.Spec.TwoInputs || a.Spec.Diff != nil) && !splitTable
	var panes []*mint.Pane
	if splitTable {
		// A table opens its row itself; there is no Details or Actions pane
		// to keep in step with the selection.
	} else if splitDiff {
		title := a.outputSectionName(a.outputFallback(mode))
		panes = []*mint.Pane{{
			Title: title,
			Refresh: func(p *mint.Pane) {
				if a.showsInlineResults() {
					p.Sections = []mint.Section{a.buildOutputDisplaySection(title)}
				}
			},
		}}
	}
	scr.Sections = a.formSections(mode, canPoll, interval, a.showsInlineResults() && !splitDiff)
	scr.SetPanes(panes)
	for _, p := range scr.Panes {
		if p != nil && p.Refresh != nil {
			p.Refresh(p)
		}
	}
}

func (a *ToolApp) layoutDetailsOnly(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	sections := a.formSections(mode, canPoll, interval, true)
	top, rest := takePane(sections)
	var panes []*mint.Pane
	if len(rest) > 0 {
		panes = append(panes, chunkedPanes("Options", rest)...)
	}

	scr.Sections = top
	scr.SetPanes(panes)
	for _, p := range scr.Panes {
		if p != nil && p.Refresh != nil {
			p.Refresh(p)
		}
	}
}

// packFirst puts chrome and then option headings into the top pane until
// it holds 10 sections. The rest are Options panes.
func packFirst(chrome, flags []mint.Section) (top []mint.Section, options []*mint.Pane) {
	all := append(append([]mint.Section{}, chrome...), flags...)
	top, rest := takePane(all)
	if len(rest) > 0 {
		options = chunkedPanes("Options", rest)
	}
	return top, options
}

func (a *ToolApp) layoutInspect(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	chrome := a.chromeSections(mode, canPoll, interval)
	if table, ok := a.tableOnlySection(a.outputSectionName("Report")); ok {
		chrome = append(chrome, table)
	}
	top, options := packFirst(chrome, a.flagSections())
	var panes []*mint.Pane
	panes = append(panes, options...)
	panes = append(panes, chunkedPanes("Report", a.outputBlocks())...)
	scr.Sections = top
	scr.SetPanes(panes)
	for _, p := range scr.Panes {
		if p != nil && p.Refresh != nil {
			p.Refresh(p)
		}
	}
}

func (a *ToolApp) layoutRead(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	top, options := packFirst(a.chromeSections(mode, canPoll, interval), a.flagSections())
	var panes []*mint.Pane
	panes = append(panes, options...)
	panes = append(panes, chunkedPanes("Page", a.outputBlocks())...)
	scr.Sections = top
	scr.SetPanes(panes)
}

func (a *ToolApp) layoutProbe(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	top, options := packFirst(a.chromeSections(mode, canPoll, interval), a.flagSections())
	title := a.outputSectionName("Discovered Targets")
	var panes []*mint.Pane
	panes = append(panes, options...)
	panes = append(panes, &mint.Pane{Title: "Discovered targets", Sections: []mint.Section{a.buildOutputDisplaySection(title)}})
	scr.Sections = top
	scr.SetPanes(panes)
}

func (a *ToolApp) layoutAudit(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	top, options := packFirst(a.chromeSections(mode, canPoll, interval), a.flagSections())
	var panes []*mint.Pane
	panes = append(panes, options...)
	panes = append(panes, chunkedPanes("Findings", a.findingsSections())...)
	scr.Sections = top
	scr.SetPanes(panes)
}

func (a *ToolApp) layoutConvert(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	top, options := packFirst(a.chromeSections(mode, canPoll, interval), a.flagSections())
	scr.Sections = top
	scr.SetPanes(options)
}

func (a *ToolApp) layoutToolkit(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	top, options := packFirst(a.chromeSections(mode, canPoll, interval), a.flagSections())
	var panes []*mint.Pane
	panes = append(panes, options...)
	title := a.outputSectionName("Output")
	panes = append(panes, &mint.Pane{Title: "Output", Sections: []mint.Section{a.buildOutputDisplaySection(title)}})
	scr.Sections = top
	scr.SetPanes(panes)
}

func (a *ToolApp) layoutTransfer(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	top, options := packFirst(a.chromeSections(mode, canPoll, interval), a.flagSections())
	title := a.outputSectionName("Progress")
	var panes []*mint.Pane
	panes = append(panes, options...)
	panes = append(panes, &mint.Pane{Title: "Progress", Sections: []mint.Section{a.buildOutputDisplaySection(title)}})
	scr.Sections = top
	scr.SetPanes(panes)
}

func (a *ToolApp) layoutMirror(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	top, options := packFirst(a.chromeSections(mode, canPoll, interval), a.flagSections())
	var panes []*mint.Pane
	panes = append(panes, options...)
	panes = append(panes, chunkedPanes("What will change", a.mirrorChangeSections())...)
	scr.Sections = top
	scr.SetPanes(panes)
}

func (a *ToolApp) mirrorChangeSections() []mint.Section {
	var sections []mint.Section
	if a.Spec.Preview != nil {
		plan, err := a.MakePlan()
		if err != nil {
			sections = append(sections, mint.Section{Name: "Preview", Rows: []mint.Row{mint.Note(err.Error())}})
		} else {
			head, files := a.previewParts(plan)
			sections = append(sections, head...)
			sections = append(sections, files...)
		}
	}
	sections = append(sections, a.buildOutputDisplaySection(a.outputSectionName("Changes")))
	return sections
}

func (a *ToolApp) layoutMeasure(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	chrome := a.chromeSections(mode, canPoll, interval)
	// The stats table stays with the run. Option headings that still fit
	// stay here too; the rest are the Options pane.
	if a.showsInlineResults() {
		chrome = append(chrome, a.buildOutputDisplaySection(a.outputSectionName(a.outputFallback(mode))))
	}
	top, options := packFirst(chrome, a.flagSections())
	if len(options) == 0 {
		options = chunkedPanes("Options", nil)
	}
	scr.Sections = top
	scr.SetPanes(options)
}
