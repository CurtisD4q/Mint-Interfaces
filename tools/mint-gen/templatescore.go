package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// templateSignals are the words and phrases that point to each template.
// They are matched as whole words (a phrase's spaces match any spacing),
// so "pid" never matches "rapid" and "chain" never matches "toolchain".
var templateSignals = map[string][]string{
	"T01": {"kernel parameter", "kernel parameters", "sysctl", "system clock", "time zone", "timezone", "system locale", "host name", "hostname", "system hostname"},
	"T02": {"dotfile", "dotfiles", "get and set", "config key", "user configuration", "configuration value", "configuration values"},
	"T03": {"sound card", "soundcard", "audio mixer", "mixer", "pulseaudio", "pipewire", "backlight", "brightness", "bluetooth", "rfkill", "networkmanager", "display resolution", "screen resolution"},
	"T04": {"firewall", "iptables", "netfilter", "nftables", "routing table", "packet filter", "firewall rule", "firewall rules"},
	"T05": {"copy files", "move files", "remove files", "remove the", "delete files", "rename", "transfer files", "file transfer", "overwrite", "securely delete", "synchronize", "unlink", "create the directory", "permissions", "file mode", "ownership", "access and modification times", "symbolic link"},
	"T06": {"transcode", "transcoder", "converter", "convert encoding", "encoding", "media", "video", "audio", "image", "images", "markup", "pdf", "qr code", "graphviz", "render", "rendering", "document converter"},
	"T07": {"download", "downloads", "downloader", "http client", "transfer a url", "retrieve files", "fetch", "web server"},
	"T08": {"archive", "archives", "compress", "compressed", "decompress", "uncompress", "compressor", "compression", "tarball", "zip archive"},
	"T09": {"backup", "backups", "snapshot", "snapshots", "restore", "deduplication", "deduplicating"},
	"T10": {"compiler", "compile", "build", "builds", "interpreter", "programming language", "javascript", "run tests", "unit test", "unit tests", "makefile", "source code", "script", "scripts", "debugger"},
	"T11": {"benchmark", "benchmarking", "stress test", "load test", "load testing", "requests per second", "bandwidth", "throughput"},
	"T12": {"port scan", "port scanner", "network scan", "network scanner", "network mapper", "host discovery", "open ports", "syn scan"},
	"T13": {"pcap", "packet capture", "capture packets", "network traffic", "sniffer", "promiscuous"},
	"T14": {"whois", "dns", "dns lookup", "domain name", "subdomain", "subdomains", "traceroute", "reconnaissance", "osint", "name server", "nameserver"},
	"T15": {"vulnerability", "vulnerabilities", "security audit", "malware", "rootkit", "exploit", "cve", "hardening", "sql injection", "web server scanner", "virus"},
	"T16": {"message digest", "checksum", "checksums", "hash", "hashes", "sha256", "md5", "sha1", "cipher", "cryptographic", "x.509", "certificate", "password cracker", "encrypt", "decrypt", "signature"},
	"T17": {"memory statistics", "virtual memory", "cpu statistics", "i/o statistics", "free memory", "used memory", "sensors", "uptime", "load average", "processing units", "system information", "cpu usage"},
	"T18": {"systemd", "service manager", "init system", "daemon", "daemons", "unit files", "services"},
	"T19": {"process", "processes", "pid", "pids", "signal", "signals", "process status", "process tree", "open files", "niceness", "scheduling priority", "time limit", "kill", "hangup"},
	"T20": {"journal", "log messages", "system log", "kernel ring buffer", "audit log", "follow log", "syslog"},
	"T21": {"version control", "commit", "commits", "branch", "branches", "merge", "pull request", "working tree", "revision", "revisions"},
	"T22": {"container", "containers", "docker", "podman", "kubernetes", "virtual machine", "virtual machines", "hypervisor", "pod", "pods"},
	"T23": {"package manager", "install packages", "install package", "remove packages", "upgrade packages", "plugin manager", "packages"},
	"T24": {"block device", "block devices", "partition", "partitions", "partition table", "mounted filesystems", "file system", "file systems", "filesystem", "filesystems", "disk usage", "disk space", "inodes", "mount", "persistent storage"},
	"T25": {"search", "searches", "find files", "regular expression", "file type", "printable strings", "metadata", "matching", "locate", "list information", "directory contents", "list directory"},
	"T26": {"json", "yaml", "csv", "tsv", "sql", "database", "query", "stream editor", "lines", "line", "line by line", "fields", "columns", "random permutation", "shuffle", "concatenate", "paragraph", "tabs", "characterwise", "sorted"},
	"T27": {"manual page", "manual pages", "man page", "man pages", "reference manual", "documentation", "info document", "whatis", "tldr", "pager", "file perusal"},
	"T28": {"cron", "crontab", "schedule", "scheduled", "timer", "timers", "at job"},
}

// suiteOnlyTemplates are for programs with subcommands; their words count
// only when the program has some (git commit, docker run…).
var suiteOnlyTemplates = map[string]bool{"T09": true, "T21": true, "T22": true, "T23": true, "T28": true}

// Where a word is found, and how much that counts.
const (
	weightName    = 8 // the program's own name
	weightSummary = 5 // its one-line summary
	weightUsage   = 2 // its usage lines
	weightBody    = 1 // anywhere else in the help or man page
	maxBodyHits   = 2 // per phrase, so a long man page cannot win on length
	maxBodyScore  = 4 // per template, likewise; below one summary hit, so the summary wins

	minTemplateScore  = 4 // the winner needs at least this…
	minTemplateMargin = 2 // …and this much more than the runner-up
)

