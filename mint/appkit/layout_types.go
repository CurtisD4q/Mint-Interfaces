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
			OnActivate: func(*mint.Ctx) { a.useSubcommand(name) },
		})
	}
	return []mint.Section{{Name: "Commands", Rows: rows}}
}

// Live monitor: a program that keeps running and shows rows or values. The
// options are a narrow column on the left; the live values take the rest of
// the width beside them, and the controls (poll now, pause, interval) stay
// docked under both.

const titleLive = "Live values"

// monitorLayout reports whether the app draws as a live monitor. A monitor
// that cannot read its state (so is not polled) keeps the classic screen.
func (a *ToolApp) monitorLayout(mode execMode) bool {
	return mode == execPoll && a.Spec.CanReadState() && a.usesType(TypeLiveMonitor)
}

// HasOwnLayout reports whether interface type t has a layout of its own;
// the other types draw the classic screen.
func HasOwnLayout(t InterfaceTypeID) bool {
	switch t {
	case TypeCommandSuite, TypeLiveMonitor:
		return true
	}
	return false
}

// SpecHasOwnLayout reports whether an app built from spec draws its
// interface type's own layout, not the classic screen: the type has one,
// and the app has what that layout needs (commands for a suite, a state it
// can read for a monitor).
func SpecHasOwnLayout(spec ToolSpec) bool {
	switch PlanFor(FactsFromSpec(spec)).Type {
	case TypeCommandSuite:
		return len(spec.Subcommands) > 0
	case TypeLiveMonitor:
		// As executionMode: polled only when it can read its state and
		// has an interval or wants live refresh, and never full-screen.
		return !spec.FullScreen && spec.CanReadState() && (spec.PollInterval > 0 || spec.Blueprint().Has(FeatLiveRefresh))
	}
	return false
}
