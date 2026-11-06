package main

import (
	"fmt"
	"io"
	"os"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// interfaceTypeLine is the screen the app will have and why, from the
// spec's interface plan: "Command suite (command-suite), because it has
// commands of its own".
func interfaceTypeLine(spec appkit.ToolSpec) string {
	p := appkit.PlanFor(appkit.FactsFromSpec(spec))
	name := string(p.Type)
	if t, ok := appkit.LookupInterfaceType(p.Type); ok {
		name = fmt.Sprintf("%s (%s)", t.Name, t.ID)
	}
	line := name + ", because " + p.Why
	switch {
	case !appkit.HasOwnLayout(p.Type):
		line += "; drawn with the classic screen for now"
	case !appkit.SpecHasOwnLayout(spec):
		line += "; drawn with the classic screen, since this program has nothing for that layout to show yet"
	}
	if os.Getenv("MINT_CLASSIC_LAYOUT") == "1" {
		line += "; MINT_CLASSIC_LAYOUT=1 is set, so apps run now draw the classic screen"
	}
	return line
}

// printInterfaceType writes the build output's screen line.
func printInterfaceType(w io.Writer, spec appkit.ToolSpec, indent string) {
	fmt.Fprint(w, safeTerminalText(indent+"Screen:    "+interfaceTypeLine(spec))+"\n")
}
