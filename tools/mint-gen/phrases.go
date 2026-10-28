package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Phrase lists. Each entry is one idea; "a|b|c" lists ways of saying it,
// and the idea counts once per place it is found however many ways appear.
// Entries starting with "~" are weak: ordinary words that many programs use
// ("lines", "process", "search"), which count a little in a program's
// name, summary or usage and not at all in the rest of its help. Matching
// is by whole words, with common endings allowed (see signalPattern), and a
// phrase found inside a longer matched phrase does not count on its own
// ("SQL injection" is security evidence, not also "SQL" data evidence).

// archetypeBroadPhrases are each archetype's broader phrases: the subjects
// its programs are usually about (media, packages, version control…),
// where archetypeSignals (archetype.go) say what its programs do. They
// count for a share of their points (archWeights.BroadShare).
var archetypeBroadPhrases = map[string][]string{
	"settings":   {"sysctl|hostnamectl|timedatectl|localectl", "kernel parameter", "system clock|system time|time and date|date and time", "time zone|timezone", "system locale|locale", "~hostname|host name", "system hostname|static hostname", "keyboard layout", "ntp|time synchronization", "dotfile", "get and set", "config key|configuration key|configuration value", "user configuration", "settings database|configuration database|configuration system", "dconf|gsettings|gconf|xfconf", "schema", "~settings", "~key"},
	"device":     {"sound card|soundcard|audio mixer|mixer", "alsa", "pulseaudio|pipewire", "backlight|brightness", "bluetooth", "rfkill|wireless", "networkmanager", "display resolution|screen resolution|xrandr|randr", "~volume", "~monitor|output"},
	"rules":      {"firewall|firewall rule", "iptables|ip6tables|netfilter|nftables", "routing table|routing", "packet filter|packet filtering", "network device|network devices", "tunnel", "~interface", "~chain", "~rule", "nat|masquerade"},
	"edit-files": {"copy files|file copy|copy a file|copies files", "~copy", "move files", "~move", "remove files|delete files", "~rename", "transfer files|file transfer|remote file|remote host", "~overwrite", "securely delete|shred", "synchronize|synchronization|mirror", "unlink", "create the directory", "permissions|file mode", "ownership", "access and modification times", "symbolic link"},
	"convert":    {"transcode|converter|document converter|format conversion", "convert encoding", "~encoding", "~audio|video|media|image|multimedia", "markup", "~pdf", "qr code", "graphviz|graph|diagram|layout", "~render", "codec", "postscript", "~convert", "message digest", "checksum", "hash|sha256|sha1|md5|sha512", "cipher", "cryptographic|cryptography", "x.509", "password cracker", "encrypt|decrypt", "~signature|signing", "openpgp|pgp|gnupg", "~ssl|tls", "~certificate", "~private key|public key"},
	"transfer":   {"download|downloader", "http client", "transfer a url|transfer data|retrieve files", "~fetch", "~url", "web server", "~ftp", "~http"},
	"pack":       {"archive|archiver|tarball", "compress|decompress|uncompress|compression|compressor", "zip archive", "~extract", "~zip|7z|gzip|bzip2|xz|zstd"},
	"history":    {"backup", "snapshot", "~restore", "deduplication|deduplicating", "cloud storage", "~repository", "incremental backup", "version control|revision control|source control|source code management|scm", "~commit", "~branch", "~merge", "pull request", "working tree|working copy", "revision|changeset", "mercurial|subversion", "~clone"},
	"build":      {"compiler|compile", "~build", "interpreter", "programming language", "javascript", "run tests|test suite|testing|test framework|unit test", "makefile", "~source code", "~script", "debugger", "build system|build tool", "assembler|linker"},
	"measure":    {"benchmark|stress test|load test", "requests per second", "~bandwidth", "~throughput", "~performance", "~latency", "tester", "workload"},
	"probe":      {"port scan|port scanner|network scanner|network scan", "network mapper", "host discovery", "open ports", "syn scan", "~scan|scanner", "arp", "ping sweep", "~port"},
	"record":     {"pcap|libpcap|pcapng", "packet capture|capture packets|captured packets", "network traffic|traffic", "sniffer|sniff", "promiscuous", "~packet", "dump traffic", "~network interface"},
	"look-up":    {"whois", "dns|nameserver|name server|dns lookup", "domain name", "subdomain", "traceroute|route packets|hop", "reconnaissance|osint", "~lookup", "~host"},
	"audit":      {"vulnerability", "security audit|security auditing|auditing tool", "malware", "rootkit", "exploit", "cve", "hardening", "sql injection", "web server scanner", "virus|antivirus", "~security", "penetration testing|pentest", "compliance"},
	"watch":      {"memory statistics|virtual memory", "cpu statistics|cpu usage|cpu utilization", "i/o statistics|input/output statistics", "free memory|used memory|amount of free", "sensor|temperature|fan speed", "uptime|load average", "processing units", "system information", "~statistics", "resource monitor|system monitor", "~resource"},
	"control":    {"systemd", "service manager|init system", "daemon", "unit files", "~service", "supervisord|supervisor|process control system", "control applications"},
	"list":       {"~process", "pid", "~signal", "process status|process tree|process viewer|list of processes|process list|current processes", "open files|lsof", "niceness|scheduling priority", "~time limit", "kill", "~hangup", "process id", "block device", "partition|partition table", "mounted filesystems", "~file system|filesystem", "disk usage|disk space|space usage|file space", "~disk", "inode", "~mount", "persistent storage", "s.m.a.r.t|smart", "hard disk|hard drive|nvme|sata"},
	"follow":     {"journal|journald", "log messages|log entries|log entry|log files", "~log", "system log|syslog", "kernel ring buffer|ring buffer", "audit log", "~follow", "logged in|login|last logged|wtmp|btmp"},
	"toolkit":    {"container", "docker|podman", "kubernetes|k8s", "virtual machine|hypervisor|virtualization|libvirt", "~pod", "~guest"},
	"records":    {"package manager|package management", "install packages|install package|remove packages|upgrade packages", "plugin manager", "~package", "~install", "~upgrade", "~dependency", "rpm|dpkg|pypi"},
	"search":     {"~search", "find files", "regular expression|regex", "file type", "printable strings", "metadata", "~matching", "locate", "list information", "directory contents|list directory", "match patterns|lines that match|matching lines", "~pattern"},
	"query":      {"~json|yaml|csv|tsv", "json processor", "~sql|database", "sqlite|database shell|sql shell", "~query", "stream editor", "~line", "line by line", "~field", "~column", "random permutation|shuffle", "concatenate", "paragraph", "tab", "characterwise", "~sorted", "pattern scanning and processing|text processing|transform text"},
	"read":       {"manual page|man page|manual", "reference manual", "~documentation", "info document|texinfo", "whatis", "tldr", "pager", "file perusal", "help pages|cheat sheet|cheatsheet"},
	"schedule":   {"cron|crontab", "~schedule|scheduled|scheduling", "~timer", "at job", "later execution|later time", "periodic|periodically", "~job", "~batch", "anacron"},
}

