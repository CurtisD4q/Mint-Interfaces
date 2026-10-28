// Starter for the History archetype: one screen, a subcommand for each action.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "git",
		Title:             "◈ Git",
		Desc:              "Distributed version control system suite",
		ArchetypeID:       "history",
		TargetLabel:       "Path / ref / arguments",
		TargetPlaceholder: "main",
		TableOutput:       true,
		CollapsibleOutput: true,
		Subcommands: []appkit.SubcommandSpec{
			{Name: "status", Label: "Status", Desc: "Show the working tree", Args: []string{"status"}, ReadOnly: true},
			{Name: "log", Label: "Log", Desc: "Show recent commits", Args: []string{"log", "--oneline"}, ReadOnly: true},
		},
		Flags: []appkit.FlagSpec{
			{Flag: "-v", Kind: appkit.FlagBool, Section: "General", Label: "Be verbose (-v)"},
		},
	})
}
