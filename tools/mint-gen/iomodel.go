package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// IOEndpoint is one place a tool reads from or writes to.
type IOEndpoint struct {
	Flag     string         // the option ("-o"), or "" for an argument
	Name     string         // the argument's name ("outfile"), or "" for an option
	Position int            // for an argument: its place in the first usage form (0-based)
	Role     appkit.ArgRole // input file, output file, log…
	Ext      string         // the extension of a file it writes, when known
	Why      string         // the strongest reason
}

func (e IOEndpoint) label() string {
	if e.Flag != "" {
		return e.Flag
	}
	return e.Name + " (argument)"
}

// Save modes: how "Save output to file" works for a tool.
const (
	SaveByOption     = "option"     // the tool writes the file itself, through its output option
	SaveByPositional = "positional" // the tool writes the file itself, as its last argument
	SaveByCapture    = "capture"    // Mint saves what the tool prints (its results)
	SaveLog          = "log"        // Mint can save what the tool prints, which is only messages
	SaveNone         = "none"       // nothing worth saving (binary output, no output option)
)

// IOModel is mint-gen's single answer to "what does this tool read and
// write?". It decides the output settings of the generated app, and each
// decision keeps its reason (see --explain).
type IOModel struct {
	Tool      string
	Inputs    []IOEndpoint
	Outputs   []IOEndpoint
	Logs      []IOEndpoint
	Formats   []IOEndpoint
	Configs   []IOEndpoint
	Stdout    string // StdoutResults, StdoutLog, StdoutBinary or "" (unknown)
	StdoutWhy string
	Save      string // a Save… mode
	SaveWhy   string
	Main      *IOEndpoint // where the app's output file goes, for SaveByOption/SaveByPositional
	Ext       string      // the output file's extension (see chooseExtension)
	ExtWhy    string
	Known     bool   // the tool is in the known-tools list
	Usage     string // the first usage form
	Template  TemplateChoice
}

// BuildIOModel combines the options' roles (ClassifyArg), the usage forms
// (ParseSynopsis) and the known-tools list (KnownToolIO). The app writes to
// a path only when that is certain: a listed or clearly scored output
// option, or an output argument confirmed by the known-tools list. An output
// argument found only in the usage line is reported, never written to.
func BuildIOModel(spec appkit.ToolSpec, syn Synopsis) IOModel {
	m := IOModel{Tool: spec.Name}
	kt, known := KnownToolIO(spec.Name)
	m.Known = known
	if len(syn.Forms) > 0 {
		m.Usage = syn.Forms[0].Line
	}

	add := func(e IOEndpoint) {
		switch {
		case e.Role == appkit.RoleLogFile:
			m.Logs = append(m.Logs, e)
		case e.Role == appkit.RoleFormat:
			m.Formats = append(m.Formats, e)
		case e.Role == appkit.RoleConfigFile:
			m.Configs = append(m.Configs, e)
		case e.Role.Writes():
			m.Outputs = append(m.Outputs, e)
		case e.Role == appkit.RoleInputFile || e.Role == appkit.RoleInputDir:
			m.Inputs = append(m.Inputs, e)
		}
	}
	if spec.OutputFlag != "" {
		e := IOEndpoint{Flag: spec.OutputFlag, Role: appkit.RoleOutputFile, Why: "the output option"}
		if known {
			if o, ok := kt.option(spec.OutputFlag); ok {
				e.Ext = o.Ext
				if o.Main {
					e.Why = "the main output in the known-tools list"
				}
			}
		}
		add(e)
	}
	for _, f := range spec.Flags {
		if f.Role == appkit.RoleValue || f.Role == appkit.RoleFile || f.Role == appkit.RoleDir {
			continue
		}
		e := IOEndpoint{Flag: f.Flag, Role: f.Role}
		if known {
			if o, ok := kt.option(f.Flag); ok {
				e.Ext = o.Ext
			}
		}
		_, ev := ClassifyArg(spec.Name, f)
		if s := sortEvidence(ev); len(s) > 0 {
			e.Why = s[0].Reason
		}
		add(e)
	}
	// Options the known-tools list names but the help text did not show.
	if known {
		have := map[string]bool{spec.OutputFlag: true}
		for _, f := range spec.Flags {
			have[f.Flag] = true
		}
		for _, o := range kt.Options {
			if !have[o.Flag] {
				add(IOEndpoint{Flag: o.Flag, Role: appkit.ArgRole(o.Role), Ext: o.Ext, Why: "known-tools list (not found in the help text)"})
			}
		}
	}
	if len(syn.Forms) > 0 {
		for i, p := range syn.Forms[0].Positionals {
			if p.Role == appkit.RoleValue {
				continue
			}
			add(IOEndpoint{Name: p.Name, Position: i, Role: p.Role, Why: p.Why})
		}
	}

	// What the tool prints.
	_, _, synOut := syn.OutputPositional()
	switch {
	case known && kt.Stdout != "":
		m.Stdout, m.StdoutWhy = kt.Stdout, "known-tools list"
	case synOut:
		m.Stdout, m.StdoutWhy = StdoutLog, "it writes its result to an argument ("+m.Usage+"), so it prints only messages"
	default:
		m.Stdout, m.StdoutWhy = "", "unknown; treated as results"
	}

	// How "Save output to file" works. Main is a copy, taken once the
	// outputs are complete.
	switch {
	case spec.OutputFlag != "":
		m.Save, m.SaveWhy = SaveByOption, "the tool writes the file itself, through "+spec.OutputFlag
		for _, e := range m.Outputs {
			if e.Flag == spec.OutputFlag {
				e := e
				m.Main = &e
				m.Ext = e.Ext // the known-tools list's, if any
				break
			}
		}
	case known && kt.OutputPositional:
		m.Save, m.SaveWhy = SaveByPositional, "the tool writes the file itself, as its last argument (known-tools list)"
		m.Ext = kt.OutputExt
		at := -1
		if _, i, ok := syn.OutputPositional(); ok {
			for j := range m.Outputs {
				if m.Outputs[j].Flag == "" && m.Outputs[j].Position == i {
					at = j
				}
			}
		}
		if at < 0 {
			m.Outputs = append(m.Outputs, IOEndpoint{Name: "output", Role: appkit.RoleOutputFile, Why: "known-tools list"})
			at = len(m.Outputs) - 1
		}
		m.Outputs[at].Ext = m.Ext
		e := m.Outputs[at]
		m.Main = &e
	case m.Stdout == StdoutBinary:
		m.Save, m.SaveWhy = SaveNone, "it prints binary data, which is not shown or saved as text"
	case m.Stdout == StdoutLog:
		m.Save, m.SaveWhy = SaveLog, "it prints only messages; saving them is offered but off by default"
	default:
		m.Save, m.SaveWhy = SaveByCapture, "Mint saves what the tool prints"
	}
	return m
}

