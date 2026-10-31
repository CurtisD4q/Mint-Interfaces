package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	if err := spec.Validate(); err != nil {
		return ScaffoldResult{}, fmt.Errorf("the spec for %s is not valid: %w", tool, err)
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
	if spec.ArchetypeID != "" {
		fmt.Fprintf(&buf, "\t\tArchetypeID:       %q,\n", spec.ArchetypeID)
	}
	fmt.Fprintf(&buf, "\t\tTargetLabel:       %q,\n", spec.TargetLabel)
	fmt.Fprintf(&buf, "\t\tTargetPlaceholder: %q,\n", spec.TargetPlaceholder)
	if spec.TargetDefault != "" {
		fmt.Fprintf(&buf, "\t\tTargetDefault:     %q,\n", spec.TargetDefault)
	}
	if spec.SecondTargetLabel != "" {
		fmt.Fprintf(&buf, "\t\tSecondTargetLabel: %q,\n", spec.SecondTargetLabel)
	}
	if spec.SecondTargetPlaceholder != "" {
		fmt.Fprintf(&buf, "\t\tSecondTargetPlaceholder: %q,\n", spec.SecondTargetPlaceholder)
	}
	if spec.WantTarget {
		buf.WriteString("\t\tWantTarget:        true,\n")
	}
	if spec.HideTarget {
		buf.WriteString("\t\tHideTarget:        true,\n")
	}
	if spec.TwoInputs {
		buf.WriteString("\t\tTwoInputs:         true,\n")
	}
	if len(spec.PositionalRoles) > 0 {
		buf.WriteString("\t\tPositionalRoles:   []appkit.ArgRole{")
		for i, r := range spec.PositionalRoles {
			if i > 0 {
				buf.WriteString(", ")
			}
			fmt.Fprintf(&buf, "appkit.%s", roleGoName(r))
		}
		buf.WriteString("},\n")
	}
	if spec.OutputsFile {
		buf.WriteString("\t\tOutputsFile:       true,\n")
	}
	if spec.OutputFlag != "" {
		fmt.Fprintf(&buf, "\t\tOutputFlag:        %q,\n", spec.OutputFlag)
	}
	if spec.OutputPositional {
		buf.WriteString("\t\tOutputPositional:  true,\n")
	}
	if id := stdoutIdent(spec.StdoutKind); id != "" {
		fmt.Fprintf(&buf, "\t\tStdoutKind:        appkit.%s,\n", id)
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
	if r := spec.Reorder; r != nil {
		fmt.Fprintf(&buf, "\t\tReorder: &appkit.ReorderSpec{Adapter: %q", r.Adapter)
		if r.Scope != "" {
			fmt.Fprintf(&buf, ", Scope: %q", r.Scope)
		}
		if r.ConfirmWindow != 0 {
			fmt.Fprintf(&buf, ", ConfirmWindow: %d /* %v */", int64(r.ConfirmWindow), r.ConfirmWindow)
		}
		buf.WriteString("},\n")
	}
	if d := spec.Diff; d != nil {
		fmt.Fprintf(&buf, "\t\tDiff: &appkit.DiffSpec{Format: %q", d.Format)
		for _, f := range []struct {
			name string
			v    []string
		}{{"ForceArgs", d.ForceArgs}, {"FormatFlags", d.FormatFlags}, {"ReadableFlags", d.ReadableFlags}} {
			if len(f.v) > 0 {
				fmt.Fprintf(&buf, ", %s: %s", f.name, goStrings(f.v))
			}
		}
		if d.LeftLabel != "" {
			fmt.Fprintf(&buf, ", LeftLabel: %q", d.LeftLabel)
		}
		if d.RightLabel != "" {
			fmt.Fprintf(&buf, ", RightLabel: %q", d.RightLabel)
		}
		buf.WriteString("},\n")
	}
	if f := spec.Findings; f != nil {
		fmt.Fprintf(&buf, "\t\tFindings: &appkit.FindingsSpec{Adapter: %q", f.Adapter)
		for _, x := range []struct {
			name string
			v    []string
		}{{"ForceArgs", f.ForceArgs}, {"FormatFlags", f.FormatFlags}, {"ReadableFlags", f.ReadableFlags}} {
			if len(x.v) > 0 {
				fmt.Fprintf(&buf, ", %s: %s", x.name, goStrings(x.v))
			}
		}
		if f.ReportFile != "" {
			fmt.Fprintf(&buf, ", ReportFile: %q", f.ReportFile)
		}
		if len(f.Patterns) > 0 {
			buf.WriteString(", Patterns: []appkit.FindingPattern{")
			for i, p := range f.Patterns {
				if i > 0 {
					buf.WriteString(", ")
				}
				fmt.Fprintf(&buf, "{Severity: %q, Regex: %q}", p.Severity, p.Regex)
			}
			buf.WriteString("}")
		}
		buf.WriteString("},\n")
	}
	if p := spec.Preview; p != nil {
		fmt.Fprintf(&buf, "\t\tPreview: &appkit.PreviewSpec{Adapter: %q", p.Adapter)
		if len(p.DryRunArgs) > 0 {
			fmt.Fprintf(&buf, ", DryRunArgs: %s", goStrings(p.DryRunArgs))
		}
		buf.WriteString("},\n")
	}
	if f := spec.Follow; f != nil {
		fmt.Fprintf(&buf, "\t\tFollow: &appkit.FollowSpec{Format: %q", f.Format)
		if f.Flag != "" {
			fmt.Fprintf(&buf, ", Flag: %q", f.Flag)
		}
		for _, x := range []struct {
			name string
			v    []string
		}{{"FollowArgs", f.FollowArgs}, {"FollowFlags", f.FollowFlags}, {"FormatArgs", f.FormatArgs},
			{"FormatFlags", f.FormatFlags}, {"ReadableFlags", f.ReadableFlags}} {
			if len(x.v) > 0 {
				fmt.Fprintf(&buf, ", %s: %s", x.name, goStrings(x.v))
			}
		}
		if f.AutoStart {
			buf.WriteString(", AutoStart: true")
		}
		if f.Restart != "" {
			fmt.Fprintf(&buf, ", Restart: %q", f.Restart)
		}
		for i, l := range f.Levels {
			if i == 0 {
				buf.WriteString(", Levels: []appkit.FollowLevel{")
			} else {
				buf.WriteString(", ")
			}
			fmt.Fprintf(&buf, "{Level: %q, Regex: %q}", l.Level, l.Regex)
			if i == len(f.Levels)-1 {
				buf.WriteString("}")
			}
		}
		buf.WriteString("},\n")
	}
	if v := spec.Service; v != nil {
		fmt.Fprintf(&buf, "\t\tService: &appkit.ServiceSpec{Adapter: %q", v.Adapter)
		if v.DefaultPort != 0 {
			fmt.Fprintf(&buf, ", DefaultPort: %d", v.DefaultPort)
		}
		for _, x := range []struct{ name, val string }{{"DefaultBind", v.DefaultBind}, {"Scheme", v.Scheme},
			{"PortFlag", v.PortFlag}, {"BindFlag", v.BindFlag}, {"DirFlag", v.DirFlag}} {
			if x.val != "" {
				fmt.Fprintf(&buf, ", %s: %q", x.name, x.val)
			}
		}
		if v.ServesDir {
			buf.WriteString(", ServesDir: true")
		}
		if v.PortLast {
			buf.WriteString(", PortLast: true")
		}
		buf.WriteString("},\n")
	}
	if tr := spec.Transcript; tr != nil {
		fmt.Fprintf(&buf, "\t\tTranscript: &appkit.TranscriptSpec{Adapter: %q", tr.Adapter)
		if len(tr.QuitWords) > 0 {
			fmt.Fprintf(&buf, ", QuitWords: %s", goStrings(tr.QuitWords))
		}
		buf.WriteString("},\n")
	}
	if spec.OperandSeparator {
		buf.WriteString("\t\tOperandSeparator:  true,\n")
	}
	if len(spec.ExitMeanings) > 0 {
		codes := make([]int, 0, len(spec.ExitMeanings))
		for c := range spec.ExitMeanings {
			codes = append(codes, c)
		}
		sort.Ints(codes)
		buf.WriteString("\t\tExitMeanings:      map[int]string{")
		for i, c := range codes {
			if i > 0 {
				buf.WriteString(", ")
			}
			fmt.Fprintf(&buf, "%d: %q", c, spec.ExitMeanings[c])
		}
		buf.WriteString("},\n")
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
	if len(spec.RowActions) > 0 {
		buf.WriteString("\t\tRowActions: []appkit.RowActionSpec{\n")
		for _, a := range spec.RowActions {
			fmt.Fprintf(&buf, "\t\t\t{Label: %q, Hint: %q, Subcommand: %s, Column: %d, Confirm: %t},\n",
				a.Label, a.Hint, goStrings(a.Subcommand), a.Column, a.Confirm)
		}
		buf.WriteString("\t\t},\n")
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
			if id := roleIdent(f.Role); id != "" {
				fmt.Fprintf(&buf, "\t\t\t\tRole:    appkit.%s,\n", id)
			}
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
			if f.SecretEnv != "" {
				fmt.Fprintf(&buf, "\t\t\t\tSecretEnv: %q,\n", f.SecretEnv)
			}
			if f.EnvFlag != "" {
				fmt.Fprintf(&buf, "\t\t\t\tEnvFlag: %q,\n", f.EnvFlag)
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
	return fmt.Sprintf(`;;; %[1]s-mint.el --- Launch the %[1]s Mint interface in Emacs -*- lexical-binding: t; -*-

;; Load this file, then use M-x %[1]s-mint (or M-x mint).  With emacs/mint.el
;; installed, the app opens the way mint.el opens apps (handover or term,
;; theme matching, key profile); without it, it opens in ansi-term.

(require 'mint nil t)

(defcustom %[1]s-mint-binary
  (expand-file-name "%[1]s-mint"
                    (file-name-directory (or load-file-name buffer-file-name default-directory)))
  "Path to the compiled %[1]s-mint static Go binary.
Set it before this file is loaded."
  :type 'file
  :group 'mint)

(if (fboundp 'mint-run)
    ;; mint-define-app defines M-x %[1]s-mint and offers the app in M-x mint.
    (eval '(mint-define-app %[1]s-mint %[2]q %[1]s-mint-binary %[3]q) t)
  (defun %[1]s-mint ()
    %[3]q
    (interactive)
    (ansi-term %[1]s-mint-binary "%[1]s-mint")))

(provide '%[1]s-mint)
;;; %[1]s-mint.el ends here
`, tool, tool, "Open the "+tool+" Mint interface."+elispDocDesc(desc))
}

// elispDocDesc is desc as the body of an Emacs docstring: after a blank
// line, wrapped to stay within 80 columns.
func elispDocDesc(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	line := 0
	for _, w := range strings.Fields(desc) {
		if line > 0 && line+1+len(w) > 70 {
			b.WriteString("\n")
			line = 0
		} else if line == 0 {
			b.WriteString("\n")
		}
		if line > 0 {
			b.WriteByte(' ')
			line++
		}
		b.WriteString(w)
		line += len(w)
	}
	return b.String()
}

// roleGoName is roleIdent, and RoleValue, which roleIdent omits so a flag
// with no role is not written out. Positional lists need the empty role
// so later arguments keep their place.
func roleGoName(r appkit.ArgRole) string {
	if r == appkit.RoleValue {
		return "RoleValue"
	}
	if id := roleIdent(r); id != "" {
		return id
	}
	return "RoleValue"
}

// roleIdent is the Go name of a role, from a fixed list, so nothing from a
// help text is ever written into generated code unquoted. Unknown roles
// and RoleValue give "" (not written).
func roleIdent(r appkit.ArgRole) string {
	switch r {
	case appkit.RoleFile:
		return "RoleFile"
	case appkit.RoleDir:
		return "RoleDir"
	case appkit.RoleInputFile:
		return "RoleInputFile"
	case appkit.RoleInputDir:
		return "RoleInputDir"
	case appkit.RoleOutputFile:
		return "RoleOutputFile"
	case appkit.RoleOutputDir:
		return "RoleOutputDir"
	case appkit.RoleOutputBase:
		return "RoleOutputBase"
	case appkit.RoleLogFile:
		return "RoleLogFile"
	case appkit.RoleConfigFile:
		return "RoleConfigFile"
	case appkit.RoleFormat:
		return "RoleFormat"
	}
	return ""
}

// stdoutIdent is the Go name of a stdout kind, from a fixed list ("" for
// unknown values, which are then not written).
func stdoutIdent(k string) string {
	switch k {
	case appkit.StdoutResults:
		return "StdoutResults"
	case appkit.StdoutLog:
		return "StdoutLog"
	case appkit.StdoutBinary:
		return "StdoutBinary"
	}
	return ""
}

// goStrings writes ss as a Go []string literal.
func goStrings(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = strconv.Quote(s)
	}
	return "[]string{" + strings.Join(q, ", ") + "}"
}
