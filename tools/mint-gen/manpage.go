package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

// manDate is the revision date of the wording around the generated lists.
// The lists themselves are filled from the code whenever the page is built.
const manDate = "November 7, 2026"

// manPage is mint-interfaces(1), in mdoc. Flags come from the flag set
// mint-gen parses; archetypes, interface types and add-ons come from the
// registries the running library actually uses.
func manPage(fs *flag.FlagSet) string {
	w := &manw{}
	w.line(".Dd " + manDate)
	w.line(".Dt MINT-INTERFACES 1")
	w.line(".Os")
	writeManName(w)
	writeManSynopsis(w, fs)
	writeManDescription(w)
	writeManManualPages(w)
	writeManOptions(w, fs)
	writeManOperands(w)
	writeManArchetypes(w)
	writeManInterfaceTypes(w)
	writeManAddons(w)
	writeManKeyboard(w)
	writeManEnvironment(w)
	writeManFiles(w)
	writeManExamples(w)
	writeManSeeAlso(w)
	return w.String()
}

type manw struct {
	b strings.Builder
}

func (w *manw) String() string { return w.b.String() }

func (w *manw) line(s string) {
	w.b.WriteString(strings.TrimRight(s, " \t"))
	w.b.WriteByte('\n')
}

func (w *manw) sh(title string) {
	w.line(".Sh " + roffQuote(title))
}

func (w *manw) ss(title string) {
	w.line(".Ss " + roffQuote(title))
}

func (w *manw) prose(s string) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return
	}
	// mandoc warns when a new sentence starts mid-line, so each sentence
	// begins on its own line. A long sentence may still wrap.
	for _, sent := range splitSentences(s) {
		for _, ln := range wrapWords(sent, 72) {
			w.line(roffText(ln))
		}
	}
}

// splitSentences breaks s after ".", "?" or "!" when the next word starts
// with a capital letter. Abbreviations such as "e.g. probe" stay intact.
func splitSentences(s string) []string {
	var out []string
	var b strings.Builder
	for i := 0; i < len(s); {
		b.WriteByte(s[i])
		if (s[i] == '.' || s[i] == '?' || s[i] == '!') && i+1 < len(s) && s[i+1] == ' ' {
			j := i + 1
			for j < len(s) && s[j] == ' ' {
				j++
			}
			if j < len(s) && s[j] >= 'A' && s[j] <= 'Z' {
				out = append(out, strings.TrimSpace(b.String()))
				b.Reset()
				i = j
				continue
			}
		}
		i++
	}
	if tail := strings.TrimSpace(b.String()); tail != "" {
		out = append(out, tail)
	}
	return out
}

func (w *manw) pp(s string) {
	w.line(".Pp")
	w.prose(s)
}

func (w *manw) literal(lines ...string) {
	w.line(".Bd -literal -offset indent")
	for _, ln := range lines {
		w.line(roffText(ln))
	}
	w.line(".Ed")
}

func (w *manw) item(tag string) {
	w.line(".It " + roffQuote(tag))
}

func (w *manw) tagList(width int) {
	if width < 6 {
		width = 6
	}
	w.line(fmt.Sprintf(".Bl -tag -width %dn", width))
}

func (w *manw) endList() { w.line(".El") }

func roffQuote(s string) string {
	if strings.ContainsAny(s, " \t\"") {
		return `"` + strings.ReplaceAll(s, `"`, "") + `"`
	}
	return s
}

// roffText keeps a free-text line from being read as a macro.
func roffText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	if s == "" {
		return `\&`
	}
	if s[0] == '.' || s[0] == '\'' {
		return `\&` + s
	}
	return s
}

func wrapWords(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := words[0]
	for _, word := range words[1:] {
		if len(cur)+1+len(word) > width && cur != "" {
			lines = append(lines, cur)
			cur = word
			continue
		}
		cur += " " + word
	}
	lines = append(lines, cur)
	return lines
}

func joinWords(words []string, conj string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	default:
		return strings.Join(words[:len(words)-1], ", ") + " " + conj + " " + words[len(words)-1]
	}
}

func writeManName(w *manw) {
	w.sh("NAME")
	w.line(".Nm mint-interfaces ,")
	w.line(".Nm mint-gen")
	w.line(".Nd terminal interfaces for command-line programs")
}

