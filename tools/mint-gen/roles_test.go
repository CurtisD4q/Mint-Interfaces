package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func opt(name, hint, desc string, kind appkit.FlagKind) appkit.FlagSpec {
	return appkit.FlagSpec{Flag: name, Hint: hint, Label: desc + " (" + name + ")", Kind: kind}
}

func TestClassifyArg(t *testing.T) {
	F, D, T := appkit.FlagFile, appkit.FlagDir, appkit.FlagText
	cases := []struct {
		tool string
		f    appkit.FlagSpec
		want appkit.ArgRole
	}{
		{"wget", opt("-O", "file", "Write documents to FILE", F), appkit.RoleOutputFile},
		{"wget", opt("-o", "file", "Log messages to FILE", F), appkit.RoleLogFile},
		{"wget", opt("-a", "file", "Append messages to FILE", F), appkit.RoleLogFile},
		{"wget", opt("-P", "prefix", "Save files to PREFIX/..", T), appkit.RoleOutputDir},
		{"wget", opt("--load-cookies", "file", "Load cookies from FILE before session", F), appkit.RoleInputFile},
		{"wget", opt("--config", "file", "Specify config file to use", F), appkit.RoleConfigFile},
		{"wget", opt("--certificate", "file", "Client certificate file", F), appkit.RoleFile},
		{"curl", opt("-o", "file", "Write to file instead of stdout", F), appkit.RoleOutputFile},
		{"curl", opt("-w", "format", "Use output FORMAT after completion", T), appkit.RoleValue},
		{"jq", opt("-L", "directory", "Search modules from the directory;", D), appkit.RoleInputDir},
		{"sort", opt("-T", "dir", "Use DIR for temporaries, not $TMPDIR or /tmp", D), appkit.RoleDir},
		{"tar", opt("-T", "file", "Get names to extract or create from FILE", F), appkit.RoleInputFile},
		{"tar", opt("--one-top-level", "[=dir]", "Create a subdirectory to avoid having loose files extracted", D), appkit.RoleOutputDir},
		{"grep", opt("--include", "glob", "Search only files that match GLOB (a file pattern)", T), appkit.RoleValue},
		{"nmap", opt("-oN", "<file>", "Output scan in normal format", F), appkit.RoleOutputFile},
		{"nmap", opt("-oX", "<file>", "Output scan in XML format", F), appkit.RoleFile}, // only -oN is listed (for now)
		{"nmap", opt("-oA", "<basename>", "Output in the three major formats at once", T), appkit.RoleOutputBase},
		{"pandoc", opt("-t", "{html|pdf|docx|markdown}", "Output format", appkit.FlagChoice), appkit.RoleFormat},
		{"tool", opt("--format", "format", "Output format: json or text", T), appkit.RoleFormat},
		{"ls", opt("--color", "[=when]", "Colorize the output; WHEN can be always, auto or never", T), appkit.RoleValue},
		{"tool", opt("-v", "", "Verbose", appkit.FlagBool), appkit.RoleValue},
		{"tool", opt("--password", "", "Password", appkit.FlagSecret), appkit.RoleValue},
	}
	for _, c := range cases {
		got, ev := ClassifyArg(c.tool, c.f)
		if got != c.want {
			t.Errorf("%s %s: %s, want %s\n  evidence: %v", c.tool, c.f.Flag, got, c.want, sortEvidence(ev))
		}
	}
}

// The app writes to an option's path only for roles that write, so those
// roles need clear evidence. Descriptions about reading, settings, or
// nothing in particular never give one.
func TestClassifyArgNeverGuessesWrites(t *testing.T) {
	descs := []string{
		"Read input from FILE", "Load the filter from FILE", "Use FILE as the configuration",
		"Client certificate file", "File with the bundle of CAs", "Exclude patterns listed in FILE",
		"The file", "", "Use archive file or device ARCHIVE", "Import keys from FILE",
		"Search FILE for patterns", "Take PATTERNS from FILE", "Path to the socket",
	}
	for _, d := range descs {
		for _, name := range []string{"-x", "--thing", "--input"} {
			role, ev := ClassifyArg("tool", opt(name, "file", d, appkit.FlagFile))
			if role.Writes() {
				t.Errorf("%s %q: %s (a role that writes)\n  %v", name, d, role, ev)
			}
		}
	}
}

// Roles reach generated code only as fixed identifiers.
func TestRoleIdentIsFixed(t *testing.T) {
	for _, r := range appkit.Roles() {
		if r == appkit.RoleValue {
			continue
		}
		if id := roleIdent(r); id == "" || !strings.HasPrefix(id, "Role") {
			t.Errorf("%q has no identifier", r)
		}
	}
	for _, bad := range []appkit.ArgRole{"../../x", "output-file\"); os.Exit(1); //", "RoleOutputFile", "unknown"} {
		if id := roleIdent(bad); id != "" {
			t.Errorf("%q gave identifier %q", bad, id)
		}
	}
	spec := appkit.ToolSpec{Name: "x", Flags: []appkit.FlagSpec{{Flag: "-o", Kind: appkit.FlagFile, Role: "\"); panic(1); //"}}}
	src, err := renderSpecGo(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "panic") || strings.Contains(string(src), "Role:") {
		t.Fatalf("an unknown role reached the generated code:\n%s", src)
	}
}

func TestFixedValueAlias(t *testing.T) {
	cases := []struct {
		first, second, arg string
		want               bool
	}{
		{"-p", "--indicator-style", "=slash", true},
		{"-C", "--check", "=quiet", true},
		{"-o", "--output", "=FILE", false},
		{"-S", "--suffix", "=.suf", false},
		{"-f", "--file", "=file", false},
		{"--long", "--other", "=x", false},
	}
	for _, c := range cases {
		if got := isFixedValueAlias(c.first, c.second, c.arg); got != c.want {
			t.Errorf("%s, %s%s: %v", c.first, c.second, c.arg, got)
		}
	}
}
