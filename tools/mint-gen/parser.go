package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

var (
	reManName    = regexp.MustCompile(`^[A-Za-z0-9_.,\s-]+?\s+(?:-+|—|–)\s+(.+)$`)
	reOptLine    = regexp.MustCompile(`^\s{1,12}(-[A-Za-z0-9?]|--[A-Za-z0-9][-A-Za-z0-9_.]*)(?:,\s*(-[A-Za-z0-9?]|--[A-Za-z0-9][-A-Za-z0-9_.]*))*((?:=[<\[\({]?[A-Za-z0-9_|.,/+-]+[>\]\)}]?)|\s+(?:<[^>\s]+>|\[[^\]\s]+\]|\{[^}\s]+\}|[A-Z][A-Z0-9_-]{1,15}))?(?:\s{2,}|\t+|:?\s*$)(.*)$`)
	reTargetHint = regexp.MustCompile(`(?i)\b(target|host|url|uri|domain|address|file|path|pattern|dir|directory|archive|input|interface|device|package|image|container)\b`)
)

// ParseProgramIO is ParseProgram, also returning the input/output model
// (for --explain). The model lists the help that was read and the notes
// on help that could not be, instead of printing them.
func ParseProgramIO(bin string) (appkit.ToolSpec, IOModel) {
	name := filepath.Base(strings.TrimSpace(bin))
	srcs, notes := CollectHelpNotes(bin)
	manText, helpText := manAndHelp(srcs)
	spec, model := ParseManAndHelpIO(name, manText, helpText)
	model.HelpRead = describeHelpSources(srcs)
	model.HelpNotes = notes
	return spec, model
}

// describeHelpSources names each piece of help read, and why when it was
// read because the help pointed to it.
func describeHelpSources(srcs []HelpSource) []string {
	var out []string
	for _, s := range srcs {
		d := s.Name
		if s.Name == "man" {
			d = "man page"
		}
		if s.Why != "" {
			d += " (" + s.Why + ")"
		}
		out = append(out, d)
	}
	return out
}

// ParseProgram reads the program's man page, its --help (or -h) and any
// fuller help it points to (see CollectHelp), and returns a populated
// appkit.ToolSpec.
func ParseProgram(bin string) (appkit.ToolSpec, error) {
	name := filepath.Base(strings.TrimSpace(bin))
	srcs, notes := CollectHelpNotes(bin)
	printHelpNotes(notes)
	manText, helpText := manAndHelp(srcs)
	spec := ParseManAndHelp(name, manText, helpText)
	return spec, nil
}

// ParseManAndHelp extracts a ToolSpec from raw man-page text and/or --help text.
// All lines are cleaned through mint.CleanLine so backspace overstrikes (\b)
// and ANSI escape sequences are normalized first.
func ParseManAndHelp(bin, manText, helpText string) appkit.ToolSpec {
	spec, _ := ParseManAndHelpIO(bin, manText, helpText)
	return spec
}