var templatePatterns = func() map[string][]*regexp.Regexp {
	out := map[string][]*regexp.Regexp{}
	for id, phrases := range templateSignals {
		for _, ph := range phrases {
			words := strings.Fields(ph)
			for i, w := range words {
				words[i] = regexp.QuoteMeta(w)
			}
			out[id] = append(out[id], regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])`+strings.Join(words, `\s+`)+`(?:$|[^A-Za-z0-9_])`))
		}
	}
	return out
}()

// TemplateScore is how well a template fits a program, and why.
type TemplateScore struct {
	ID    string
	Score int
	Hits  []string // "summary: converter", "body: pdf ×2"…
}

// ScoreTemplates scores every template for a program by the words in its
// name, summary, usage lines and the rest of its help, best first.
func ScoreTemplates(name, summary string, usage []string, body string, hasSubcmds bool) []TemplateScore {
	zones := []struct {
		label  string
		text   string
		weight int
		capped bool
	}{
		{"name", strings.NewReplacer("-", " ", "_", " ").Replace(name), weightName, false},
		{"summary", summary, weightSummary, false},
		{"usage", strings.Join(usage, "\n"), weightUsage, false},
		{"body", body, weightBody, true},
	}
	var out []TemplateScore
	for id, pats := range templatePatterns {
		if suiteOnlyTemplates[id] && !hasSubcmds {
			continue
		}
		ts := TemplateScore{ID: id}
		bodyScore := 0
		counted := map[string]bool{} // zone+stem: "line" and "lines" are one piece of evidence
		for i, re := range pats {
			phrase := templateSignals[id][i]
			for _, z := range zones {
				if z.text == "" || counted[z.label+"|"+phraseStem(phrase)] {
					continue
				}
				n := len(re.FindAllStringIndex(z.text, -1))
				if n == 0 {
					continue
				}
				counted[z.label+"|"+phraseStem(phrase)] = true
				if !z.capped {
					ts.Score += z.weight
					ts.Hits = append(ts.Hits, z.label+": "+phrase)
					continue
				}
				if n > maxBodyHits {
					n = maxBodyHits
				}
				add := n * z.weight
				if bodyScore+add > maxBodyScore {
					add = maxBodyScore - bodyScore
				}
				if add > 0 {
					bodyScore += add
					ts.Score += add
					ts.Hits = append(ts.Hits, fmt.Sprintf("body: %s ×%d", phrase, n))
				}
			}
		}
		if ts.Score > 0 {
			out = append(out, ts)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// pickTemplate chooses the clear winner, or "" when none is clear enough.
func pickTemplate(scores []TemplateScore) (string, string) {
	if len(scores) == 0 {
		return "", "no template's words appear in the help"
	}
	best := scores[0]
	if best.Score < minTemplateScore {
		return "", fmt.Sprintf("too little evidence (best: %s with %d)", best.ID, best.Score)
	}
	if len(scores) > 1 && best.Score-scores[1].Score < minTemplateMargin {
		return "", fmt.Sprintf("no clear winner (%s %d, %s %d)", best.ID, best.Score, scores[1].ID, scores[1].Score)
	}
	return best.ID, fmt.Sprintf("best evidence (%d points)", best.Score)
}

// TemplateChoice is the template a program got, and why.
type TemplateChoice struct {
	ID     string
	Why    string
	Scores []TemplateScore // best first (for --explain)
}

// chooseTemplate picks a template: the built-in tool list first, then the
// best-scoring template, then a fallback (a suite for programs with
// subcommands, else the general batch runner).
func chooseTemplate(bin string, spec appkit.ToolSpec, manText, helpText string) TemplateChoice {
	name := strings.ToLower(toolBaseName(bin))
	if tpl, ok := appkit.TemplateForTool(name); ok {
		return TemplateChoice{ID: string(tpl.ID), Why: "listed for " + name + " in the template's tools"}
	}
	manLines, helpLines := cleanLines(manText), cleanLines(helpText)
	subcmds := extractSubcommands(manLines, helpLines)
	lowerAll := strings.ToLower(manText + "\n" + helpText)
	hasSubcmds := len(subcmds) >= 2 || strings.Contains(lowerAll, "subcommand") || strings.Contains(lowerAll, "available commands")
	usage := usageLines(name, manLines, helpLines)
	// The real summary only: not the "<Name> command-line interface" stand-in.
	summary := extractSummary(name, manLines, helpLines)
	scores := ScoreTemplates(name, summary, usage, manText+"\n"+helpText, hasSubcmds)
	if id, why := pickTemplate(scores); id != "" {
		return TemplateChoice{ID: id, Why: why, Scores: scores}
	} else if hasSubcmds {
		return TemplateChoice{ID: "T21", Why: why + "; it has subcommands", Scores: scores}
	} else {
		return TemplateChoice{ID: "T05", Why: why + "; the general runner", Scores: scores}
	}
}

// phraseStem is a phrase without a plural "s"/"es", so word forms of one
// idea ("line", "lines"; "hash", "hashes") count once.
func phraseStem(p string) string {
	p = strings.ToLower(p)
	switch {
	case strings.HasSuffix(p, "ses") || strings.HasSuffix(p, "shes") || strings.HasSuffix(p, "ches"):
		return strings.TrimSuffix(p, "es")
	case strings.HasSuffix(p, "s") && !strings.HasSuffix(p, "ss") && len(p) > 3:
		return strings.TrimSuffix(p, "s")
	}
	return p
}
