package appkit

import (
	"strings"
	"time"
)

// Archetype identifies one of the 6 base TUI interaction patterns.
type Archetype string

const (
	ArchetypeConfig   Archetype = "config"   // State Editor (read -> tweak -> apply)
	ArchetypeRunner   Archetype = "runner"   // Task & Pipeline (input + flags -> progress -> output)
	ArchetypeScanner  Archetype = "scanner"  // Target -> Live Table + Log
	ArchetypeMonitor  Archetype = "monitor"  // Live Poller & Controller (tick -> gauges/table -> row actions)
	ArchetypeExplorer Archetype = "explorer" // Query -> Results -> Detail
	ArchetypeSuite    Archetype = "suite"    // Multi-Tab Subcommand Manager
)

// AllArchetypes returns the 6 base archetypes in canonical order.
func AllArchetypes() []Archetype {
	return []Archetype{
		ArchetypeConfig,
		ArchetypeRunner,
		ArchetypeScanner,
		ArchetypeMonitor,
		ArchetypeExplorer,
		ArchetypeSuite,
	}
}

// MixinName identifies a composable capability mixin (an add-on).
type MixinName string

const (
	MixinFileOutput        MixinName = "WithFileOutput"
	MixinTableOutput       MixinName = "WithTableOutput"
	MixinCollapsibleOutput MixinName = "WithCollapsibleOutput"
	MixinProfiles          MixinName = "WithProfiles"
	MixinDynamicChoices    MixinName = "WithDynamicChoices"
	MixinRootAuth          MixinName = "WithRootAuth"
	MixinConfirm           MixinName = "WithConfirm"
	MixinHistory           MixinName = "WithHistory"
)

// Mixins describes which of the 8 original composable mixins are enabled
// for a template or spec. Templates set these; later add-ons are switched
// on by their own ToolSpec field and are listed by ToolSpec.EnabledMixinNames.
type Mixins struct {
	WithFileOutput        bool `json:"with_file_output,omitempty"`
	WithTableOutput       bool `json:"with_table_output,omitempty"`
	WithCollapsibleOutput bool `json:"with_collapsible_output,omitempty"`
	WithProfiles          bool `json:"with_profiles,omitempty"`
	WithDynamicChoices    bool `json:"with_dynamic_choices,omitempty"`
	WithRootAuth          bool `json:"with_root_auth,omitempty"`
	WithConfirm           bool `json:"with_confirm,omitempty"`
	WithHistory           bool `json:"with_history,omitempty"`
}

// Names returns the list of enabled mixin identifiers in canonical order.
func (m Mixins) Names() []MixinName {
	var out []MixinName
	if m.WithFileOutput {
		out = append(out, MixinFileOutput)
	}
	if m.WithTableOutput {
		out = append(out, MixinTableOutput)
	}
	if m.WithCollapsibleOutput {
		out = append(out, MixinCollapsibleOutput)
	}
	if m.WithProfiles {
		out = append(out, MixinProfiles)
	}
	if m.WithDynamicChoices {
		out = append(out, MixinDynamicChoices)
	}
	if m.WithRootAuth {
		out = append(out, MixinRootAuth)
	}
	if m.WithConfirm {
		out = append(out, MixinConfirm)
	}
	if m.WithHistory {
		out = append(out, MixinHistory)
	}
	return out
}

// TemplateID is the canonical T01..T28 identifier.
type TemplateID string

