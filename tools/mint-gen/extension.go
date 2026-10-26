package main

import (
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// textExtensions are extensions of text files, which suit what Mint saves
// from a tool's printed output.
var textExtensions = map[string]bool{"txt": true, "log": true, "out": true, "md": true, "csv": true,
	"tsv": true, "json": true, "jsonl": true, "xml": true, "yaml": true, "html": true, "nmap": true, "gnmap": true}

func isTextExtension(ext string) bool {
	return textExtensions[strings.ToLower(lastWordPart(ext))]
}

// chooseExtension decides the extension of the app's output file, and
// why. In order:
//  1. the known-tools list (tcpdump -w .pcap, ffmpeg .mkv, jq's JSON);
//  2. a format named in the output option's own description ("write JSON
//     to FILE");
//  3. ".log" for a tool that only prints messages;
//  4. the template's, when the tool writes the file itself, or when it is
//     a text extension (Mint saving printed text never calls it .pcap);
//  5. ".txt".
//
// At run time the app also switches the extension of its automatic file
// name to match a chosen output format (pandoc -t html, --json).
func chooseExtension(spec appkit.ToolSpec, m IOModel, outputDesc, tplExt string) (string, string) {
	kt, known := KnownToolIO(spec.Name)
	writes := m.Save == SaveByOption || m.Save == SaveByPositional
	switch {
	case writes && m.Ext != "":
		return m.Ext, "known-tools list"
	case m.Save == SaveByCapture && known && kt.StdoutExt != "":
		return kt.StdoutExt, "known-tools list (what it prints)"
	}
	if writes {
		if ext, ok := extFromDescription(outputDesc); ok {
			return ext, "named in the output option's description"
		}
	}
	if m.Save == SaveLog {
		return "log", "it prints only messages"
	}
	if tplExt != "" && (writes || isTextExtension(tplExt)) {
		return tplExt, "the template's default (" + spec.Template + ")"
	}
	return "txt", "default"
}

// extFromDescription finds a format named in a description: "Write the
// report as HTML to FILE" gives "html".
func extFromDescription(desc string) (string, bool) {
	for _, w := range reWordsIn.FindAllString(desc, -1) {
		if ext, ok := appkit.FormatExtension(w); ok && distinctFormats[strings.ToLower(w)] {
			return ext, true
		}
	}
	return "", false
}

// formatOptions lists the options whose value changes the output format at
// run time: RoleFormat options, and switches named after a format (--json).
func formatOptions(spec appkit.ToolSpec) []string {
	var out []string
	for _, f := range spec.Flags {
		name := strings.TrimPrefix(f.Flag, "--")
		_, isFmt := appkit.FormatExtension(name)
		if f.Role == appkit.RoleFormat || (f.Kind == appkit.FlagBool && strings.HasPrefix(f.Flag, "--") && isFmt && distinctFormats[name]) {
			out = append(out, f.Flag)
		}
	}
	return out
}
