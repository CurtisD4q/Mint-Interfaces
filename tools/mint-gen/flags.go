package main

import "flag"

// mintOpts is every option mint-gen accepts. newMintOpts registers them
// on one flag set. The manual is built from that set, so a flag is
// documented by being added here.
type mintOpts struct {
	fs             *flag.FlagSet
	checkTarget    string
	explainTarget  string
	archetypeFlag  string
	themeFlag      string
	listArchetypes bool
	listApps       bool
	force          bool
	buildBin       bool
	runApp         bool
	removeApp      bool
	purgeApps      bool
	rootFlag       bool
	noRootFlag     bool
	outDir         string
	repoRoot       string
	tuiMode        bool
	probeTerm      bool
	install        bool
	system         bool
	man            bool
}

func newMintOpts() *mintOpts {
	o := &mintOpts{}
	fs := flag.NewFlagSet("mint-gen", flag.ContinueOnError)
	fs.BoolVar(&o.install, "install", false, "Install the mint-gen launcher for you, in ~/.local/bin/mint-interfaces/mint-gen, and this manual in ~/.local/share/man/man1 (with --system: print the commands for everyone)")
	fs.BoolVar(&o.system, "system", false, "With --install: print the commands that install Mint-Interfaces for everyone on this computer")
	fs.StringVar(&o.checkTarget, "check", "", "Check whether a program can run inside a pure Mint TUI")
	fs.StringVar(&o.explainTarget, "explain", "", "Explain what a program reads and writes, and why, as mint-gen sees it")
	fs.StringVar(&o.archetypeFlag, "archetype", "", "Choose the archetype yourself, or several separated by commas, the first being the main one (e.g. probe, \"look up\", toolkit,records; see --list-archetypes)")
	fs.BoolVar(&o.listArchetypes, "list-archetypes", false, "List the 38 archetypes and the add-ons")
	fs.BoolVar(&o.listApps, "list", false, "List the apps in <repo>/apps")
	fs.BoolVar(&o.probeTerm, "probe-term", false, "Probe current terminal capabilities and refresh ~/.config/mint-interfaces/termcaps.json")
	fs.BoolVar(&o.force, "force", false, "Generate interface even if the TUI compatibility check fails")
	fs.BoolVar(&o.buildBin, "build", false, "Compile static Go binary (<tool>-mint) after scaffolding")
	fs.BoolVar(&o.runApp, "run", false, "Launch the compiled <tool>-mint interface (builds it first if missing)")
	fs.BoolVar(&o.removeApp, "remove", false, "Remove the scaffolded apps/<tool> directory and compiled <tool>-mint binary")
	fs.BoolVar(&o.purgeApps, "purge", false, "Remove every app in <repo>/apps that mint-gen created, after asking to confirm (y/n)")
	fs.BoolVar(&o.rootFlag, "sudo", false, "With --run: start the app with sudo without asking (sudo asks for the password)")
	fs.BoolVar(&o.noRootFlag, "no-sudo", false, "With --run: start the app without sudo, without asking")
	fs.StringVar(&o.themeFlag, "theme", "", "Set the colour theme for every Mint app (saved in ~/.config/mint-interfaces/theme.conf): default, terminal, nord, gruvbox, dracula, catppuccin or custom")
	fs.StringVar(&o.outDir, "out", "", "Output directory (default: <repo>/apps/<tool>)")
	fs.StringVar(&o.repoRoot, "root", "", "Mint-Interfaces repository root (auto-detected if empty)")
	fs.BoolVar(&o.tuiMode, "tui", false, "Launch interactive Mint TUI generator")
	fs.BoolVar(&o.man, "man", false, "Write the Mint-Interfaces manual (roff, section 1) to standard output")
	o.fs = fs
	return o
}