const (
	T01 TemplateID = "T01" // System & Daemon Configurator
	T02 TemplateID = "T02" // Dotfile / App Config Editor
	T03 TemplateID = "T03" // Hardware & Device Controller
	T04 TemplateID = "T04" // Firewall & Rule-Table Editor
	T05 TemplateID = "T05" // Single-Target Batch Runner
	T06 TemplateID = "T06" // Media & Document Transcoder
	T07 TemplateID = "T07" // Downloader & Stream Fetcher
	T08 TemplateID = "T08" // Archive & Compression Tool
	T09 TemplateID = "T09" // Backup & Snapshot Manager
	T10 TemplateID = "T10" // Build, Test & Compiler Runner
	T11 TemplateID = "T11" // Benchmark & Stress Tester
	T12 TemplateID = "T12" // Network & Port Scanner
	T13 TemplateID = "T13" // Packet & Traffic Analyser
	T14 TemplateID = "T14" // OSINT & Recon Gatherer
	T15 TemplateID = "T15" // Security & Vulnerability Auditor
	T16 TemplateID = "T16" // Password, Hash & Crypto Utility
	T17 TemplateID = "T17" // Live System Resource Monitor
	T18 TemplateID = "T18" // Service & Daemon Controller
	T19 TemplateID = "T19" // Process & Open-File Inspector
	T20 TemplateID = "T20" // Log Streamer & Journal Analyser
	T21 TemplateID = "T21" // Version Control & Repo Suite
	T22 TemplateID = "T22" // Container & VM Orchestrator
	T23 TemplateID = "T23" // Package & Plugin Manager
	T24 TemplateID = "T24" // Disk, Partition & Filesystem Inspector
	T25 TemplateID = "T25" // File & Content Searcher
	T26 TemplateID = "T26" // Structured Data & Query Explorer
	T27 TemplateID = "T27" // Reference & Documentation Browser
	T28 TemplateID = "T28" // Schedule, Cron & Timer Manager
)

// TemplateDef defines one of the 28 CLI template presets (T01..T28), mapping it
// to one of the 6 base archetypes and a combination of the composable mixins.
type TemplateDef struct {
	ID                 TemplateID       `json:"id"`
	Slug               string           `json:"slug"`
	Name               string           `json:"name"`
	Archetype          Archetype        `json:"archetype"`
	Mixins             Mixins           `json:"mixins"`
	Description        string           `json:"description"`
	ExampleTools       []string         `json:"example_tools"`
	TargetLabel        string           `json:"target_label,omitempty"`
	TargetPlaceholder  string           `json:"target_placeholder,omitempty"`
	DefaultExt         string           `json:"default_ext,omitempty"`
	PollInterval       time.Duration    `json:"poll_interval,omitempty"`
	DefaultSubcommands []SubcommandSpec `json:"default_subcommands,omitempty"`
}

// ApplyTo applies the template's archetype, mixins, and default configuration to spec
// without overwriting fields the caller already customized.
func (t TemplateDef) ApplyTo(spec *ToolSpec) {
	if spec == nil {
		return
	}
	spec.Template = string(t.ID)
	if spec.Archetype == "" {
		spec.Archetype = t.Archetype
	}
	if spec.TargetLabel == "" && t.TargetLabel != "" {
		spec.TargetLabel = t.TargetLabel
	}
	if spec.TargetPlaceholder == "" && t.TargetPlaceholder != "" {
		spec.TargetPlaceholder = t.TargetPlaceholder
	}
	if spec.DefaultExt == "" && t.DefaultExt != "" {
		spec.DefaultExt = t.DefaultExt
	}
	if spec.PollInterval <= 0 && t.PollInterval > 0 {
		spec.PollInterval = t.PollInterval
	}

	if t.Mixins.WithFileOutput {
		spec.OutputsFile = true
	}
	if t.Mixins.WithTableOutput {
		spec.TableOutput = true
	}
	if t.Mixins.WithCollapsibleOutput {
		spec.CollapsibleOutput = true
		if spec.ChunkSize <= 0 {
			spec.ChunkSize = 100
		}
	}
	if t.Mixins.WithRootAuth {
		spec.NeedsRoot = true
	}
	if t.Mixins.WithConfirm {
		spec.ConfirmRun = true
	}
	if t.Mixins.WithHistory {
		spec.KeepHistory = true
	}
	if t.Mixins.WithProfiles && len(spec.Profiles) == 0 {
		spec.Profiles = defaultProfilesForTemplate(t.ID, spec.Flags)
	}
	if t.Archetype == ArchetypeSuite && len(spec.Subcommands) == 0 && len(t.DefaultSubcommands) > 0 {
		spec.Subcommands = append([]SubcommandSpec(nil), t.DefaultSubcommands...)
	}
}