// ParseManAndHelpIO is ParseManAndHelp, also returning the input/output
// model the spec's output settings came from.
func ParseManAndHelpIO(bin, manText, helpText string) (appkit.ToolSpec, IOModel) {
	name := filepath.Base(strings.TrimSpace(bin))
	titleName := titleWords(strings.ReplaceAll(name, "-", " "))
	spec := appkit.ToolSpec{
		Name:  name,
		Title: "◈ " + titleName,
		Desc:  titleName + " command-line interface",
		// TargetLabel stays empty until the synopsis or the archetype sets
		// one. A generic label here would look explicit to ApplyTo, and a
		// list / watch / generate / control tool would keep a target box
		// it does not take.
		// No extension yet: the archetype may give one, and chooseExtension
		// decides at the end.
	}

	manLines := cleanLines(manText)
	helpLines := cleanLines(helpText)

	// 1. Extract one-line summary from man NAME section or --help header
	if desc := extractSummary(name, manLines, helpLines); desc != "" {
		spec.Desc = desc
	}

	// 2. Extract SYNOPSIS / Usage line to configure target label & placeholder
	if syn := extractSynopsis(name, manLines, helpLines); syn != "" {
		applyTargetFromSynopsis(&spec, syn)
	}

	// 3. Parse flags from both man page and --help, deduplicating by canonical flag
	var flags []appkit.FlagSpec
	seen := map[string]bool{}
	at := map[string]int{} // flag -> index in flags
	// In a program with subcommands, one option can mean different things
	// in different commands (hg's -f), so the first definition stays.
	suite := hasSubcommands(manText, helpText, manLines, helpLines)

	outputDesc := "" // the output option's description, for chooseExtension
	knownMain, hasKnownMain := KnownOption{}, false
	if kt, ok := KnownToolIO(name); ok {
		knownMain, hasKnownMain = kt.mainOutput()
	}
	addFlags := func(parsed []appkit.FlagSpec, optional map[string]bool) {
		for _, f := range parsed {
			if f.Flag == "" || f.Flag == "--help" || f.Flag == "-h" || f.Flag == "--version" || f.Flag == "-help" || f.Flag == "-version" || f.Flag == "--usage" {
				continue
			}
			f.Role, _ = ClassifyArg(name, f)
			// The option that writes the tool's output to a file (see isOutputFileFlag)
			if !spec.OutputsFile && f.Role == appkit.RoleOutputFile && (!hasKnownMain || f.Flag == knownMain.Flag) {
				spec.OutputsFile = true
				spec.OutputFlag = f.Flag
				outputDesc = stripFlagSuffix(f.Label, f.Flag) + " " + f.Hint
				seen[f.Flag] = true
				continue
			}
			if seen[f.Flag] {
				// A short help table often leaves out an option's value
				// ("-t  Use this anacrontab"); a definition that shows
				// the value ("-t anacrontab") replaces it, in place. An
				// optional value ("--json[=false]") is still a switch.
				if i, ok := at[f.Flag]; ok && !suite && flags[i].Kind == appkit.FlagBool && f.Kind != appkit.FlagBool && !optional[f.Flag] {
					flags[i] = f
				}
				continue
			}
			seen[f.Flag] = true
			at[f.Flag] = len(flags)
			flags = append(flags, f)
		}
	}

	if len(helpLines) > 0 {
		addFlags(parseFlagLines(helpLines, false))
	}
	if len(manLines) > 0 {
		addFlags(parseFlagLines(manLines, true))
	}
	// find's numeric tests take "+n" (more than n) and "-n" (less than n),
	// which a number stepper cannot give: when the help explains them, its
	// "n" options stay text.
	if reSignedN.MatchString(manText) || reSignedN.MatchString(helpText) {
		for i := range flags {
			if flags[i].Kind == appkit.FlagInt && ParsePlaceholder(flags[i].Hint).Word == "n" {
				flags[i].Kind, flags[i].Min, flags[i].Max = appkit.FlagText, 0, 0
			}
		}
	}
	// The known-tools list names this tool's output option, but the help
	// did not show it (a short --help, an unusual man page): the list is
	// checked by hand, so it is used as it is.
	if hasKnownMain && !spec.OutputsFile && spec.OutputFlag == "" && !seen[knownMain.Flag] {
		spec.OutputsFile = true
		spec.OutputFlag = knownMain.Flag
		seen[knownMain.Flag] = true
	}

	// Mark flags beyond the first 12 (or in non-primary sections) as Advanced so
	// they automatically fold inside mint.Collapsible sections.
	primaryCount := 0
	for i := range flags {
		secLower := strings.ToLower(flags[i].Section)
		if strings.Contains(secLower, "advanced") ||
			strings.Contains(secLower, "debug") ||
			strings.Contains(secLower, "misc") ||
			strings.Contains(secLower, "evasion") ||
			strings.Contains(secLower, "spoof") {
			flags[i].Advanced = true
			continue
		}
		primaryCount++
		if primaryCount > 12 {
			flags[i].Advanced = true
		}
	}

	flags = append(flags, knownOnlyFlags(name, flags, spec.OutputFlag)...)
	markSecretFlags(flags)
	spec.Flags = flags
	text := programTextFrom(name, manText, helpText)
	choice := chooseArchetype(name, manText, helpText)
	def, _ := appkit.LookupArchetype(string(choice.ID))
	def.ApplyTo(&spec)
	if def.WantsSubcommands() && len(spec.Subcommands) == 0 {
		if subs := extractSubcommands(manLines, helpLines); len(subs) > 0 {
			spec.Subcommands = subs
		}
	}
	archExt := spec.DefaultExt // the archetype's, if any
	appkit.ApplySafetyDefaults(&spec)
	// What the help is about can only raise the data level.
	if lvl, why := topicDataLevel(text); dataRank(lvl) > dataRank(spec.Data) {
		spec.Data, spec.DataReason = lvl, why
	}
	if d, ok := appkit.KnownDiffSpec(spec.BinaryName()); ok && spec.Diff == nil {
		spec.Diff = &d
	}
	if f, ok := appkit.KnownFindingsSpec(spec.BinaryName()); ok && spec.Findings == nil {
		spec.Findings = &f
	}
	if p, ok := appkit.KnownPreviewSpec(spec.BinaryName()); ok && spec.Preview == nil {
		spec.Preview = &p
	}
	if f, ok := appkit.KnownFollowSpec(spec.BinaryName()); ok && spec.Follow == nil {
		spec.Follow = &f
	}
	// Unlisted follow / serve / session tools get their interactive screen
	// only when the help actually named the flag or the prompt.
	attachParsedScreens(&spec, manText+"\n"+helpText)
	if spec.TargetLabel == "" && !spec.HideTarget {
		spec.TargetLabel = "Target / arguments"
	}
	if spec.TargetPlaceholder == "" && !spec.HideTarget {
		spec.TargetPlaceholder = "arguments"
	}
	// Changing a rule order needs the user to review and apply it, and
	// goes back by itself unless kept, so it is safe to offer.
	if r, ok := appkit.KnownReorderSpec(spec.BinaryName()); ok && spec.Reorder == nil {
		spec.Reorder = &r
	}
	// One answer for what the tool reads and writes (see BuildIOModel).
	syn := ParseSynopsis(name, manLines, helpLines)
	model := BuildIOModel(spec, syn)
	applyIOModel(&spec, model)
	if len(syn.Forms) > 0 {
		spec.PositionalRoles = make([]appkit.ArgRole, len(syn.Forms[0].Positionals))
		for i, p := range syn.Forms[0].Positionals {
			spec.PositionalRoles[i] = p.Role
		}
	}
	model.Ext, model.ExtWhy = chooseExtension(spec, model, outputDesc, archExt)
	model.Archetype = choice
	spec.DefaultExt = model.Ext
	attachEnvSecrets(&spec, extractEnvSecrets(manLines, helpLines))
	// What the help shows clearly about how the program behaves: caution
	// and handling on top of the archetype (see signals.go).
	model.Signals = readSignals(name, manText, helpText)
	applySignals(&spec, model.Signals)
	// A full-screen program with a batch mode runs in that mode instead
	// (see batch.go).
	if rule, ok := curatedExclusions[name]; (ok && rule.Category == CatFullScreenTUI) || model.Signals.FullScreen {
		if b, ok := findBatchMode(name, spec); ok {
			applyBatchMode(&spec, b, choice.Confidence == ConfidenceListed, model.Signals.Changes, manText+"\n"+helpText)
			model.Batch = &b
			model.Signals.FullScreen, model.Signals.MaybeFullScreen = false, false
		}
	}
	model.Notes = signalNotes(appkit.ArchetypeID(spec.ArchetypeID), model.Signals)
	if model.Batch != nil {
		model.Notes = append([]string{batchNote(name, *model.Batch)}, model.Notes...)
	}
	return spec, model
}

