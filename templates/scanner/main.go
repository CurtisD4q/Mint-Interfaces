// Starter for the scanner layout (appkit.LayoutScanner), used by the Probe and Record archetypes.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "nmap",
		Title:             "◈ Nmap",
		Desc:              "Network exploration and security / port scanner",
		ArchetypeID:       "probe", // any archetype on the scanner layout: see mint-gen --list-archetypes
		Archetype:         appkit.LayoutScanner,
		TargetLabel:       "Target host / CIDR / range",
		TargetPlaceholder: "scanme.nmap.org or 192.168.1.0/24",
		OutputsFile:       true,
		OutputFlag:        "-oN",
		DefaultExt:        "txt",
		TableOutput:       true,
		CollapsibleOutput: true,
		NeedsRoot:         true,
		KeepHistory:       true,
		Flags: []appkit.FlagSpec{
			{Flag: "-sV", Kind: appkit.FlagBool, Section: "Scan", Label: "Probe open ports to determine service/version (-sV)"},
			{Flag: "-F", Kind: appkit.FlagBool, Section: "Scan", Label: "Fast mode — scan fewer ports (-F)"},
			{Flag: "-Pn", Kind: appkit.FlagBool, Section: "Discovery", Label: "Treat all hosts as online (-Pn)"},
			{Flag: "-p", Kind: appkit.FlagText, Section: "Ports", Label: "Port ranges (-p)", Hint: "1-1024,8080"},
		},
	})
}