func defaultProfilesForTemplate(id TemplateID, flags []FlagSpec) []ProfileSpec {
	base := map[string]string{}
	for _, f := range flags {
		if f.Kind == FlagBool {
			base[f.Flag] = "false"
		}
	}
	switch id {
	case T12:
		return []ProfileSpec{
			{Name: "Quick", Desc: "Fast scan of common ports", Flags: base},
			{Name: "Intense", Desc: "Version and service detection", Flags: base, ExtraArgs: []string{"-v"}},
			{Name: "Default", Desc: "Standard parameters", Flags: base},
		}
	case T06, T07, T10, T11, T15, T16:
		return []ProfileSpec{
			{Name: "Default", Desc: "Standard invocation", Flags: base},
			{Name: "Fast / Quiet", Desc: "Minimal output", Flags: base},
			{Name: "Verbose", Desc: "Detailed diagnostic output", Flags: base},
		}
	default:
		return []ProfileSpec{
			{Name: "Default", Flags: base},
		}
	}
}

var templateRegistry = []TemplateDef{
	{
		ID:                T01,
		Slug:              "system-config",
		Name:              "System & Daemon Configurator",
		Archetype:         ArchetypeConfig,
		Mixins:            Mixins{WithRootAuth: true, WithTableOutput: true},
		Description:       "Reads system/kernel/daemon parameters and applies updated key-value settings safely.",
		ExampleTools:      []string{"sysctl", "timedatectl", "hostnamectl", "localectl", "loginctl", "resolvectl"},
		TargetLabel:       "Parameter / key",
		TargetPlaceholder: "e.g. net.ipv4.ip_forward=1",
	},
	{
		ID:                T02,
		Slug:              "dotfile-config",
		Name:              "Dotfile / App Config Editor",
		Archetype:         ArchetypeConfig,
		Mixins:            Mixins{WithCollapsibleOutput: true, WithTableOutput: true},
		Description:       "Inspects and edits scoped application configuration keys (global, user, local).",
		ExampleTools:      []string{"git-config", "npm-config", "pip-config", "gh-config", "gsettings", "dconf", "xfconf-query"},
		TargetLabel:       "Config key / value",
		TargetPlaceholder: "section.key value",
	},
	{
		ID:                T03,
		Slug:              "hardware-controller",
		Name:              "Hardware & Device Controller",
		Archetype:         ArchetypeConfig,
		Mixins:            Mixins{WithDynamicChoices: true, WithTableOutput: true},
		Description:       "Discovers connected devices/outputs dynamically and adjusts hardware state or levels.",
		ExampleTools:      []string{"xrandr", "amixer", "pactl", "wpctl", "brightnessctl", "bluetoothctl", "nmcli", "iwctl", "rfkill"},
		TargetLabel:       "Device / interface",
		TargetPlaceholder: "e.g. eth0, eDP-1, Master",
	},
	{
		ID:                T04,
		Slug:              "rule-table-editor",
		Name:              "Firewall & Rule-Table Editor",
		Archetype:         ArchetypeConfig,
		Mixins:            Mixins{WithTableOutput: true, WithRootAuth: true, WithConfirm: true},
		Description:       "Displays ordered rule or routing tables and adds/deletes rules with confirmation.",
		ExampleTools:      []string{"ufw", "iptables", "ip6tables", "nft", "firewall-cmd", "ip-route", "ip-rule", "route", "ip"},
		TargetLabel:       "Rule specification",
		TargetPlaceholder: "e.g. allow 22/tcp",
	},
	{
		ID:                T05,
		Slug:              "batch-runner",
		Name:              "Single-Target Batch Runner",
		Archetype:         ArchetypeRunner,
		Mixins:            Mixins{WithConfirm: true, WithHistory: true},
		Description:       "Runs file-transfer, syncing, or block-level batch jobs with live progress and confirmation.",
		ExampleTools:      []string{"rsync", "scp", "sftp", "dd", "shred", "cp", "mv", "install"},
		TargetLabel:       "Source & destination",
		TargetPlaceholder: "src/ dst/",
	},
	{
		ID:                T06,
		Slug:              "media-transcoder",
		Name:              "Media & Document Transcoder",
		Archetype:         ArchetypeRunner,
		Mixins:            Mixins{WithFileOutput: true, WithProfiles: true, WithHistory: true},
		Description:       "Converts media, images, diagrams, or documents into user-named output files.",
		ExampleTools:      []string{"ffmpeg", "magick", "convert", "mogrify", "pandoc", "sox", "gs", "qrencode", "dot", "gnuplot"},
		TargetLabel:       "Input file",
		TargetPlaceholder: "/path/to/input",
		DefaultExt:        "out",
	},
	{
		ID:                T07,
		Slug:              "downloader",
		Name:              "Downloader & Stream Fetcher",
		Archetype:         ArchetypeRunner,
		Mixins:            Mixins{WithFileOutput: true, WithProfiles: true, WithHistory: true},
		Description:       "Fetches URLs or streams with live transfer progress and output directory management.",
		ExampleTools:      []string{"curl", "wget", "yt-dlp", "aria2c", "axel", "http", "xh"},
		TargetLabel:       "Target URL",
		TargetPlaceholder: "https://example.com/file",
		DefaultExt:        "dat",
	},
	{
		ID:          T08,
		Slug:        "archive-manager",
		Name:        "Archive & Compression Tool",
		Archetype:   ArchetypeSuite,
		Mixins:      Mixins{WithFileOutput: true, WithTableOutput: true, WithHistory: true},
		Description: "Creates, extracts, tests, and lists compressed archives across subcommand tabs.",
		ExampleTools: []string{
			"tar", "7z", "7za", "zip", "unzip", "gzip", "gunzip", "bzip2", "xz", "zstd", "lz4", "cpio", "ar",
		},
		TargetLabel:       "Archive / files",
		TargetPlaceholder: "archive.tar.gz [paths...]",
		DefaultExt:        "tar.gz",
		DefaultSubcommands: []SubcommandSpec{
			{Name: "list", Label: "List Contents", Desc: "List entries in archive", Args: []string{"-tvf"}, ReadOnly: true},
			{Name: "extract", Label: "Extract", Desc: "Extract archive contents", Args: []string{"-xvf"}},
			{Name: "create", Label: "Create", Desc: "Create a new archive", Args: []string{"-czvf"}},
		},
	},
	{
		ID:                T09,
		Slug:              "backup-manager",
		Name:              "Backup & Snapshot Manager",
		Archetype:         ArchetypeSuite,
		Mixins:            Mixins{WithTableOutput: true, WithConfirm: true, WithHistory: true},
		Description:       "Manages backup repositories, snapshots, pruning, and restoration across tabs.",
		ExampleTools:      []string{"borg", "restic", "rclone", "timeshift", "snapper", "duplicity", "kopia"},
		TargetLabel:       "Repository / path",
		TargetPlaceholder: "/path/to/repo",
		DefaultSubcommands: []SubcommandSpec{
			{Name: "snapshots", Label: "Snapshots", Desc: "List snapshots in repository", Args: []string{"snapshots"}, ReadOnly: true},
			{Name: "backup", Label: "Create Backup", Desc: "Run a new backup job", Args: []string{"backup"}},
			{Name: "check", Label: "Verify", Desc: "Check repository integrity", Args: []string{"check"}, ReadOnly: true},
			{Name: "prune", Label: "Prune", Desc: "Prune old snapshots", Args: []string{"prune"}, Confirm: true},
		},
	},
	{
		ID:                T10,
		Slug:              "build-test-runner",
		Name:              "Build, Test & Compiler Runner",
		Archetype:         ArchetypeRunner,
		Mixins:            Mixins{WithCollapsibleOutput: true, WithProfiles: true, WithHistory: true},
		Description:       "Runs compilers, build targets, linters, and test suites with foldable pass/fail output.",
		ExampleTools:      []string{"go", "make", "cargo", "ninja", "cmake", "pytest", "gcc", "g++", "clang", "javac", "tsc", "eslint", "shellcheck"},
		TargetLabel:       "Target / package / file",
		TargetPlaceholder: "./...",
	},
	{
		ID:                T11,
		Slug:              "benchmark-tester",
		Name:              "Benchmark & Stress Tester",
		Archetype:         ArchetypeRunner,
		Mixins:            Mixins{WithTableOutput: true, WithProfiles: true, WithHistory: true},
		Description:       "Executes load, latency, I/O, or CPU benchmarks and formats summary metrics into a table.",
		ExampleTools:      []string{"hyperfine", "ab", "wrk", "hey", "vegeta", "sysbench", "fio", "iperf3", "stress-ng"},
		TargetLabel:       "Target command / endpoint",
		TargetPlaceholder: "https://example.com or command",
	},
	{
		ID:                T12,
		Slug:              "network-scanner",
		Name:              "Network & Port Scanner",
		Archetype:         ArchetypeScanner,
		Mixins:            Mixins{WithTableOutput: true, WithCollapsibleOutput: true, WithFileOutput: true, WithProfiles: true, WithRootAuth: true, WithHistory: true},
		Description:       "Scans hosts, networks, and ports with live table parsing and saved scan reports.",
		ExampleTools:      []string{"nmap", "masscan", "rustscan", "zmap", "arp-scan", "fping", "netdiscover", "nbtscan", "onesixtyone"},
		TargetLabel:       "Target host / CIDR",
		TargetPlaceholder: "scanme.nmap.org or 192.168.1.0/24",
		DefaultExt:        "txt",
	},
	{
		ID:                T13,
		Slug:              "packet-analyser",
		Name:              "Packet & Traffic Analyser",
		Archetype:         ArchetypeScanner,
		Mixins:            Mixins{WithDynamicChoices: true, WithTableOutput: true, WithFileOutput: true, WithRootAuth: true, WithHistory: true},
		Description:       "Captures or inspects live network packets and sockets with interface discovery and pcap export.",
		ExampleTools:      []string{"tshark", "tcpdump", "ngrep", "dumpcap", "ss", "netstat", "conntrack"},
		TargetLabel:       "Capture / display filter",
		TargetPlaceholder: "port 443 or host 10.0.0.1",
		DefaultExt:        "pcap",
	},
	{
		ID:                T14,
		Slug:              "osint-recon",
		Name:              "OSINT & Recon Gatherer",
		Archetype:         ArchetypeScanner,
		Mixins:            Mixins{WithTableOutput: true, WithCollapsibleOutput: true, WithFileOutput: true, WithHistory: true},
		Description:       "Queries DNS, WHOIS, routing paths, subdomains, and OSINT records into structured tables.",
		ExampleTools:      []string{"whois", "dig", "host", "nslookup", "drill", "traceroute", "tracepath", "mtr", "subfinder", "amass", "theHarvester", "dnsrecon"},
		TargetLabel:       "Domain / IP / host",
		TargetPlaceholder: "example.com",
		DefaultExt:        "txt",
	},
	{
		ID:                T15,
		Slug:              "security-auditor",
		Name:              "Security & Vulnerability Auditor",
		Archetype:         ArchetypeRunner,
		Mixins:            Mixins{WithCollapsibleOutput: true, WithFileOutput: true, WithProfiles: true, WithRootAuth: true, WithHistory: true},
		Description:       "Runs host, container, web, or malware audits and groups findings into collapsible sections.",
		ExampleTools:      []string{"nikto", "lynis", "clamscan", "rkhunter", "chkrootkit", "trivy", "grype", "checksec", "sqlmap", "gobuster", "ffuf", "nuclei", "wpscan"},
		TargetLabel:       "Audit target / path / URL",
		TargetPlaceholder: "https://example.com or /",
		DefaultExt:        "report.txt",
	},
	{
		ID:                T16,
		Slug:              "crypto-hash-tool",
		Name:              "Password, Hash & Crypto Utility",
		Archetype:         ArchetypeRunner,
		Mixins:            Mixins{WithFileOutput: true, WithProfiles: true, WithHistory: true},
		Description:       "Computes digests, encrypts/decrypts files, inspects certificates, or audits password hashes.",
		ExampleTools:      []string{"openssl", "gpg", "age", "sha256sum", "sha512sum", "sha1sum", "md5sum", "b2sum", "cksum", "john", "hashcat", "base64"},
		TargetLabel:       "Input file / subject",
		TargetPlaceholder: "/path/to/file",
		DefaultExt:        "out",
	},
	{
		ID:                T17,
		Slug:              "resource-monitor",
		Name:              "Live System Resource Monitor",
		Archetype:         ArchetypeMonitor,
		Mixins:            Mixins{WithTableOutput: true},
		Description:       "Polls memory, CPU, disk, temperature, or battery metrics at a configurable interval.",
		ExampleTools:      []string{"free", "vmstat", "iostat", "mpstat", "pidstat", "sensors", "acpi", "uptime", "df", "w", "who", "nvidia-smi"},
		TargetLabel:       "Filter / arguments",
		TargetPlaceholder: "optional arguments",
		PollInterval:      2 * time.Second,
	},
	{
		ID:                T18,
		Slug:              "service-controller",
		Name:              "Service & Daemon Controller",
		Archetype:         ArchetypeMonitor,
		Mixins:            Mixins{WithTableOutput: true, WithRootAuth: true, WithConfirm: true},
		Description:       "Monitors system services/units and provides Start, Stop, Restart, Enable, and Logs actions.",
		ExampleTools:      []string{"systemctl", "supervisorctl", "rc-service", "service", "sv", "s6-svc"},
		TargetLabel:       "Unit / service filter",
		TargetPlaceholder: "e.g. nginx.service",
		PollInterval:      3 * time.Second,
	},
	{
		ID:                T19,
		Slug:              "process-inspector",
		Name:              "Process & Open-File Inspector",
		Archetype:         ArchetypeMonitor,
		Mixins:            Mixins{WithTableOutput: true, WithConfirm: true},
		Description:       "Lists running processes or open file descriptors in a live table with signal/kill controls.",
		ExampleTools:      []string{"ps", "pgrep", "pkill", "killall", "lsof", "fuser", "pstree", "lsns", "ipcs"},
		TargetLabel:       "Process / user / port filter",
		TargetPlaceholder: "e.g. -ef or process name",
		PollInterval:      2 * time.Second,
	},
	{
		ID:                T20,
		Slug:              "log-analyser",
		Name:              "Log Streamer & Journal Analyser",
		Archetype:         ArchetypeMonitor,
		Mixins:            Mixins{WithCollapsibleOutput: true, WithFileOutput: true},
		Description:       "Streams and filters system journals, kernel ring buffers, and log files with export support.",
		ExampleTools:      []string{"journalctl", "dmesg", "tail", "ausearch", "last", "lastlog", "faillock"},
		TargetLabel:       "Log unit / file / grep",
		TargetPlaceholder: "-n 100 or /var/log/syslog",
		DefaultExt:        "log",
		PollInterval:      2 * time.Second,
	},
	{
		ID:                T21,
		Slug:              "vcs-suite",
		Name:              "Version Control & Repo Suite",
		Archetype:         ArchetypeSuite,
		Mixins:            Mixins{WithTableOutput: true, WithCollapsibleOutput: true},
		Description:       "Multi-tab version control interface covering status, commit log, branches, diff, and remotes.",
		ExampleTools:      []string{"git", "gh", "glab", "hg", "svn", "fossil"},
		TargetLabel:       "Repository / arguments",
		TargetPlaceholder: ".",
		DefaultSubcommands: []SubcommandSpec{
			{Name: "status", Label: "Status", Desc: "Working tree status", Args: []string{"status", "--short", "--branch"}, ReadOnly: true},
			{Name: "log", Label: "Commit Log", Desc: "Recent commit history", Args: []string{"log", "--oneline", "-n", "30"}, ReadOnly: true},
			{Name: "branch", Label: "Branches", Desc: "Local and remote branches", Args: []string{"branch", "-a"}, ReadOnly: true},
			{Name: "diff", Label: "Diff", Desc: "Unstaged changes", Args: []string{"diff", "--stat"}, ReadOnly: true},
		},
	},
	{
		ID:                T22,
		Slug:              "container-orchestrator",
		Name:              "Container & VM Orchestrator",
		Archetype:         ArchetypeSuite,
		Mixins:            Mixins{WithTableOutput: true, WithConfirm: true},
		Description:       "Multi-tab manager for containers, images, volumes, pods, or virtual machines.",
		ExampleTools:      []string{"docker", "podman", "nerdctl", "kubectl", "helm", "virsh", "qm", "pct", "lxc", "incus", "vagrant"},
		TargetLabel:       "Resource / container / pod",
		TargetPlaceholder: "optional filter or ID",
		DefaultSubcommands: []SubcommandSpec{
			{Name: "ps", Label: "Containers", Desc: "Running and stopped containers", Args: []string{"ps", "-a"}, ReadOnly: true},
			{Name: "images", Label: "Images", Desc: "Local container images", Args: []string{"images"}, ReadOnly: true},
			{Name: "volumes", Label: "Volumes", Desc: "Managed storage volumes", Args: []string{"volume", "ls"}, ReadOnly: true},
			{Name: "networks", Label: "Networks", Desc: "Configured networks", Args: []string{"network", "ls"}, ReadOnly: true},
		},
	},
	{
		ID:                T23,
		Slug:              "package-manager",
		Name:              "Package & Plugin Manager",
		Archetype:         ArchetypeSuite,
		Mixins:            Mixins{WithTableOutput: true, WithRootAuth: true, WithConfirm: true},
		Description:       "Searches, lists, installs, upgrades, and removes system or language packages.",
		ExampleTools:      []string{"pacman", "yay", "paru", "apt", "apt-get", "apt-cache", "dnf", "yum", "zypper", "apk", "brew", "flatpak", "snap", "pip", "pipx", "npm", "pnpm", "gem"},
		TargetLabel:       "Package name / query",
		TargetPlaceholder: "package name",
		DefaultSubcommands: []SubcommandSpec{
			{Name: "list", Label: "Installed", Desc: "List installed packages", Args: []string{"list"}, ReadOnly: true},
			{Name: "search", Label: "Search", Desc: "Search available packages", Args: []string{"search"}, ReadOnly: true},
			{Name: "install", Label: "Install", Desc: "Install a package", Args: []string{"install"}},
			{Name: "remove", Label: "Remove", Desc: "Remove a package", Args: []string{"remove"}, Confirm: true},
		},
	},
	{
		ID:                T24,
		Slug:              "disk-inspector",
		Name:              "Disk, Partition & Filesystem Inspector",
		Archetype:         ArchetypeExplorer,
		Mixins:            Mixins{WithDynamicChoices: true, WithTableOutput: true, WithRootAuth: true},
		Description:       "Explores block devices, mountpoints, SMART health, and directory disk usage.",
		ExampleTools:      []string{"lsblk", "blkid", "findmnt", "du", "smartctl", "hdparm", "nvme", "btrfs", "zfs", "zpool", "lvs", "vgs", "pvs", "mdadm"},
		TargetLabel:       "Device / mountpoint / path",
		TargetPlaceholder: "/dev/sda or /",
	},
	{
		ID:                T25,
		Slug:              "content-searcher",
		Name:              "File & Content Searcher",
		Archetype:         ArchetypeExplorer,
		Mixins:            Mixins{WithTableOutput: true, WithFileOutput: true},
		Description:       "Searches file names, regular expressions, or file metadata and drills into matches.",
		ExampleTools:      []string{"rg", "grep", "egrep", "fgrep", "find", "fd", "locate", "plocate", "ack", "ag", "exiftool", "mediainfo", "ffprobe", "file", "stat", "strings", "readelf", "objdump", "nm", "ldd"},
		TargetLabel:       "Search pattern & path",
		TargetPlaceholder: "pattern [path...]",
		DefaultExt:        "matches.txt",
	},
	{
		ID:                T26,
		Slug:              "data-query-explorer",
		Name:              "Structured Data & Query Explorer",
		Archetype:         ArchetypeExplorer,
		Mixins:            Mixins{WithTableOutput: true, WithCollapsibleOutput: true, WithFileOutput: true},
		Description:       "Runs expressions or SQL/pipeline queries against JSON, YAML, CSV, SQLite, or text files.",
		ExampleTools:      []string{"jq", "yq", "sqlite3", "psql", "mysql", "csvtk", "xsv", "mlr", "awk", "gawk", "sed", "cut", "sort", "uniq", "tr", "column", "wc"},
		TargetLabel:       "Query expression & input file",
		TargetPlaceholder: "'.' data.json",
		DefaultExt:        "out.txt",
	},
	{
		ID:                T27,
		Slug:              "doc-browser",
		Name:              "Reference & Documentation Browser",
		Archetype:         ArchetypeExplorer,
		Mixins:            Mixins{WithCollapsibleOutput: true},
		Description:       "Looks up manuals, cheat sheets, and reference pages and folds each section cleanly.",
		ExampleTools:      []string{"man", "tldr", "apropos", "whatis", "info", "help", "perldoc", "pydoc3", "go-doc"},
		TargetLabel:       "Topic / command name",
		TargetPlaceholder: "e.g. printf, socket, git",
	},
	{
		ID:                T28,
		Slug:              "schedule-manager",
		Name:              "Schedule, Cron & Timer Manager",
		Archetype:         ArchetypeSuite,
		Mixins:            Mixins{WithTableOutput: true, WithConfirm: true},
		Description:       "Lists, adds, and removes scheduled cron jobs, at queues, and recurring tasks.",
		ExampleTools:      []string{"crontab", "at", "atq", "atrm", "anacron", "batch"},
		TargetLabel:       "Schedule / job specification",
		TargetPlaceholder: "-l",
		DefaultSubcommands: []SubcommandSpec{
			{Name: "list", Label: "Active Jobs", Desc: "List scheduled entries", Args: []string{"-l"}, ReadOnly: true},
			{Name: "remove", Label: "Clear / Remove", Desc: "Remove scheduled entries", Args: []string{"-r"}, Confirm: true},
		},
	},
}

// AllTemplates returns a copy of all 28 TemplateDef presets (T01..T28).
func AllTemplates() []TemplateDef {
	out := make([]TemplateDef, len(templateRegistry))
	copy(out, templateRegistry)
	return out
}

// LookupTemplate finds a TemplateDef by ID ("T01".."T28"), slug ("network-scanner"),
// or case-insensitive name.
func LookupTemplate(idOrSlug string) (TemplateDef, bool) {
	q := strings.TrimSpace(idOrSlug)
	if q == "" {
		return TemplateDef{}, false
	}
	upper := strings.ToUpper(q)
	lower := strings.ToLower(q)
	for _, t := range templateRegistry {
		if string(t.ID) == upper || t.Slug == lower || strings.EqualFold(t.Name, q) {
			return t, true
		}
	}
	return TemplateDef{}, false
}

// TemplateForTool checks whether bin is a known tool in any of the 28 templates
// and returns its matching TemplateDef.
func TemplateForTool(bin string) (TemplateDef, bool) {
	name := strings.ToLower(strings.TrimSpace(bin))
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	for _, t := range templateRegistry {
		for _, ex := range t.ExampleTools {
			if strings.EqualFold(ex, name) {
				return t, true
			}
		}
	}
	return TemplateDef{}, false
}
