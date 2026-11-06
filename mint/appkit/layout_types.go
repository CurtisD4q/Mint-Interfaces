package appkit

import (
	"os"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// Interface-type layouts.
//
// MainScreen builds the classic screen unless the app's interface plan
// (see InterfacePlan) names a type that has a layout of its own. The old
// screen is always there as the fallback: set MINT_CLASSIC_LAYOUT=1 to
// draw every app with it, and an app whose type has no layout yet gets it
// too. Each type moves over in its own change.

// classicLayout reports whether the classic screen is forced.
func classicLayout() bool { return os.Getenv("MINT_CLASSIC_LAYOUT") == "1" }

// usesType reports whether the app draws with the layout of interface
// type t: its plan says so, and the classic screen is not forced.
func (a *ToolApp) usesType(t InterfaceTypeID) bool {
	return !classicLayout() && a.InterfacePlan().Type == t
}

// Command suite: a program made of commands. The commands are a list on
// the left; the form for the selected one is beside it on the right, and
// the result is under both.

const (
	titleCommands = "Commands"
	titleOptions  = "Options"
)

// suiteLayout reports whether the app draws as a command suite.
func (a *ToolApp) suiteLayout() bool {
	return len(a.Spec.Subcommands) > 0 && a.usesType(TypeCommandSuite)
}

// suiteList is the list of commands. Enter (or a click) selects one; the
// selected command is marked, and the form beside the list follows it.
func (a *ToolApp) suiteList() []mint.Section {
	subs := a.Spec.Subcommands
	if a.ActiveSubcommand == "" && len(subs) > 0 {
		a.ActiveSubcommand = subs[0].Name
	}
	rows := make([]mint.Row, len(subs))
	for i := range subs {
		name := subs[i].Name
		rows[i] = mint.Action(mint.ActionOpts{
			ID:    "suite:command:" + name,
			Label: name,
			Get: func() string {
				if a.ActiveSubcommand == name {
					return "● selected"
				}
				return "select"
			},
			OnActivate: func(*mint.Ctx) { a.ActiveSubcommand = name },
		})
	}
	return []mint.Section{{Name: "Commands", Rows: rows}}
}
