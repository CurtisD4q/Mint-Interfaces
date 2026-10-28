package appkit

import (
	"fmt"
	"sort"
)

// A mixin (add-on) is registered here once, from the file that implements
// it, so adding one never means editing a shared list.
//
//	func init() {
//		registerMixin(MixinInfo{Name: MixinDiff, Order: 100, Summary: "…",
//			Enabled: func(s *ToolSpec) bool { return s.Diff != nil },
//			Validate: validateDiff})
//	}

// MixinInfo describes a registered mixin.
type MixinInfo struct {
	Name    MixinName
	Summary string // one line, for mint-gen --list-archetypes
	Fields  string // the ToolSpec fields that switch it on and configure it
	Order   int    // canonical position: the original 8 are 1..8

	// Enabled reports whether s uses this mixin.
	Enabled func(s *ToolSpec) bool
	// Validate checks this mixin's settings on s, when it is enabled. It
	// may be nil.
	Validate func(s *ToolSpec) error
}

var mixinRegistry []MixinInfo

// registerMixin adds a mixin. Registering a name twice, or two mixins at
// the same Order, panics: both are programming errors caught by any test.
func registerMixin(m MixinInfo) {
	if m.Name == "" || m.Enabled == nil {
		panic("appkit: mixin needs a name and an Enabled func")
	}
	for _, have := range mixinRegistry {
		if have.Name == m.Name {
			panic(fmt.Sprintf("appkit: mixin %s registered twice", m.Name))
		}
		if have.Order == m.Order {
			panic(fmt.Sprintf("appkit: mixins %s and %s share order %d", have.Name, m.Name, m.Order))
		}
	}
	mixinRegistry = append(mixinRegistry, m)
	sort.SliceStable(mixinRegistry, func(i, j int) bool { return mixinRegistry[i].Order < mixinRegistry[j].Order })
}

// MixinInfos returns every registered mixin in canonical order.
func MixinInfos() []MixinInfo {
	return append([]MixinInfo(nil), mixinRegistry...)
}

// AllMixins returns every registered mixin name in canonical order.
func AllMixins() []MixinName {
	out := make([]MixinName, len(mixinRegistry))
	for i, m := range mixinRegistry {
		out[i] = m.Name
	}
	return out
}

// EnabledMixinNames returns the mixins s uses, in canonical order,
// including add-ons beyond the original 8 that Mixins describes.
func (s ToolSpec) EnabledMixinNames() []MixinName {
	var out []MixinName
	for _, m := range mixinRegistry {
		if m.Enabled(&s) {
			out = append(out, m.Name)
		}
	}
	return out
}

// Validate checks the settings of every mixin s uses, and returns the
// first problem found. mint-gen runs it before writing spec.go.
func (s ToolSpec) Validate() error {
	for _, m := range mixinRegistry {
		if m.Validate == nil || !m.Enabled(&s) {
			continue
		}
		if err := m.Validate(&s); err != nil {
			return fmt.Errorf("%s: %w", m.Name, err)
		}
	}
	return nil
}

func init() {
	// The original 8, in their original order.
	for i, m := range []MixinInfo{
		{Name: MixinFileOutput, Fields: "OutputsFile, OutputFlag, DefaultExt",
			Summary: "save results to a file", Enabled: func(s *ToolSpec) bool { return s.OutputsFile }},
		{Name: MixinTableOutput, Fields: "TableOutput, TableColumns",
			Summary: "parse output into a table", Enabled: func(s *ToolSpec) bool { return s.TableOutput }},
		{Name: MixinCollapsibleOutput, Fields: "CollapsibleOutput, ChunkSize",
			Summary: "fold long output into sections", Enabled: func(s *ToolSpec) bool { return s.CollapsibleOutput }},
		{Name: MixinProfiles, Fields: "Profiles",
			Summary: "named sets of option values", Enabled: func(s *ToolSpec) bool { return len(s.Profiles) > 0 }},
		{Name: MixinDynamicChoices, Fields: "FlagSpec.ChoicesCmd",
			Summary: "option choices read from the machine", Enabled: func(s *ToolSpec) bool {
				for _, f := range s.Flags {
					if len(f.ChoicesCmd) > 0 {
						return true
					}
				}
				return false
			}},
		{Name: MixinRootAuth, Fields: "NeedsRoot",
			Summary: "explain how to run as root (never asks for a password)", Enabled: func(s *ToolSpec) bool { return s.NeedsRoot }},
		{Name: MixinConfirm, Fields: "ConfirmRun, ConfirmPrompt",
			Summary: "confirm before running", Enabled: func(s *ToolSpec) bool { return s.ConfirmRun }},
		{Name: MixinHistory, Fields: "KeepHistory",
			Summary: "keep a list of past runs", Enabled: func(s *ToolSpec) bool { return s.KeepHistory }},
	} {
		m.Order = i + 1
		registerMixin(m)
	}
}