func writeManSynopsis(w *manw, fs *flag.FlagSet) {
	w.sh("SYNOPSIS")
	w.line(".Nm mint-gen")
	w.line(".Op Ar options")
	w.line(".Ar program")
	w.line(".Op Ar output-dir")
	for _, f := range collectFlags(fs) {
		w.line(".Nm mint-gen")
		w.line(".Fl -" + f.Name)
		if flagTakesValue(f) {
			w.line(".Ar " + flagArgName(f.Name))
		}
	}
	w.line(".Nm mint-gen")
	w.line(".Fl -help")
}

func writeManDescription(w *manw) {
	w.sh("DESCRIPTION")
	w.prose(fmt.Sprintf("Mint-Interfaces %s is a terminal user interface library and a generator for command-line programs. The library draws a rounded frame, a home list, rows (values, toggles, pickers, sliders, actions), panes and a key-hint footer. The keyboard and the mouse both work: a click selects, a double-click opens, and the wheel scrolls. Redraws happen only when something on screen changes, so an idle screen stays as drawn. The library uses only the Go standard library.", mint.Version))
	w.pp("The library itself contains no settings logic and no system backend. An app describes its screens and handles its own data. mint-gen is the generator. It reads a program's manual and its help, decides what kind of program it is and how the screen should be laid out, and writes an app under apps/<tool>/ in the Mint-Interfaces repository. The compiled program is a static binary named <tool>-mint.")
	w.pp(fmt.Sprintf("The colour themes are %s.", joinWords(theme.Names, "and")))
	w.pp("A minimal working example, from a checkout of the repository:")
	w.literal(
		"go run ./tools/mint-gen --build ps",
		"./apps/ps/ps-mint",
	)
	w.pp("Check a program, build the app, then run the binary. Those are the three steps for any program mint-gen can host. mint-gen --help prints a short guide for the steps and for using the finished app.")
}

func writeManManualPages(w *manw) {
	w.sh("MANUAL PAGES")
	w.prose("This manual is mint-interfaces(1). It describes Mint-Interfaces and the mint-gen generator. mint-gen --man writes it as roff, in mdoc, on standard output. Save that output as mint-interfaces.1 and read it with mandoc or man -l. The flag list is the flag set mint-gen parses. The archetypes and interface types are the ones the library registers, so building the page again follows the source.")
	w.pp("mint-gen --help prints the text guide: how to build an app, how to use one, then the same flags in the layout the Go flag package prints. The guide is the quick introduction. This manual is the page that stays aligned with the code.")
	w.pp("A generated app has its own --man. ps-mint --man opens that app's Manual screen, which is also the Manual item on its home menu. The screen shows the wrapped program's manual: the page man would show for that program, or the program's --help when that program has no manual of its own. The app's --help is a short usage list (open the interface, open one screen, choose a theme for one run, print the version, print JSON for a launcher) and points at mint-gen --help for the guide.")
}

func writeManOptions(w *manw, fs *flag.FlagSet) {
	w.sh("OPTIONS")
	w.prose("Each flag below is registered on mint-gen's flag set, and the sentence is that flag's own description. The long spelling is the one you type. The single-dash spelling is the one mint-gen --help prints. --help is handled before the flag set is parsed, so it is listed after the registered flags. mint-gen has no subcommands; the program name is an operand.")
	flags := collectFlags(fs)
	helpTag := "--help, -h, -help"
	width := len(helpTag)
	tags := make([]string, len(flags))
	for i, f := range flags {
		tags[i] = flagTag(f.Name)
		if len(tags[i]) > width {
			width = len(tags[i])
		}
	}
	w.tagList(width)
	for i, f := range flags {
		w.item(tags[i])
		usage := strings.TrimSpace(f.Usage)
		if flagTakesValue(f) {
			usage = "Argument: " + flagArgName(f.Name) + ". " + usage
		}
		w.prose(usage)
	}
	w.item(helpTag)
	w.prose("Print the text guide and the flag list. -h and -help select the same guide. The guide and this manual are described under MANUAL PAGES.")
	w.endList()
}

func writeManOperands(w *manw) {
	w.sh("OPERANDS")
	w.prose("A word that does not begin with a dash is an operand. Actions such as building, running, listing and removing are flags, listed under OPTIONS.")
	w.tagList(12)
	w.item("program")
	w.prose("The command-line program to read and wrap. mint-gen reads its manual and its help, then scaffolds apps/<tool>/. With --run, words after this name are arguments for the app (its own --help, --version or --man, for example) and stay out of mint-gen's flag set.")
	w.item("output-dir")
	w.prose("Directory for the scaffold when --out is absent. The default is apps/<tool>/ inside the Mint-Interfaces repository.")
	w.endList()
	w.pp("A double dash ends mint-gen's own flags. Together with --run, it is one way to hand a flag to the app.")
}

