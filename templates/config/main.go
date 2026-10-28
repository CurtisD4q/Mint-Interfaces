// Starter for the config layout (appkit.ArchetypeConfig), used by the Settings, Device and Rules archetypes.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "sysctl",
		Title:             "◈ Sysctl",
		Desc:              "Configure kernel parameters at runtime",
		ArchetypeID:       "settings", // any archetype on the config layout: see mint-gen --list-archetypes
		Archetype:         appkit.ArchetypeConfig,
		TargetLabel:       "Parameter / key=value",
		TargetPlaceholder: "net.ipv4.ip_forward=1",
		NeedsRoot:         true,
		TableOutput:       true,
		Flags: []appkit.FlagSpec{
			{Flag: "-a", Kind: appkit.FlagBool, Section: "Query", Label: "Display all values currently available (-a)"},
			{Flag: "-w", Kind: appkit.FlagBool, Section: "Modify", Label: "Write / change a sysctl setting (-w)"},
		},
	})
}