var (
	// reEnvName finds an environment variable name, with or without $ or
	// backticks: GH_TOKEN, $RESTIC_PASSWORD, `PGPASSWORD`.
	reEnvName = regexp.MustCompile("`?\\$?\\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*)\\b`?")
	// envSecretParts mark a variable that holds a secret itself.
	envSecretParts = []string{"PASSWORD", "PASSWD", "PASSPHRASE", "PASS", "TOKEN", "SECRET", "APIKEY", "API_KEY", "CREDENTIAL"}
	// envNotSecret are variables naming where a secret is, not the secret.
	envNotSecret = []string{"_FILE", "_COMMAND", "_CMD", "_PATH", "_DIR", "_HELPER", "_PROMPT"}
)

func envNameLooksSecret(name string) bool {
	for _, suffix := range envNotSecret {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}
	for _, p := range envSecretParts {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// extractEnvSecrets finds environment variables that carry a secret, from
// the man page's ENVIRONMENT section and any line of the man page or help
// that talks about environment variables.
func extractEnvSecrets(manLines, helpLines []string) []string {
	var out []string
	seen := map[string]bool{}
	scan := func(lines []string, isMan bool) {
		inEnv := false
		for _, l := range lines {
			if isMan && isManSectionHeader(l) {
				inEnv = strings.HasPrefix(strings.TrimSpace(l), "ENVIRONMENT")
				continue
			}
			if !inEnv && !strings.Contains(strings.ToLower(l), "environment") {
				continue
			}
			for _, m := range reEnvName.FindAllStringSubmatch(l, -1) {
				name := m[1]
				if len(name) >= 4 && !seen[name] && envNameLooksSecret(name) {
					seen[name] = true
					out = append(out, name)
				}
			}
		}
	}
	scan(manLines, true)
	scan(helpLines, false)
	return out
}

// secretClass groups secrets by what they are, so a variable is only
// matched to an option holding the same kind of secret.
func secretClass(text string) string {
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "pass"):
		return "password"
	case strings.Contains(t, "token"):
		return "token"
	case strings.Contains(t, "key"):
		return "key"
	case strings.Contains(t, "secret"):
		return "secret"
	}
	return ""
}

// attachEnvSecrets routes command-line secrets through environment
// variables found in the documentation. A variable replaces an option only
// when exactly one option holds the same kind of secret; otherwise it is
// added as its own secret entry, so nothing is guessed wrongly.
func attachEnvSecrets(spec *appkit.ToolSpec, names []string) {
	for _, name := range names {
		if hasSecretEnv(spec, name) {
			continue
		}
		class := secretClass(name)
		match := -1
		for i, f := range spec.Flags {
			// lftp's "-u user[,pass]" holds a user name too: the
			// variable (the password alone) stays a separate entry.
			combined := strings.Contains(strings.ToLower(f.Hint), "user") || strings.ContainsAny(f.Hint, ",:@")
			if f.Kind == appkit.FlagSecret && f.SecretRoute() == appkit.RouteArgument && !combined &&
				class != "" && secretClass(f.Flag+" "+f.Label) == class {
				if match >= 0 {
					match = -2 // more than one: don't guess
					break
				}
				match = i
			}
		}
		if match >= 0 {
			spec.Flags[match].SecretEnv = name
			continue
		}
		spec.Flags = append(spec.Flags, appkit.FlagSpec{Kind: appkit.FlagSecret, Section: "Credentials",
			Label: envLabel(name), SecretEnv: name})
	}
}

func hasSecretEnv(spec *appkit.ToolSpec, name string) bool {
	for _, f := range spec.Flags {
		if f.SecretEnv == name {
			return true
		}
	}
	return false
}

// envLabel turns RESTIC_PASSWORD into "Restic password ($RESTIC_PASSWORD)".
func envLabel(name string) string {
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(name, "_", " ")))
	return capitalizeFirst(strings.Join(words, " ")) + " ($" + name + ")"
}

// markSecretFlags turns options whose value is a password, token or key into
// FlagSecret, so the interface asks for them with a password dialog and never
// saves them. A bare --password-stdin style option becomes a secret sent on
// standard input. Options naming a file (such as --password-file) are left as
// paths: the file name itself is not secret.
func markSecretFlags(flags []appkit.FlagSpec) {
	for i := range flags {
		f := &flags[i]
		if !appkit.LooksSecret(f.Flag, f.Label, f.Hint) {
			continue
		}
		switch {
		case f.Kind == appkit.FlagBool && strings.HasSuffix(f.Flag, "-stdin"):
			f.Kind, f.SecretStdin = appkit.FlagSecret, true
		case f.Kind == appkit.FlagText || f.Kind == appkit.FlagInt:
			f.Kind, f.Choices, f.Min, f.Max = appkit.FlagSecret, nil, 0, 0
		// "-upw <string>  user password (for encrypted files)": the
		// description's "files" made it a file, but its value is the
		// password itself. A password file keeps a path placeholder
		// (--password-file FILE) and stays a file.
		case (f.Kind == appkit.FlagFile || f.Kind == appkit.FlagDir) && !ParsePlaceholder(f.Hint).IsFile():
			f.Kind, f.Choices, f.Min, f.Max = appkit.FlagSecret, nil, 0, 0
		}
	}
}

func cleanLines(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	rawLines := strings.Split(raw, "\n")
	out := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		out = append(out, mint.CleanLine(l))
	}
	return out
}

