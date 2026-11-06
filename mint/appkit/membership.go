package appkit

import (
	"sort"
)

// A program can belong to more than one archetype: tar is mostly Pack and
// partly Convert, docker is a Toolkit with a Records side. ToolSpec.Archetypes
// lists them with weights, strongest first; ArchetypeID is the first one
// (the primary), so specs and code that know only one archetype keep
// working, and a spec with only ArchetypeID belongs to that one at weight 1.
//
// What each part of an app takes from the memberships:
//
//   - Features (the blueprint) are the weighted blend of every member's
//     preferences, so a strong secondary switches on what it needs.
//   - Behaviours an archetype brings (a state editor, findings, a preview of
//     changes, subcommands) apply when the program is a member at
//     memberMin or more (IsA).
//   - Data sensitivity is the highest of all members, whatever their
//     weight: a secondary archetype can raise the data level, never lower it.
//   - Wording (titles, labels, the target box) is the primary's.
//   - The interface type is chosen from the answers, with the blend as its
//     tie-breaking hints (see FactsFromSpec).

// Membership is one archetype a program belongs to, and how much.
type Membership struct {
	ID     ArchetypeID `json:"id"`
	Weight float64     `json:"weight"`
}

// MaxMemberships is the most archetypes one program belongs to.
const MaxMemberships = 3

// memberMin is the weight at which a membership brings its archetype's
// behaviours (IsA). A weaker one still counts in the feature blend and in
// the data level.
const memberMin = 0.25

// NormalizeMemberships drops unknown archetypes, repeats and weights of
// zero or less, keeps the MaxMemberships strongest (the first listed wins a
// tie, so the primary stays first), and scales the weights to sum to 1.
func NormalizeMemberships(ms []Membership) []Membership {
	var out []Membership
	seen := map[ArchetypeID]bool{}
	for _, m := range ms {
		def, ok := LookupArchetype(string(m.ID))
		if !ok || m.Weight <= 0 || seen[def.ID] {
			continue
		}
		seen[def.ID] = true
		out = append(out, Membership{ID: def.ID, Weight: m.Weight})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Weight > out[j].Weight })
	if len(out) > MaxMemberships {
		out = out[:MaxMemberships]
	}
	sum := 0.0
	for _, m := range out {
		sum += m.Weight
	}
	for i := range out {
		out[i].Weight /= sum
	}
	return out
}

// Memberships is the archetypes the spec belongs to, strongest first,
// weights summing to 1: its Archetypes, else its ArchetypeID alone, else
// none.
func (s ToolSpec) Memberships() []Membership {
	if ms := NormalizeMemberships(s.Archetypes); len(ms) > 0 {
		return ms
	}
	if def, ok := LookupArchetype(s.ArchetypeID); ok {
		return []Membership{{ID: def.ID, Weight: 1}}
	}
	return nil
}

// IsA reports whether the program belongs to archetype id at memberMin or
// more, or has it as its primary.
func (s ToolSpec) IsA(id ArchetypeID) bool {
	for i, m := range s.Memberships() {
		if m.ID == id && (i == 0 || m.Weight >= memberMin) {
			return true
		}
	}
	return false
}

// Members lists the archetypes the program IsA, strongest first.
func (s ToolSpec) Members() []ArchetypeDef {
	var out []ArchetypeDef
	for _, m := range s.Memberships() {
		if s.IsA(m.ID) {
			if def, ok := LookupArchetype(string(m.ID)); ok {
				out = append(out, def)
			}
		}
	}
	return out
}

// BlendMemberships is the weighted blend of the members' preferences.
func BlendMemberships(ms []Membership) Preferences {
	out := Preferences{Features: map[Feature]float64{}}
	for _, m := range NormalizeMemberships(ms) {
		def, _ := LookupArchetype(string(m.ID))
		for f, v := range def.Prefer.Features {
			out.Features[f] += m.Weight * v
		}
	}
	for f, v := range out.Features {
		if v <= 0 {
			delete(out.Features, f)
		}
	}
	return out
}

// Preferences is the spec's preferences: the blend of its archetypes, and
// never less than the primary's own, so belonging to more archetypes only
// ever adds features (a 40% Follow program keeps its live log).
func (s ToolSpec) Preferences() Preferences {
	ms := s.Memberships()
	out := BlendMemberships(ms)
	if len(ms) > 0 {
		primary, _ := LookupArchetype(string(ms[0].ID))
		for f, v := range primary.Prefer.Features {
			if v > out.Features[f] {
				out.Features[f] = v
			}
		}
	}
	return out
}

// Blueprint is the features the spec's archetypes switch on, together.
func (s ToolSpec) Blueprint() Blueprint { return BlueprintFrom(s.Preferences()) }

// blendedDef is an archetype that stands for all of a spec's memberships:
// the primary's wording and defaults, with the blended preferences, and the
// first target label any member has when the primary has none.
func blendedDef(ms []Membership) (ArchetypeDef, bool) {
	ms = NormalizeMemberships(ms)
	if len(ms) == 0 {
		return ArchetypeDef{}, false
	}
	def, _ := LookupArchetype(string(ms[0].ID))
	if len(ms) == 1 {
		return def, true
	}
	def.Prefer = (ToolSpec{Archetypes: ms}).Preferences()
	for _, m := range ms[1:] {
		other, _ := LookupArchetype(string(m.ID))
		if def.TargetLabel == "" && other.TargetLabel != "" && m.Weight >= memberMin {
			def.TargetLabel, def.TargetPlaceholder = other.TargetLabel, other.TargetPlaceholder
		}
		if def.SecondTargetLabel == "" && other.SecondTargetLabel != "" && m.Weight >= memberMin {
			def.SecondTargetLabel, def.SecondTargetPlaceholder = other.SecondTargetLabel, other.SecondTargetPlaceholder
		}
		if def.PollInterval <= 0 && other.PollInterval > 0 && m.Weight >= memberMin {
			def.PollInterval = other.PollInterval
		}
	}
	return def, true
}

// ApplyMemberships sets spec up as all of ms together, the way ApplyTo
// does for one archetype, and records them.
func ApplyMemberships(spec *ToolSpec, ms []Membership) {
	if spec == nil {
		return
	}
	ms = NormalizeMemberships(ms)
	def, ok := blendedDef(ms)
	if !ok {
		return
	}
	def.ApplyTo(spec)
	spec.ArchetypeID = string(ms[0].ID)
	if len(ms) > 1 {
		spec.Archetypes = ms
	} else {
		spec.Archetypes = nil
	}
}

// WantsSubcommands reports whether any archetype the program IsA is a set
// of commands, so a generator should read them from the help.
func (s ToolSpec) WantsSubcommands() bool {
	for _, def := range s.Members() {
		if def.WantsSubcommands() {
			return true
		}
	}
	return false
}
