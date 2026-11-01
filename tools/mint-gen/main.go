package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

func main() {
	os.Exit(runCLI(os.Args[1:]))
}

func runCLI(args []string) int {
	fs := flag.NewFlagSet("mint-gen", flag.ContinueOnError)
	var (
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
		rootFlag       bool
		noRootFlag     bool
		outDir         string
		repoRoot       string
		tuiMode        bool
		probeTerm      bool
	)
	fs.StringVar(&checkTarget, "check", "", "Check whether a program can run inside a pure Mint TUI")
	fs.StringVar(&explainTarget, "explain", "", "Explain what a program reads and writes, and why, as mint-gen sees it")
	fs.StringVar(&archetypeFlag, "archetype", "", "Choose the archetype yourself (e.g. probe, \"look up\"; see --list-archetypes)")
	fs.BoolVar(&listArchetypes, "list-archetypes", false, "List the 38 archetypes and the add-ons")
	fs.BoolVar(&listApps, "list", false, "List the apps in <repo>/apps")
	fs.BoolVar(&probeTerm, "probe-term", false, "Probe current terminal capabilities and refresh ~/.config/mint-interfaces/termcaps.json")
	fs.BoolVar(&force, "force", false, "Generate interface even if the TUI compatibility check fails")
	fs.BoolVar(&buildBin, "build", false, "Compile static Go binary (<tool>-mint) after scaffolding")
	fs.BoolVar(&runApp, "run", false, "Launch the compiled <tool>-mint interface (builds it first if missing)")
	fs.BoolVar(&removeApp, "remove", false, "Remove the scaffolded apps/<tool> directory and compiled <tool>-mint binary")
	fs.BoolVar(&rootFlag, "sudo", false, "With --run: start the app with sudo without asking (sudo asks for the password)")
	fs.BoolVar(&noRootFlag, "no-sudo", false, "With --run: start the app without sudo, without asking")
	fs.StringVar(&themeFlag, "theme", "", "Build the app with this colour theme (implies --build); it starts in it when its user has no theme of their own: default, terminal, nord, gruvbox, dracula, catppuccin or custom")
	fs.StringVar(&outDir, "out", "", "Output directory (default: <repo>/apps/<tool>)")
	fs.StringVar(&repoRoot, "root", "", "Mint-Interfaces repository root (auto-detected if empty)")
	fs.BoolVar(&tuiMode, "tui", false, "Launch interactive Mint TUI generator")

	ordered := flagsFirst(fs, args)
	if helpRequested(ordered) {
		writeMintGuide(os.Stdout, fs)
		return 0
	}
	if err := fs.Parse(ordered); err != nil {
		return 2
	}

	if listArchetypes {
		printArchetypeCatalogue()
		return 0
	}

	if probeTerm {
		prof, cachePath, _ := ensureTermProfile(true)
		fmt.Printf("Terminal:    %s\n", formatTermSummary(prof, false))
		fmt.Printf("Saved to:    %s\n", cachePath)
		if checkTarget == "" && fs.NArg() == 0 {
			return 0
		}
	}

	if repoRoot == "" {
		repoRoot = detectRepoRoot()
	}

	if listApps {
		return printApps(os.Stdout, os.Stderr, repoRoot)
	}

	if tuiMode {
		target := ""
		if fs.NArg() > 0 {
			target = fs.Arg(0)
		}
		if err := runInteractiveTUI(target, repoRoot); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	if explainTarget != "" {
		spec, model := ParseProgramIO(explainTarget)
		printIOModel(os.Stdout, model, spec)
		fmt.Println()
		explainOptions(os.Stdout, spec)
		printKnownToolWarnings(os.Stderr)
		return 0
	}

	if checkTarget != "" {
		prof, _, cached := ensureTermProfile(false)
		rep := CheckCompatibility(checkTarget)
		printCompatReport(rep)
		fmt.Printf("Terminal:    %s\n", formatTermSummary(prof, cached))
		if !rep.Compatible && rep.Category == CatFullScreenTUI {
			if _, m := ParseProgramIO(checkTarget); m.Batch != nil {
				fmt.Printf("Batch mode:  %s %s (%s): mint-gen can build it in this mode\n", checkTarget, strings.Join(m.Batch.Args, " "), m.Batch.Why)
				rep.Compatible = true
			}
		}
		if !rep.Compatible {
			return 1
		}
		if spec, model := ParseProgramIO(checkTarget); spec.Name != "" {
			printHelpNotes(model.HelpNotes)
			fmt.Println()
			printSignalNotes(os.Stdout, model.Notes)
			printSafetyReport(os.Stdout, spec)
			printKnownToolWarnings(os.Stderr)
		}
		return 0
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: mint-gen [--list] [--list-archetypes] [--explain <program>] [--archetype <archetype>] [--check <program>] [--force] [--build] [--run] [--remove] [--out <dir>] <program> [output-dir]")
		return 2
	}

	program := fs.Arg(0)
	var appArgs []string
	if themeFlag != "" {
		themeFlag = strings.ToLower(strings.TrimSpace(themeFlag))
		if !knownThemeName(themeFlag) {
			fmt.Fprintf(os.Stderr, "mint-gen: unknown theme %q. Themes: %s\n", safeTerminalText(themeFlag), strings.Join(theme.Names, ", "))
			return 2
		}
		// The theme is built into the app, so build it: mint-gen --theme
		// nord nmap leaves an nmap-mint that starts in nord.
		if !removeApp {
			buildBin = true
		}
	}
	if runApp {
		appArgs = fs.Args()[1:]
		// mint-gen --theme NAME --run tool: this run is in that theme,
		// whatever the user has saved.
		if themeFlag != "" {
			appArgs = append([]string{"--theme", themeFlag}, appArgs...)
		}
	} else if outDir == "" && fs.NArg() >= 2 {
		outDir = fs.Arg(1)
	}

	if removeApp {
		return removeScaffoldedApp(program, outDir, repoRoot)
	}

	if runApp && !buildBin {
		binPath := existingBinaryPath(program, outDir, repoRoot)
		if st, err := os.Stat(binPath); err == nil && !st.IsDir() {
			return runBuiltApp(binPath, appArgs, startAsRoot(program, nil, rootFlag, noRootFlag))
		}
		buildBin = true
	}

	rep := CheckCompatibility(program)
	if !rep.Compatible && !force && rep.Category == CatFullScreenTUI {
		// A full-screen program with a batch mode is built in that mode.
		if _, m := ParseProgramIO(program); m.Batch != nil {
			rep.Compatible = true
		}
	}
	if !rep.Compatible && !force {
		printCompatReport(rep)
		fmt.Fprintf(os.Stderr, "\nRefusing to scaffold %q (excluded category: %s). Pass --force to override.\n", program, rep.Category)
		return 1
	}

	spec, model := ParseProgramIO(program)
	printHelpNotes(model.HelpNotes)
	spec.Theme = themeFlag

	if archetypeFlag != "" {
		a, ok := resolveArchetypeFlag(archetypeFlag)
		if !ok {
			fmt.Fprintf(os.Stderr, "mint-gen: unknown archetype %q (run `mint-gen --list-archetypes` to see them)\n", safeTerminalText(archetypeFlag))
			return 2
		}
		srcs, _ := CollectHelpNotes(program)
		manText, helpText := manAndHelp(srcs)
		applyChosenArchetype(&spec, a, manText+"\n"+helpText)
		// The signals' caution stays with any archetype.
		applySignals(&spec, model.Signals)
		model.Notes = signalNotes(a.ID, model.Signals)
	}

	prof, _, cached := ensureTermProfile(false)
	res, err := ScaffoldApp(spec, outDir, repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mint-gen: scaffold error: %v\n", err)
		return 1
	}

	fmt.Printf("Terminal: %s\n", formatTermSummary(prof, cached))
	fmt.Printf("Scaffolded %s [archetype=%s] (%d flags, outputs_file=%v) in %s:\n",
		spec.Name, spec.ArchetypeID, len(spec.Flags), spec.OutputsFile, res.AppDir)
	for _, f := range res.Files {
		fmt.Printf("  - %s\n", f)
	}
	printArchetypeChoice(os.Stdout, program, model.Archetype, archetypeFlag)
	printSignalNotes(os.Stdout, model.Notes)
	fmt.Println()
	printSafetyReport(os.Stdout, spec)
	printKnownToolWarnings(os.Stderr)

	if buildBin {
		if err := buildScaffoldedApp(res.AppDir, res.BinaryOut); err != nil {
			fmt.Fprintf(os.Stderr, "mint-gen: build error: %v\n", err)
			return 1
		}
		fmt.Printf("Built static binary: %s\n", res.BinaryOut)
	}
	if runApp {
		return runBuiltApp(res.BinaryOut, appArgs, startAsRoot(program, &spec, rootFlag, noRootFlag))
	}
	return 0
}

// flagsFirst moves options written after the program name (as in
// `mint-gen --remove nmap --out dir`) in front of it, because the flag
// package stops at the first plain argument. Arguments after "--", and
// everything after the program name when --run is given, are left alone.
func flagsFirst(fs *flag.FlagSet, args []string) []string {
	var flags, plain []string
	run := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			plain = append(plain, args[i:]...)
			break
		}
		if run && len(plain) > 0 {
			plain = append(plain, a)
			continue
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			plain = append(plain, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if name == "run" {
			run = true
		}
		// Options that take a value consume the next argument.
		if f := fs.Lookup(name); f != nil {
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !bf.IsBoolFlag() {
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
			}
		}
	}
	return append(flags, plain...)
}

func printArchetypeCatalogue() {
	fmt.Printf("Mint-Interfaces — %d Composable Mixins:\n", len(appkit.AllMixins()))
	for _, m := range appkit.MixinInfos() {
		fmt.Printf("  - %-22s %s (%s)\n", m.Name, m.Summary, m.Fields)
	}
	fmt.Println()
	fmt.Printf("Mint-Interfaces — %d Archetypes (what kind of program it is):\n", len(appkit.AllArchetypeDefs()))
	fmt.Println("  A screen is built from the archetype: what it takes, how it runs, and what it shows.")
	fmt.Printf("  %-12s  %-19s  %s\n", "ARCHETYPE", "GROUP", "WHAT IT IS")
	for _, a := range appkit.AllArchetypeDefs() {
		fmt.Printf("  %-12s  %-19s  %s\n", a.ID, a.Group, a.Is)
	}
}

func printCompatReport(rep CompatReport) {
	status := "COMPATIBLE"
	if !rep.Compatible {
		status = "EXCLUDED (" + string(rep.Category) + ")"
	}
	fmt.Printf("Program:     %s\n", rep.Program)
	if rep.BinaryPath != "" {
		fmt.Printf("Binary:      %s\n", rep.BinaryPath)
	}
	fmt.Printf("Status:      %s\n", status)
	fmt.Printf("Reason:      %s\n", rep.Reason)
	if rep.Alternative != "" {
		fmt.Printf("Alternative: %s\n", rep.Alternative)
	}
	if rep.BatchHint != "" {
		fmt.Printf("Batch flag:  %s\n", rep.BatchHint)
	}
	if len(rep.LinkedLibs) > 0 {
		fmt.Printf("Libraries:   %s\n", strings.Join(rep.LinkedLibs, ", "))
	}
}

func removeScaffoldedApp(program, outDir, repoRoot string) int {
	tool := sanitizeToolName(program)
	dir := outDir
	if dir == "" {
		if repoRoot != "" {
			dir = filepath.Join(repoRoot, "apps", tool)
		} else {
			dir = filepath.Join("apps", tool)
		}
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mint-gen: remove error: %v\n", err)
		return 1
	}
	st, err := os.Stat(absDir)
	if err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "mint-gen: no build found for %q at %s\n", program, absDir)
		return 1
	}
	// Only a folder mint-gen created, and only the files it created: anything
	// else in the folder is left alone.
	if !isScaffold(absDir, tool) {
		fmt.Fprintf(os.Stderr, "mint-gen: refusing to remove %s: mint-gen did not create it\n", absDir)
		return 1
	}
	for _, name := range scaffoldFiles(tool) {
		if err := os.Remove(filepath.Join(absDir, name)); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "mint-gen: remove error: %v\n", err)
			return 1
		}
	}
	if err := os.Remove(absDir); err != nil {
		fmt.Printf("Removed the generated files of %s; kept %s because it holds other files\n", tool, absDir)
		return 0
	}
	// Remove a /usr/local/bin/<tool>-mint link only if it pointed at this app.
	symlinkPath := filepath.Join("/usr/local/bin", tool+"-mint")
	if dest, err := os.Readlink(symlinkPath); err == nil && filepath.Clean(dest) == filepath.Join(absDir, tool+"-mint") {
		_ = os.Remove(symlinkPath)
	}
	fmt.Printf("Removed %s (%s)\n", tool, absDir)
	return 0
}

