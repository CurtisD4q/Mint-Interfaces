// Starter for the Transfer archetype: a URL in, a file out, with progress.
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "curl",
		Title:             "◈ Curl",
		Desc:              "Transfer a URL with live progress and file output",
		ArchetypeID:       "transfer",
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
