package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestCheckCompatibilityCategories(t *testing.T) {
	cases := []struct {
		bin      string
		wantComp bool
		wantCat  ExclusionCategory
	}{
		// 1. Shell built-ins & parent-environment mutators
		{"cd", false, CatShellBuiltin},
		{"export", false, CatShellBuiltin},
		{"alias", false, CatShellBuiltin},
		{"source", false, CatShellBuiltin},
		{"fg", false, CatShellBuiltin},
		// 2. Full-screen TUIs / curses / pagers / multiplexers
		{"vim", false, CatFullScreenTUI},
		{"htop", false, CatFullScreenTUI},
		{"top", false, CatFullScreenTUI},
		{"less", false, CatFullScreenTUI},
		{"tmux", false, CatFullScreenTUI},
		{"ncdu", false, CatFullScreenTUI},
		// 3. Graphical X11/Wayland apps
		{"wireshark", false, CatGraphicalGUI},
		{"gimp", false, CatGraphicalGUI},
		{"firefox", false, CatGraphicalGUI},
		{"zenity", false, CatGraphicalGUI},
		// 4. Direct /dev/tty prompts
		{"passwd", false, CatDirectTTY},
		{"su", false, CatDirectTTY},
		{"ssh-add", false, CatDirectTTY},
		{"visudo", false, CatDirectTTY},
		// 5. Terminal graphics renderers
		{"chafa", false, CatTerminalGraphics},
		{"viu", false, CatTerminalGraphics},
		{"timg", false, CatTerminalGraphics},
		{"lsix", false, CatTerminalGraphics},
		// 6. Terminal state manipulators & recorders
		{"script", false, CatTerminalState},
		{"reset", false, CatTerminalState},
		{"stty", false, CatTerminalState},
		{"tput", false, CatTerminalState},
		// Compatible CLI tools
		{"ls", true, CatCompatible},
		{"grep", true, CatCompatible},
		{"nmap", true, CatCompatible},
	}

	for _, tc := range cases {
		rep := CheckCompatibility(tc.bin)
		if rep.Compatible != tc.wantComp {
			t.Errorf("CheckCompatibility(%q).Compatible = %v, want %v (reason: %s)", tc.bin, rep.Compatible, tc.wantComp, rep.Reason)
		}
		if rep.Category != tc.wantCat {
			t.Errorf("CheckCompatibility(%q).Category = %q, want %q", tc.bin, rep.Category, tc.wantCat)
		}
	}
}

func TestParseManAndHelpOverstrikeAndFlags(t *testing.T) {
	manText := "N\bNA\bAM\bME\bE\n" +
		"       f\bfa\bast\btsx\bx - high-speed network scanner\n\n" +
		"S\bSY\bYN\bNO\bOP\bPS\bSI\bIS\bS\n" +
		"       fastsx [OPTIONS] <target>\n\n" +
		"S\bSC\bCA\bAN\bN O\bOP\bPT\bTI\bIO\bON\bNS\bS\n" +
		"       -v, --verbose  Enable verbose output\n" +
		"       -t <threads>   Number of worker threads\n" +
		"       --format={json|csv|text}  Output format\n" +
		"       -c <config-file>  Configuration file path\n" +
		"       -d <data-dir>     Directory for cache data\n" +
		"       -o <file>         Write output to file\n"

	spec := ParseManAndHelp("fastsx", manText, "")
	if spec.Desc != "High-speed network scanner" {
		t.Fatalf("spec.Desc = %q, want %q", spec.Desc, "High-speed network scanner")
	}
	if !strings.Contains(strings.ToLower(spec.TargetLabel), "target") {
		t.Fatalf("spec.TargetLabel = %q, expected target host label", spec.TargetLabel)
	}
	if !spec.OutputsFile || spec.OutputFlag != "-o" {
		t.Fatalf("expected OutputsFile=true OutputFlag=-o, got OutputsFile=%v OutputFlag=%q", spec.OutputsFile, spec.OutputFlag)
	}

	byFlag := map[string]appkit.FlagSpec{}
	for _, f := range spec.Flags {
		byFlag[f.Flag] = f
	}
	if byFlag["-v"].Kind != appkit.FlagBool {
		t.Errorf("-v kind = %q, want FlagBool", byFlag["-v"].Kind)
	}
	if byFlag["-t"].Kind != appkit.FlagInt {
		t.Errorf("-t kind = %q, want FlagInt", byFlag["-t"].Kind)
	}
	if byFlag["--format"].Kind != appkit.FlagChoice || len(byFlag["--format"].Choices) != 4 {
		t.Errorf("--format = %+v, expected FlagChoice with 4 choices (Default + 3)", byFlag["--format"])
	}
	if byFlag["-c"].Kind != appkit.FlagFile {
		t.Errorf("-c kind = %q, want FlagFile", byFlag["-c"].Kind)
	}
	if byFlag["-d"].Kind != appkit.FlagDir {
		t.Errorf("-d kind = %q, want FlagDir", byFlag["-d"].Kind)
	}
}

