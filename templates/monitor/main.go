// Starter for the Control archetype: a live list of running things, with an action on each.
package main

import (
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "systemctl",
		Title:             "◈ Systemctl",
		Desc:              "Control the systemd system and service manager",
		ArchetypeID:       "control",
		TargetLabel:       "Service / unit pattern",
		TargetPlaceholder: "nginx.service",
		PollInterval:      3 * time.Second,
		TableOutput:       true,
		NeedsRoot:         true,
		ConfirmRun:        true,
		Flags: []appkit.FlagSpec{
			{Flag: "--all", Kind: appkit.FlagBool, Section: "Filter", Label: "Show all loaded units (--all)"},
			{Flag: "--failed", Kind: appkit.FlagBool, Section: "Filter", Label: "Show only failed units (--failed)"},
		},
	})
}