func writeManArchetypes(w *manw) {
	defs := appkit.AllArchetypeDefs()
	w.sh("ARCHETYPES")
	w.prose(fmt.Sprintf("An archetype says what kind of program a tool is: what it takes, how it runs, and what it shows. There are %d, from appkit.AllArchetypeDefs, in that order. mint-gen chooses one. --archetype names one, or several separated by commas, and the first is the main one. The screen is assembled from that choice.", len(defs)))
	group := ""
	open := false
	for _, a := range defs {
		if a.Group != group {
			if open {
				w.endList()
			}
			group = a.Group
			w.ss(group)
			w.tagList(14)
			open = true
		}
		w.item(string(a.ID))
		w.prose(archetypeBody(a))
	}
	if open {
		w.endList()
	}
}

func archetypeBody(a appkit.ArchetypeDef) string {
	parts := []string{a.Name + "."}
	if s := strings.TrimSpace(a.Is); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(a.Takes); s != "" {
		parts = append(parts, "Takes: "+s)
	}
	if s := strings.TrimSpace(a.Runs); s != "" {
		parts = append(parts, "Runs: "+s)
	}
	if s := strings.TrimSpace(a.Shows); s != "" {
		parts = append(parts, "Shows: "+s)
	}
	return strings.Join(parts, " ")
}

func writeManInterfaceTypes(w *manw) {
	types := appkit.InterfaceTypes()
	w.sh("INTERFACE TYPES")
	w.prose(fmt.Sprintf("An interface type is how the screen is laid out. An archetype is what the program is for. There are %d types, from appkit.InterfaceTypes, tried in the order below. The first rule that matches is the one the app uses. Archetypes break ties, and when a program fits more than one archetype every archetype's features count.", len(types)))
	w.tagList(16)
	for _, t := range types {
		w.item(string(t.ID))
		w.prose(interfaceBody(t))
	}
	w.endList()
	w.pp("The command suite puts the commands beside the selected command's options, with the result under both. The live monitor puts options on the left and the live values beside them. The other types draw the classic screen. MINT_CLASSIC_LAYOUT=1 draws every app that way. mint-gen names the type when it builds an app, and mint-gen --explain program says why.")
}

func interfaceBody(t appkit.InterfaceType) string {
	body := strings.TrimSpace(t.Name + ". " + t.Is)
	why := strings.TrimSpace(t.Why)
	if why == "" {
		return body
	}
	if !strings.HasSuffix(why, ".") {
		why += "."
	}
	return body + " Chosen when " + why
}

func writeManAddons(w *manw) {
	infos := appkit.MixinInfos()
	w.sh("ADD-ONS")
	w.prose(fmt.Sprintf("Add-ons, also called mixins, are the extra pieces an archetype can switch on, such as a saved file, a table or a confirm. mint-gen --list-archetypes prints them with the archetypes. There are %d, from appkit.MixinInfos.", len(infos)))
	width := 8
	for _, m := range infos {
		if n := len(string(m.Name)); n > width {
			width = n
		}
	}
	w.tagList(width)
	for _, m := range infos {
		w.item(string(m.Name))
		sum := strings.TrimSpace(m.Summary)
		if sum != "" && !strings.HasSuffix(sum, ".") {
			sum += "."
		}
		fields := strings.TrimSpace(m.Fields)
		if fields != "" {
			sum += " Fields: " + fields + "."
		}
		w.prose(sum)
	}
	w.endList()
}