func existingBinaryPath(program, outDir, repoRoot string) string {
	tool := sanitizeToolName(program)
	dir := outDir
	if dir == "" {
		if repoRoot != "" {
			dir = filepath.Join(repoRoot, "apps", tool)
		} else {
			dir = filepath.Join("apps", tool)
		}
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Join(dir, tool+"-mint")
}

func runBuiltApp(binPath string, appArgs []string, asRoot bool) int {
	cmd, err := launchCommand(binPath, appArgs, asRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mint-gen: %v\n", err)
		return 1
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "mint-gen: run error: %v\n", err)
		return 1
	}
	return 0
}

func buildScaffoldedApp(appDir, binOut string) error {
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = appDir
	if out, err := tidy.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %v\n%s", err, string(out))
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binOut, ".")
	cmd.Dir = appDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, string(out))
	}
	return nil
}

func detectRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			if strings.Contains(string(data), "module github.com/CurtisD4q/Mint-Interfaces") &&
				!strings.Contains(string(data), "module github.com/CurtisD4q/Mint-Interfaces/apps/") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func runInteractiveTUI(initialProgram, repoRoot string) error {
	prog := initialProgram
	if prog == "" {
		prog = "curl"
	}
	var (
		rep       = CheckCompatibility(prog)
		spec, _   = ParseProgram(prog)
		openTable = true
	)
	reload := func(name string) {
		prog = strings.TrimSpace(name)
		rep = CheckCompatibility(prog)
		spec, _ = ParseProgram(prog)
	}

	archOptions := make([]string, len(appkit.AllArchetypeDefs()))
	for i, a := range appkit.AllArchetypeDefs() {
		archOptions[i] = string(a.ID)
	}

	mainScr := func() *mint.Screen {
		scr := &mint.Screen{
			Icon:  "◈",
			Title: "mint-gen",
			Status: func() []mint.Badge {
				if rep.Compatible {
					return []mint.Badge{{Text: prog + " · " + spec.ArchetypeID, Kind: mint.BadgeOK}}
				}
				return []mint.Badge{{Text: prog + " excluded", Kind: mint.BadgeError}}
			},
		}
		scr.Refresh = func(s *mint.Screen) {
			statusText := "Compatible"
			if !rep.Compatible {
				statusText = "Excluded (" + string(rep.Category) + ")"
			}
			var tableRows [][]string
			for _, f := range spec.Flags {
				adv := "no"
				if f.Advanced {
					adv = "yes"
				}
				tableRows = append(tableRows, []string{f.Flag, string(f.Kind), f.Section, adv, f.Label})
			}
			s.Sections = []mint.Section{
				{
					Name: "Target Program & Archetype",
					Rows: []mint.Row{
						mint.Text(mint.TextOpts{
							ID:          "program",
							Label:       "Program binary",
							Placeholder: "e.g. curl, nmap, tshark, systemctl",
							Get:         func() string { return prog },
							Set: func(v string) {
								reload(v)
							},
						}),
						mint.Picker(mint.PickerOpts{
							ID:      "archetype",
							Label:   "Archetype",
							Options: archOptions,
							Get:     func() string { return spec.ArchetypeID },
							Set: func(v string) {
								if a, ok := appkit.LookupArchetype(v); ok {
									applyChosenArchetype(&spec, a, spec.Desc)
								}
							},
						}),
						mint.Value(mint.ValueOpts{
							ID:    "mixins",
							Label: "Active mixins",
							Get: func() string {
								var names []string
								for _, m := range spec.EnabledMixinNames() {
									names = append(names, string(m))
								}
								return strings.Join(names, ", ")
							},
						}),
						mint.Value(mint.ValueOpts{
							ID:    "compat",
							Label: "TUI compatibility",
							Get:   func() string { return statusText },
						}),
						mint.Action(mint.ActionOpts{
							ID:    "scaffold",
							Label: "Scaffold apps/" + sanitizeToolName(prog),
							Hint:  "generate",
							OnActivate: func(c *mint.Ctx) {
								if !rep.Compatible {
									c.ToastError("Excluded: " + rep.Reason)
									return
								}
								res, err := ScaffoldApp(spec, "", repoRoot)
								if err != nil {
									c.ToastError("Error: " + err.Error())
									return
								}
								mint.Toast("Scaffolded " + res.AppDir)
							},
						}),
					},
				},
				{
					Name: "Parsed Specification",
					Rows: []mint.Row{
						mint.Value(mint.ValueOpts{
							ID:    "summary",
							Label: "Summary",
							Get:   func() string { return spec.Desc },
						}),
						mint.Value(mint.ValueOpts{
							ID:    "target_label",
							Label: "Target label",
							Get:   func() string { return spec.TargetLabel },
						}),
						mint.Value(mint.ValueOpts{
							ID:    "outputs_file",
							Label: "Outputs file",
							Get:   func() string { return fmt.Sprintf("%v (%s)", spec.OutputsFile, spec.OutputFlag) },
						}),
						mint.Collapsible(mint.CollapsibleOpts{
							ID:    "parsed-flags",
							Title: fmt.Sprintf("Parsed Flags (%d)", len(spec.Flags)),
							Open:  &openTable,
							Rows: []mint.Row{
								mint.Table(mint.TableOpts{
									ID:      "flags-table",
									Headers: []string{"FLAG", "KIND", "SECTION", "COLLAPSED", "LABEL"},
									Rows:    tableRows,
									Empty:   "No flags parsed",
								}),
							},
						}),
					},
				},
				{
					Name: "Safety",
					Rows: safetyRows(spec),
				},
			}
		}
		return scr
	}

	app := &mint.App{
		Title: "◈ Mint Generator",
		Crumb: "mint-gen",
		Note:  "man-page & --help interface scaffolder (38 Archetypes · Mixins)",
		Items: []mint.HomeItem{
			{Icon: "▸", Label: "Inspect & Scaffold", Desc: "Detect the archetype, inspect parsed flags, and scaffold apps/<tool>", Open: mainScr},
		},
	}
	app.Run()
	return nil
}

var _ = appkit.FlagBool

// printApps lists the folders in <repo>/apps, one per line, like ls.
func printApps(out, errOut io.Writer, repoRoot string) int {
	dir := filepath.Join(repoRoot, "apps")
	if repoRoot == "" {
		dir = "apps"
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(errOut, "No apps yet (%s does not exist)\n", dir)
		return 0
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	n := 0
	for _, e := range entries { // ReadDir sorts by name
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			fmt.Fprintln(out, e.Name())
			n++
		}
	}
	if n == 0 {
		fmt.Fprintf(errOut, "No apps yet in %s\n", dir)
	}
	return 0
}

// printKnownToolWarnings reports known-tools JSON files that were ignored,
// and why, so a mistake (or someone else's file) is never silent.
func printKnownToolWarnings(w io.Writer) {
	for _, msg := range KnownToolWarnings() {
		fmt.Fprintln(w, "mint-gen: tools.d: "+safeTerminalText(msg))
	}
}

// safeTerminalText removes control characters (such as escape sequences
// from a file name) before text is printed to a terminal.
func safeTerminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// knownThemeName reports whether name is one of Mint's themes.
func knownThemeName(name string) bool {
	for _, n := range theme.Names {
		if n == name {
			return true
		}
	}
	return false
}
