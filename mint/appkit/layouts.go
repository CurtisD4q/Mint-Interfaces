package appkit

// Archetype identifies one of the 6 base TUI interaction patterns.
type Archetype string

const (
	ArchetypeConfig   Archetype = "config"   // State Editor (read -> tweak -> apply)
	ArchetypeRunner   Archetype = "runner"   // Task & Pipeline (input + flags -> progress -> output)
	ArchetypeScanner  Archetype = "scanner"  // Target -> Live Table + Log
	ArchetypeMonitor  Archetype = "monitor"  // Live Poller & Controller (tick -> gauges/table -> row actions)
	ArchetypeExplorer Archetype = "explorer" // Query -> Results -> Detail
	ArchetypeSuite    Archetype = "suite"    // Multi-Tab Subcommand Manager
)

// AllArchetypes returns the 6 base archetypes in canonical order.
func AllArchetypes() []Archetype {
	return []Archetype{
		ArchetypeConfig,
		ArchetypeRunner,
		ArchetypeScanner,
		ArchetypeMonitor,
		ArchetypeExplorer,
		ArchetypeSuite,
	}
}

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
// for a spec. Archetypes set these; later add-ons are switched on by their
// own ToolSpec field and are listed by ToolSpec.EnabledMixinNames.
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