func writeManKeyboard(w *manw) {
	w.sh("KEYBOARD")
	w.prose("These keys are for a generated app, and for any app built on the library. Arrow keys, Enter, Space, Esc, b and q work in every profile. Home, End, Page Up and Page Down move through a list.")
	w.pp("On the home menu, Up and Down move, Enter opens an item, b goes back, and q quits. On the main screen you set the target and the options, then run the command. Extra arguments, at the top, takes options the app has no box for. Press Enter in a target, path, option or Extra arguments box and a leading ~ or an environment variable in that box is expanded. Presets stores a set of options. Run History lists earlier runs. Theme sets the colours for every Mint app, as mint-gen --theme does.")
	w.pp("Some screens are made of panes. Tab and Shift-Tab move between panes, and a click focuses the pane under the pointer. On a wide terminal some panes sit side by side; on a narrow one they stack. [ shrinks the focused pane and ] grows it. | hides the focused stacked pane, and | again shows it. In a wide table, Left and Right scroll sideways, H narrows the column you are on, and L widens it.")
	w.pp("y copies the current row, or the whole output in the viewer. In the output viewer, s saves to a file, / searches, and n and N move between matches. Text that an app treats as secret stays off the clipboard.")
	w.pp(fmt.Sprintf("A key profile adds its own keys on top of the ones above. Set MINT_KEYS to one of %s. An app may choose a default with its Keys field. In the vim profile, letters are keys only outside a text field, so typing still works. The footer hints follow the profile.", joinWords(mint.KeyProfiles, "or")))
	blurbs := map[string]string{
		"default": "Arrows move and change a value. Left, Right and Space change the selected control. Esc and b go back.",
		"emacs":   "C-n and C-p move, C-f and C-b change a value, C-g goes back, C-v and M-v page, C-a and M-< go to the first item, and C-e and M-> go to the last.",
		"vim":     "j and k move, h and l change a value, g and G go to the first and last item, C-u and C-b page up, and C-d and C-f page down.",
	}
	width := 8
	for _, name := range mint.KeyProfiles {
		if len(name) > width {
			width = len(name)
		}
	}
	w.tagList(width)
	for _, name := range mint.KeyProfiles {
		w.item(name)
		if b, ok := blurbs[name]; ok {
			w.prose(b)
			continue
		}
		w.prose("Select this profile with MINT_KEYS.")
	}
	w.endList()
}

func writeManEnvironment(w *manw) {
	w.sh("ENVIRONMENT")
	w.prose("Generated apps and mint-gen read these variables. Leaving one unset keeps the usual behaviour.")
	rows := []struct{ name, desc string }{
		{"NO_COLOR", "When set to any value, Mint draws no colour. Emphasis is bold or reverse, and a warning that would have been a red row is carried by the label."},
		{"TERM", "Terminal type. A value of dumb, or a name ending in -mono or -m, also turns colour off."},
		{"COLORTERM", "Set to truecolor or 24bit when the terminal can show 24-bit colour. Otherwise a theme's RGB colours use the nearest 256-colour value."},
		{"LANG, LC_ALL, LC_CTYPE", "With a UTF-8 locale, Mint draws its frame. With any other locale, apps draw in ASCII."},
		{"MINT_KEYS", fmt.Sprintf("Key profile for an app: %s.", joinWords(mint.KeyProfiles, "or"))},
		{"MINT_PALETTE", "Replaces individual colours for one run, for example accent=#ff5f87,selbg=236. The roles are accent, bright, text, muted, on, amber, err and selbg. Emacs passes its theme this way."},
		{"MINT_HOVER", "Set to 0 to stop mouse-motion reports. Clicks and the wheel still work."},
		{"MINT_LINKS", "Set to 0 to turn off OSC 8 hyperlinks in program output and in values."},
		{"MINT_DIM_UNFOCUSED", "Set to 0 to keep an app bright when its window loses focus."},
		{"MINT_CLASSIC_LAYOUT", "Set to 1 to draw every app as the classic single screen, including types that have a layout of their own."},
		{"MINT_INTERFACES_ROOT", "The Mint-Interfaces repository, used when mint-gen cannot find it from the current directory, the launcher or an install location."},
		{"MINT_TERMCAPS", "Path of the terminal capability cache. When unset, the cache is termcaps.json in the shared config directory."},
		{"MINT_KEY_TIMEOUT", "How long the secret store keeps its key, for example 5m, or 0 until the app closes. When unset, security.json applies, and then 15 minutes."},
		{"MINT_SEALER", "Name of the sealer used for newly sealed files."},
		{"XDG_CONFIG_HOME", "Directory that contains the shared config directory mint-interfaces. When unset, ~/.config is used. Under sudo the shared directory is root's own."},
		{"XDG_DATA_HOME", "Directory that contains the user install of the repository (mint-interfaces). When unset, ~/.local/share is used. An app's run history stays under ~/.local/share/mint-interfaces-apps/<tool>/ either way."},
		{"HOME", "Home directory. A leading ~ in a text box expands to it, and per-app settings and data are stored under it."},
		{"SUDO_USER", "When an app is started with sudo, its settings, history and results stay in this user's home. Theme and security files written as root stay in root's config directory."},
	}
	width := 8
	for _, row := range rows {
		if len(row.name) > width {
			width = len(row.name)
		}
	}
	w.tagList(width)
	for _, row := range rows {
		if strings.ContainsAny(row.name, " ,") {
			w.item(row.name)
		} else {
			w.line(".It Ev " + row.name)
		}
		w.prose(row.desc)
	}
	w.endList()
}

