package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

var (
	reManName    = regexp.MustCompile(`^[A-Za-z0-9_.,\s-]+?\s+(?:-+|—|–)\s+(.+)$`)
	reOptLine    = regexp.MustCompile(`^\s{1,12}(-[A-Za-z0-9?]|--[A-Za-z0-9][-A-Za-z0-9_.]*)(?:,\s*(-[A-Za-z0-9?]|--[A-Za-z0-9][-A-Za-z0-9_.]*))*((?:=[<\[\({]?[A-Za-z0-9_|.,/+-]+[>\]\)}]?)|\s+(?:<[^>\s]+>|\[[^\]\s]+\]|\{[^}\s]+\}|[A-Z][A-Z0-9_-]{1,15}))?(?:\s{2,}|\t+|:?\s*$)(.*)$`)
	reTargetHint = regexp.MustCompile(`(?i)\b(target|host|url|uri|domain|address|file|path|pattern|dir|directory|archive|input|interface|device|package|image|container)\b`)
)

// ParseProgram reads `man -P cat <bin>` and `<bin> --help` (falling back to `-h`)
// and returns a populated appkit.ToolSpec.
func ParseProgram(bin string) (appkit.ToolSpec, error) {
	name := filepath.Base(strings.TrimSpace(bin))
	manText := runCapture(3*time.Second, "man", "-P", "cat", name)
	helpText := runCapture(2*time.Second, bin, "--help")
	if strings.TrimSpace(helpText) == "" {
		helpText = runCapture(2*time.Second, bin, "-h")
	}
	spec := ParseManAndHelp(name, manText, helpText)
	return spec, nil
}

func runCapture(timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), "MANWIDTH=100", "LC_ALL=C", "COLUMNS=100")
	out, _ := cmd.CombinedOutput()
	return string(out)
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
			// Detect dedicated output file flag (-o, --output, -w, -oN)
			if isOutputFileFlag(f) && !spec.OutputsFile {
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
	return spec
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

		m := reOptLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}

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
			if nextTrim == "" || isManSectionHeader(next) || reOptLine.MatchString(next) {
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

		join := " "
		if strings.HasPrefix(argPart, "=") {
			join = "="
			argPart = strings.TrimPrefix(argPart, "=")
		}

		kind, choices, minVal, maxVal := classifyFlag(canonFlag, argPart, desc)
		label := cleanFlagLabel(canonFlag, secondFlag, desc)
		hint := cleanArgHint(argPart)

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

	combined := strings.ToLower(argClean + " " + desc)
	if strings.Contains(combined, "dir") || strings.Contains(combined, "directory") || strings.Contains(combined, "folder") {
		return appkit.FlagDir, nil, 0, 0
	}
	if strings.Contains(combined, "file") || strings.Contains(combined, "path") || strings.Contains(combined, "config") {
		return appkit.FlagFile, nil, 0, 0
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

func isOutputFileFlag(f appkit.FlagSpec) bool {
	switch f.Flag {
	case "-o", "--output", "--output-file", "-oN", "-w":
		if f.Kind != appkit.FlagBool {
			return true
		}
	}
	lower := strings.ToLower(f.Label)
	return (f.Flag == "-o" || f.Flag == "--output") &&
		(strings.Contains(lower, "file") || strings.Contains(lower, "output") || strings.Contains(lower, "write"))
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
