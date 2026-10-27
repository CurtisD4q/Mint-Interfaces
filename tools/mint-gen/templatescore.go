package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// templateSignals are the words and phrases that point to each template.
//
// Each entry is one idea; "a|b|c" lists ways of saying it, and the idea
// counts once per place it is found however many ways appear. Entries
// starting with "~" are weak: ordinary words that many programs use
// ("lines", "process", "search"), which count a little in a program's
// name, summary or usage and not at all in the rest of its help. Matching
// is by whole words, with common endings allowed (see signalPattern), and a
// phrase found inside a longer matched phrase does not count on its own
// ("SQL injection" is security evidence, not also "SQL" data evidence).
var templateSignals = map[string][]string{
	"T01": {"sysctl|hostnamectl|timedatectl|localectl", "kernel parameter", "system clock|system time|time and date|date and time",
		"time zone|timezone", "system locale|locale", "~hostname|host name", "system hostname|static hostname", "keyboard layout", "ntp|time synchronization"},
	"T02": {"dotfile", "get and set", "config key|configuration key|configuration value", "user configuration",
		"settings database|configuration database|configuration system", "dconf|gsettings|gconf|xfconf", "schema", "~settings", "~key"},
	"T03": {"sound card|soundcard|audio mixer|mixer", "alsa", "pulseaudio|pipewire", "backlight|brightness", "bluetooth",
		"rfkill|wireless", "networkmanager", "display resolution|screen resolution|xrandr|randr", "~volume", "~monitor|output"},
	"T04": {"firewall|firewall rule", "iptables|ip6tables|netfilter|nftables", "routing table|routing", "packet filter|packet filtering",
		"network device|network devices", "tunnel", "~interface", "~chain", "~rule", "nat|masquerade"},
	"T05": {"copy files|file copy|copy a file|copies files", "~copy", "move files", "~move", "remove files|delete files", "~rename",
		"transfer files|file transfer|remote file|remote host", "~overwrite", "securely delete|shred", "synchronize|synchronization|mirror",
		"unlink", "create the directory", "permissions|file mode", "ownership", "access and modification times", "symbolic link"},
	"T06": {"transcode|converter|document converter|format conversion", "convert encoding", "~encoding", "~audio|video|media|image|multimedia",
		"markup", "~pdf", "qr code", "graphviz|graph|diagram|layout", "~render", "codec", "postscript", "~convert"},
	"T07": {"download|downloader", "http client", "transfer a url|transfer data|retrieve files", "~fetch", "~url", "web server", "~ftp", "~http"},
	"T08": {"archive|archiver|tarball", "compress|decompress|uncompress|compression|compressor", "zip archive", "~extract",
		"~zip|7z|gzip|bzip2|xz|zstd"},
	"T09": {"backup", "snapshot", "~restore", "deduplication|deduplicating", "cloud storage", "~repository", "incremental backup"},
	"T10": {"compiler|compile", "~build", "interpreter", "programming language", "javascript", "run tests|test suite|testing|test framework|unit test",
		"makefile", "~source code", "~script", "debugger", "build system|build tool", "assembler|linker"},
	"T11": {"benchmark|stress test|load test", "requests per second", "~bandwidth", "~throughput", "~performance", "~latency",
		"tester", "workload"},
	"T12": {"port scan|port scanner|network scanner|network scan", "network mapper", "host discovery", "open ports", "syn scan",
		"~scan|scanner", "arp", "ping sweep", "~port"},
	"T13": {"pcap|libpcap|pcapng", "packet capture|capture packets|captured packets", "network traffic|traffic", "sniffer|sniff",
		"promiscuous", "~packet", "dump traffic", "~network interface"},
	"T14": {"whois", "dns|nameserver|name server|dns lookup", "domain name", "subdomain", "traceroute|route packets|hop",
		"reconnaissance|osint", "~lookup", "~host"},
	"T15": {"vulnerability", "security audit|security auditing|auditing tool", "malware", "rootkit", "exploit", "cve", "hardening",
		"sql injection", "web server scanner", "virus|antivirus", "~security", "penetration testing|pentest", "compliance"},
	"T16": {"message digest", "checksum", "hash|sha256|sha1|md5|sha512", "cipher", "cryptographic|cryptography", "x.509",
		"password cracker", "encrypt|decrypt", "~signature|signing", "openpgp|pgp|gnupg", "~ssl|tls", "~certificate",
		"~private key|public key"},
	"T17": {"memory statistics|virtual memory", "cpu statistics|cpu usage|cpu utilization", "i/o statistics|input/output statistics",
		"free memory|used memory|amount of free", "sensor|temperature|fan speed", "uptime|load average", "processing units",
		"system information", "~statistics", "resource monitor|system monitor", "~resource"},
	"T18": {"systemd", "service manager|init system", "daemon", "unit files", "~service", "supervisord|supervisor|process control system",
		"control applications"},
	"T19": {"~process", "pid", "~signal", "process status|process tree|process viewer|list of processes|process list|current processes",
		"open files|lsof", "niceness|scheduling priority", "~time limit", "kill", "~hangup", "process id"},
	"T20": {"journal|journald", "log messages|log entries|log entry|log files", "~log", "system log|syslog", "kernel ring buffer|ring buffer",
		"audit log", "~follow", "logged in|login|last logged|wtmp|btmp"},
	"T21": {"version control|revision control|source control|source code management|scm", "~commit", "~branch", "~merge", "pull request",
		"working tree|working copy", "revision|changeset", "~repository", "mercurial|subversion", "~clone"},
	"T22": {"container", "docker|podman", "kubernetes|k8s", "virtual machine|hypervisor|virtualization|libvirt", "~pod", "~guest"},
	"T23": {"package manager|package management", "install packages|install package|remove packages|upgrade packages", "plugin manager",
		"~package", "~install", "~upgrade", "~dependency", "rpm|dpkg|pypi"},
	"T24": {"block device", "partition|partition table", "mounted filesystems", "~file system|filesystem", "disk usage|disk space|space usage|file space", "~disk",
		"inode", "~mount", "persistent storage", "s.m.a.r.t|smart", "hard disk|hard drive|nvme|sata"},
	"T25": {"~search", "find files", "regular expression|regex", "file type", "printable strings", "metadata", "~matching", "locate",
		"list information", "directory contents|list directory", "match patterns|lines that match|matching lines", "~pattern"},
	"T26": {"~json|yaml|csv|tsv", "json processor", "~sql|database", "sqlite|database shell|sql shell", "~query", "stream editor", "~line", "line by line", "~field",
		"~column", "random permutation|shuffle", "concatenate", "paragraph", "tab", "characterwise", "~sorted",
		"pattern scanning and processing|text processing|transform text"},
	"T27": {"manual page|man page|manual", "reference manual", "~documentation", "info document|texinfo", "whatis", "tldr", "pager",
		"file perusal", "help pages|cheat sheet|cheatsheet"},
	"T28": {"cron|crontab", "~schedule|scheduled|scheduling", "~timer", "at job", "later execution|later time", "periodic|periodically",
		"~job", "~batch", "anacron"},
}

