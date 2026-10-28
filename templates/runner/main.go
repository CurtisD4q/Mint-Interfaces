// Starter for the runner layout (appkit.LayoutRunner), used by the Audit, Build, Convert, Edit files, Filter, Generate, Measure, Mirror, Present, Serve, Session, Transfer, Wrap and General archetypes.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "curl",
		Title:             "◈ Curl",
		Desc:              "Transfer a URL with live progress and file output",
		ArchetypeID:       "transfer", // any archetype on the runner layout: see mint-gen --list-archetypes
		Archetype:         appkit.LayoutRunner,
		TargetLabel:       "Target URL",
		TargetPlaceholder: "https://example.com/file.tar.gz",
		OutputsFile:       true,
		OutputFlag:        "-o",
		DefaultExt:        "dat",
		KeepHistory:       true,
		Flags: []appkit.FlagSpec{
			{Flag: "-L", Kind: appkit.FlagBool, Section: "Transfer", Label: "Follow redirects (-L)", Default: "true"},
			{Flag: "-I", Kind: appkit.FlagBool, Section: "Transfer", Label: "Fetch headers only (-I)"},
			{Flag: "-v", Kind: appkit.FlagBool, Section: "Debug", Label: "Verbose output (-v)", Advanced: true},
		},
	})
}
