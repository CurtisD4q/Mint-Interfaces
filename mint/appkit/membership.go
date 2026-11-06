package appkit

// A program can fit more than one archetype. When the evidence cannot
// settle between two or three of them (a close call), the program belongs
// to each: find is a Search and a Query, sqlite3 a Session and a Query.
// ToolSpec.Archetypes lists them, the main one first; ArchetypeID is that
// main one, so specs and code that know one archetype keep working, and a
// spec with only ArchetypeID belongs to that one alone.
//
// Nothing is blended or weighted. The program is fully each of its
// archetypes:
//
//   - Features: every archetype's features are on (the union of their
//     blueprints).
//   - Behaviours an archetype brings (a state editor, findings, a preview of
//     changes, commands, the follow, service and session screens) all apply
//     (IsA).
//   - Data sensitivity is the highest of them, so a second archetype can
//     raise the data level, never lower it.
//   - Wording (titles, labels, the target box) is the main archetype's.
//   - The interface type is chosen from the answers, with all the
//     archetypes' features as its tie-breaking hints (see FactsFromSpec).

// MaxArchetypes is the most archetypes one program belongs to.
const MaxArchetypes = 3

// NormalizeArchetypes drops unknown archetypes and repeats, keeps the order
// (the main one first), and keeps at most MaxArchetypes.
func NormalizeArchetypes(ids []ArchetypeID) []ArchetypeID {
	var out []ArchetypeID
	seen := map[ArchetypeID]bool{}
	for _, id := range ids {
		def, ok := LookupArchetype(string(id))
		if !ok || seen[def.ID] {
			continue
		}
		seen[def.ID] = true
		out = append(out, def.ID)
		if len(out) == MaxArchetypes {
			break
		}
	}
	return out
}

// ArchetypeList is the archetypes the spec belongs to, the main one first:
// its Archetypes, else its ArchetypeID alone, else none.
func (s ToolSpec) ArchetypeList() []ArchetypeID {
	if ids := NormalizeArchetypes(s.Archetypes); len(ids) > 0 {
		return ids
	}
	if def, ok := LookupArchetype(s.ArchetypeID); ok {
		return []ArchetypeID{def.ID}
	}
	return nil
}

// IsA reports whether the program belongs to archetype id.
func (s ToolSpec) IsA(id ArchetypeID) bool {
	for _, x := range s.ArchetypeList() {
		if x == id {
			return true
		}
	}
	return false
}

// Members is the archetypes the program belongs to, the main one first.
func (s ToolSpec) Members() []ArchetypeDef {
	var out []ArchetypeDef
	for _, id := range s.ArchetypeList() {
		if def, ok := LookupArchetype(string(id)); ok {
			out = append(out, def)
		}
	}
	return out
}

// preferencesOf is each feature at the strongest any of the archetypes
// wants it, so every archetype's own features stay on.
func preferencesOf(ids []ArchetypeID) Preferences {
	out := Preferences{Features: map[Feature]float64{}}
	for _, id := range NormalizeArchetypes(ids) {
		def, _ := LookupArchetype(string(id))
		for f, v := range def.Prefer.Features {
			if v > out.Features[f] {
				out.Features[f] = v
			}
		}
	}
	return out
}

// Preferences is what the program's archetypes want, each feature at the
// strongest any of them wants it.
func (s ToolSpec) Preferences() Preferences { return preferencesOf(s.ArchetypeList()) }

// Blueprint is every feature any of the program's archetypes switches on.
func (s ToolSpec) Blueprint() Blueprint { return BlueprintFrom(s.Preferences()) }

// combinedDef is one archetype standing for all of ids: the main one's
// wording and defaults, every archetype's features, and the first target
// label (or poll interval) any of them has when the main one has none.
func combinedDef(ids []ArchetypeID) (ArchetypeDef, bool) {
	ids = NormalizeArchetypes(ids)
	if len(ids) == 0 {
		return ArchetypeDef{}, false
	}
	def, _ := LookupArchetype(string(ids[0]))
	if len(ids) == 1 {
		return def, true
	}
	def.Prefer = preferencesOf(ids)
	for _, id := range ids[1:] {
		other, _ := LookupArchetype(string(id))
		if def.TargetLabel == "" && other.TargetLabel != "" {
			def.TargetLabel, def.TargetPlaceholder = other.TargetLabel, other.TargetPlaceholder
		}
		if def.SecondTargetLabel == "" && other.SecondTargetLabel != "" {
			def.SecondTargetLabel, def.SecondTargetPlaceholder = other.SecondTargetLabel, other.SecondTargetPlaceholder
		}
		if def.PollInterval <= 0 && other.PollInterval > 0 {
			def.PollInterval = other.PollInterval
		}
	}
	return def, true
}

// ApplyArchetypes sets spec up as all of ids, the way ApplyTo does for one
// archetype, and records them.
func ApplyArchetypes(spec *ToolSpec, ids []ArchetypeID) {
	if spec == nil {
		return
	}
	ids = NormalizeArchetypes(ids)
	def, ok := combinedDef(ids)
	if !ok {
		return
	}
	def.ApplyTo(spec)
	spec.ArchetypeID = string(ids[0])
	if len(ids) > 1 {
		spec.Archetypes = ids
	} else {
		spec.Archetypes = nil
	}
}

// WantsSubcommands reports whether any of the program's archetypes is a set
// of commands, so a generator should read them from the help.
func (s ToolSpec) WantsSubcommands() bool {
	for _, def := range s.Members() {
		if def.WantsSubcommands() {
			return true
		}
	}
	return false
}