// scoreWeights says how much each kind of evidence counts and when the
// best template is clear enough to choose. defaultWeights was chosen with
// the template benchmark (bench_test.go) on its dev programs and checked on
// its held-out ones.
type scoreWeights struct {
	Name, Summary, Description, Usage, Body           int // a word in the program's name, summary, description, usage lines, rest of its help
	WeakName, WeakSummary, WeakDescription, WeakUsage int // weak words (see templateSignals); they never count in the body
	BodyHits                                          int // at most this many body matches per idea…
	BodyCap                                           int // …and this many body points per template, so a long man page cannot win on length
	Related, RelatedCap                               int // per related program (man SEE ALSO) in the template's list, and the most per template
	MinScore, MinMargin                               int // the winner needs at least MinScore, and MinMargin more than the runner-up
}

var defaultWeights = scoreWeights{
	Name: 8, Summary: 5, Description: 3, Usage: 2, Body: 1,
	WeakName: 3, WeakSummary: 2, WeakDescription: 1, WeakUsage: 1,
	BodyHits: 2, BodyCap: 4,
	Related: 2, RelatedCap: 4,
	MinScore: 4, MinMargin: 3,
}

// signal is one idea of one template.
type signal struct {
	template string
	label    string // the first way of saying it, for reports
	weak     bool
	re       *regexp.Regexp
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

var templateSignalList = func() []signal {
	var out []signal
	ids := make([]string, 0, len(templateSignals))
	for id := range templateSignals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	all := map[string][]string{"": neutralPhrases} // "": scores for no template
	for id, entries := range templateSignals {
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
			sig := signal{template: id, label: ways[0], weak: weak, re: re}
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
}()

// TemplateScore is how well a template fits a program, and why.
type TemplateScore struct {
	ID    string
	Score int
	Hits  []string // "summary: converter", "body: pdf ×2", "related: host"…
}

type signalMatch struct {
	sig        int // index in templateSignalList
	start, end int
}

// zoneMatches counts each idea's matches in text, dropping any match that
// lies inside a longer match of any template: the longer phrase is the
// meaning ("SQL injection", not "SQL").
//
// In prose (a description or a whole man page), a match joined to more of
// a word by a hyphen is part of an option or key name, not prose:
// "--layout", "kill-line", "--smart-case". The summary and usage lines
// keep it ("--json" in a usage line is a fair hint).
func zoneMatches(text string, prose bool) map[int]int {
	var all []signalMatch
	lower := ""
	if len(text) > longZone {
		lower = asciiLower(text)
	}
	for i, s := range templateSignalList {
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

// ScoreTemplates scores every template for a program by the words in its
// name, summary, usage lines and the rest of its help, and by the related
// programs its man page names (see seeAlsoPrograms), best first.
func ScoreTemplates(name, summary string, usage []string, body string, related []string) []TemplateScore {
	return scoreEvidence(defaultWeights, findEvidence(programText{Name: name, Summary: summary, Usage: usage, Body: body, Related: related}))
}

// programText is what the scorer reads about a program.
type programText struct {
	Name        string
	Summary     string   // one line: what it is (see extractSummary)
	Description string   // the opening of the man page's DESCRIPTION (see extractDescription)
	Usage       []string // usage lines
	Body        string   // all of the man page and help
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
		Related:     seeAlsoPrograms(manText),
		manLines:    manLines,
		helpLines:   helpLines,
		options:     append(parseFlagsFromLines(helpLines, false), parseFlagsFromLines(manLines, true)...),
	}
}

// templateEvidence is what was found in a program's help, before weighing:
// how often each idea appears in each zone, and the related programs that
// a template lists. Finding it is the slow part; weighing it is quick.
type templateEvidence struct {
	zones   [numZones]map[int]int // see zoneLabels: idea index -> matches
	related []string              // related programs (not the program itself) that a template lists
}

func findEvidence(p programText) templateEvidence {
	name, summary, usage, body, related := p.Name, p.Summary, p.Usage, p.Body, p.Related
	var ev templateEvidence
	// A program's own name in its usage and help is not evidence about
	// itself: "crontab" all through crontab's help says nothing a
	// template's tool list does not.
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
			ev.zones[z] = zoneMatches(text, z == zoneDescription || z == zoneBody)
		}
	}
	for _, r := range related {
		if r == strings.ToLower(name) {
			continue
		}
		if _, ok := appkit.TemplateForTool(r); ok {
			ev.related = append(ev.related, r)
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

// scoreEvidence weighs evidence with w, best template first.
func scoreEvidence(w scoreWeights, ev templateEvidence) []TemplateScore {
	strong := [numZones]int{w.Name, w.Summary, w.Description, w.Usage, w.Body}
	weak := [numZones]int{w.WeakName, w.WeakSummary, w.WeakDescription, w.WeakUsage, 0}
	scores := map[string]*TemplateScore{}
	get := func(id string) *TemplateScore {
		if scores[id] == nil {
			scores[id] = &TemplateScore{ID: id}
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
			s := templateSignalList[i]
			if s.template == "" {
				continue // a neutral phrase
			}
			pts := strong[z]
			if s.weak {
				pts = weak[z]
			}
			if pts == 0 {
				continue
			}
			ts := get(s.template)
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
			if bodyScore[s.template]+add > w.BodyCap {
				add = w.BodyCap - bodyScore[s.template]
			}
			if add > 0 {
				bodyScore[s.template] += add
				ts.Score += add
				ts.Hits = append(ts.Hits, fmt.Sprintf("body: %s ×%d", s.label, n))
			}
		}
	}
	// Related programs that a template lists are evidence for it.
	relatedScore := map[string]int{}
	for _, r := range ev.related {
		tpl, _ := appkit.TemplateForTool(r)
		id := string(tpl.ID)
		if w.Related == 0 || relatedScore[id]+w.Related > w.RelatedCap {
			continue
		}
		relatedScore[id] += w.Related
		ts := get(id)
		ts.Score += w.Related
		ts.Hits = append(ts.Hits, "related: "+r)
	}
	var out []TemplateScore
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

// pickTemplate chooses the clear winner, or "" when none is clear enough.
func pickTemplate(scores []TemplateScore) (string, string) {
	return pickTemplateWith(defaultWeights, scores)
}

func pickTemplateWith(w scoreWeights, scores []TemplateScore) (string, string) {
	if len(scores) == 0 {
		return "", "no template's words appear in the help"
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

// TemplateChoice is the template a program got, and why.
type TemplateChoice struct {
	ID         string
	Why        string
	Scores     []TemplateScore  // best first (for --explain)
	Archetypes []ArchetypeScore // best first, when the archetype was chosen first (for --explain)
}

// chooseTemplate picks a template: the built-in tool list first, then the
// best-scoring template, then a fallback (a suite for programs with
// subcommands, else the general batch runner).
func chooseTemplate(bin string, spec appkit.ToolSpec, manText, helpText string) TemplateChoice {
	name := strings.ToLower(toolBaseName(bin))
	if tpl, ok := appkit.TemplateForTool(name); ok {
		return TemplateChoice{ID: string(tpl.ID), Why: "listed for " + name + " in the template's tools"}
	}
	return scoreTemplateChoice(name, manText, helpText)
}

// scoreTemplateChoice chooses a template from the help text alone, as for a
// program no list knows (the template benchmark uses it directly).
func scoreTemplateChoice(name, manText, helpText string) TemplateChoice {
	manLines, helpLines := cleanLines(manText), cleanLines(helpText)
	scores := scoreEvidence(defaultWeights, findEvidence(programTextFrom(name, manText, helpText)))
	if id, why := pickTemplate(scores); id != "" {
		return TemplateChoice{ID: id, Why: why, Scores: scores}
	} else if hasSubcommands(manText, helpText, manLines, helpLines) {
		return TemplateChoice{ID: "T21", Why: why + "; it has subcommands", Scores: scores}
	} else {
		return TemplateChoice{ID: "T05", Why: why + "; the general runner", Scores: scores}
	}
}

// hasSubcommands reports whether a program's help lists subcommands.
func hasSubcommands(manText, helpText string, manLines, helpLines []string) bool {
	lowerAll := strings.ToLower(manText + "\n" + helpText)
	return len(extractSubcommands(manLines, helpLines)) >= 2 ||
		strings.Contains(lowerAll, "subcommand") || strings.Contains(lowerAll, "available commands")
}
