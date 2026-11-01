package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// mintGuide is the one guide for mint-gen and for the apps it builds.
// mint-gen --help prints it, then the flag list.
const mintGuide = `Mint-Interfaces

mint-gen --help is the guide for building an app and for using one.

Using mint-gen

mint-gen reads a program's manual and builds a terminal app for it.

Scaffold an app:

  mint-gen ps

The files land in apps/ps/. Compile the app at the same time with --build:

  mint-gen --build ps

Run an app directly. If the binary is not there yet, mint-gen builds it first:

  mint-gen --run ps

Other flags you will want: --check (will this program work in the app?), --explain (what mint-gen understood), --list (apps already built), --list-archetypes (the kinds of program), --archetype (pick a kind yourself), --theme (the colours the app starts in), --remove (delete an app), and --out (choose the folder). Every flag is listed at the bottom.

Using a Mint app

The app opens on a home menu. Up and down move through the items, Enter opens one, b goes back, and q quits.

The main screen is where you set the command up. Choose the target, switch flags on or off, and fill in values. Then run the command from that screen.

Some screens stack into panes. Tab and Shift-Tab move between panes. [ shrinks the focused pane and ] grows it. | hides the focused pane, and | again shows it. Click a pane to focus it.

On a wide table, ← and → scroll sideways. H narrows the column you are on and L widens it.

Presets stores the options you have set, so you can bring them back. Run History is the list of earlier runs and the output they kept.

The Manual item on the home menu opens the program's full manual. --man opens that same screen from the command line (for example, ps-mint --man). The app's own --help is only a short usage list. This guide stays here.
`

// helpRequested reports whether mint-gen itself was asked for --help.
// A --help that belongs to a program being run (mint-gen --run ps --help)
// is left for that app.
func helpRequested(ordered []string) bool {
	for _, a := range ordered {
		if a == "--" || a == "-" || !strings.HasPrefix(a, "-") {
			return false
		}
		switch a {
		case "-h", "--help", "-help":
			return true
		}
	}
	return false
}

// writeMintGuide prints the guide and then the flag list.
func writeMintGuide(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, mintGuide)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	if fs == nil {
		return
	}
	fs.SetOutput(w)
	fs.PrintDefaults()
}
