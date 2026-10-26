package main

import (
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

var reSubcmdLine = regexp.MustCompile(`^\s{2,8}([a-z][a-z0-9_-]{1,22})\s{2,}(.+)$`)

// DetectTemplate classifies a program into one of the 28 Template Types:
// the built-in tool list first, then the template whose words best match
// its name, summary, usage lines and help (see ScoreTemplates).
func DetectTemplate(bin string, spec appkit.ToolSpec, manText, helpText string) appkit.TemplateDef {
	tpl, _ := appkit.LookupTemplate(chooseTemplate(bin, spec, manText, helpText).ID)
	return tpl
}

func extractSubcommands(manLines, helpLines []string) []appkit.SubcommandSpec {
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
					if cmdName != "help" && cmdName != "version" && !seen[cmdName] && len(out) < 10 {
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
