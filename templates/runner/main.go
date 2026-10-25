// Starter for ArchetypeRunner (T05..T07, T10..T11, T15..T16: Batch, Media, Download, Build, Benchmark, Audit, Crypto).
package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(appkit.ToolSpec{
		Name:              "curl",
		Title:             "◈ Curl",
		Desc:              "Transfer a URL with live progress and file output",
		Template:          "T07", // T05..T07, T10..T11, T15..T16
		Archetype:         appkit.ArchetypeRunner,
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
