package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// printSafetyReport shows how a generated interface for spec will behave:
// when it runs, which options are secrets and how each is passed, and which
// options look destructive. Check it before building.
func printSafetyReport(w io.Writer, spec appkit.ToolSpec) {
	r := spec.SafetyReport()
	fmt.Fprintln(w, "Safety:")
	fmt.Fprintf(w, "  Running:     %s\n", r.Running)
	if len(r.Secrets) == 0 {
		fmt.Fprintln(w, "  Secrets:     none found")
	} else {
		fmt.Fprintf(w, "  Secrets:     %s (asked for with a password dialog, never saved)\n", count(len(r.Secrets), "option"))
		for _, s := range r.Secrets {
			flag := s.Flag
			if flag == "" {
				flag = "(no option)"
			}
			fmt.Fprintf(w, "    %-20s %-34s %s: %s\n", flag, short(stripFlag(s.Label, s.Flag), 34), routeText(spec, s), appkit.RouteNote(s.Route))
		}
	}
	fmt.Fprintf(w, "  Data:        %s\n", dataText(r))
	for _, f := range r.Revealing {
		fmt.Fprintf(w, "    %-20s %s\n", f.Flag, short(stripFlag(f.Label, f.Flag), 60))
	}
	if len(r.Destructive) == 0 {
		fmt.Fprintln(w, "  Destructive: no options found")
	} else {
		fmt.Fprintf(w, "  Destructive: %s to review (they delete, overwrite or skip a prompt)\n", count(len(r.Destructive), "option"))
		for _, f := range r.Destructive {
			fmt.Fprintf(w, "    %-20s %s\n", f.Flag, short(stripFlag(f.Label, f.Flag), 60))
		}
	}
	if line := rowActionsLine(r.RowActions); line != "" {
		fmt.Fprintf(w, "  Row actions: %s\n", line)
	}
}

// stripFlag drops the " (-x)" suffix mint-gen adds to labels.
func stripFlag(label, flag string) string {
	return strings.TrimSuffix(label, " ("+flag+")")
}

func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// safetyRows is the safety report as rows for the interactive generator.
func safetyRows(spec appkit.ToolSpec) []mint.Row {
	r := spec.SafetyReport()
	rows := []mint.Row{mint.Label(mint.StyleBody, "Runs: "+r.Running)}
	if len(r.Secrets) == 0 {
		rows = append(rows, mint.Note("No secret options found."))
	} else {
		var tbl [][]string
		for _, s := range r.Secrets {
			flag := s.Flag
			if flag == "" {
				flag = "(no option)"
			}
			tbl = append(tbl, []string{flag, routeText(spec, s), appkit.RouteNote(s.Route)})
		}
		rows = append(rows, mint.Table(mint.TableOpts{ID: "safety-secrets",
			Headers: []string{"SECRET", "PASSED ON", "NOTE"}, Rows: tbl}))
	}
	rows = append(rows, mint.Label(mint.StyleBody, "Data: "+dataText(r)))
	for _, f := range r.Revealing {
		rows = append(rows, mint.Note("  "+f.Flag+"  "+stripFlag(f.Label, f.Flag)))
	}
	if len(r.Destructive) == 0 {
		rows = append(rows, mint.Note("No destructive options found."))
	} else {
		var tbl [][]string
		for _, f := range r.Destructive {
			tbl = append(tbl, []string{f.Flag, stripFlag(f.Label, f.Flag)})
		}
		rows = append(rows, mint.Table(mint.TableOpts{ID: "safety-destructive",
			Headers: []string{"DESTRUCTIVE", "WHAT IT DOES"}, Rows: tbl}))
	}
	if line := rowActionsLine(r.RowActions); line != "" {
		rows = append(rows, mint.Label(mint.StyleBody, "Row actions: "+line))
	}
	return rows
}

// rowActionsLine lists attached row actions, marking the ones that ask
// before they run.
func rowActionsLine(actions []appkit.RowActionSpec) string {
	if len(actions) == 0 {
		return ""
	}
	parts := make([]string, len(actions))
	for i, a := range actions {
		parts[i] = a.Label
		if a.Confirm {
			parts[i] += " (confirm)"
		}
	}
	return strings.Join(parts, ", ")
}

// routeText is the route, naming the variable and where it was found.
func routeText(spec appkit.ToolSpec, s appkit.SecretOption) string {
	if s.Route != appkit.RouteEnv {
		return s.Route
	}
	found := "man page"
	if appkit.KnownSecretEnv(spec.BinaryName(), s.Env) {
		found = "built-in list"
	}
	return fmt.Sprintf("%s %s (%s)", s.Route, s.Env, found)
}

// dataText says how sensitive the results are and what the app does about it.
func dataText(r appkit.SafetyReport) string {
	var t string
	switch r.Data {
	case appkit.DataSecret:
		t = "secret (" + r.DataReason + "): result files readable only by you, kept in memory by default"
	case appkit.DataSensitive:
		t = "sensitive (" + r.DataReason + "): result files readable only by you"
	default:
		t = "normal"
	}
	if n := len(r.Revealing); n > 0 {
		verb := "make"
		if n == 1 {
			verb = "makes"
		}
		t += fmt.Sprintf("; %s %s a run secret", count(n, "option"), verb)
	}
	return t
}
