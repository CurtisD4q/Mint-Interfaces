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
			fmt.Fprintf(w, "    %-20s %-34s %s: %s\n", s.Flag, short(stripFlag(s.Label, s.Flag), 34), s.Route, appkit.RouteNote(s.Route))
		}
	}
	if len(r.Destructive) == 0 {
		fmt.Fprintln(w, "  Destructive: no options found")
	} else {
		fmt.Fprintf(w, "  Destructive: %s to review (they delete, overwrite or skip a prompt)\n", count(len(r.Destructive), "option"))
		for _, f := range r.Destructive {
			fmt.Fprintf(w, "    %-20s %s\n", f.Flag, short(stripFlag(f.Label, f.Flag), 60))
		}
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
			tbl = append(tbl, []string{s.Flag, s.Route, appkit.RouteNote(s.Route)})
		}
		rows = append(rows, mint.Table(mint.TableOpts{ID: "safety-secrets",
			Headers: []string{"SECRET", "PASSED ON", "NOTE"}, Rows: tbl}))
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
	return rows
}
