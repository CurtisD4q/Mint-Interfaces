// Starter for ArchetypeExplorer (T24..T27: Disk Inspectors, File/Content Searchers, Data Query Explorers, Doc Browsers).
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "rg",
		Title:             "◈ Ripgrep",
		Desc:              "Recursively search directories for a regex pattern",
		Template:          "T25", // T24..T27
		Archetype:         appkit.ArchetypeExplorer,
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
