package main

import (
	"strconv"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// attachParsedScreens gives an unlisted follow, serve, or session tool its
// interactive screen when the help actually named what that screen needs.
// A tool Mint already configured (Follow, Service, or Transcript set) is
// left alone.
func attachParsedScreens(spec *appkit.ToolSpec, help string) {
	if spec == nil || spec.ArchetypeID == "" {
		return
	}
	def, ok := appkit.LookupArchetype(spec.ArchetypeID)
	if !ok {
		return
	}
	b := def.Blueprint()
	if spec.Follow == nil && (spec.ArchetypeID == "follow" || b.Has(appkit.FeatFollow)) {
		if flag := parsedFollowFlag(spec.Flags); flag != "" {
			spec.Follow = &appkit.FollowSpec{Flag: flag}
		}
	}
	if spec.Service == nil && spec.ArchetypeID == "serve" {
		if flag, port := parsedListenFlag(spec.Flags); flag != "" {
			if port == 0 {
				port = 8080
			}
			spec.Service = &appkit.ServiceSpec{
				Adapter:     appkit.ServiceGeneric,
				PortFlag:    flag,
				DefaultPort: port,
			}
		}
	}
	if spec.Transcript == nil && spec.ArchetypeID == "session" && sessionCuesMatch(help) {
		tr := appkit.TranscriptSpec{Adapter: "lines", QuitWords: []string{"quit", "exit"}}
		if known, ok := appkit.KnownTranscriptSpec(spec.BinaryName()); ok {
			tr.Adapter = known.Adapter
		}
		spec.Transcript = &tr
	}
}

// parsedFollowFlag is the -f or --follow option in flags, preferring the
// long form. Empty when the help did not list one.
func parsedFollowFlag(flags []appkit.FlagSpec) string {
	short := ""
	for _, f := range flags {
		name := flagName(f.Flag)
		switch name {
		case "--follow":
			return name
		case "-f":
			short = name
		}
	}
	return short
}

// parsedListenFlag is a port or listen option (-p, --port, --listen),
// preferring --port, then -p, then --listen. port is a numeric default
// written on that option, or 0.
func parsedListenFlag(flags []appkit.FlagSpec) (flag string, port int) {
	rank := map[string]int{"--port": 3, "-p": 2, "--listen": 1}
	best := 0
	for _, f := range flags {
		name := flagName(f.Flag)
		s := rank[name]
		if s <= best {
			continue
		}
		best = s
		flag = name
		port = 0
		if n, err := strconv.Atoi(strings.TrimSpace(f.Default)); err == nil && n > 0 && n <= 65535 {
			port = n
		}
	}
	return flag, port
}

// applyChosenArchetype makes def the spec's archetype: layout, target box,
// and the interactive screens that the help can support. A label this
// archetype itself applied is cleared first, so the new one can hide the
// target box; a label taken from the usage line is kept.
func applyChosenArchetype(spec *appkit.ToolSpec, a appkit.ArchetypeDef, help string) {
	if prev, ok := appkit.LookupArchetype(spec.ArchetypeID); ok && prev.ID != a.ID {
		if spec.TargetLabel == prev.TargetLabel {
			spec.TargetLabel = ""
		}
		if spec.TargetPlaceholder == prev.TargetPlaceholder {
			spec.TargetPlaceholder = ""
		}
		if spec.SecondTargetLabel == prev.SecondTargetLabel {
			spec.SecondTargetLabel = ""
		}
		if spec.SecondTargetPlaceholder == prev.SecondTargetPlaceholder {
			spec.SecondTargetPlaceholder = ""
		}
	}
	spec.TwoInputs = false
	spec.WantTarget = false
	spec.HideTarget = false
	clearParsedScreens(spec)
	a.ApplyTo(spec)
	if spec.TargetLabel == "" && !spec.HideTarget {
		spec.TargetLabel = "Target / arguments"
		if spec.TargetPlaceholder == "" {
			spec.TargetPlaceholder = "arguments"
		}
	}
	attachParsedScreens(spec, help)
}

// clearParsedScreens drops follow, service, and transcript values that
// attachParsedScreens added, so a later --archetype choice can attach its
// own. Specs Mint already knew (follow args, a named service adapter, a
// transcript without quit words) stay.
func clearParsedScreens(spec *appkit.ToolSpec) {
	if spec.Follow != nil && spec.Follow.Flag != "" && len(spec.Follow.FollowArgs) == 0 && spec.Follow.Format == "" {
		spec.Follow = nil
	}
	if spec.Service != nil && spec.Service.Adapter == appkit.ServiceGeneric && spec.Service.BindFlag == "" && spec.Service.DirFlag == "" {
		spec.Service = nil
	}
	if spec.Transcript != nil && len(spec.Transcript.QuitWords) > 0 {
		spec.Transcript = nil
	}
}

func flagName(flag string) string {
	if i := strings.IndexByte(flag, '='); i >= 0 {
		return flag[:i]
	}
	return flag
}