// dataTopicPhrases say what a program's help is about when that makes its
// results private: keys and passwords (secret); captured traffic, people
// and organisations, security findings and logs (sensitive). See
// topicDataLevel.
var dataTopicPhrases = map[string][]string{
	"keys and passwords":       {"message digest", "checksum", "hash|sha256|sha1|md5|sha512", "cipher", "cryptographic|cryptography", "x.509", "password cracker", "encrypt|decrypt", "~signature|signing", "openpgp|pgp|gnupg", "~ssl|tls", "~certificate", "~private key|public key"},
	"captured traffic":         {"pcap|libpcap|pcapng", "packet capture|capture packets|captured packets", "network traffic|traffic", "sniffer|sniff", "promiscuous", "~packet", "dump traffic", "~network interface"},
	"people and organisations": {"whois", "dns|nameserver|name server|dns lookup", "domain name", "subdomain", "traceroute|route packets|hop", "reconnaissance|osint", "~lookup", "~host"},
	"security findings":        {"vulnerability", "security audit|security auditing|auditing tool", "malware", "rootkit", "exploit", "cve", "hardening", "sql injection", "web server scanner", "virus|antivirus", "~security", "penetration testing|pentest", "compliance"},
	"logs":                     {"journal|journald", "log messages|log entries|log entry|log files", "~log", "system log|syslog", "kernel ring buffer|ring buffer", "audit log", "~follow", "logged in|login|last logged|wtmp|btmp"},
}

