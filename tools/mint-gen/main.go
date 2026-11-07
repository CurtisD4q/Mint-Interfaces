package main

import (
	"bufio"
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
	callerDir()
	os.Exit(runCLI(os.Args[1:]))
}

func runCLI(args []string) int {
	opt := newMintOpts()
	fs := opt.fs

	ordered := flagsFirst(fs, args)
	if helpRequested(ordered) {
		writeMintGuide(os.Stdout, fs)
		return 0
	}
	if err := fs.Parse(ordered); err != nil {
		return 2
	}
	if opt.man {
		fmt.Fprint(os.Stdout, manPage(fs))
		return 0
	}

	// --theme NAME saves the theme for every Mint app, as the Theme
	// screen's "Use this theme" does. On its own it does only that.
	if opt.themeFlag != "" {
		if code := saveThemeForEveryApp(opt.themeFlag); code != 0 {
			return code
		}
		if fs.NArg() == 0 && !opt.listArchetypes && !opt.probeTerm && !opt.listApps && !opt.tuiMode && opt.explainTarget == "" && opt.checkTarget == "" {
			return 0
		}
	}

	if opt.listArchetypes {
		printArchetypeCatalogue()
		return 0
	}

	if opt.probeTerm {
		prof, cachePath, _ := ensureTermProfile(true)
		fmt.Printf("Terminal:    %s\n", formatTermSummary(prof, false))
		fmt.Printf("Saved to:    %s\n", cachePath)
		if opt.checkTarget == "" && fs.NArg() == 0 {
			return 0
		}
	}

	if opt.repoRoot == "" {
		opt.repoRoot = detectRepoRoot()
	} else if abs, err := filepath.Abs(opt.repoRoot); err == nil && isRepoRoot(abs) {
		opt.repoRoot = abs
	} else {
		fmt.Fprintf(os.Stderr, "mint-gen: %s is not the Mint-Interfaces folder (no go.mod for github.com/CurtisD4q/Mint-Interfaces)\n", safeTerminalText(opt.repoRoot))
		return 2
	}
	if opt.install {
		if opt.system {
			return installSystemHelp(os.Stdout, opt.repoRoot)
		}
		return installUser(os.Stdout, os.Stderr, opt.repoRoot)
	}

	// Everything below except --check and --explain builds, lists or
	// removes apps, which live in the Mint-Interfaces folder.
	needRoot := func() bool {
		if opt.repoRoot == "" {
			fmt.Fprintln(os.Stderr, noRootMessage)
			return false
		}
		return true
	}

	if opt.listApps {
		if !needRoot() {
			return 2
		}
		return printApps(os.Stdout, os.Stderr, opt.repoRoot)
	}

	if opt.purgeApps {
		if !needRoot() {
			return 2
		}
		return purgeAllApps(os.Stdin, os.Stdout, os.Stderr, opt.repoRoot)
	}

	if opt.tuiMode {
		if !needRoot() {
			return 2
		}
		target := ""
		if fs.NArg() > 0 {
			target = fs.Arg(0)
		}
		if err := runInteractiveTUI(target, opt.repoRoot); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	if opt.explainTarget != "" {
		spec, model := ParseProgramIO(opt.explainTarget)
		printIOModel(os.Stdout, model, spec)
		fmt.Println()
		explainOptions(os.Stdout, spec)
		printKnownToolWarnings(os.Stderr)
		return 0
	}

	if opt.checkTarget != "" {
		prof, _, cached := ensureTermProfile(false)
		rep := CheckCompatibility(opt.checkTarget)
		printCompatReport(rep)
		fmt.Printf("Terminal:    %s\n", formatTermSummary(prof, cached))
		if !rep.Compatible && rep.Category == CatFullScreenTUI {
			if _, m := ParseProgramIO(opt.checkTarget); m.Batch != nil {
				fmt.Printf("Batch mode:  %s %s (%s): mint-gen can build it in this mode\n", opt.checkTarget, strings.Join(m.Batch.Args, " "), m.Batch.Why)
				rep.Compatible = true
			}
		}
		if !rep.Compatible {
			return 1
		}
		if spec, model := ParseProgramIO(opt.checkTarget); spec.Name != "" {
			printHelpNotes(model.HelpNotes)
			fmt.Println()
			printSignalNotes(os.Stdout, model.Notes)
			printSafetyReport(os.Stdout, spec)
			printKnownToolWarnings(os.Stderr)
		}
		return 0
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: mint-gen [--list] [--list-archetypes] [--explain <program>] [--archetype <archetype>] [--check <program>] [--force] [--build] [--run] [--remove] [--out <dir>] [--man] <program> [output-dir]")
		return 2
	}

	if !needRoot() {
		return 2
	}

	program := fs.Arg(0)
	var appArgs []string
	if opt.runApp {
		appArgs = fs.Args()[1:]
	} else if opt.outDir == "" && fs.NArg() >= 2 {
		opt.outDir = fs.Arg(1)
	}

	if opt.removeApp {
		return removeScaffoldedApp(program, opt.outDir, opt.repoRoot)
	}

	if opt.runApp && !opt.buildBin {
		binPath := existingBinaryPath(program, opt.outDir, opt.repoRoot)
		if st, err := os.Stat(binPath); err == nil && !st.IsDir() {
			return runBuiltApp(binPath, appArgs, startAsRoot(program, nil, opt.rootFlag, opt.noRootFlag, binPath))
		}
		opt.buildBin = true
	}

	rep := CheckCompatibility(program)
	if !rep.Compatible && !opt.force && rep.Category == CatFullScreenTUI {
		// A full-screen program with a batch mode is built in that mode.
		if _, m := ParseProgramIO(program); m.Batch != nil {
			rep.Compatible = true
		}
	}
	if !rep.Compatible && !opt.force {
		printCompatReport(rep)
		fmt.Fprintf(os.Stderr, "\nRefusing to scaffold %q (excluded category: %s). Pass --force to override.\n", program, rep.Category)
		return 1
	}

	spec, model := ParseProgramIO(program)
	printHelpNotes(model.HelpNotes)

	if opt.archetypeFlag != "" {
		ms, bad := resolveArchetypeList(opt.archetypeFlag)
		if bad != "" {
			fmt.Fprintf(os.Stderr, "mint-gen: unknown archetype %q (run `mint-gen --list-archetypes` to see them)\n", safeTerminalText(bad))
			return 2
		}
		srcs, _ := CollectHelpNotes(program)
		manText, helpText := manAndHelp(srcs)
		applyChosenArchetype(&spec, ms, manText+"\n"+helpText)
		// The signals' caution stays with any archetype.
		applySignals(&spec, model.Signals)
		model.Notes = signalNotes(ms[0], model.Signals)
	}

	prof, _, cached := ensureTermProfile(false)
	res, err := ScaffoldApp(spec, opt.outDir, opt.repoRoot)
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
	printArchetypeChoice(os.Stdout, program, model.Archetype, opt.archetypeFlag)
	printInterfaceType(os.Stdout, spec, "")
	printSignalNotes(os.Stdout, model.Notes)
	fmt.Println()
	printSafetyReport(os.Stdout, spec)
	printKnownToolWarnings(os.Stderr)

	if opt.buildBin {
		if err := buildScaffoldedApp(res.AppDir, res.BinaryOut); err != nil {
			fmt.Fprintf(os.Stderr, "mint-gen: build error: %v\n", err)
			return 1
		}
		fmt.Printf("Built static binary: %s\n", res.BinaryOut)
	}
	if opt.runApp {
		return runBuiltApp(res.BinaryOut, appArgs, startAsRoot(program, &spec, opt.rootFlag, opt.noRootFlag, res.BinaryOut))
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
	// Remove a <tool>-mint link (in /usr/local/bin or
	// ~/.local/bin/mint-interfaces) only if it pointed at this app.
	for _, d := range appLinkDirs() {
		symlinkPath := filepath.Join(d, tool+"-mint")
		if dest, err := os.Readlink(symlinkPath); err == nil && filepath.Clean(dest) == filepath.Join(absDir, tool+"-mint") {
			_ = os.Remove(symlinkPath)
		}
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

// installedRoot is where the README's global install puts Mint-Interfaces.
var installedRoot = "/usr/local/share/mint-interfaces"

// isRepoRoot reports whether dir is the Mint-Interfaces folder: its go.mod
// is the module itself, not one of the apps inside it.
func isRepoRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
			return f[1] == "github.com/CurtisD4q/Mint-Interfaces"
		}
	}
	return false
}

// rootAbove is the Mint-Interfaces folder that dir is in (or is), or "".
func rootAbove(dir string) string {
	for dir != "" {
		if isRepoRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// detectRepoRoot finds the Mint-Interfaces folder, so apps are always
// built in its apps/ folder wherever mint-gen is run from: the folder
// you are in (or one above it), then the one `go -C <repo> run` started
// it in (the launcher), then the folder mint-gen itself was built in (a
// copy on PATH, or a link to one), then $MINT_INTERFACES_ROOT, then the
// user install (~/.local/share/mint-interfaces), then the system-wide
// one (/usr/local/share/mint-interfaces). It
// returns "" when there is none; mint-gen then stops rather than writing
// apps into the folder you happen to be in.
func detectRepoRoot() string {
	if wd, err := os.Getwd(); err == nil {
		if r := rootAbove(wd); r != "" {
			return r
		}
	}
	if launchRoot != "" && isRepoRoot(launchRoot) {
		return launchRoot
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		if r := rootAbove(filepath.Dir(exe)); r != "" {
			return r
		}
	}
	if env := os.Getenv("MINT_INTERFACES_ROOT"); env != "" {
		if abs, err := filepath.Abs(env); err == nil && isRepoRoot(abs) {
			return abs
		}
	}
	if r := userInstallRoot(); r != "" && isRepoRoot(r) {
		return r
	}
	if isRepoRoot(installedRoot) {
		return installedRoot
	}
	return ""
}

// noRootMessage says what to do when the Mint-Interfaces folder is not
// found.
const noRootMessage = "mint-gen: cannot find the Mint-Interfaces folder, so it will not build apps here. Run it from inside that folder, put it in ~/.local/share/mint-interfaces, pass --root /path/to/mint-interfaces, or set MINT_INTERFACES_ROOT to that folder."

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
									applyChosenArchetype(&spec, []appkit.ArchetypeID{a.ID}, spec.Desc)
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

// purgeAllApps removes every app in <repo>/apps that mint-gen created,
// after listing them and asking to confirm (y/n). Only what --remove would
// delete goes: folders mint-gen did not create are named and left alone,
// and in an app folder only the files mint-gen wrote are removed. Anything
// but y or yes (including no answer) removes nothing.
func purgeAllApps(in io.Reader, out, errOut io.Writer, repoRoot string) int {
	dir := filepath.Join(repoRoot, "apps")
	if repoRoot == "" {
		dir = "apps"
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(out, "No apps to remove (%s does not exist)\n", dir)
		return 0
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	var apps, others []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		// A link is never followed: it is named and left alone.
		if e.Type()&os.ModeSymlink != 0 {
			others = append(others, name)
			continue
		}
		if !e.IsDir() {
			continue
		}
		// A real folder, named as mint-gen names one, that it created.
		if sanitizeToolName(name) == name && isScaffold(filepath.Join(dir, name), name) {
			apps = append(apps, name)
		} else {
			others = append(others, name)
		}
	}
	if len(others) > 0 {
		fmt.Fprintf(out, "Left alone (mint-gen did not create them): %s\n", safeTerminalText(strings.Join(others, ", ")))
	}
	if len(apps) == 0 {
		fmt.Fprintf(out, "No apps to remove in %s\n", dir)
		return 0
	}
	fmt.Fprintf(out, "This removes %d app(s) from %s, with their built programs:\n", len(apps), dir)
	for _, a := range apps {
		fmt.Fprintf(out, "  %s\n", a)
	}
	fmt.Fprint(out, "Remove them all? (y/n) ")
	answer, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
	default:
		fmt.Fprintln(out, "Nothing removed.")
		return 1
	}
	failed := 0
	for _, a := range apps {
		if removeScaffoldedApp(a, "", repoRoot) != 0 {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(errOut, "mint-gen: %d of %d apps could not be removed\n", failed, len(apps))
		return 1
	}
	fmt.Fprintf(out, "Removed %d app(s).\n", len(apps))
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

// saveThemeForEveryApp saves name as the theme every Mint app uses. It
// returns the exit code: 2 for a name Mint does not have, 1 when the file
// cannot be written.
func saveThemeForEveryApp(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if !knownThemeName(name) {
		fmt.Fprintf(os.Stderr, "mint-gen: unknown theme %q. Themes: %s\n", safeTerminalText(name), strings.Join(theme.Names, ", "))
		return 2
	}
	if err := theme.Set(name); err != nil {
		fmt.Fprintf(os.Stderr, "mint-gen: could not save the theme: %v\n", err)
		return 1
	}
	fmt.Printf("Theme: %s for every Mint app (saved in %s). Apps that are open use it when they next start.\n", name, theme.ConfigPath)
	if os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
		fmt.Println("       This is root's theme file. Run mint-gen --theme without sudo to set your own.")
	}
	return 0
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
