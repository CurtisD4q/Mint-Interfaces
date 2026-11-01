package main

import (
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

var reSubcmdLine = regexp.MustCompile(`^\s{2,8}([a-z][a-z0-9_-]{1,22})\s{2,}(.+)$`)

// DetectArchetype classifies a program into one of the 36 archetypes: the
// archetype lists first, then its behaviour and words (see
// chooseArchetype).
func DetectArchetype(bin string, manText, helpText string) appkit.ArchetypeDef {
	a, _ := appkit.LookupArchetype(string(chooseArchetype(bin, manText, helpText).ID))
	return a
}

// dataRank orders data levels: normal < sensitive < secret.
func dataRank(level string) int {
	switch level {
	case appkit.DataSecret:
		return 2
	case appkit.DataSensitive:
		return 1
	}
	return 0
}

func extractSubcommands(manLines, helpLines []string) []appkit.SubcommandSpec {
	return extractSubcommandsN(manLines, helpLines, 10)
}

// extractSubcommandsN is extractSubcommands keeping up to limit commands.
func extractSubcommandsN(manLines, helpLines []string, limit int) []appkit.SubcommandSpec {
	var out []appkit.SubcommandSpec
	seen := map[string]bool{}

	scan := func(lines []string) {
		inCmdSec := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if strings.HasSuffix(lower, "commands:") ||
				lower == "commands" ||
				lower == "subcommands" ||
				lower == "available commands" ||
				strings.Contains(lower, "main commands") {
				inCmdSec = true
				continue
			}
			if inCmdSec {
				if isManSectionHeader(line) || isHelpSectionHeader(line) {
					inCmdSec = false
					continue
				}
				if m := reSubcmdLine.FindStringSubmatch(line); m != nil {
					cmdName := strings.TrimSpace(m[1])
					cmdDesc := strings.TrimSpace(m[2])
					if cmdName != "help" && cmdName != "version" && !seen[cmdName] && len(out) < limit {
						seen[cmdName] = true
						out = append(out, appkit.SubcommandSpec{
							Name:  cmdName,
							Label: titleWords(cmdName),
							Desc:  capitalizeFirst(cmdDesc),
							Args:  []string{cmdName},
						})
					}
				}
			}
		}
	}

	scan(helpLines)
	if len(out) == 0 {
		scan(manLines)
	}
	return out
}
