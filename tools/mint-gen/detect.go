package main

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

var reSubcmdLine = regexp.MustCompile(`^\s{2,8}([a-z][a-z0-9_-]{1,22})\s{2,}(.+)$`)

// DetectTemplate classifies any CLI program into one of the 28 Template Types
// (T01..T28) using exact tool lookup first, followed by multi-signal heuristics
// over its binary name, parsed flags, synopsis, man page, and --help text.
func DetectTemplate(bin string, spec appkit.ToolSpec, manText, helpText string) appkit.TemplateDef {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(bin)))
	if tpl, ok := appkit.TemplateForTool(name); ok {
		return tpl
	}

	corpus := strings.ToLower(name + " " + spec.Desc + " " + spec.TargetLabel + "\n" + manText + "\n" + helpText)
	subcmds := extractSubcommands(cleanLines(manText), cleanLines(helpText))
	hasSubcmds := len(subcmds) >= 2 || strings.Contains(corpus, "subcommand") || strings.Contains(corpus, "available commands")

	mustLookup := func(id string) appkit.TemplateDef {
		tpl, _ := appkit.LookupTemplate(id)
		return tpl
	}

	// 1. Multi-subcommand suites (T08, T09, T21, T22, T23, T28)
	if hasSubcmds {
		switch {
		case containsAny(corpus, "container", "podman", "docker", "kubernetes", "virtual machine", "hypervisor", "instance"):
			return mustLookup("T22")
		case containsAny(corpus, "package manager", "install package", "remove package", "repository", "upgrade packages", "plugin manager"):
			return mustLookup("T23")
		case containsAny(corpus, "version control", "commit", "branch", "merge", "pull request", "clone repository", "working tree"):
			return mustLookup("T21")
		case containsAny(corpus, "backup", "snapshot", "restore", "deduplicat", "repository", "archive-name"):
			return mustLookup("T09")
		case containsAny(corpus, "archive", "compress", "extract", "tarball", "zip"):
			return mustLookup("T08")
		case containsAny(corpus, "cron", "schedule", "timer", "at job"):
			return mustLookup("T28")
		}
	}

	// 2. Network, Packet, OSINT & Security Scanners/Auditors (T12, T13, T14, T15)
	switch {
	case containsAny(corpus, "pcap", "packet capture", "network traffic", "sniffer", "tcpdump", "wireshark", "promiscuous"):
		return mustLookup("T13")
	case containsAny(corpus, "port scan", "network scan", "network mapper", "host discovery", "syn scan", "open ports", "masscan", "scanner"):
		return mustLookup("T12")
	case containsAny(corpus, "whois", "dns lookup", "domain name", "subdomain", "traceroute", "osint", "reconnaissance", "nameserver"):
		return mustLookup("T14")
	case containsAny(corpus, "vulnerability", "security audit", "malware", "rootkit", "exploit", "cve-", "hardening", "sql injection", "web server scanner"):
		return mustLookup("T15")
	}

	// 3. System Config, Dotfile, Hardware & Firewall Rule Editors (T01, T02, T03, T04)
	switch {
	case containsAny(corpus, "firewall", "iptables", "netfilter", "nftables", "routing table", "packet filter", "chain"):
		return mustLookup("T04")
	case containsAny(corpus, "systemd", "service manager", "init system", "start/stop", "daemon controller", "unit files"):
		return mustLookup("T18")
	case containsAny(corpus, "kernel parameters", "sysctl", "system clock", "timezone", "system locale", "system hostname"):
		return mustLookup("T01")
	case containsAny(corpus, "soundcard", "audio mixer", "pulseaudio", "pipewire", "backlight", "brightness", "xrandr", "bluetooth", "rfkill", "networkmanager"):
		return mustLookup("T03")
	case containsAny(corpus, "dotfile", "configuration file", "get and set", "user configuration", "config key"):
		return mustLookup("T02")
	}

	// 4. Live Monitors, Process Inspectors & Log Analysers (T17, T19, T20)
	switch {
	case containsAny(corpus, "systemd journal", "kernel ring buffer", "log messages", "system log", "audit log", "follow log"):
		return mustLookup("T20")
	case containsAny(corpus, "running processes", "process status", "open files", "signal processes", "process tree", "pid"):
		return mustLookup("T19")
	case containsAny(corpus, "virtual memory statistics", "cpu statistics", "i/o statistics", "free and used memory", "hardware sensors", "system uptime"):
		return mustLookup("T17")
	}

	// 5. Explorers: Disk, Search, Data Query & Documentation (T24, T25, T26, T27)
	switch {
	case containsAny(corpus, "block devices", "partition table", "mounted filesystems", "disk usage", "s.m.a.r.t.", "inodes"):
		return mustLookup("T24")
	case containsAny(corpus, "manual page", "reference manual", "whatis database", "tldr", "info document"):
		return mustLookup("T27")
	case containsAny(corpus, "json processor", "yaml processor", "csv", "sql database", "pattern scanning and processing", "stream editor"):
		return mustLookup("T26")
	case containsAny(corpus, "search for patterns", "regular expression", "search files", "find files", "file type", "printable strings", "metadata"):
		return mustLookup("T25")
	}

	// 6. Specialized Runners: Crypto, Benchmark, Build, Downloader, Transcoder, Archive (T06, T07, T08, T10, T11, T16)
	switch {
	case containsAny(corpus, "message digest", "sha256", "md5", "encrypt", "decrypt", "cipher", "x.509", "password cracker", "cryptographic"):
		return mustLookup("T16")
	case containsAny(corpus, "benchmark", "stress test", "load testing", "requests per second", "bandwidth measurement"):
		return mustLookup("T11")
	case containsAny(corpus, "compiler", "build tool", "run tests", "unit test", "makefile", "source code"):
		return mustLookup("T10")
	case containsAny(corpus, "download", "transfer a url", "retrieve files", "http client", "video downloader"):
		return mustLookup("T07")
	case containsAny(corpus, "transcode", "media converter", "image manipulation", "audio converter", "markup converter", "qr code", "graphviz"):
		return mustLookup("T06")
	case containsAny(corpus, "archive", "compress", "decompress"):
		return mustLookup("T08")
	}

	if hasSubcmds {
		return mustLookup("T21")
	}
	if spec.OutputsFile {
		return mustLookup("T06")
	}
	return mustLookup("T05")
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func extractSubcommands(manLines, helpLines []string) []appkit.SubcommandSpec {
	var out []appkit.SubcommandSpec
	seen := map[string]bool{}

	scan := func(lines []string) {
		inCmdSec := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if strings.HasSuffix(lower, "commands:") ||
				lower == "commands" ||
				lower == "subcommands" ||
				lower == "available commands" ||
				strings.Contains(lower, "main commands") {
				inCmdSec = true
				continue
			}
			if inCmdSec {
				if isManSectionHeader(line) || isHelpSectionHeader(line) {
					inCmdSec = false
					continue
				}
				if m := reSubcmdLine.FindStringSubmatch(line); m != nil {
					cmdName := strings.TrimSpace(m[1])
					cmdDesc := strings.TrimSpace(m[2])
					if cmdName != "help" && cmdName != "version" && !seen[cmdName] && len(out) < 10 {
						seen[cmdName] = true
						out = append(out, appkit.SubcommandSpec{
							Name:  cmdName,
							Label: titleWords(cmdName),
							Desc:  capitalizeFirst(cmdDesc),
							Args:  []string{cmdName},
						})
					}
				}
			}
		}
	}

	scan(helpLines)
	if len(out) == 0 {
		scan(manLines)
	}
	return out
}