func writeManFiles(w *manw) {
	w.sh("FILES")
	w.prose("In the paths below, ~ is the home directory and <tool> is the app's name. Per-app settings are kept in ~/.config/mint-interfaces/<tool>/. Per-app data is kept in ~/.local/share/mint-interfaces-apps/<tool>/. An earlier version used ~/.config/mint-<tool>/ and ~/.local/share/mint-<tool>/. The first run moves the files it knows.")
	rows := []struct{ path, desc string }{
		{"~/.config/mint-interfaces/<tool>/", "Settings for one app, in settings.json: the target, extra arguments and options, kept across runs and rebuilds."},
		{"~/.local/share/mint-interfaces-apps/<tool>/", "Data for one app: runs.json (run history), presets.json, commands.json (the command transcript), findings-last.json and rule-backups."},
		{"~/.config/mint-interfaces/theme.conf", "The colour theme shared by every Mint app."},
		{"~/.config/mint-interfaces/termcaps.json", "What Mint has learned about terminals. mint-gen --probe-term refreshes the entry for the current one."},
		{"~/.config/mint-interfaces/security.json", "Key timeout and sealer for the secret store."},
		{"/etc/mint-interfaces/security.json", "The administrator's security settings, read when the user file does not set them."},
		{"~/.config/mint-interfaces/tools.d/", "JSON files of extra facts about programs, read by mint-gen. /etc/mint-interfaces/tools.d/ is the system copy."},
		{"~/.local/share/mint-interfaces/", "The repository in a user install. Generated apps live in its apps/ directory. The launcher is ~/.local/bin/mint-interfaces/mint-gen."},
		{"/usr/local/share/mint-interfaces/", "The repository in a system install. The launcher is /usr/local/bin/mint-gen."},
		{"apps/<tool>/", "The scaffold mint-gen writes inside the repository, and the <tool>-mint binary after --build. --remove and --purge delete the files mint-gen wrote and leave saved results in the folder."},
		{"~/mint-results/<tool>/", "Where an app saves output when it has no directory of its own, or that directory has moved."},
	}
	width := 8
	for _, row := range rows {
		if len(row.path) > width {
			width = len(row.path)
		}
	}
	w.tagList(width)
	for _, row := range rows {
		w.line(".It Pa " + roffQuote(row.path))
		w.prose(row.desc)
	}
	w.endList()
}

func writeManExamples(w *manw) {
	w.sh("EXAMPLES")
	w.prose("From a checkout of the repository, check a program, build its app and run the binary:")
	w.literal(
		"go run ./tools/mint-gen --check nmap",
		"go run ./tools/mint-gen --build nmap",
		"./apps/nmap/nmap-mint",
	)
	w.pp("Build if needed, then run, as one command:")
	w.literal("go run ./tools/mint-gen --run nmap")
	w.pp("Choose the archetype, set every app's colours, see what mint-gen understood, and write this manual:")
	w.literal(
		"go run ./tools/mint-gen --archetype probe --build nmap",
		"go run ./tools/mint-gen --theme nord",
		"go run ./tools/mint-gen --explain ffmpeg",
		"go run ./tools/mint-gen --man > mint-interfaces.1",
		"mandoc -Tutf8 mint-interfaces.1",
	)
	w.pp("After mint-gen --install, the same flags work as mint-gen from any directory. Apps are still written in the repository's apps/ directory.")
}

func writeManSeeAlso(w *manw) {
	w.sh("SEE ALSO")
	w.prose("man(1) and mandoc(1) format this page. MANUAL PAGES, above, explains how this manual, mint-gen --help, and a generated app's --man differ.")
	w.line("See")
	w.line(".Sx \"MANUAL PAGES\" .")
}

func collectFlags(fs *flag.FlagSet) []*flag.Flag {
	if fs == nil {
		fs = newMintOpts().fs
	}
	var flags []*flag.Flag
	fs.VisitAll(func(f *flag.Flag) { flags = append(flags, f) })
	return flags
}

func flagTag(name string) string {
	return "--" + name + ", -" + name
}

func flagTakesValue(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !bf.IsBoolFlag()
}

func flagArgName(name string) string {
	switch name {
	case "check", "explain":
		return "program"
	case "archetype":
		return "archetype"
	case "theme":
		return "name"
	case "out", "root":
		return "dir"
	default:
		return "value"
	}
}
