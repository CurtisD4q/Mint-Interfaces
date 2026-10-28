package appkit

// Layout is a deprecated name for the six old screen skeletons
// (config, runner, scanner, monitor, explorer, suite). MainScreen no
// longer branches on it: a screen is assembled from the archetype's
// features. The type and constants remain so saved specs and older
// callers still compile. New code should set ArchetypeID.
type Layout string

const (
	LayoutConfig   Layout = "config"   // State Editor (read -> tweak -> apply)
	LayoutRunner   Layout = "runner"   // Task & Pipeline (input + flags -> progress -> output)
	LayoutScanner  Layout = "scanner"  // Target -> Live Table + Log
	LayoutMonitor  Layout = "monitor"  // Live Poller & Controller (tick -> gauges/table -> row actions)
	LayoutExplorer Layout = "explorer" // Query -> Results -> Detail
	LayoutSuite    Layout = "suite"    // Multi-Tab Subcommand Manager
)

// Deprecated: Archetype is the former name of Layout. It remains so saved
// specs and callers that still say Archetype keep compiling. JSON specs
// keep the key "archetype" (and also accept "layout").
type Archetype = Layout

// Deprecated names for the six layouts. Prefer the Layout* constants.
const (
	ArchetypeConfig   = LayoutConfig
	ArchetypeRunner   = LayoutRunner
	ArchetypeScanner  = LayoutScanner
	ArchetypeMonitor  = LayoutMonitor
	ArchetypeExplorer = LayoutExplorer
	ArchetypeSuite    = LayoutSuite
)

// AllLayouts returns the 6 base layouts in canonical order.
func AllLayouts() []Layout {
	return []Layout{
		LayoutConfig,
		LayoutRunner,
		LayoutScanner,
		LayoutMonitor,
		LayoutExplorer,
		LayoutSuite,
	}
}

// AllArchetypes is the former name of AllLayouts.
func AllArchetypes() []Layout { return AllLayouts() }

// MixinName identifies a composable capability mixin (an add-on).
type MixinName string

const (
	MixinFileOutput        MixinName = "WithFileOutput"
	MixinTableOutput       MixinName = "WithTableOutput"
	MixinCollapsibleOutput MixinName = "WithCollapsibleOutput"
	MixinProfiles          MixinName = "WithProfiles"
	MixinDynamicChoices    MixinName = "WithDynamicChoices"
	MixinRootAuth          MixinName = "WithRootAuth"
	MixinConfirm           MixinName = "WithConfirm"
	MixinHistory           MixinName = "WithHistory"
)

// Mixins describes which of the 8 original composable mixins are enabled
// for a spec. Archetype features set these; later add-ons are switched on
// by their own ToolSpec field and are listed by ToolSpec.EnabledMixinNames.
type Mixins struct {
	WithFileOutput        bool `json:"with_file_output,omitempty"`
	WithTableOutput       bool `json:"with_table_output,omitempty"`
	WithCollapsibleOutput bool `json:"with_collapsible_output,omitempty"`
	WithProfiles          bool `json:"with_profiles,omitempty"`
	WithDynamicChoices    bool `json:"with_dynamic_choices,omitempty"`
	WithRootAuth          bool `json:"with_root_auth,omitempty"`
	WithConfirm           bool `json:"with_confirm,omitempty"`
	WithHistory           bool `json:"with_history,omitempty"`
}

// Names returns the list of enabled mixin identifiers in canonical order.
func (m Mixins) Names() []MixinName {
	var out []MixinName
	if m.WithFileOutput {
		out = append(out, MixinFileOutput)
	}
	if m.WithTableOutput {
		out = append(out, MixinTableOutput)
	}
	if m.WithCollapsibleOutput {
		out = append(out, MixinCollapsibleOutput)
	}
	if m.WithProfiles {
		out = append(out, MixinProfiles)
	}
	if m.WithDynamicChoices {
		out = append(out, MixinDynamicChoices)
	}
	if m.WithRootAuth {
		out = append(out, MixinRootAuth)
	}
	if m.WithConfirm {
		out = append(out, MixinConfirm)
	}
	if m.WithHistory {
		out = append(out, MixinHistory)
	}
	return out
}