// scoreWeights says how much each kind of phrase evidence counts and when
// the best group is clear enough to choose.
type scoreWeights struct {
	Name, Summary, Description, Usage, Body           int // a word in the program's name, summary, description, usage lines, rest of its help
	WeakName, WeakSummary, WeakDescription, WeakUsage int // weak words ("~"); they never count in the body
	BodyHits                                          int // at most this many body matches per idea…
	BodyCap                                           int // …and this many body points per group, so a long man page cannot win on length
	MinScore, MinMargin                               int // the winner needs at least MinScore, and MinMargin more than the runner-up
}

var defaultWeights = scoreWeights{
	Name: 8, Summary: 5, Description: 3, Usage: 2, Body: 1,
	WeakName: 3, WeakSummary: 2, WeakDescription: 1, WeakUsage: 1,
	BodyHits: 2, BodyCap: 4,
	MinScore: 4, MinMargin: 3,
}

// signal is one idea of one group.
type signal struct {
	group string // the group (an archetype or a data topic) it is evidence for; "" for a neutral phrase
	label string // the first way of saying it, for reports
	weak  bool
	re    *regexp.Regexp
	// For long texts: a lower-case word every match starts with (one per
	// way of saying it), and how far past it a match can reach. The regexp
	// then runs only around those words (see zoneMatches).
	firsts []string
	reach  int
}

// signalFirstWord is the literal a way of saying an idea always starts
// with: its first word, without the last letter of a one-word way (whose
// ending can change: "query" is also "queries").
func signalFirstWord(way string) string {
	words := strings.Fields(strings.ToLower(way))
	if len(words) == 0 {
		return ""
	}
	w := words[0]
	if len(words) == 1 && len(w) > 3 {
		w = w[:len(w)-1]
	}
	return w
}

var reAlphaWord = regexp.MustCompile(`^[a-z]+$`)

// signalPattern matches one way of saying an idea as whole words, letting
// the last word take common endings: plurals for all ("virus" matches
// "viruses", "query" matches "queries"), and -ed/-ing/-ion/-er for longer
// words ("encrypt" matches "encryption", "benchmark" matches "benchmarking").
func signalPattern(phrase string) string {
	words := strings.Fields(strings.ToLower(phrase))
	for i, w := range words {
		q := regexp.QuoteMeta(w)
		if i == len(words)-1 && reAlphaWord.MatchString(w) {
			switch {
			case len(w) >= 4 && strings.HasSuffix(w, "y") && !strings.HasSuffix(w, "ey"):
				q = regexp.QuoteMeta(w[:len(w)-1]) + `(?:y|ies|ied|ying)` // query, queries; repository, repositories
			case len(w) >= 5 && strings.HasSuffix(w, "e"):
				q = regexp.QuoteMeta(w[:len(w)-1]) + `(?:e|es|ed|ing|er|ers|ion|ions)`
			case len(w) >= 5:
				q += `(?:s|es|ed|ing|er|ers|ion|ions)?`
			default:
				q += `(?:s|es)?`
			}
		}
		words[i] = q
	}
	return strings.Join(words, `[\s/-]+`)
}

// neutralPhrases mean something else than the shorter signals inside
// them: a "user interface" is not a network interface, and a "command
// line" is not a line of text. They score nothing, but as longer matches
// they hide the shorter ones (see zoneMatches).
var neutralPhrases = []string{
	"user interface|command line interface|graphical interface|programming interface|text interface|terminal interface|web interface",
	"command line|cmd line",
}

