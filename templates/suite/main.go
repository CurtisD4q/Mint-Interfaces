// Starter for the suite layout (appkit.ArchetypeSuite), used by the Pack, History, Records, Schedule and Toolkit archetypes.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "git",
		Title:             "◈ Git",
		Desc:              "Distributed version control system suite",
		ArchetypeID:       "history", // any archetype on the suite layout: see mint-gen --list-archetypes
		Archetype:         appkit.ArchetypeSuite,
		TargetLabel:       "Path / ref / arguments",
		TargetPlaceholder: "main",
		TableOutput:       true,
		CollapsibleOutput: true,
		Flags: []appkit.FlagSpec{
			{Flag: "-v", Kind: appkit.FlagBool, Section: "General", Label: "Be verbose (-v)"},
		},
	})
}
