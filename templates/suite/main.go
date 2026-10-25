// Starter for ArchetypeSuite (T08..T09, T21..T23, T28: Archive, Backup, VCS, Container, Package, and Schedule Suites).
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "git",
		Title:             "◈ Git",
		Desc:              "Distributed version control system suite",
		Template:          "T21", // T08, T09, T21, T22, T23, T28
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