var (
	broadSignalList     = buildSignals(archetypeBroadPhrases)
	dataTopicSignalList = buildSignals(dataTopicPhrases)
	// The data topics compete with the other subjects a program can be
	// about: a topic decides only when it clearly leads them all, so a
	// package manager that mentions checksums is not about keys.
	topicContestList = buildSignals(topicContest())
)

// topicContest is the data topics and, as rivals, each archetype's broad
// phrases without the topics' own.
func topicContest() map[string][]string {
	inTopic := map[string]bool{}
	out := map[string][]string{}
	for t, entries := range dataTopicPhrases {
		out[t] = entries
		for _, e := range entries {
			inTopic[e] = true
		}
	}
	for a, entries := range archetypeBroadPhrases {
		var rest []string
		for _, e := range entries {
			if !inTopic[e] {
				rest = append(rest, e)
			}
		}
		if len(rest) > 0 {
			out[rivalPrefix+a] = rest
		}
	}
	return out
}

const rivalPrefix = "rival:"

// buildSignals compiles groups of phrases (group -> phrases; see the
// notation above), with the neutral phrases under "" so they hide the
// shorter signals inside them.
func buildSignals(groups map[string][]string) []signal {
	var out []signal
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	all := map[string][]string{"": neutralPhrases} // "": scores for nothing
	for id, entries := range groups {
		all[id] = entries
	}
	for _, id := range append([]string{""}, ids...) {
		for _, entry := range all[id] {
			weak := strings.HasPrefix(entry, "~")
			ways := strings.Split(strings.TrimPrefix(entry, "~"), "|")
			var alts []string
			for _, a := range ways {
				alts = append(alts, signalPattern(a))
			}
			// Longer ways first, so the regexp prefers "port scanner" to "port".
			sort.SliceStable(alts, func(i, j int) bool { return len(alts[i]) > len(alts[j]) })
			re := regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])(` + strings.Join(alts, "|") + `)(?:$|[^A-Za-z0-9_])`)
			sig := signal{group: id, label: ways[0], weak: weak, re: re}
			for _, w := range ways {
				sig.firsts = append(sig.firsts, signalFirstWord(w))
				if n := 4*len(w) + 32; n > sig.reach {
					sig.reach = n
				}
			}
			out = append(out, sig)
		}
	}
	return out
}

// PhraseScore is how well a group of phrases fits a program, and why.
type PhraseScore struct {
	ID    string
	Score int
	Hits  []string // "summary: converter", "body: pdf ×2"…
}

type signalMatch struct {
	sig        int // index in the signal list
	start, end int
}

// zoneMatchesIn counts each idea's matches in text, dropping any match that
// lies inside a longer match of any group: the longer phrase is the
// meaning ("SQL injection", not "SQL").
//
// In prose (a description or a whole man page), a match joined to more of
// a word by a hyphen is part of an option or key name, not prose:
// "--layout", "kill-line", "--smart-case". The summary and usage lines
// keep it ("--json" in a usage line is a fair hint).

// zoneMatchesIn is zoneMatches for another signal list.
func zoneMatchesIn(list []signal, text string, prose bool) map[int]int {
	var all []signalMatch
	lower := ""
	if len(text) > longZone {
		lower = asciiLower(text)
	}
	for i, s := range list {
		for _, loc := range signalLocations(s, text, lower) {
			if prose && inHyphenatedName(text, loc[2], loc[3]) {
				continue
			}
			all = append(all, signalMatch{i, loc[2], loc[3]})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].end-all[i].start > all[j].end-all[j].start })
	counts := map[int]int{}
	var kept []signalMatch
	for _, m := range all {
		inside := false
		for _, k := range kept {
			if k.start <= m.start && m.end <= k.end && k.end-k.start > m.end-m.start {
				inside = true
				break
			}
		}
		if !inside {
			kept = append(kept, m)
			counts[m.sig]++
		}
	}
	return counts
}

// programText is what the scorer reads about a program.
type programText struct {
	Name        string
	Summary     string   // one line: what it is (see extractSummary)
	Description string   // the opening of the man page's DESCRIPTION (see extractDescription)
	Usage       []string // usage lines
	Body        string   // all of the man page and help
	ArchBody    string   // the body the archetype phrases are read in: without credits, SEE ALSO and BUGS (see quietBody)
	Related     []string // programs from SEE ALSO

	manLines, helpLines []string          // cleaned, for reading behaviour (see archetype.go)
	options             []appkit.FlagSpec // options from the help, then the man page
}

// programTextFrom gathers a programText from man and help text.
func programTextFrom(name, manText, helpText string) programText {
	manLines, helpLines := cleanLines(manText), cleanLines(helpText)
	return programText{
		Name:        name,
		Summary:     extractSummary(name, manLines, helpLines),
		Description: extractDescription(manLines),
		Usage:       usageLines(name, manLines, helpLines),
		Body:        manText + "\n" + helpText,
		ArchBody:    quietBody(manText, helpText, manLines, helpLines),
		Related:     seeAlsoPrograms(manText),
		manLines:    manLines,
		helpLines:   helpLines,
		options:     append(parseFlagsFromLines(helpLines, false), parseFlagsFromLines(manLines, true)...),
	}
}

// phraseEvidence is what was found in a program's help, before weighing:
// how often each idea appears in each zone. Finding it is the slow part;
// weighing it is quick.
type phraseEvidence struct {
	zones [numZones]map[int]int // see zoneLabels: idea index -> matches
}

// findEvidenceIn finds the phrases of list in a program's help.
func findEvidenceIn(list []signal, p programText) phraseEvidence {
	name, summary, usage, body := p.Name, p.Summary, p.Usage, p.Body
	var ev phraseEvidence
	// A program's own name in its usage and help is not evidence about
	// itself: "crontab" all through crontab's help says nothing about
	// what crontab is.
	self := regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(strings.ToLower(name)) + `($|[^A-Za-z0-9_-])`)
	// Matches cannot share the character between two uses of the name
	// ("crontab crontab"), so repeat until none is left. Each pass removes
	// at least one, so this always ends (in practice after two passes).
	mask := func(t string) string {
		for {
			m := self.ReplaceAllString(t, "${1}${2}")
			if m == t {
				return t
			}
			t = m
		}
	}
	nameZone := strings.NewReplacer("-", " ", "_", " ").Replace(name)
	if !strings.ContainsAny(name, "-_") {
		nameZone = "" // a one-word name is the program itself
	}
	// The summary is the exception: there the name is used as a word
	// ("Print sensors information", "maintain crontab files").
	texts := [numZones]string{nameZone, summary, mask(p.Description), mask(strings.Join(usage, "\n")), mask(body)}
	for z, text := range texts {
		if text != "" {
			ev.zones[z] = zoneMatchesIn(list, text, z == zoneDescription || z == zoneBody)
		}
	}
	return ev
}

// longZone is the length above which a signal's regexp runs only around
// the words its matches start with. Regexps that ignore case and check
// word edges cannot skip ahead on their own, and a 2 MB man page (rclone's)
// took half a minute; the result is the same.
const longZone = 8 << 10

// asciiLower lower-cases ASCII letters only, so every position in the
// result is the same position in the original.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// signalLocations is s.re.FindAllStringSubmatchIndex(text, -1). For a long
// text (lower is its asciiLower) it runs the regexp only in windows around
// the words a match must start with, merged where they overlap. A window
// starts one byte before the word, so the regexp still sees what comes
// before it.
func signalLocations(s signal, text, lower string) [][]int {
	if lower == "" {
		return s.re.FindAllStringSubmatchIndex(text, -1)
	}
	type span struct{ from, to int }
	var spans []span
	for _, w := range s.firsts {
		if w == "" {
			return s.re.FindAllStringSubmatchIndex(text, -1)
		}
		for off := 0; ; {
			i := strings.Index(lower[off:], w)
			if i < 0 {
				break
			}
			at := off + i
			from, to := at-1, at+s.reach
			if from < 0 {
				from = 0
			}
			if to > len(text) {
				to = len(text)
			}
			spans = append(spans, span{from, to})
			off = at + 1
		}
	}
	if len(spans) == 0 {
		return nil
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].from < spans[j].from })
	merged := spans[:1]
	for _, sp := range spans[1:] {
		last := &merged[len(merged)-1]
		if sp.from <= last.to {
			if sp.to > last.to {
				last.to = sp.to
			}
			continue
		}
		merged = append(merged, sp)
	}
	var out [][]int
	for _, sp := range merged {
		for _, loc := range s.re.FindAllStringSubmatchIndex(text[sp.from:sp.to], -1) {
			for k := range loc {
				if loc[k] >= 0 {
					loc[k] += sp.from
				}
			}
			out = append(out, loc)
		}
	}
	return out
}

// inHyphenatedName reports whether text[start:end] is joined by a hyphen
// to a letter or digit on either side, or follows one ("--layout").
func inHyphenatedName(text string, start, end int) bool {
	isWordByte := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
	}
	if start > 0 && text[start-1] == '-' {
		return true
	}
	return end+1 < len(text) && text[end] == '-' && isWordByte(text[end+1])
}

// The zones a program's text is read in, most telling first.
const (
	zoneName = iota
	zoneSummary
	zoneDescription
	zoneUsage
	zoneBody
	numZones
)

var zoneLabels = [numZones]string{"name", "summary", "description", "usage", "body"}

// scoreEvidenceIn weighs evidence found with list, best group first.
func scoreEvidenceIn(list []signal, w scoreWeights, ev phraseEvidence) []PhraseScore {
	strong := [numZones]int{w.Name, w.Summary, w.Description, w.Usage, w.Body}
	weak := [numZones]int{w.WeakName, w.WeakSummary, w.WeakDescription, w.WeakUsage, 0}
	scores := map[string]*PhraseScore{}
	get := func(id string) *PhraseScore {
		if scores[id] == nil {
			scores[id] = &PhraseScore{ID: id}
		}
		return scores[id]
	}
	bodyScore := map[string]int{}
	for z, counts := range ev.zones {
		idx := make([]int, 0, len(counts))
		for i := range counts {
			idx = append(idx, i)
		}
		sort.Ints(idx)
		for _, i := range idx {
			s := list[i]
			if s.group == "" {
				continue // a neutral phrase
			}
			pts := strong[z]
			if s.weak {
				pts = weak[z]
			}
			if pts == 0 {
				continue
			}
			ts := get(s.group)
			if z != zoneBody {
				ts.Score += pts
				ts.Hits = append(ts.Hits, zoneLabels[z]+": "+s.label)
				continue
			}
			n := counts[i]
			if n > w.BodyHits {
				n = w.BodyHits
			}
			add := n * pts
			if bodyScore[s.group]+add > w.BodyCap {
				add = w.BodyCap - bodyScore[s.group]
			}
			if add > 0 {
				bodyScore[s.group] += add
				ts.Score += add
				ts.Hits = append(ts.Hits, fmt.Sprintf("body: %s ×%d", s.label, n))
			}
		}
	}
	var out []PhraseScore
	for _, ts := range scores {
		if ts.Score > 0 {
			out = append(out, *ts)
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

var (
	reSeeAlsoRef = regexp.MustCompile(`([A-Za-z0-9][A-Za-z0-9_.+-]{0,40})\(([18])[a-zA-Z]*\)`)
	reHasLetter  = regexp.MustCompile(`[A-Za-z]`)
)

const maxSeeAlsoRefs = 40

// seeAlsoPrograms lists the commands (sections 1 and 8) a man page's SEE
// ALSO section names, such as "host(1)" in dig's, at most maxSeeAlsoRefs.
func seeAlsoPrograms(manText string) []string {
	var out []string
	in := false
	for _, raw := range strings.Split(manText, "\n") {
		l := mint.CleanLine(raw)
		t := strings.TrimSpace(l)
		if t == "SEE ALSO" {
			in = true
			continue
		}
		if !in {
			continue
		}
		if t != "" && !strings.HasPrefix(l, " ") && strings.ToUpper(t) == t && reHasLetter.MatchString(t) {
			break // the next section
		}
		for _, m := range reSeeAlsoRef.FindAllStringSubmatch(l, -1) {
			if len(out) >= maxSeeAlsoRefs {
				return out
			}
			out = append(out, strings.ToLower(m[1]))
		}
	}
	return out
}

// pickGroup chooses the clear winner, or "" when none is clear enough.
func pickGroup(scores []PhraseScore) (string, string) {
	return pickGroupWith(defaultWeights, scores)
}

func pickGroupWith(w scoreWeights, scores []PhraseScore) (string, string) {
	if len(scores) == 0 {
		return "", "none of the phrases appear in the help"
	}
	best := scores[0]
	if best.Score < w.MinScore {
		return "", fmt.Sprintf("too little evidence (best: %s with %d)", best.ID, best.Score)
	}
	if len(scores) > 1 && best.Score-scores[1].Score < w.MinMargin {
		return "", fmt.Sprintf("no clear winner (%s %d, %s %d)", best.ID, best.Score, scores[1].ID, scores[1].Score)
	}
	return best.ID, fmt.Sprintf("best evidence (%d points)", best.Score)
}

// topicDataLevel is how sensitive a program's results are by what its
// help is about (dataTopicPhrases): keys and passwords (secret), captured
// traffic, people and organisations, security findings and logs
// (sensitive). It only ever raises a level (see ParseManAndHelpIO): an
// archetype says what a program does, not what it handles.
func topicDataLevel(p programText) (level, reason string) {
	scores := scoreEvidenceIn(topicContestList, defaultWeights, findEvidenceIn(topicContestList, p))
	scores = addTopicRelated(scores, p)
	id, _ := pickGroup(scores)
	switch {
	case id == "" || strings.HasPrefix(id, rivalPrefix):
		return "", ""
	case id == "keys and passwords":
		return appkit.DataSecret, "its help is about keys, passwords or encrypted data"
	}
	return appkit.DataSensitive, "its help is about captured traffic, gathered information, security findings or logs"
}

// Programs named under SEE ALSO count for a data topic: one whose results
// are secret for keys and passwords, and one that records traffic, looks
// up names, audits or follows logs for that topic; 2 points each, at most
// 4 per topic.
const topicRelated, topicRelatedCap = 2, 4

func addTopicRelated(scores []PhraseScore, p programText) []PhraseScore {
	byArchetype := map[appkit.ArchetypeID]string{"record": "captured traffic", "look-up": "people and organisations", "audit": "security findings", "follow": "logs"}
	add := map[string][]string{}
	for _, r := range p.Related {
		if strings.EqualFold(r, p.Name) {
			continue
		}
		spec := appkit.ToolSpec{Name: r}
		a, listed := appkit.ArchetypeForTool(r)
		if listed {
			spec.ArchetypeID = string(a.ID)
		}
		topic := ""
		if lvl, _ := appkit.ClassifyData(spec); lvl == appkit.DataSecret {
			topic = "keys and passwords"
		} else if listed {
			topic = byArchetype[a.ID]
		}
		if topic != "" && len(add[topic])*topicRelated < topicRelatedCap {
			add[topic] = append(add[topic], r)
		}
	}
	if len(add) == 0 {
		return scores
	}
	byID := map[string]int{}
	for i, s := range scores {
		byID[s.ID] = i
	}
	topics := make([]string, 0, len(add))
	for t := range add {
		topics = append(topics, t)
	}
	sort.Strings(topics)
	for _, t := range topics {
		i, ok := byID[t]
		if !ok {
			scores = append(scores, PhraseScore{ID: t})
			i = len(scores) - 1
		}
		for _, r := range add[t] {
			scores[i].Score += topicRelated
			scores[i].Hits = append(scores[i].Hits, "related: "+r)
		}
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].ID < scores[j].ID
	})
	return scores
}

// hasSubcommands reports whether a program's help lists subcommands.
func hasSubcommands(manText, helpText string, manLines, helpLines []string) bool {
	lowerAll := strings.ToLower(manText + "\n" + helpText)
	return len(extractSubcommands(manLines, helpLines)) >= 2 ||
		strings.Contains(lowerAll, "subcommand") || strings.Contains(lowerAll, "available commands")
}
