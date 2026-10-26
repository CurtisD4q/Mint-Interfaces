package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// ScaffoldResult records the files written by ScaffoldApp.
type ScaffoldResult struct {
	AppDir    string
	Files     []string
	BinaryOut string
}

// ScaffoldApp writes an isolated apps/<tool>/ module (go.mod, main.go,
// spec.go, and <tool>-mint.el) wired to github.com/CurtisD4q/Mint-Interfaces/mint/appkit.
func ScaffoldApp(spec appkit.ToolSpec, outDir, repoRoot string) (ScaffoldResult, error) {
	tool := sanitizeToolName(spec.Name)
	if outDir == "" {
		if repoRoot != "" {
			outDir = filepath.Join(repoRoot, "apps", tool)
		} else {
			outDir = filepath.Join("apps", tool)
		}
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return ScaffoldResult{}, err
	}
	// Never write go.mod and main.go over somebody's project: only an empty
	// (or new) folder, or one mint-gen made, may be used.
	if entries, err := os.ReadDir(absOut); err == nil && len(entries) > 0 && !isScaffold(absOut, tool) {
		return ScaffoldResult{}, fmt.Errorf("%s already has files that mint-gen did not create; choose an empty folder with --out", absOut)
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return ScaffoldResult{}, err
	}

	relRoot := "../.."
	if repoRoot != "" {
		if absRoot, err := filepath.Abs(repoRoot); err == nil {
			if r, err := filepath.Rel(absOut, absRoot); err == nil {
				relRoot = filepath.ToSlash(r)
			}
		}
	}

	goModPath := filepath.Join(absOut, "go.mod")
	mainGoPath := filepath.Join(absOut, "main.go")
	specGoPath := filepath.Join(absOut, "spec.go")
	elispPath := filepath.Join(absOut, tool+"-mint.el")

	if err := os.WriteFile(goModPath, []byte(renderGoMod(tool, relRoot)), 0o644); err != nil {
		return ScaffoldResult{}, err
	}
	if err := os.WriteFile(mainGoPath, []byte(renderMainGo()), 0o644); err != nil {
		return ScaffoldResult{}, err
	}
	specSrc, err := renderSpecGo(spec)
	if err != nil {
		return ScaffoldResult{}, err
	}
	if err := os.WriteFile(specGoPath, specSrc, 0o644); err != nil {
		return ScaffoldResult{}, err
	}
	if err := os.WriteFile(elispPath, []byte(renderElisp(tool, spec.Desc)), 0o644); err != nil {
		return ScaffoldResult{}, err
	}
	if err := os.WriteFile(filepath.Join(absOut, scaffoldMarker), []byte(markerText(tool)), 0o644); err != nil {
		return ScaffoldResult{}, err
	}

	return ScaffoldResult{
		AppDir:    absOut,
		Files:     []string{goModPath, mainGoPath, specGoPath, elispPath},
		BinaryOut: filepath.Join(absOut, tool+"-mint"),
	}, nil
}

// scaffoldMarker is written into every folder mint-gen creates. --remove and
// re-scaffolding only touch folders that have it (or an older scaffold's
// exact files).
const scaffoldMarker = ".mint-gen"

func markerText(tool string) string {
	return "Created by mint-gen. `mint-gen --remove " + tool + "` deletes only these files:\n" +
		strings.Join(scaffoldFiles(tool), "\n") + "\n"
}

// scaffoldFiles are the only files mint-gen creates in an app folder.
func scaffoldFiles(tool string) []string {
	return []string{"go.mod", "go.sum", "main.go", "spec.go", tool + "-mint.el", tool + "-mint", scaffoldMarker}
}

// isScaffold reports whether dir was made by mint-gen for tool: it has the
// marker, or (scaffolds made before the marker existed) its main.go and
// spec.go are exactly the generated kind.
func isScaffold(dir, tool string) bool {
	if _, err := os.Stat(filepath.Join(dir, scaffoldMarker)); err == nil {
		return true
	}
	mainSrc, err1 := os.ReadFile(filepath.Join(dir, "main.go"))
	specSrc, err2 := os.ReadFile(filepath.Join(dir, "spec.go"))
	mod, err3 := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	return strings.TrimSpace(string(mainSrc)) == strings.TrimSpace(renderMainGo()) &&
		strings.Contains(string(specSrc), "func ToolSpec() appkit.ToolSpec") &&
		strings.Contains(string(mod), "github.com/CurtisD4q/Mint-Interfaces")
}

func sanitizeToolName(name string) string {
	name = strings.ToLower(filepath.Base(strings.TrimSpace(name)))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		return "tool"
	}
	return res
}

