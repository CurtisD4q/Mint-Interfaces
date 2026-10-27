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
		{"nmap", opt("-oX", "<file>", "Output scan in XML format", F), appkit.RoleOutputFile}, // listed in the known-tools list (XML output)
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

// A folder option is never given the app's output file, however usual its
// name: tcpflow's "-o outdir" is the folder it writes transcripts into.
func TestFolderOptionsAreNotOutputFiles(t *testing.T) {
	for _, c := range []struct {
		flag, hint, desc string
		want             appkit.ArgRole
	}{
		{"-o", "outdir", "Specifies the output directory where the transcript files will be written.", appkit.RoleOutputDir},
		{"--dest", "destdir", "where to save the results", appkit.RoleOutputDir},
		{"-C", "path", "the working directory to run in", appkit.RoleDir},
		// A description about directories does not make a time or a name a folder.
		{"--changed-since", "time", "List the files that have changed in the destination directory since the given time.", appkit.RoleValue},
		{"--imap-mailbox", "imap_mailbox", "Name of the imap folder to store backups", appkit.RoleValue},
	} {
		f := appkit.FlagSpec{Flag: c.flag, Kind: appkit.FlagText, Hint: c.hint, Label: c.desc + " (" + c.flag + ")"}
		role, ev := ClassifyArg("tool9", f)
		if role == appkit.RoleOutputFile {
			t.Errorf("%s %s: output file %v", c.flag, c.hint, ev)
		}
		if c.want == appkit.RoleValue && role != appkit.RoleValue {
			t.Errorf("%s %s: %s, want a plain value %v", c.flag, c.hint, role, ev)
		}
		if c.want == appkit.RoleOutputDir && role != appkit.RoleOutputDir {
			t.Errorf("%s %s: %s, want output dir %v", c.flag, c.hint, role, ev)
		}
	}
}

// "Send output (stdout) from podman to a file" writes the output, whatever
// "from" suggests.
func TestSendOutputToFile(t *testing.T) {
	for _, desc := range []string{"Send output (stdout) from podman to a file", "Redirect the output of podman to the specified path"} {
		f := appkit.FlagSpec{Flag: "--out", Kind: appkit.FlagFile, Hint: "path", Label: desc + " (--out)"}
		if role, ev := ClassifyArg("tool9", f); role != appkit.RoleOutputFile {
			t.Errorf("%q: %s %v", desc, role, ev)
		}
	}
}

// A "prefix" placeholder is a folder only when the description is about
// paths: "--prefix=PREFIX install into PREFIX/bin", not "--default-search
// PREFIX  Use this prefix for unqualified URLs".
func TestPrefixPlaceholder(t *testing.T) {
	folder := appkit.FlagSpec{Flag: "--prefix", Kind: appkit.FlagText, Hint: "PREFIX", Label: "install files under this directory (--prefix)"}
	if role, ev := ClassifyArg("tool9", folder); role != appkit.RoleDir && role != appkit.RoleOutputDir {
		t.Errorf("--prefix: %s %v", role, ev)
	}
	install := appkit.FlagSpec{Flag: "--prefix", Kind: appkit.FlagText, Hint: "prefix", Label: "Override the installation prefix (--prefix)"}
	if role, ev := ClassifyArg("tool9", install); role != appkit.RoleDir && role != appkit.RoleOutputDir {
		t.Errorf("installation prefix: %s %v", role, ev)
	}
	url := appkit.FlagSpec{Flag: "--default-search", Kind: appkit.FlagText, Hint: "PREFIX", Label: "Use this prefix for unqualified URLs, e.g. downloads two videos (--default-search)"}
	if role, ev := ClassifyArg("tool9", url); role != appkit.RoleValue {
		t.Errorf("--default-search: %s %v", role, ev)
	}
}

// --explain gives the reason for the role an option got.
func TestExplainReasonMatchesRole(t *testing.T) {
	// The strongest evidence here is for an input file (+5, "reads"); the
	// option's role is output file, so its reason must be the output one.
	spec := appkit.ToolSpec{Name: "tool9", Flags: []appkit.FlagSpec{{Flag: "-o", Kind: appkit.FlagFile, Hint: "path",
		Label: "write the result; reads the input first (-o)", Role: appkit.RoleOutputFile}}}
	m := BuildIOModel(spec, Synopsis{})
	if len(m.Outputs) != 1 || strings.Contains(m.Outputs[0].Why, "reads") {
		t.Fatalf("outputs: %+v", m.Outputs)
	}
}