func extractSummary(name string, manLines, helpLines []string) string {
	// The man page's NAME line, the help's first description, then the
	// first sentence of the man page's DESCRIPTION: the first that says
	// something ("Hyperfine", "DNF Command Reference" and "Supervisorctl
	// Documentation" do not), else the first there is.
	cands := []string{manNameSummary(manLines), helpSummary(name, helpLines), manDescriptionSentence(manLines)}
	for _, c := range cands {
		if informativeSummary(name, c) {
			return c
		}
	}
	// None says much: the one that says most ("A command-line
	// benchmarking tool" over "Hyperfine"), the first on a tie.
	best, most := "", -1
	for _, c := range cands {
		if n := summaryWords(name, c); c != "" && n > most {
			best, most = c, n
		}
	}
	return best
}

func manNameSummary(manLines []string) string {
	inName := false
	for _, l := range manLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "NAME" {
			inName = true
			continue
		}
		if inName {
			if trimmed == "" {
				continue
			}
			if isManSectionHeader(l) {
				break
			}
			if m := reManName.FindStringSubmatch(trimmed); m != nil {
				return capitalizeFirst(strings.TrimSpace(m[1]))
			}
			return capitalizeFirst(trimmed)
		}
	}
	return ""
}

// helpSummary is the first descriptive line of the help.
func helpSummary(name string, helpLines []string) string {
	for _, l := range helpLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "usage:") || strings.HasPrefix(lower, "synopsis") {
			continue
		}
		// The summary comes before the options and before any section
		// ("Commands:"), not from a line of a section's list.
		if strings.HasPrefix(trimmed, "-") || isHelpSectionHeader(l) {
			break
		}
		if !looksLikeSummary(name, trimmed) {
			continue
		}
		if m := reManName.FindStringSubmatch(trimmed); m != nil {
			return capitalizeFirst(strings.TrimSpace(m[1]))
		}
		if len(trimmed) > 8 && len(trimmed) < 120 {
			return capitalizeFirst(trimmed)
		}
	}
	return ""
}

// extractDescription is the opening of the man page's DESCRIPTION: its
// first paragraph, at most about 600 characters. It says what a program
// is ("CVS is a version control system…"); the rest of a long man page
// mentions much that it is not.
func extractDescription(manLines []string) string {
	text := manDescriptionParagraph(manLines)
	if len(text) > 600 {
		if cut := strings.LastIndex(text[:600], ". "); cut > 100 {
			text = text[:cut+1]
		} else {
			text = text[:strings.LastIndexByte(text[:600], ' ')]
		}
	}
	return text
}

// manDescriptionSentence is the first sentence of the man page's
// DESCRIPTION, at most about 160 characters.
func manDescriptionSentence(manLines []string) string {
	text := manDescriptionParagraph(manLines)
	if i := strings.Index(text, ". "); i >= 0 {
		text = text[:i]
	}
	text = strings.TrimRight(text, ".:")
	if len(text) > 160 {
		cut := strings.LastIndexByte(text[:160], ' ')
		if cut < 40 {
			cut = 160
		}
		text = text[:cut]
	}
	if len(text) < 10 || !reHasLower.MatchString(text) {
		return ""
	}
	return capitalizeFirst(text)
}

// manDescriptionParagraph is the first paragraph of the man page's
// DESCRIPTION, as one line.
func manDescriptionParagraph(manLines []string) string {
	var para []string
	in := false
	for _, l := range manLines {
		trimmed := strings.TrimSpace(l)
		if !in {
			in = trimmed == "DESCRIPTION" && isManSectionHeader(l)
			continue
		}
		if isManSectionHeader(l) || (trimmed == "" && len(para) > 0) {
			break
		}
		if trimmed == "" {
			continue
		}
		// A word split at the line end ("dis-" / "tributions") is joined.
		if n := len(para); n > 0 && strings.HasSuffix(para[n-1], "-") && trimmed[0] >= 'a' && trimmed[0] <= 'z' {
			para[n-1] = strings.TrimSuffix(para[n-1], "-") + trimmed
		} else {
			para = append(para, trimmed)
		}
	}
	return strings.Join(strings.Fields(strings.Join(para, " ")), " ")
}

// Words that say nothing about what a program does.
var summaryFiller = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "for": true, "and": true, "to": true,
	"documentation": true, "docs": true, "reference": true, "command": true, "commands": true,
	"manual": true, "man": true, "page": true, "pages": true, "program": true, "utility": true,
	"tool": true, "cli": true, "line": true, "version": true,
}