func TestScaffoldAppAndCLI(t *testing.T) {
	tmp := t.TempDir()
	repoRoot := detectRepoRoot()

	// 1. --check on excluded program returns non-zero
	if code := runCLI([]string{"--check", "vim"}); code == 0 {
		t.Fatalf("expected non-zero exit code for --check vim")
	}
	// 2. --check on compatible program returns 0
	if code := runCLI([]string{"--check", "grep"}); code != 0 {
		t.Fatalf("expected 0 exit code for --check grep, got %d", code)
	}
	// 3. Scaffolding an excluded program without --force fails
	if code := runCLI([]string{"--out", filepath.Join(tmp, "vim"), "vim"}); code == 0 {
		t.Fatalf("expected non-zero exit code when scaffolding vim without --force")
	}

	// 4. Scaffold a valid spec into tmp/demo
	spec := appkit.ToolSpec{
		Name:        "demo",
		Title:       "◈ Demo",
		Desc:        "Demo CLI tool",
		TargetLabel: "Input",
		OutputsFile: true,
		OutputFlag:  "-o",
		DefaultExt:  "txt",
		Flags: []appkit.FlagSpec{
			{Flag: "-v", Kind: appkit.FlagBool, Section: "General", Label: "Verbose (-v)"},
		},
	}
	outDir := filepath.Join(tmp, "demo")
	res, err := ScaffoldApp(spec, outDir, repoRoot)
	if err != nil {
		t.Fatalf("ScaffoldApp error: %v", err)
	}
	for _, f := range res.Files {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected file %s to exist: %v", f, err)
		}
	}
	// Verify the generated app module builds cleanly into a static binary
	if err := buildScaffoldedApp(res.AppDir, res.BinaryOut); err != nil {
		t.Fatalf("buildScaffoldedApp failed: %v", err)
	}
	if _, err := os.Stat(res.BinaryOut); err != nil {
		t.Fatalf("expected compiled binary %s to exist: %v", res.BinaryOut, err)
	}
	// Verify mint-gen --run launches the compiled binary (passing --version so it exits 0 without a TTY)
	if code := runCLI([]string{"--run", "--out", outDir, "demo", "--version"}); code != 0 {
		t.Fatalf("expected --run demo --version to return 0, got %d", code)
	}
}

func TestDetectTemplateAndCLIFlags(t *testing.T) {
	if code := runCLI([]string{"--list-templates"}); code != 0 {
		t.Fatalf("--list-templates returned %d, want 0", code)
	}
	if code := runCLI([]string{"--template", "INVALID-T99", "echo"}); code == 0 {
		t.Fatalf("expected non-zero exit code for invalid --template")
	}

	knownCases := []struct {
		bin     string
		wantID  appkit.TemplateID
		wantArc appkit.Archetype
	}{
		{"sysctl", appkit.T01, appkit.ArchetypeConfig},
		{"ufw", appkit.T04, appkit.ArchetypeConfig},
		{"ffmpeg", appkit.T06, appkit.ArchetypeRunner},
		{"curl", appkit.T07, appkit.ArchetypeRunner},
		{"tar", appkit.T08, appkit.ArchetypeSuite},
		{"nmap", appkit.T12, appkit.ArchetypeScanner},
		{"tshark", appkit.T13, appkit.ArchetypeScanner},
		{"whois", appkit.T14, appkit.ArchetypeScanner},
		{"nikto", appkit.T15, appkit.ArchetypeRunner},
		{"systemctl", appkit.T18, appkit.ArchetypeMonitor},
		{"journalctl", appkit.T20, appkit.ArchetypeMonitor},
		{"git", appkit.T21, appkit.ArchetypeSuite},
		{"docker", appkit.T22, appkit.ArchetypeSuite},
		{"lsblk", appkit.T24, appkit.ArchetypeExplorer},
		{"rg", appkit.T25, appkit.ArchetypeExplorer},
		{"jq", appkit.T26, appkit.ArchetypeExplorer},
		{"man", appkit.T27, appkit.ArchetypeExplorer},
		{"crontab", appkit.T28, appkit.ArchetypeSuite},
	}

	for _, tc := range knownCases {
		tpl := DetectTemplate(tc.bin, appkit.ToolSpec{Name: tc.bin}, "", "")
		if tpl.ID != tc.wantID || tpl.Archetype != tc.wantArc {
			t.Errorf("DetectTemplate(%q) = (%s, %s), want (%s, %s)",
				tc.bin, tpl.ID, tpl.Archetype, tc.wantID, tc.wantArc)
		}
	}

	// Test heuristic classification on unknown CLI tools
	heuristics := []struct {
		bin    string
		man    string
		help   string
		wantID appkit.TemplateID
	}{
		{"customscan", "NAME\n customscan - fast SYN port scan and host discovery tool\n", "", appkit.T12},
		{"customcap", "NAME\n customcap - promiscuous packet capture sniffer to pcap\n", "", appkit.T13},
		{"customfw", "NAME\n customfw - netfilter firewall chain rule editor\n", "", appkit.T04},
		{"customjq", "NAME\n customjq - lightweight JSON processor and YAML query tool\n", "", appkit.T26},
		{"customctl", "", "Available Commands:\n  list    List all containers\n  start   Start a container\n", appkit.T22},
	}
	for _, hc := range heuristics {
		spec := ParseManAndHelp(hc.bin, hc.man, hc.help)
		if spec.Template != string(hc.wantID) {
			t.Errorf("ParseManAndHelp(%q).Template = %q, want %q", hc.bin, spec.Template, hc.wantID)
		}
	}
}

func TestProbeTermFlag(t *testing.T) {
	t.Setenv("MINT_TERMCAPS", t.TempDir()+"/termcaps.json")
	t.Setenv("TERM", "xterm-256color")
	if code := runCLI([]string{"--probe-term"}); code != 0 {
		t.Fatalf("expected --probe-term exit code 0, got %d", code)
	}
}
