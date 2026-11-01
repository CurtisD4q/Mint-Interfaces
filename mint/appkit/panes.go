package appkit

import (
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// Every archetype keeps ordinary options and advanced options in separate
// panes: Basic Options, then Advanced + Additional, then Output.
const (
	titleBasic    = "Basic Options"
	titleAdvanced = "Advanced + Additional"
	titleOutput   = "Output"
)

// advancedInOwnPane reports whether an archetype puts its advanced options
// in their own pane. Every archetype does; the pane only appears when a
// tool has advanced options.
func advancedInOwnPane(id string) bool { return true }

func (a *ToolApp) advancedInOwnPane() bool {
	return a != nil && advancedInOwnPane(a.Spec.ArchetypeID)
}

func (a *ToolApp) hasAdvancedOptions() bool {
	for _, f := range a.Spec.Flags {
		if f.Advanced {
			return true
		}
	}
	return false
}

// dropEmpty hides sections that have no rows, so a pane is never padded
// with a blank heading.
func dropEmpty(sections []mint.Section) []mint.Section {
	out := make([]mint.Section, 0, len(sections))
	for _, s := range sections {
		if len(s.Rows) == 0 {
			continue
		}
		out = append(out, s)
	}
	return out
}

// onePane is one scrolling pane. An empty list is no pane: the same kind
// of content is never repeated to fill a slot, and a pane is never blank.
func onePane(title string, sections []mint.Section) *mint.Pane {
	sections = dropEmpty(sections)
	if title == "" || len(sections) == 0 {
		return nil
	}
	return &mint.Pane{Title: title, Sections: sections}
}

func appendPane(panes []*mint.Pane, p *mint.Pane) []*mint.Pane {
	if p == nil {
		return panes
	}
	return append(panes, p)
}

// layoutMain stacks Basic Options, then at most one Advanced + Additional
// pane, then any pane whose job is neither options nor output,
// then Output. The run command is docked under Output.
func (a *ToolApp) layoutMain(scr *mint.Screen, mode execMode, canPoll bool, interval time.Duration) {
	basic, advanced := a.optionSections()
	var panes []*mint.Pane
	if a.advancedInOwnPane() && a.hasAdvancedOptions() {
		panes = appendPane(panes, onePane(titleAdvanced, advanced))
	}
	panes = append(panes, a.rolePanes()...)
	title := a.outputSectionName(a.outputFallback(mode))
	panes = appendPane(panes, onePane(titleOutput, []mint.Section{a.buildOutputDisplaySection(title)}))

	scr.TopTitle = titleBasic
	scr.Sections = basic
	scr.SetPanes(panes)
	scr.Dock = dropEmpty([]mint.Section{a.executionZone(mode, canPoll, interval)})
}

// optionSections is the ordinary options, and the advanced ones for their
// own pane. With nothing advanced, the second list
// is empty and Extra arguments stay with the ordinary options.
func (a *ToolApp) optionSections() (basic, advanced []mint.Section) {
	split := a.advancedInOwnPane() && a.hasAdvancedOptions()
	var flags, adv []mint.Section
	if split {
		flags, adv = a.splitFlagSections()
	} else {
		flags = a.flagSections()
	}
	basic = a.inputAndFlags(flags, !split)
	basic = a.withFileAndRoot(basic)
	basic = dropEmpty(basic)
	if !split {
		return basic, nil
	}
	// Extra arguments are additional options, so they travel with the
	// advanced pane. If that would leave Basic Options empty, they stay.
	if len(basic) == 0 {
		basic = dropEmpty([]mint.Section{a.extraArgsSection()})
		return basic, dropEmpty(adv)
	}
	adv = append(adv, a.extraArgsSection())
	return basic, dropEmpty(adv)
}

func (a *ToolApp) inputAndFlags(flags []mint.Section, includeExtra bool) []mint.Section {
	var sections []mint.Section
	if in := a.inputSection(includeExtra); len(in.Rows) > 0 {
		sections = append(sections, in)
	}
	sections = append(sections, flags...)
	return sections
}

func (a *ToolApp) withFileAndRoot(sections []mint.Section) []mint.Section {
	if a.Spec.OutputsFile {
		sections = append(sections, a.outputFileSection())
	}
	if root := a.rootAuthSection(); root != nil && len(root.Rows) > 0 {
		sections = append(sections, *root)
	}
	return sections
}

func (a *ToolApp) extraArgsSection() mint.Section {
	return mint.Section{Name: "Extra arguments", Rows: []mint.Row{a.extraArgsRow()}}
}

func (a *ToolApp) helpPrefix() []string {
	if a.Spec.ArchetypeID == "toolkit" && a.ActiveSubcommand != "" {
		return []string{a.ActiveSubcommand}
	}
	return nil
}

// rolePanes are the panes that do a different job from options and from
// the output log: findings, or a preview of what will change. A report,
// a page or a table of targets is the output, so it stays in that pane.
func (a *ToolApp) rolePanes() []*mint.Pane {
	var panes []*mint.Pane
	switch a.Spec.ArchetypeID {
	case "audit":
		panes = appendPane(panes, onePane("Findings", a.findingsSections()))
	case "mirror", "edit-files":
		panes = appendPane(panes, onePane("What will change", a.changePreviewSections()))
	}
	return panes
}

// changePreviewSections is the preview of a mirror or an edit, without the
// output log. No preview means no pane.
func (a *ToolApp) changePreviewSections() []mint.Section {
	if a.Spec.Preview == nil {
		return nil
	}
	plan, err := a.MakePlan()
	if err != nil {
		return []mint.Section{{Name: "Preview", Rows: []mint.Row{mint.Note(err.Error())}}}
	}
	head, files := a.previewParts(plan)
	return dropEmpty(append(head, files...))
}
