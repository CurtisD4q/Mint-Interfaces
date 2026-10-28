// Starter for the Search archetype: a pattern in, matches out.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "rg",
		Title:             "◈ Ripgrep",
		Desc:              "Recursively search directories for a regex pattern",
		ArchetypeID:       "search",
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