func renderGoMod(tool, relRoot string) string {
	return fmt.Sprintf(`module github.com/CurtisD4q/Mint-Interfaces/apps/%s

go 1.22

require github.com/CurtisD4q/Mint-Interfaces v0.0.0

replace github.com/CurtisD4q/Mint-Interfaces => %s
`, tool, relRoot)
}

func renderMainGo() string {
	return `package main

import "github.com/CurtisD4q/Mint-Interfaces/mint/appkit"

func main() {
	appkit.Run(ToolSpec())
}
`
}

func archetypeIdent(a appkit.Archetype) string {
	switch a {
	case appkit.ArchetypeConfig:
		return "ArchetypeConfig"
	case appkit.ArchetypeScanner:
		return "ArchetypeScanner"
	case appkit.ArchetypeMonitor:
		return "ArchetypeMonitor"
	case appkit.ArchetypeExplorer:
		return "ArchetypeExplorer"
	case appkit.ArchetypeSuite:
		return "ArchetypeSuite"
	default:
		return "ArchetypeRunner"
	}
}

func renderSpecGo(spec appkit.ToolSpec) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("package main\n\n")
	buf.WriteString("import \"github.com/CurtisD4q/Mint-Interfaces/mint/appkit\"\n\n")
	buf.WriteString("// ToolSpec returns the declarative Mint interface specification.\n")
	buf.WriteString("func ToolSpec() appkit.ToolSpec {\n")
	buf.WriteString("\treturn appkit.ToolSpec{\n")
	fmt.Fprintf(&buf, "\t\tName:              %q,\n", spec.Name)
	fmt.Fprintf(&buf, "\t\tTitle:             %q,\n", spec.Title)
	fmt.Fprintf(&buf, "\t\tDesc:              %q,\n", spec.Desc)
	if spec.Template != "" {
		fmt.Fprintf(&buf, "\t\tTemplate:          %q,\n", spec.Template)
	}
	if spec.Archetype != "" {
		fmt.Fprintf(&buf, "\t\tArchetype:         appkit.%s,\n", archetypeIdent(spec.Archetype))
	}
	fmt.Fprintf(&buf, "\t\tTargetLabel:       %q,\n", spec.TargetLabel)
	fmt.Fprintf(&buf, "\t\tTargetPlaceholder: %q,\n", spec.TargetPlaceholder)
	if spec.TargetDefault != "" {
		fmt.Fprintf(&buf, "\t\tTargetDefault:     %q,\n", spec.TargetDefault)
	}
	if spec.OutputsFile {
		buf.WriteString("\t\tOutputsFile:       true,\n")
	}
	if spec.OutputFlag != "" {
		fmt.Fprintf(&buf, "\t\tOutputFlag:        %q,\n", spec.OutputFlag)
	}
	if spec.DefaultExt != "" {
		fmt.Fprintf(&buf, "\t\tDefaultExt:        %q,\n", spec.DefaultExt)
	}
	if spec.TableOutput {
		buf.WriteString("\t\tTableOutput:       true,\n")
	}
	if spec.CollapsibleOutput {
		buf.WriteString("\t\tCollapsibleOutput: true,\n")
	}
	if spec.NeedsRoot {
		buf.WriteString("\t\tNeedsRoot:         true,\n")
	}
	if spec.ConfirmRun {
		buf.WriteString("\t\tConfirmRun:        true,\n")
	}
	if spec.KeepHistory {
		buf.WriteString("\t\tKeepHistory:       true,\n")
	}
	if spec.ReadOnly {
		buf.WriteString("\t\tReadOnly:          true,\n")
	}
	if spec.Irreversible {
		buf.WriteString("\t\tIrreversible:      true,\n")
	}
	if len(spec.StatusArgs) > 0 {
		buf.WriteString("\t\tStatusArgs:        []string{")
		for i, a := range spec.StatusArgs {
			if i > 0 {
				buf.WriteString(", ")
			}
			fmt.Fprintf(&buf, "%q", a)
		}
		buf.WriteString("},\n")
	}
	if len(spec.Subcommands) > 0 {
		buf.WriteString("\t\tSubcommands: []appkit.SubcommandSpec{\n")
		for _, sub := range spec.Subcommands {
			buf.WriteString("\t\t\t{\n")
			fmt.Fprintf(&buf, "\t\t\t\tName:  %q,\n", sub.Name)
			fmt.Fprintf(&buf, "\t\t\t\tLabel: %q,\n", sub.Label)
			if sub.Desc != "" {
				fmt.Fprintf(&buf, "\t\t\t\tDesc:  %q,\n", sub.Desc)
			}
			if len(sub.Args) > 0 {
				buf.WriteString("\t\t\t\tArgs:  []string{")
				for i, a := range sub.Args {
					if i > 0 {
						buf.WriteString(", ")
					}
					fmt.Fprintf(&buf, "%q", a)
				}
				buf.WriteString("},\n")
			}
			if sub.Confirm {
				buf.WriteString("\t\t\t\tConfirm: true,\n")
			}
			buf.WriteString("\t\t\t},\n")
		}
		buf.WriteString("\t\t},\n")
	}
	if len(spec.Flags) > 0 {
		buf.WriteString("\t\tFlags: []appkit.FlagSpec{\n")
		for _, f := range spec.Flags {
			buf.WriteString("\t\t\t{\n")
			fmt.Fprintf(&buf, "\t\t\t\tFlag:    %q,\n", f.Flag)
			if f.Join != "" && f.Join != " " {
				fmt.Fprintf(&buf, "\t\t\t\tJoin:    %q,\n", f.Join)
			}
			fmt.Fprintf(&buf, "\t\t\t\tKind:    appkit.%s,\n", flagKindIdent(f.Kind))
			fmt.Fprintf(&buf, "\t\t\t\tSection: %q,\n", f.Section)
			fmt.Fprintf(&buf, "\t\t\t\tLabel:   %q,\n", f.Label)
			if f.Hint != "" {
				fmt.Fprintf(&buf, "\t\t\t\tHint:    %q,\n", f.Hint)
			}
			if f.Default != "" {
				fmt.Fprintf(&buf, "\t\t\t\tDefault: %q,\n", f.Default)
			}
			if len(f.Choices) > 0 {
				buf.WriteString("\t\t\t\tChoices: []appkit.Choice{\n")
				for _, c := range f.Choices {
					fmt.Fprintf(&buf, "\t\t\t\t\t{Label: %q, Value: %q},\n", c.Label, c.Value)
				}
				buf.WriteString("\t\t\t\t},\n")
			}
			if len(f.ChoicesCmd) > 0 {
				buf.WriteString("\t\t\t\tChoicesCmd: []string{")
				for i, tok := range f.ChoicesCmd {
					if i > 0 {
						buf.WriteString(", ")
					}
					fmt.Fprintf(&buf, "%q", tok)
				}
				buf.WriteString("},\n")
			}
			if f.Kind == appkit.FlagInt && (f.Min != 0 || f.Max != 0) {
				fmt.Fprintf(&buf, "\t\t\t\tMin:     %d,\n", f.Min)
				fmt.Fprintf(&buf, "\t\t\t\tMax:     %d,\n", f.Max)
				step := f.Step
				if step <= 0 {
					step = 1
				}
				fmt.Fprintf(&buf, "\t\t\t\tStep:    %d,\n", step)
			}
			if f.Advanced {
				buf.WriteString("\t\t\t\tAdvanced: true,\n")
			}
			if f.SecretStdin {
				buf.WriteString("\t\t\t\tSecretStdin: true,\n")
			}
			buf.WriteString("\t\t\t},\n")
		}
		buf.WriteString("\t\t},\n")
	}
	buf.WriteString("\t}\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), nil
	}
	return formatted, nil
}

