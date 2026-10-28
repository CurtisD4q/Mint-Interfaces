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
	// Verify mint-gen --remove removes the scaffolded app directory and binary
	if code := runCLI([]string{"--remove", "--out", outDir, "demo"}); code != 0 {
		t.Fatalf("expected --remove demo to return 0, got %d", code)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat err = %v", outDir, err)
	}
}

func TestDetectArchetypeAndCLIFlags(t *testing.T) {
	if code := runCLI([]string{"--list-archetypes"}); code != 0 {
		t.Fatalf("--list-archetypes returned %d, want 0", code)
	}
	if code := runCLI([]string{"--archetype", "INVALID-T99", "echo"}); code == 0 {
		t.Fatal("expected a non-zero exit code for an unknown --archetype")
	}
	// The old template names and flags are gone.
	if code := runCLI([]string{"--archetype", "T12", "echo"}); code == 0 {
		t.Fatal("an old template ID was accepted")
	}
	for _, flag := range []string{"--template", "--list-templates"} {
		if code := runCLI([]string{flag}); code == 0 {
			t.Fatalf("%s is still accepted", flag)
		}
	}

	// Listed programs: their archetype and base layout.
	known := []struct {
		bin    string
		want   appkit.ArchetypeID
		layout appkit.Archetype
	}{
		{"sysctl", "settings", appkit.ArchetypeConfig},
		{"ufw", "rules", appkit.ArchetypeConfig},
		{"ffmpeg", "convert", appkit.ArchetypeRunner},
		{"curl", "transfer", appkit.ArchetypeRunner},
		{"tar", "pack", appkit.ArchetypeSuite},
		{"nmap", "probe", appkit.ArchetypeScanner},
		{"tshark", "record", appkit.ArchetypeScanner},
		{"whois", "look-up", appkit.ArchetypeExplorer},
		{"nikto", "audit", appkit.ArchetypeRunner},
		{"systemctl", "control", appkit.ArchetypeMonitor},
		{"journalctl", "follow", appkit.ArchetypeMonitor},
		{"git", "history", appkit.ArchetypeSuite},
		{"docker", "toolkit", appkit.ArchetypeSuite},
		{"lsblk", "list", appkit.ArchetypeExplorer},
		{"rg", "search", appkit.ArchetypeExplorer},
		{"jq", "query", appkit.ArchetypeExplorer},
		{"man", "read", appkit.ArchetypeExplorer},
		{"crontab", "schedule", appkit.ArchetypeSuite},
	}
	for _, tc := range known {
		a := DetectArchetype(tc.bin, "", "")
		if a.ID != tc.want || a.Blueprint().Layout != tc.layout {
			t.Errorf("DetectArchetype(%q) = (%s, %s), want (%s, %s)", tc.bin, a.ID, a.Blueprint().Layout, tc.want, tc.layout)
		}
	}

	// Unlisted programs, from their help.
	heuristics := []struct {
		bin, man, help string
		want           appkit.ArchetypeID
	}{
		{"customscan", "NAME\n customscan - fast SYN port scan and host discovery tool\n", "Usage: customscan [options] HOST...\n", "probe"},
		{"customcap", "NAME\n customcap - promiscuous packet capture sniffer to pcap\n", "", "record"},
		{"customfw", "NAME\n customfw - netfilter firewall chain rule editor\n", "", "rules"},
		{"customjq", "NAME\n customjq - lightweight JSON processor and YAML query tool\n", "Usage: customjq [options] FILTER [FILE]...\n", "query"},
		{"customctl", "", "Available Commands:\n  list    List all services\n  start   Start a service\n  stop    Stop a service\n  status  Show status\n", "control"},
	}
	for _, hc := range heuristics {
		spec := ParseManAndHelp(hc.bin, hc.man, hc.help)
		if spec.ArchetypeID != string(hc.want) {
			t.Errorf("ParseManAndHelp(%q).ArchetypeID = %q, want %q", hc.bin, spec.ArchetypeID, hc.want)
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