// applyIOModel sets the generated app's output settings from the model.
func applyIOModel(spec *appkit.ToolSpec, m IOModel) {
	spec.StdoutKind = m.Stdout
	switch m.Save {
	case SaveByOption:
		spec.OutputsFile = true
	case SaveByPositional:
		spec.OutputsFile, spec.OutputPositional = true, true
	case SaveNone:
		spec.OutputsFile = false
	}
}

// printIOModel explains the model, for mint-gen --explain. Everything that
// came from a help text or a file is cleaned of control characters first.
func printIOModel(w io.Writer, m IOModel, spec appkit.ToolSpec) {
	p := func(format string, a ...any) { fmt.Fprint(w, safeTerminalText(fmt.Sprintf(format, a...))+"\n") }
	p("%s: what goes in and what comes out", m.Tool)
	if m.Template.ID != "" {
		tpl, _ := appkit.LookupTemplate(m.Template.ID)
		p("  Template: %s %s (%s)", m.Template.ID, tpl.Slug, m.Template.Why)
		for i, sc := range m.Template.Scores {
			if i == 3 {
				break
			}
			hits := sc.Hits
			if len(hits) > 4 {
				hits = append(hits[:4:4], "…")
			}
			p("            %s %3d  %s", sc.ID, sc.Score, strings.Join(hits, ", "))
		}
	}
	if m.Usage != "" {
		p("  Usage:    %s", m.Usage)
	}
	if m.Known {
		kt, _ := KnownToolIO(m.Tool)
		p("  Known:    %s", kt.describe())
	}
	list := func(title string, es []IOEndpoint) {
		for i, e := range es {
			head := ""
			if i == 0 {
				head = title
			}
			ext := ""
			if e.Ext != "" {
				ext = " (." + e.Ext + ")"
			}
			p("  %-9s %-18s %-14s %s%s", head, e.label(), roleName(e.Role), e.Why, ext)
		}
	}
	list("Inputs:", m.Inputs)
	list("Outputs:", m.Outputs)
	list("Logs:", m.Logs)
	list("Config:", m.Configs)
	list("Formats:", m.Formats)
	stdout := m.Stdout
	if stdout == "" {
		stdout = "unknown"
	}
	p("  Prints:   %s (%s)", stdout, m.StdoutWhy)
	switch m.Save {
	case SaveByOption, SaveByPositional:
		p("  Saving:   the app passes its output file to %s: %s", m.Main.label(), m.SaveWhy)
	default:
		p("  Saving:   %s", m.SaveWhy)
	}
	if ext := spec.DefaultExt; ext != "" {
		p("  File:     <name>.%s (%s)", ext, m.ExtWhy)
		if fmts := formatOptions(spec); len(fmts) > 0 {
			p("            the app switches the extension to match the format chosen in %s", strings.Join(fmts, ", "))
		}
	}
}

// explainOptions lists every option's role and the evidence for it.
func explainOptions(w io.Writer, spec appkit.ToolSpec) {
	p := func(format string, a ...any) { fmt.Fprint(w, safeTerminalText(fmt.Sprintf(format, a...))+"\n") }
	p("Options and why:")
	for _, f := range spec.Flags {
		if f.Kind == appkit.FlagBool {
			continue
		}
		role, ev := ClassifyArg(spec.Name, f)
		var reasons []string
		for _, e := range sortEvidence(ev) {
			if e.Weight != 0 {
				reasons = append(reasons, e.String())
			}
		}
		if len(reasons) == 0 {
			reasons = []string{"no evidence for a path role"}
		}
		p("  %-22s %-14s %s", f.Flag, roleName(role), strings.Join(reasons, "; "))
	}
}
