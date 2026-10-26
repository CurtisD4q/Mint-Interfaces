package appkit

import (
	"regexp"
	"strings"
)

// formatExtensions maps output format names, as tools spell them, to file
// extensions. Only extensions from this fixed list are ever used, so a
// format value typed into an app can never put anything else into a file
// name.
var formatExtensions = map[string]string{
	"json": "json", "jsonl": "jsonl", "ndjson": "ndjson", "xml": "xml", "csv": "csv", "tsv": "tsv",
	"yaml": "yaml", "yml": "yaml", "toml": "toml",
	"html": "html", "html4": "html", "html5": "html", "xhtml": "xhtml",
	"markdown": "md", "md": "md", "gfm": "md", "commonmark": "md", "commonmark_x": "md", "markdown_strict": "md",
	"pdf": "pdf", "docx": "docx", "odt": "odt", "rtf": "rtf", "epub": "epub", "epub2": "epub", "epub3": "epub",
	"latex": "tex", "tex": "tex", "rst": "rst", "org": "org", "asciidoc": "adoc", "pptx": "pptx", "ipynb": "ipynb",
	"plain": "txt", "text": "txt", "txt": "txt",
	"png": "png", "svg": "svg", "jpg": "jpg", "jpeg": "jpg", "gif": "gif", "webp": "webp",
	"mp4": "mp4", "mkv": "mkv", "webm": "webm", "mp3": "mp3", "ogg": "ogg", "wav": "wav", "flac": "flac",
	"pcap": "pcap", "pcapng": "pcapng", "sqlite": "sqlite",
}

// FormatExtension returns the file extension for a format value such as
// "html5", "gfm+smart" or "JSON", if it is a known format.
func FormatExtension(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.IndexAny(v, "+-:,"); i > 0 { // pandoc's "gfm+smart", "markdown-raw_html"
		v = v[:i]
	}
	ext, ok := formatExtensions[v]
	return ext, ok
}

// outputExt is the extension for the app's automatic file name: the one
// for the output format chosen in a format option (pandoc -t html), or a
// switch named after a format (--json), when it is a known format; else
// Spec.DefaultExt.
func (a *ToolApp) outputExt() string {
	for _, f := range a.Spec.Flags {
		v := strings.TrimSpace(a.Values[f.Flag])
		switch {
		case f.Role == RoleFormat && v != "":
			if ext, ok := FormatExtension(v); ok {
				return ext
			}
		case f.Kind == FlagBool && (v == "true" || v == "1" || v == "on" || v == "yes"):
			if ext, ok := formatExtensions[strings.TrimPrefix(f.Flag, "--")]; ok && strings.HasPrefix(f.Flag, "--") {
				return ext
			}
		}
	}
	if ext, ok := a.formatInExtraArgs(); ok {
		return ext
	}
	if ext := strings.TrimPrefix(a.Spec.DefaultExt, "."); ext != "" {
		return ext
	}
	return "txt"
}

// formatInExtraArgs finds a format option typed into Extra arguments
// ("-t docx", "--to=docx", "--json").
func (a *ToolApp) formatInExtraArgs() (string, bool) {
	formatFlags := map[string]bool{}
	for _, f := range a.Spec.Flags {
		if f.Role == RoleFormat {
			formatFlags[f.Flag] = true
		}
	}
	words := splitShellWords(a.ExtraArgs)
	for i, w := range words {
		name, val, hasEq := strings.Cut(w, "=")
		switch {
		case formatFlags[name] && hasEq:
			if ext, ok := FormatExtension(val); ok {
				return ext, true
			}
		case formatFlags[w] && i+1 < len(words):
			if ext, ok := FormatExtension(words[i+1]); ok {
				return ext, true
			}
		case strings.HasPrefix(w, "--") && !hasEq:
			if ext, ok := formatExtensions[strings.TrimPrefix(w, "--")]; ok {
				return ext, true
			}
		}
	}
	return "", false
}

// withOutputExt gives an automatic file name (tool-YYYYMMDD-HHMMSS.ext,
// from DefaultOutputFilename) the extension of the chosen output format.
// Any other name, typed by the user, is left exactly as it is.
func (a *ToolApp) withOutputExt(name string) string {
	stamp := regexp.MustCompile(`^` + regexp.QuoteMeta(outputNameBase(a.Spec.Name)) + `-\d{8}-\d{6}\.[A-Za-z0-9.+-]+$`)
	if !stamp.MatchString(name) {
		return name
	}
	return name[:strings.IndexByte(name, '.')] + "." + a.outputExt()
}
