// Starter for the explorer layout (appkit.LayoutExplorer), used by the Ask, Look up, Inspect, List, Search, Query, Compare and Read archetypes.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "rg",
		Title:             "◈ Ripgrep",
		Desc:              "Recursively search directories for a regex pattern",
		ArchetypeID:       "search", // any archetype on the explorer layout: see mint-gen --list-archetypes
		Archetype:         appkit.LayoutExplorer,
		TargetLabel:       "Search pattern & path",
		TargetPlaceholder: "TODO ./mint",
		OutputsFile:       true,
		DefaultExt:        "txt",
		TableOutput:       true,
		Flags: []appkit.FlagSpec{
			{Flag: "-i", Kind: appkit.FlagBool, Section: "Search", Label: "Case-insensitive search (-i)"},
			{Flag: "-l", Kind: appkit.FlagBool, Section: "Output", Label: "Only print filenames with matches (-l)"},
			{Flag: "-g", Kind: appkit.FlagText, Section: "Filter", Label: "Include/exclude glob (-g)", Hint: "*.go"},
		},
	})
}