func flagKindIdent(k appkit.FlagKind) string {
	switch k {
	case appkit.FlagBool:
		return "FlagBool"
	case appkit.FlagText:
		return "FlagText"
	case appkit.FlagInt:
		return "FlagInt"
	case appkit.FlagChoice:
		return "FlagChoice"
	case appkit.FlagFile:
		return "FlagFile"
	case appkit.FlagDir:
		return "FlagDir"
	case appkit.FlagSecret:
		return "FlagSecret"
	default:
		return "FlagBool"
	}
}

func renderElisp(tool, desc string) string {
	return fmt.Sprintf(`;;; %s-mint.el --- Launch %s Mint interface in Emacs -*- lexical-binding: t; -*-

(require 'mint nil t)

(defcustom %s-mint-binary
  (expand-file-name "%s-mint"
                    (file-name-directory (or load-file-name buffer-file-name default-directory)))
  "Path to the compiled %s-mint static Go binary."
  :type 'file
  :group 'mint)

;;;###autoload
(defun %s-mint ()
  "Open the %s Mint TUI (%s)."
  (interactive)
  (if (fboundp 'mint-start)
      (mint-start "%s" %s-mint-binary)
    (ansi-term %s-mint-binary "%s-mint")))

(when (fboundp 'mint-register)
  (mint-register "%s" %q #'%s-mint))

(provide '%s-mint)
;;; %s-mint.el ends here
`, tool, tool, tool, tool, tool, tool, tool, desc, tool, tool, tool, tool, tool, desc, tool, tool, tool)
}