var reWord = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*`)

// informativeSummary reports whether a summary has at least two words
// besides the program's name and filler.
func informativeSummary(name, s string) bool {
	return summaryWords(name, s) >= 2
}

// summaryWords counts a summary's words besides the program's name and
// filler.
func summaryWords(name, s string) int {
	self := map[string]bool{strings.ToLower(name): true}
	for _, part := range regexp.MustCompile(`[-_.]+`).Split(strings.ToLower(name), -1) {
		self[part] = true
	}
	n := 0
	for _, w := range reWord.FindAllString(strings.ToLower(s), -1) {
		if !self[w] && !summaryFiller[w] {
			n++
		}
	}
	return n
}

var (
	// "   o   -oL ..." / "   *  --foo": a bullet before an option.
	reBulletOption = regexp.MustCompile(`^(\s{0,12})([o*\x{2022}\x{00B7}]\s{1,4})(--?[A-Za-z0-9].*)$`)
	// The ":" right after an option's value in such a list: "-oL <filename>: sets".
	reBulletColon = regexp.MustCompile(`^\s*--?[A-Za-z0-9][-A-Za-z0-9]*\s+(?:<[^>\s]+>|[A-Z][A-Z0-9_-]+|[a-z][a-z0-9_-]*)(:\s+)\S`)
	reVersionLine = regexp.MustCompile(`(?i)\b(?:version|v)\s*\d+(?:\.\d+)+|copyright|\(c\)\s*\d{4}`)
	reHasLower    = regexp.MustCompile(`[a-z]`)
)

// looksLikeSummary reports whether a help line can describe the program:
// not another usage line ("Or:  ln [OPTION]... TARGET", "top [options]"),
// a version or copyright line, a heading in capitals, or a section label
// ending in ":" ("Input options:").
func looksLikeSummary(name, line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	if strings.HasPrefix(lower, "or:") || strings.HasPrefix(lower, "usage") {
		return false
	}
	// "supervisorctl -- control applications…": the name and a description.
	if f := strings.ToLower(strings.Fields(t)[0]); (f == strings.ToLower(name) || strings.HasSuffix(f, "/"+strings.ToLower(name))) && reManName.MatchString(t) && !reVersionLine.MatchString(t) {
		return true
	}
	// A usage fragment: "((-u|--update) | [-p|--platform PLATFORM]".
	if strings.ContainsAny(t[:1], "([{<|") || strings.Count(t, "|")+strings.Count(t, "[")+strings.Count(t, "<") >= 3 {
		return false
	}
	// "top [options]", "/usr/bin/ln [OPTION]...": a usage line, unless a
	// sentence follows the name ("tar saves many files…" is fine).
	first := strings.Fields(t)[0]
	if fl := strings.ToLower(first); fl == strings.ToLower(name) || strings.HasSuffix(fl, "/"+strings.ToLower(name)) {
		rest := strings.TrimSpace(t[len(first):])
		if rest == "" || strings.ContainsAny(rest[:1], "[<{-|") {
			return false
		}
	}
	if reVersionLine.MatchString(t) {
		return false
	}
	if !reHasLower.MatchString(t) {
		return false // a heading in capitals
	}
	return !strings.HasSuffix(t, ":") // a section label ("Input options:")
}

func extractSynopsis(name string, manLines, helpLines []string) string {
	inSyn := false
	for _, l := range manLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "SYNOPSIS" {
			inSyn = true
			continue
		}
		if inSyn {
			if trimmed == "" {
				continue
			}
			if isManSectionHeader(l) {
				break
			}
			return trimmed
		}
	}
	for _, l := range helpLines {
		trimmed := strings.TrimSpace(l)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "usage:") {
			return strings.TrimSpace(trimmed[len("usage:"):])
		}
	}
	return ""
}

func applyTargetFromSynopsis(spec *appkit.ToolSpec, syn string) {
	if m := reTargetHint.FindStringSubmatch(syn); m != nil {
		word := strings.ToLower(m[1])
		switch word {
		case "target", "host", "domain", "address":
			spec.TargetLabel = "Target host / network"
			spec.TargetPlaceholder = "example.com or 192.168.1.0/24"
		case "url", "uri":
			spec.TargetLabel = "Target URL"
			spec.TargetPlaceholder = "https://example.com"
		case "file", "path", "input", "archive":
			spec.TargetLabel = "Input file / path"
			spec.TargetPlaceholder = "/path/to/file"
		case "dir", "directory":
			spec.TargetLabel = "Directory"
			spec.TargetPlaceholder = "."
		case "pattern":
			spec.TargetLabel = "Pattern & target"
			spec.TargetPlaceholder = "pattern [file...]"
		case "interface", "device":
			spec.TargetLabel = "Interface / device"
			spec.TargetPlaceholder = "eth0"
		}
	}
}

func parseFlagsFromLines(lines []string, isMan bool) []appkit.FlagSpec {
	out, _ := parseFlagLines(lines, isMan)
	return out
}

// parseFlagLines is parseFlagsFromLines, also returning the options whose
// value is optional ("--json[=false]", "--list-catalog [ID...]").
func parseFlagLines(lines []string, isMan bool) ([]appkit.FlagSpec, map[string]bool) {
	var out []appkit.FlagSpec
	optional := map[string]bool{}
	section := "Options"
	inIgnoreSection := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect section headers
		if isMan && isManSectionHeader(line) {
			secUpper := trimmed
			if isIgnoredManSection(secUpper) {
				inIgnoreSection = true
			} else {
				inIgnoreSection = false
				section = normalizeSectionName(secUpper)
			}
			continue
		}
		if !isMan && isHelpSectionHeader(line) {
			section = normalizeSectionName(strings.TrimSuffix(trimmed, ":"))
			continue
		}
		if inIgnoreSection {
			continue
		}

		// A bulleted option list ("o   -oL <filename>: sets the output
		// format…", as in masscan's man page) is read like a plain one: the
		// bullet becomes indent, and the colon after the value a gap.
		if m := reBulletOption.FindStringSubmatch(line); m != nil {
			line = m[1] + strings.Repeat(" ", len(m[2])) + m[3]
			if b := reBulletColon.FindStringSubmatchIndex(line); b != nil {
				line = line[:b[2]] + "  " + line[b[3]:]
			}
		}

		var groups [][]string // [whole, first flag, second flag, argument, description]
		if m := reOptLine.FindStringSubmatch(line); m != nil {
			groups = append(groups, m)
		} else {
			// Lower-case placeholders and slash groups (see scanOptionLine).
			for _, o := range scanOptionLine(line) {
				second := ""
				if len(o.Names) > 1 {
					second = o.Names[1]
				}
				groups = append(groups, []string{line, o.Names[0], second, o.Placeholder.Raw, o.Desc})
			}
		}
		if len(groups) == 0 {
			continue
		}
		m := groups[0]

		firstFlag := strings.TrimSpace(m[1])
		secondFlag := strings.TrimSpace(m[2])
		argPart := strings.TrimSpace(m[3])
		desc := strings.TrimSpace(m[4])

		// Choose canonical flag (prefer short flag unless long flag is the only one or clearer)
		canonFlag := firstFlag
		if secondFlag != "" && strings.HasPrefix(firstFlag, "-") && !strings.HasPrefix(firstFlag, "--") {
			canonFlag = firstFlag
		} else if secondFlag != "" {
			canonFlag = secondFlag
		}

		// If description is empty on the option line, look ahead 1–2 indented continuation lines
		for j := i + 1; j < len(lines) && j <= i+2; j++ {
			next := lines[j]
			nextTrim := strings.TrimSpace(next)
			if nextTrim == "" || isManSectionHeader(next) || looksLikeOptionLine(next) {
				break
			}
			if strings.HasPrefix(next, "     ") || strings.HasPrefix(next, "\t") {
				if desc == "" {
					desc = nextTrim
				} else if len(desc) < 60 {
					desc += " " + nextTrim
				}
				i = j
			} else {
				break
			}
		}

		// A slash group (-oN/-oX/-oG <file>) gives one option per name,
		// sharing the description.
		for gi, g := range groups {
			if gi > 0 {
				canonFlag, secondFlag, argPart = strings.TrimSpace(g[1]), strings.TrimSpace(g[2]), strings.TrimSpace(g[3])
			}
			join := " "
			arg := argPart
			if isFixedValueAlias(firstFlag, secondFlag, arg) {
				// "-p, --indicator-style=slash": -p is a shortcut for that
				// fixed setting, so it is a switch, not an option with a value.
				canonFlag, arg = firstFlag, ""
			}
			if strings.HasPrefix(arg, "=") {
				join = "="
				arg = strings.TrimPrefix(arg, "=")
			}

			kind, choices, minVal, maxVal := classifyFlag(canonFlag, arg, desc)
			label := cleanFlagLabel(canonFlag, secondFlag, desc)
			hint := enrichRepeatableHint(cleanArgHint(arg), desc)

			if arg != "" && ParsePlaceholder(arg).Optional {
				optional[canonFlag] = true
			}
			out = append(out, appkit.FlagSpec{
				Flag:    canonFlag,
				Join:    join,
				Kind:    kind,
				Section: section,
				Label:   label,
				Hint:    hint,
				Choices: choices,
				Min:     minVal,
				Max:     maxVal,
				Step:    1,
			})
		}
	}
	return out, optional
}

func classifyFlag(flag, argPart, desc string) (appkit.FlagKind, []appkit.Choice, int, int) {
	argClean := strings.TrimSpace(argPart)
	if argClean == "" {
		return appkit.FlagBool, nil, 0, 0
	}

	// Check for inline choice list like {a|b|c} or (a|b|c)
	trimmedArg := strings.Trim(argClean, "[]<>(){}")
	if strings.Contains(trimmedArg, "|") && !strings.Contains(trimmedArg, " ") {
		parts := strings.Split(trimmedArg, "|")
		if len(parts) >= 2 && len(parts) <= 8 {
			choices := []appkit.Choice{{Label: "Default", Value: ""}}
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					choices = append(choices, appkit.Choice{Label: p, Value: p})
				}
			}
			return appkit.FlagChoice, choices, 0, 0
		}
	}

	// A placeholder that clearly is a path settles it. One that clearly is
	// not (URL, PATTERN, FORMAT, SECONDS…) rules out a file picker, however
	// the description reads ("File format…"); numbers are still checked below.
	ph := ParsePlaceholder(argClean)
	if ph.IsFile() {
		if reDirWord.MatchString(ph.Word) || reDirWord.MatchString(lastWordPart(ph.Word)) {
			return appkit.FlagDir, nil, 0, 0
		}
		return appkit.FlagFile, nil, 0, 0
	}

	combined := strings.ToLower(argClean + " " + desc)
	// The description decides only when the placeholder is vague or missing:
	// NMERGE, CHANGES or LABEL next to "…files" is not a file.
	// Whole words only: "direction", "redirect", "profile", "filesystem"
	// and "account" are not folders, files or counts.
	// A one-letter placeholder ("-type c", "-n n") is almost never a path,
	// whatever the description says ("File is of type c"); F is
	// (--files0-from=F).
	if !ph.IsNotFile() && !ph.Specific() && (len(ph.Word) != 1 || ph.Word == "f") {
		if reDescDir.MatchString(combined) {
			return appkit.FlagDir, nil, 0, 0
		}
		if reDescFile.MatchString(combined) {
			return appkit.FlagFile, nil, 0, 0
		}
	}
	if strings.Contains(argClean, "0-5") {
		return appkit.FlagInt, nil, 0, 5
	}
	// A value with a unit suffix ("10K", "5M", "1.5") is not a whole
	// number, whatever the placeholder says (shred -s N).
	if reUnitSuffix.MatchString(desc) {
		return appkit.FlagText, nil, 0, 0
	}
	// A placeholder that names a number (N, NUM, COUNT, SECONDS, PORT, #)
	// is a number, whatever the description says. Its range is open: a
	// count, a port or a timeout is not limited to 0-100.
	if numericPlaceholder(ph.Word) {
		return appkit.FlagInt, nil, 0, 0
	}
	// The description decides otherwise ("number of requests", "timeout"),
	// unless the placeholder names something that is not a plain number:
	// fd's "-S size" takes "10k", skopeo's "duration" takes "30s", and
	// "-x command" is a command.
	if reDescNumber.MatchString(combined) && !notNumberWords[ph.Word] && !notNumberWords[lastWordPart(ph.Word)] && !strings.ContainsAny(argClean, "{}") {
		return appkit.FlagInt, nil, 0, 0
	}

	return appkit.FlagText, nil, 0, 0
}

// numericWords name a whole number: a count, a time in whole units, a size
// in lines, a port. "size", "time", "limit" and "max" are left out: they
// often take "10M", "5m" or "50%" (xz --memlimit).
var numericWords = map[string]bool{
	"n": true, "num": true, "number": true, "int": true, "integer": true, "count": true, "cnt": true,
	"sec": true, "secs": true, "second": true, "seconds": true, "ms": true, "msec": true, "msecs": true,
	"millis": true, "milliseconds": true, "usec": true, "usecs": true, "minutes": true, "mins": true,
	"hours": true, "days": true, "lines": true, "depth": true, "port": true, "width": true, "height": true,
	"rows": true, "retries": true, "tries": true, "threads": true, "jobs": true,
	"workers": true, "percent": true, "pct": true, "#": true, "#n": true, "digits": true, "times": true,
	"int64": true, "uint64": true, "uint32": true,
	"iterations": true, "runs": true, "concurrency": true,
}

// reSignedN is a help explaining "+n" or "-n" values: "+n  for greater
// than n" (find).
var reSignedN = regexp.MustCompile(`(?im)^\s*[+-]n\b.*\b(?:greater|more|less|fewer)\b`)

// reUnitSuffix is a description saying the value takes a unit suffix or a
// fraction: "suffixes like K, M, G", "may be followed by a unit", "a
// floating-point number".
var reUnitSuffix = regexp.MustCompile(`(?i)\bsuffix(?:es)?\b|\b[kmg]\s*,\s*[mgt]\b|\b(?:kib|mib|gib|kb|mb|gb)\b|\bfollowed by (?:a |an )?(?:unit|multiplier|suffix)|\bfloat(?:ing[- ]point)?\b|\bdecimal (?:number|fraction)\b`)

// notNumberWords name values that are not plain whole numbers, though the
// description may talk of counts or times: sizes ("10M"), durations
// ("30s"), commands, names, patterns, formats.
var notNumberWords = map[string]bool{
	"size": true, "sizes": true, "duration": true, "time": true, "command": true, "cmd": true,
	"program": true, "format": true, "fmt": true, "name": true, "string": true, "str": true, "text": true,
	"pattern": true, "regex": true, "regexp": true, "url": true, "uri": true, "host": true, "hostname": true,
	"list": true, "type": true, "mode": true, "user": true, "key": true, "id": true, "label": true,
	"template": true, "expr": true, "expression": true, "script": true, "glob": true, "method": true,
	"signal": true, "policy": true, "style": true, "algorithm": true, "cipher": true, "action": true,
	"date": true, "charset": true, "encoding": true, "locale": true, "lang": true, "when": true, "local": true,
}

// numericPlaceholder reports whether a placeholder word names a number:
// "num", "count", or a compound ending in one ("max-count", "line_count").
func numericPlaceholder(w string) bool {
	return w != "" && (numericWords[w] || numericWords[lastWordPart(w)])
}

var (
	reDescDir    = regexp.MustCompile(`(?i)\b(?:dir|dirs|directory|directories|folder|folders)\b`)
	reDescFile   = regexp.MustCompile(`(?i)\b(?:file|files|filename|filenames|path|paths|pathname)\b|\bconfig(?:uration)?\s+file\b`)
	reDescNumber = regexp.MustCompile(`(?i)<(?:num|int|count|seconds|n)>|\bnumber\s+of\b|\b(?:count|timeout|threads|retries|seconds|milliseconds)\b`)
)

// outputFlagNames are the usual names of an output-file option. One of these
// is accepted when its value is a file; any other option only when its
// description says the tool's output is written to the file.
var outputFlagNames = map[string]bool{
	"-o": true, "-O": true, "--output": true, "--output-file": true, "--output-document": true,
	"--out": true, "--outfile": true, "--out-file": true, "-w": true, "--write": true, "-oN": true,
}

var (
	// A value that names a file or folder.
	reFileArg = regexp.MustCompile(`(?i)^(?:file|files|filename|fname|basename|outfile|out_?file|output_?file|out|path|filepath|dir|directory|folder|archive|dest|destination|target_?file)s?$`)
	// A value that is something else: a number, time, size, format, name…
	reNonFileArg = regexp.MustCompile(`(?i)^(?:n|num|number|int|integer|count|cols|columns|width|height|size|bytes|lines|sec|secs|seconds|ms|millis|minutes|time|timeout|duration|delay|wait|level|depth|port|format|fmt|template|string|str|text|pattern|regex|regexp|name|user|host|hostname|url|uri|id|key|lang|locale|color|colour|mode|type|when|rate|limit|percent|charset|encoding|suffix|suf|ext|extension|method|signal|control|policy|style|algorithm|cipher|ciphers|list|action|label|glob|globs|expr|expression|changes|date|program|command|cmd|script)$`)
	// Description words for a path, and for things that are not paths.
	reFileWords    = regexp.MustCompile(`(?i)\b(?:file|files|filename|path|directory|folder)\b`)
	reNonFileWords = regexp.MustCompile(`(?i)\b(?:width|columns|seconds|wait|number of|format|level|count|size|timeout|delay|limit)\b`)
	// Writing the tool's main output, as opposed to reading input or a log.
	reWritesOutput = regexp.MustCompile(`(?i)\b(?:write|writes|save|saves|place|places|store|dump|output)\b`)
	// "write the output to", "Send output (stdout) from podman to a file",
	// "Redirect the output of podman to the specified path".
	reMainOutput = regexp.MustCompile(`(?i)\b(?:write|save|place|store|send|redirect)s?\s+(?:the\s+)?(?:output|result|results|documents?|data)(?:\s+\([^)]{1,20}\))?(?:\s+(?:of|from)\s+[\w.-]+)?\s+(?:to|into|in)\b`)
	reNotMainOut = regexp.MustCompile(`(?i)\b(?:log|logs|logging|messages|read|reads|input|load|config|configuration)\b`)
)

// argIsFile reports whether an option's value is a file or folder path:
// from its placeholder (FILE, <file>, PATH, identity_file…), or, when the
// placeholder says nothing either way, from its description.
func argIsFile(hint, desc string) bool {
	p := ParsePlaceholder(hint)
	if p.IsFile() {
		return true
	}
	if p.IsNotFile() || p.Specific() {
		return false
	}
	return reFileWords.MatchString(desc) && !reNonFileWords.MatchString(desc)
}

// argIsNotFile reports whether an option's value is clearly not a path: a
// number, time, size, width, format, name… (ls -w COLS, wget -w SECONDS,
// curl -w <format>).
func argIsNotFile(hint, desc string) bool {
	p := ParsePlaceholder(hint)
	if p.IsNotFile() {
		return true
	}
	if p.IsFile() {
		return false
	}
	if p.Specific() {
		return true // a specific word that is not a path word
	}
	return reNonFileWords.MatchString(desc) && !reFileWords.MatchString(desc)
}

// isOutputFileFlag reports whether f is the option that writes the tool's
// main output to a file (so the app passes its output path there): its
// role, decided by ClassifyArg, is RoleOutputFile.
func isOutputFileFlag(tool string, f appkit.FlagSpec) bool {
	role, _ := ClassifyArg(tool, f)
	return role == appkit.RoleOutputFile
}

// stripFlagSuffix drops the " (-x)" mint-gen adds to labels.
func stripFlagSuffix(label, flag string) string {
	return strings.TrimSpace(strings.TrimSuffix(label, "("+flag+")"))
}

func cleanFlagLabel(flag, altFlag, desc string) string {
	desc = strings.TrimSpace(desc)
	if desc != "" {
		// Keep first sentence up to 64 chars
		if idx := strings.Index(desc, ". "); idx > 0 && idx < 64 {
			desc = desc[:idx]
		}
		if len(desc) > 64 {
			desc = strings.TrimSpace(desc[:61]) + "..."
		}
		return capitalizeFirst(desc) + " (" + flag + ")"
	}
	if altFlag != "" {
		return flag + ", " + altFlag
	}
	return flag
}

func cleanArgHint(argPart string) string {
	h := strings.TrimSpace(argPart)
	h = strings.Trim(h, "=[]<>()")
	if h == "" {
		return "value"
	}
	return strings.ToLower(h)
}

func enrichRepeatableHint(hint, desc string) string {
	lower := strings.ToLower(hint + " " + desc)
	if strings.Contains(lower, "key=val") || strings.Contains(lower, "k=v") || strings.Contains(lower, "name=value") {
		if !strings.Contains(strings.ToLower(hint), "key=val") {
			return "key=value"
		}
	}
	if strings.Contains(lower, "can be specified multiple times") ||
		strings.Contains(lower, "may be given multiple times") ||
		strings.Contains(lower, "repeatable") {
		if !strings.Contains(strings.ToLower(hint), "repeatable") {
			return hint + " (repeatable)"
		}
	}
	return hint
}

func isManSectionHeader(line string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '-' {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 || len(trimmed) > 48 {
		return false
	}
	hasLetter := false
	for _, r := range trimmed {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

func isIgnoredManSection(sec string) bool {
	switch sec {
	case "NAME", "SYNOPSIS", "SEE ALSO", "AUTHOR", "AUTHORS", "BUGS",
		"REPORTING BUGS", "COPYRIGHT", "HISTORY", "COLOPHON", "NOTES", "EXAMPLES":
		return true
	}
	return false
}

func isHelpSectionHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasSuffix(trimmed, ":") || strings.HasPrefix(trimmed, "-") {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "usage:") {
		return false
	}
	return len(trimmed) >= 4 && len(trimmed) <= 44
}

func normalizeSectionName(raw string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(raw, ":"))
	if raw == "" || strings.EqualFold(raw, "DESCRIPTION") {
		return "Options"
	}
	return titleWords(strings.ToLower(raw))
}

func titleWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func capitalizeFirst(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var reLiteralValue = regexp.MustCompile(`^=[a-z][a-z0-9-]*$`)

// isFixedValueAlias reports whether a line like "-p, --indicator-style=slash"
// or "-C, --check=quiet" names a short switch that stands for a long option
// set to a fixed value: the short option has no value of its own and the
// long one's value is a lower-case word, not a placeholder (FILE, WHEN…).
func isFixedValueAlias(first, second, arg string) bool {
	return strings.HasPrefix(first, "-") && !strings.HasPrefix(first, "--") && len(first) == 2 &&
		strings.HasPrefix(second, "--") && reLiteralValue.MatchString(arg) &&
		!ParsePlaceholder(arg).IsFile()
}

// knownOnlyFlags turns options the known-tools list names, but the help
// text did not show in a form mint-gen could read, into fields with their
// roles (pandoc -t, ffmpeg -i), so the app offers them.
func knownOnlyFlags(tool string, have []appkit.FlagSpec, outputFlag string) []appkit.FlagSpec {
	kt, ok := KnownToolIO(tool)
	if !ok {
		return nil
	}
	seen := map[string]bool{outputFlag: true}
	for _, f := range have {
		seen[f.Flag] = true
	}
	var out []appkit.FlagSpec
	for _, o := range kt.Options {
		if seen[o.Flag] {
			continue
		}
		role := appkit.ArgRole(o.Role)
		kind := appkit.FlagText
		switch role {
		case appkit.RoleInputFile, appkit.RoleConfigFile, appkit.RoleFile:
			kind = appkit.FlagFile
		case appkit.RoleInputDir, appkit.RoleOutputDir, appkit.RoleDir:
			kind = appkit.FlagDir
		}
		out = append(out, appkit.FlagSpec{Flag: o.Flag, Kind: kind, Role: role, Section: "Options",
			Label: capitalizeFirst(roleName(role)) + " (" + o.Flag + ")", Step: 1})
	}
	return out
}

// helpNotesTo is where notes about unreadable help go (tests replace it).
var helpNotesTo io.Writer = os.Stderr

func printHelpNotes(notes []string) {
	for _, n := range notes {
		fmt.Fprintln(helpNotesTo, "mint-gen: "+safeTerminalText(n))
	}
}
