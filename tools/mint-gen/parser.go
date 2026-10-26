package main

import (
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

// ParseProgram reads the program's man page, its --help (or -h) and any
// fuller help it points to (see CollectHelp), and returns a populated
// appkit.ToolSpec.
func ParseProgram(bin string) (appkit.ToolSpec, error) {
	name := filepath.Base(strings.TrimSpace(bin))
	manText, helpText := manAndHelp(CollectHelp(bin))
	spec := ParseManAndHelp(name, manText, helpText)
	return spec, nil
}

// ParseManAndHelp extracts a ToolSpec from raw man-page text and/or --help text.
// All lines are cleaned through mint.CleanLine so backspace overstrikes (\b)
// and ANSI escape sequences are normalized first.
func ParseManAndHelp(bin, manText, helpText string) appkit.ToolSpec {
	name := filepath.Base(strings.TrimSpace(bin))
	titleName := titleWords(strings.ReplaceAll(name, "-", " "))
	spec := appkit.ToolSpec{
		Name:              name,
		Title:             "◈ " + titleName,
		Desc:              titleName + " command-line interface",
		TargetLabel:       "Target / arguments",
		TargetPlaceholder: "arguments",
		DefaultExt:        "txt",
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

	addFlags := func(parsed []appkit.FlagSpec) {
		for _, f := range parsed {
			if f.Flag == "" || f.Flag == "--help" || f.Flag == "-h" || f.Flag == "--version" {
				continue
			}
			f.Role, _ = ClassifyArg(name, f)
			// The option that writes the tool's output to a file (see isOutputFileFlag)
			if !spec.OutputsFile && f.Role == appkit.RoleOutputFile {
				spec.OutputsFile = true
				spec.OutputFlag = f.Flag
				spec.DefaultExt = guessExtension(name, f.Label+" "+f.Hint)
				seen[f.Flag] = true
				continue
			}
			if seen[f.Flag] {
				continue
			}
			seen[f.Flag] = true
			flags = append(flags, f)
		}
	}

	if len(helpLines) > 0 {
		addFlags(parseFlagsFromLines(helpLines, false))
	}
	if len(manLines) > 0 {
		addFlags(parseFlagsFromLines(manLines, true))
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

	markSecretFlags(flags)
	spec.Flags = flags
	tpl := DetectTemplate(name, spec, manText, helpText)
	tpl.ApplyTo(&spec)
	if spec.Archetype == appkit.ArchetypeSuite && len(spec.Subcommands) == 0 {
		if subs := extractSubcommands(manLines, helpLines); len(subs) > 0 {
			spec.Subcommands = subs
		}
	}
	if len(spec.Profiles) == 0 {
		spec.Profiles = buildDefaultProfiles(flags)
	}
	appkit.ApplySafetyDefaults(&spec)
	attachEnvSecrets(&spec, extractEnvSecrets(manLines, helpLines))
	return spec
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
			if f.Kind == appkit.FlagSecret && f.SecretRoute() == appkit.RouteArgument &&
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
	// Check man NAME section first
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
	// Fallback to first descriptive line in --help
	for _, l := range helpLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "usage:") || strings.HasPrefix(lower, "synopsis") {
			continue
		}
		if strings.HasPrefix(trimmed, "-") {
			break
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
	var out []appkit.FlagSpec
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
	return out
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
	if !ph.IsNotFile() && !ph.Specific() {
		if strings.Contains(combined, "dir") || strings.Contains(combined, "directory") || strings.Contains(combined, "folder") {
			return appkit.FlagDir, nil, 0, 0
		}
		if strings.Contains(combined, "file") || strings.Contains(combined, "path") || strings.Contains(combined, "config") {
			return appkit.FlagFile, nil, 0, 0
		}
	}
	if strings.Contains(argClean, "0-5") {
		return appkit.FlagInt, nil, 0, 5
	}
	if strings.Contains(combined, "<num>") ||
		strings.Contains(combined, "<int>") ||
		strings.Contains(combined, "<count>") ||
		strings.Contains(combined, "<seconds>") ||
		strings.Contains(combined, "number of") ||
		strings.Contains(combined, "count") ||
		strings.Contains(combined, "timeout") ||
		strings.Contains(combined, "threads") ||
		strings.Contains(combined, "retries") {
		return appkit.FlagInt, nil, 0, 100
	}

	return appkit.FlagText, nil, 0, 0
}

// knownOutputFlags are output options the help text alone cannot settle.
var knownOutputFlags = map[string]string{
	"tcpdump": "-w", "tshark": "-w", "dumpcap": "-w", // raw packet capture files
	"nmap": "-oN",
}

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
	reMainOutput   = regexp.MustCompile(`(?i)\b(?:write|save|place|store)s?\s+(?:the\s+)?(?:output|result|results|documents?|data)\s+(?:to|into|in)\b`)
	reNotMainOut   = regexp.MustCompile(`(?i)\b(?:log|logs|logging|messages|read|reads|input|load|config|configuration)\b`)
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

func guessExtension(binName, desc string) string {
	lower := strings.ToLower(binName + " " + desc)
	switch {
	case strings.Contains(lower, "pcap") || binName == "tshark" || binName == "tcpdump":
		return "pcap"
	case strings.Contains(lower, "json"):
		return "json"
	case strings.Contains(lower, "xml"):
		return "xml"
	case strings.Contains(lower, "csv"):
		return "csv"
	case strings.Contains(lower, "png") || binName == "qrencode":
		return "png"
	default:
		return "txt"
	}
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

func buildDefaultProfiles(flags []appkit.FlagSpec) []appkit.ProfileSpec {
	if len(flags) == 0 {
		return nil
	}
	defaultMap := map[string]string{}
	for _, f := range flags {
		if f.Kind == appkit.FlagBool {
			defaultMap[f.Flag] = "false"
		}
	}
	return []appkit.ProfileSpec{
		{Name: "Default", Flags: defaultMap},
	}
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
