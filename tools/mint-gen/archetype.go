package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Choosing the archetype of a program no list names: one of the 35
// (appkit.AllArchetypeDefs), from two kinds of evidence.
//
//   - Behaviour: what the usage lines take (HOST..., PATTERN, FILE1 FILE2,
//     COMMAND [ARG]..., SOURCE DEST, an interval), what its subcommands are
//     called (add/remove, commit/log, start/stop), and options such as
//     --follow, --listen or -c COMMAND.
//   - Words: each archetype's phrases (archetypeSignals), read in the
//     program's name, summary, description, usage and the rest of its help,
//     weighed as for templates (see scoreWeights).
//
// It always chooses, and says how sure it is (see choiceConfidence).

// ArchetypeScore is how well an archetype fits a program, and why.
type ArchetypeScore struct {
	ID    appkit.ArchetypeID
	Score int
	Hits  []string
}

// archetypeSignals are each archetype's phrases, in the notation of
// templateSignals: "a|b" are ways of saying one idea, "~" marks a weak one
// (it counts in the name, summary, description and usage, not the body).
var archetypeSignals = map[string][]string{
	"ask": {"calculator|calculate|arbitrary precision", "~expression", "unit conversion|convert units|units of measure",
		"calendar", "date and time|current date|current time", "~factor|prime factors", "fortune|epigram|random quotation",
		"~answer", "how long the system has been running"},
	"look-up": {"dns lookup|dns query|domain name system|nameserver|name server|dns|resolver", "whois|registrar",
		"~lookup", "~record", "geolocation|geoip|country of", "subdomain|osint|reconnaissance", "dnssec", "~host name"},
	"inspect": {"display information|show information|print information|information about",
		"metadata|exif|tags", "file type|determine file type", "checksum|message digest|hash value|crc",
		"elf|object file|shared object|symbol table|disassemble", "~details", "media information|stream information",
		"~verify|~signature", "dependencies", "~attributes", "smart data|s.m.a.r.t", "~analyse|~analyze|~examine|~inspect"},
	"list": {"~list", "list of|show all|display all|list all", "~columns", "~table", "block device|mounted filesystems|disk usage|disk space|free space|space usage|file space",
		"namespaces|ipc facilities|sockets|connections", "logged in|login records|last logins",
		"~volumes|~devices", "directory contents|list directory|list files", "~summary"},
	"search": {"~search", "regular expression|regex|grep", "~pattern", "find files|search for files|file name|filename",
		"~matching|lines that match|matching lines|match patterns", "~locate|locate a command|binary, source",
		"fuzzy finder|fuzzy", "recursive|recursively", "case-insensitive|ignore case"},
	"query": {"query|queries|query language", "~sql|~database", "json|yaml|xml|csv|tsv", "~filter", "~expression",
		"pattern scanning|awk|programming language for", "~field|~column", "~select", "statistics on|statistical"},
	"compare": {"compare|comparison", "differences|difference between|differ", "~line by line", "side by side|side-by-side",
		"unified diff|context diff|diff", "~merge", "~common lines|lines common", "identical"},
	"read": {"manual page|man page|manual|documentation|texinfo|info document", "pager|file perusal|page through",
		"help pages|cheat sheet|cheatsheet|tldr", "~read|~view|~browse", "~topic|~section"},
	"watch": {"statistics|stats", "~monitor|~monitoring", "cpu usage|cpu utilization|memory usage|virtual memory|load average|i/o statistics",
		"~interval|~refresh|~every", "sensor|temperature|fan speed|battery", "~utilization|~usage", "system activity|resource usage",
		"~real time|~live"},
	"follow": {"~log|~logs|~journal|~syslog|~log file|~log messages", "follow|tail -f|wait for new|as they arrive|new messages", "~events|event stream",
		"kernel ring buffer|ring buffer", "~as they arrive|~appended", "audit log"},
	"record": {"capture|packet capture", "pcap|pcapng|libpcap", "~record|~recording", "~dump", "network traffic|traffic",
		"promiscuous", "sniff|sniffer", "~stream", "~to a file"},
	"probe": {"~scan|~scanner", "port scan|port scanner|network scanner|host scan", "ping|icmp|echo request", "traceroute|route packets|hop|hops",
		"host discovery|discover hosts|open ports", "~target|~targets", "~ports", "bad blocks|bad sectors", "~probe"},
	"audit": {"audit|auditing|security audit", "vulnerability|vulnerabilities|cve", "malware|virus|antivirus|rootkit",
		"lint|linter|static analysis|warnings", "check|checks|checker", "~severity|~findings", "integrity|intrusion detection",
		"compliance|hardening", "consistency|repair"},
	"measure": {"benchmark|benchmarking|load test|load testing|stress test|stress|load generator", "throughput|bandwidth|latency|performance",
		"concurrent|concurrency|simultaneous connections|~requests",
		"requests per second|transactions per second|operations per second", "~measure|~measurement", "~workload|~runs"},
	"convert": {"convert|converter|conversion|transcode", "encode|encoder|decode|decoder", "encrypt|decrypt|encryption|cipher",
		"~format", "~codec", "image|audio|video|media|pdf|postscript", "~output file", "~optimize|~compress",
		"~character encoding|~charset"},
	"filter": {"~standard input|~stdin", "~standard output|~stdout", "~text", "~lines", "~transform|~translate|~delete characters",
		"stream editor", "~sort|~sorted", "~concatenate|~reverse", "~columnate|~paragraph|~wrap"},
	"pack": {"archive|archiver|tarball", "compress|decompress|uncompress|compression|compressor", "~extract|~unpack",
		"zip|7z|gzip|bzip2|xz|zstd|lzip|lzop|cab|arj"},
	"generate": {"generate|generator", "random|randomly", "password|passwords", "uuid|unique identifier",
		"~sequence|~numbers", "banner|ascii art|large letters", "~create a new|~new key", "qr code"},
	"build": {"compile|compiler|compiling", "build|builds|build system|build tool", "~test|~tests|test suite|testing",
		"assembler|linker|parser generator", "~source code|~source files", "~project", "makefile|~targets"},
	"transfer": {"download|downloader|upload|retrieve files", "~url|~urls|~http|~https|~ftp", "~transfer|~fetch|~retrieve",
		"torrent|bittorrent", "~resume", "~remote file|~remote host"},
	"present": {"speak|speech|text to speech|synthesizer", "play|playback|sound card|audio device", "notification|notify|desktop notification",
		"print files|printer|print queue", "~message to all|~broadcast", "~display|~show"},
	"edit-files": {"~copy|~copies|~copying", "~move|~rename|~renames", "~remove|~delete|~unlink", "permissions|file mode|ownership|change owner",
		"~timestamps|access and modification times", "~create|~directory|~directories", "~truncate|~shred|~overwrite",
		"~in place", "~rotate|~rotation"},
	"mirror": {"synchronize|synchronization|sync|mirror", "~source|~destination", "~both directions|~replicas",
		"~incremental|~delta"},
	"settings": {"settings|configuration|config|preferences", "~key|~keys|~value|~values", "~get|~set",
		"time zone|timezone|locale|hostname|system clock|hardware clock", "~default application|~mime type",
		"kernel parameter|file system parameters"},
	"device": {"screen resolution|display resolution|display output|brightness|backlight|~screen|~monitor",
		"volume control|mixer|sound card|~volume", "wireless|wifi|bluetooth|network interface|~interface",
		"keyboard layout|keyboard|mouse|pointer",
		"~device|~devices", "~loop device|~loop devices", "~power"},
	"rules": {"firewall|firewall rule|packet filter|netfilter", "~rule|~rules|~chain|~chains", "routing table|routing|route",
		"partition|partition table|partitions", "arp cache|arp table|neighbour", "traffic control|queueing discipline",
		"~insert|~append|~entries", "bridge table|ethernet bridge"},
	"control": {"service|services|~daemon|~daemons|~unit|~units", "~start|~stop|~restart|~enable|~disable",
		"~process|~processes", "send a signal|signal to|kill processes|~signal|~kill", "priority|niceness|scheduling priority", "virtual machine|domains|guest",
		"init script|runlevel|system v", "~supervise|~supervisor|~monitoring daemon"},
	"records": {"package|packages|package manager", "install|uninstall|upgrade|installed|~remove", "dependency|dependencies",
		"~entries|~items|~records", "~add|~delete|~list", "certificate database|key database", "rocks|modules|gems",
		"todo|task|tasks|notes|bookmarks"},
	"history": {"version control|revision control|source control|scm", "commit|commits|branch|merge|checkout",
		"backup|backups|snapshot|snapshots|restore", "repository|repo", "~revision|~changeset|~history",
		"patch|patches|patch series", "~archive"},
	"toolkit": {"~commands|~subcommands", "toolkit|swiss army|set of tools|collection of tools|utility for",
		"container|containers|image|images|registry", "multiple repositories|many repositories",
		"~manage|~management"},
	"wrap": {"run a command|runs a command|execute a command|run a program|command line of", "~command|~program",
		"~with a time limit|timeout|time limit", "~priority|~cpu affinity|~real-time|~scheduling", "~resource limits",
		"trace system calls|system calls|library calls|~trace", "~arguments from standard input|~build and execute",
		"~line buffering|~buffering", "~lock", "~ignoring hangups|~hangup", "~container|~namespace", "~scripts in a directory"},
	"schedule": {"schedule|scheduled|scheduling", "cron|crontab|at job|later execution|later time",
		"periodic|periodically", "~timer|~jobs|~queue", "inotify|file system events"},
	"serve": {"~server", "http server|web server|httpd|serve files|serves files|file server", "~listen|~listening|~port", "~serve|~serves|~serving",
		"~document root|~directory to serve", "~daemon", "emulator|virtual machine|machine emulator"},
	"session": {"interactive|interactively", "command-line client|command line client|sql shell|database shell|database client|interactive terminal", "interpreter|repl|interactive shell|interactive terminal|command prompt|~shell|~terminal|~prompt", "~client", "~script|~scripts",
		"~commands", "~connect|~connection", "programming language|scripting language"},
}

var archetypeSignalList = buildSignals(archetypeSignals)

// Behaviour points.
const (
	cueStrong = 6
	cueMedium = 4
	cueWeak   = 2
)

var (
	// Usage arguments.
	reArgTargets = regexp.MustCompile(`(?i)^(?:hosts?|hostnames?|targets?|addresses|ips?|networks?|ranges?)$`)
	reArgOneName = regexp.MustCompile(`(?i)^(?:name|domain|hostname|host|address|ip|query|object)$`)
	reArgURL     = regexp.MustCompile(`(?i)^(?:urls?|uris?|links?|remote|source-url)$`)
	reArgPattern = regexp.MustCompile(`(?i)^(?:patterns?|regex|regexp|search|term|terms|keywords?|glob)$`)
	reArgQuery   = regexp.MustCompile(`(?i)^(?:filter|expression|expr|sql|query|statement|script-text|program-text)$`)
	reArgCommand = regexp.MustCompile(`(?i)^(?:command|cmd|utility|program|prog)$`)
	// What follows a wrapped COMMAND: its own arguments.
	reArgOfCommand = regexp.MustCompile(`^(?:arg|args|argument|arguments|initial-arguments|command-args|options?|parameters?)$`)
	reArgSource    = regexp.MustCompile(`(?i)^(?:source|src|from)$`)
	reArgDest      = regexp.MustCompile(`(?i)^(?:dest|destination|directory|dir|to)$`)
	reArgPair      = regexp.MustCompile(`(?i)^(?:file1|file2|from-file|to-file|left|right|mine|older|yours)$`)
	reArgInput     = regexp.MustCompile(`(?i)^(?:input|infile|in-file|inputfile|input-file)$`)
	reArgOutput    = regexp.MustCompile(`(?i)^(?:output|outfile|out-file|outputfile|output-file)$`)
	reArgTopic     = regexp.MustCompile(`(?i)^(?:page|topic|section|manpage|man-page|subject)$`)
	reArgDevice    = regexp.MustCompile(`(?i)^(?:device|devices|disk|disks|partition|blockdev|loopdev|interface|iface|dev)$`)
	reArgKV        = regexp.MustCompile(`(?i)(?:key|name|variable|param|parameter|setting)=(?:value|val)|^(?:key|setting|parameter|variable)$`)
	reArgTime      = regexp.MustCompile(`(?i)^(?:time|timespec|when|schedule)$`)
	reArgRepeat    = regexp.MustCompile(`(?i)^(?:interval|delay|count|seconds|secs|refresh)$`)
	reArgAsk       = regexp.MustCompile(`(?i)^(?:number|numbers|question|units?|from-unit|to-unit|month|year)$`)

	// Options.
	reOptFollow  = regexp.MustCompile(`(?im)^\s{0,12}(?:-f,\s*)?--follow\b|^\s{0,12}-[fFw],?\s+(?:--\S+\s+)?.*\b(?:follow|wait for new|new (?:messages|lines|data)|as the file grows)\b`)
	reOptListen  = regexp.MustCompile(`(?im)^\s{0,12}(?:-\w,\s*)?--(?:listen|bind|port|host|address|root|docroot)\b.*\b(?:listen|bind|serve|accept)`)
	reOptCommand = regexp.MustCompile(`(?im)^\s{0,12}-c,?\s+(?:--command\b|<?command>?|cmd)|^\s{0,12}(?:-c,\s*)?--command[ =]`)
	reOptBatch   = regexp.MustCompile(`(?im)^\s{0,12}(?:-\w,\s*)?--(?:batch|script|file)\b.*\b(?:commands|script|statements)\b`)
	reOptRepeat  = regexp.MustCompile(`(?im)^\s{0,12}(?:-\w,\s*)?--?(?:interval|delay|refresh|count|watch)\b`)
	reOptDryRun  = regexp.MustCompile(`(?im)^\s{0,12}(?:-\w,\s*)?--(?:dry-run|dryrun|simulate|no-act|just-print)\b`)
	reOptFormat  = regexp.MustCompile(`(?im)^\s{0,12}(?:-\w,\s*)?--?(?:format|codec|quality|bitrate|resolution|output-format|input-format|encoding)\b`)
	reOptLoad    = regexp.MustCompile(`(?im)^\s{0,12}(?:-\w,\s*)?--?(?:concurrency|connections|concurrent|requests|duration|threads|rate|warmup|runs|iterations|time)\b.*\b(?:concurren|connections|requests|seconds|duration|threads|runs|iterations)`)
	reOptLength  = regexp.MustCompile(`(?im)^\s{0,12}(?:-\w,\s*)?--?(?:length|count|number|num|seed|width|font)\b`)

	// What a summary or description says the program does.
	reSaysReports = regexp.MustCompile(`(?i)\b(?:display|displays|print|prints|show|shows|report|reports|determine|determines|identify|identifies|describe|describes|dump|dumps|examine|examines|inspect|inspects)\b(?:\W+\w+){0,5}?\W+(?:information|info|contents?|details|symbols|type|types|attributes|metadata|properties|dependencies|structure|sections|headers|tags|status)\b`)
	reSaysLists   = regexp.MustCompile(`(?i)^\s*(?:list|lists|show|shows|display|displays|print|prints|report|reports)\b(?:\W+\w+){0,4}?\W+\w+(?:s|es)\b`)
	reSaysStdin   = regexp.MustCompile(`(?i)\b(?:standard input|stdin)\b.{0,80}\b(?:standard output|stdout)\b|\bwrit\w* (?:the )?result to standard output\b`)
)

// subcommandKinds: subcommand names that say what a suite manages.
var subcommandKinds = []struct {
	id    appkit.ArchetypeID
	words []string
}{
	{"history", []string{"commit", "log", "checkout", "branch", "merge", "snapshot", "snapshots", "restore", "backup", "revert", "diff", "tag", "init", "clone", "push", "pull", "prune", "forget"}},
	{"records", []string{"add", "remove", "install", "uninstall", "list", "search", "show", "update", "upgrade", "delete", "edit", "done", "info", "purge", "hold", "unhold"}},
	{"control", []string{"start", "stop", "restart", "status", "enable", "disable", "reload", "kill", "suspend", "resume", "shutdown", "reboot", "destroy"}},
	{"pack", []string{"create", "extract"}},
	{"settings", []string{"get", "set", "unset", "reset", "list-keys", "list-schemas"}},
}

// archetypeBehaviour scores the archetypes by how the program works.
func archetypeBehaviour(p programText) map[appkit.ArchetypeID]*ArchetypeScore {
	out := map[appkit.ArchetypeID]*ArchetypeScore{}
	add := func(id appkit.ArchetypeID, pts int, why string) {
		s := out[id]
		if s == nil {
			s = &ArchetypeScore{ID: id}
			out[id] = s
		}
		for _, h := range s.Hits {
			if h == why {
				return
			}
		}
		s.Score += pts
		s.Hits = append(s.Hits, why)
	}

	// The usage lines' arguments.
	args := usageArgs(p.Usage)
	wrapCmd := "" // a COMMAND followed by its own arguments
	for _, a := range args {
		w := a.word
		switch {
		case reArgTargets.MatchString(w) || (a.repeat && reArgOneName.MatchString(w)):
			add("probe", cueMedium, "takes many targets ("+a.raw+")")
		case reArgURL.MatchString(w):
			add("transfer", cueMedium, "takes a URL ("+a.raw+")")
		case reArgPattern.MatchString(w):
			add("search", cueMedium, "takes a pattern ("+a.raw+")")
		case reArgQuery.MatchString(w):
			add("query", cueMedium, "takes an expression or program ("+a.raw+")")
		case reArgCommand.MatchString(w):
			if a.withArgs {
				wrapCmd = a.raw
			}
		case reArgPair.MatchString(w):
			add("compare", cueMedium, "takes two inputs ("+a.raw+")")
		case reArgTopic.MatchString(w):
			add("read", cueMedium, "takes a topic or page ("+a.raw+")")
		case reArgDevice.MatchString(w):
			add("device", cueWeak, "takes a device ("+a.raw+")")
			add("inspect", cueWeak, "takes a device ("+a.raw+")")
		case reArgKV.MatchString(w):
			add("settings", cueMedium, "takes a key or key=value ("+a.raw+")")
		case reArgTime.MatchString(w):
			add("schedule", cueMedium, "takes a time ("+a.raw+")")
		case reArgRepeat.MatchString(w):
			add("watch", cueMedium, "repeats on an interval ("+a.raw+")")
		case reArgOneName.MatchString(w):
			add("look-up", cueMedium, "takes a name ("+a.raw+")")
		case reArgAsk.MatchString(w):
			add("ask", cueWeak, "takes a number or question ("+a.raw+")")
		}
	}
	has := func(re *regexp.Regexp) bool {
		for _, a := range args {
			if re.MatchString(a.word) {
				return true
			}
		}
		return false
	}
	switch {
	case has(reArgSource) && has(reArgDest):
		add("mirror", cueWeak, "takes a source and a destination")
		add("edit-files", cueWeak, "takes a source and a destination")
	case has(reArgInput) && has(reArgOutput):
		add("convert", cueMedium, "takes an input and an output")
	}

	// Subcommands, and what they are called. A COMMAND in the usage of a
	// program with subcommands is one of them, not a command it runs.
	subs := extractSubcommandsN(p.manLines, p.helpLines, 60)
	if wrapCmd != "" && len(subs) < 2 {
		add("wrap", cueStrong, "runs a command it is given ("+wrapCmd+" with its arguments)")
	}
	if len(subs) >= 2 {
		names := map[string]bool{}
		for _, s := range subs {
			names[strings.ToLower(s.Name)] = true
		}
		best, bestN := appkit.ArchetypeID(""), 0
		for _, k := range subcommandKinds {
			n := 0
			for _, w := range k.words {
				if names[w] {
					n++
				}
			}
			if n > bestN {
				best, bestN = k.id, n
			}
		}
		if bestN >= 2 {
			add(best, cueStrong, fmt.Sprintf("its subcommands manage %s (%d of %d)", best, bestN, len(subs)))
		} else {
			add("toolkit", cueMedium, fmt.Sprintf("has %d subcommands", len(subs)))
		}
	}

	// Options.
	body := p.Body
	if reOptFollow.MatchString(body) {
		add("follow", cueMedium, "it can follow new output")
	}
	if reOptListen.MatchString(body) {
		add("serve", cueMedium, "it listens for connections")
	}
	if reOptCommand.MatchString(body) || reOptBatch.MatchString(body) {
		add("session", cueMedium, "it takes commands to run in its session")
	}
	if reOptRepeat.MatchString(body) {
		add("watch", cueWeak, "an option repeats it")
	}
	if reOptDryRun.MatchString(body) {
		add("edit-files", cueWeak, "it has a dry run")
		add("mirror", cueWeak, "it has a dry run")
	}
	if n := len(reOptLoad.FindAllString(body, 4)); n >= 2 {
		add("measure", cueMedium, "it sets a load: concurrency, requests, duration or runs")
	}
	if reOptFormat.MatchString(body) {
		add("convert", cueWeak, "it chooses a format")
	}
	if len(args) == 0 && reOptLength.MatchString(body) {
		add("generate", cueWeak, "it takes only settings (length, count…)")
	}

	// What it says it does.
	about := p.Summary + "\n" + p.Description
	// "Display information about logical volumes" lists; "display
	// information about a file" inspects.
	if reSaysLists.MatchString(p.Summary) {
		add("list", cueMedium, "it lists things")
	} else if reSaysReports.MatchString(about) {
		add("inspect", cueMedium, "it reports on its input")
	}
	if reSaysStdin.MatchString(about) {
		add("filter", cueMedium, "it reads standard input and writes standard output")
	}

	// General: anything, a little.
	add("general", 1, "it runs and prints (the usual kind)")
	return out
}

type usageArg struct {
	raw, word string
	repeat    bool
	withArgs  bool // followed by arguments of its own: COMMAND [ARG]...
}

// usageArgs lists the argument words of the usage forms once each:
// "ping [options] HOST..." -> HOST (repeat).
func usageArgs(usage []string) []usageArg {
	var out []usageArg
	seen := map[string]bool{}
	for _, u := range usage {
		toks := strings.Fields(u)
		for i, tok := range toks {
			bare := strings.Trim(tok, "[]{}()<>|,'\"")
			if i == 0 || bare == "" || strings.HasPrefix(bare, "-") || bare == "..." || strings.EqualFold(bare, "options") || strings.EqualFold(bare, "option") {
				continue
			}
			repeat := strings.Contains(tok, "...") || (i+1 < len(toks) && strings.HasPrefix(strings.Trim(toks[i+1], "[]"), "..."))
			withArgs := i+1 < len(toks) && reArgOfCommand.MatchString(strings.ToLower(strings.Trim(toks[i+1], "[]{}()<>|.,'\"…")))
			w := strings.ToLower(strings.TrimRight(bare, ".…"))
			if seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, usageArg{raw: strings.Trim(tok, "[]{}()|,"), word: w, repeat: repeat, withArgs: withArgs})
		}
	}
	return out
}

// archetypeWeights weigh the words for archetypes: as for templates, but
// the long body counts for at most 3 per archetype, and related programs
// are counted separately (see relatedArchetypes).
var archetypeWeights = func() scoreWeights {
	w := defaultWeights
	w.BodyCap = 3
	w.Related = 0
	return w
}()

// relatedArchetypes: programs named under SEE ALSO that the archetype
// lists know, at most two per archetype.
func relatedArchetypes(name string, related []string) map[appkit.ArchetypeID][]string {
	out := map[appkit.ArchetypeID][]string{}
	for _, r := range related {
		if strings.EqualFold(r, name) {
			continue
		}
		if a, ok := appkit.ArchetypeForTool(r); ok && len(out[a.ID]) < 2 {
			out[a.ID] = append(out[a.ID], r)
		}
	}
	return out
}

// scoreArchetypes ranks the 35 archetypes for a program, best first; ties
// go to the order of appkit.AllArchetypeDefs, so every run is the same.
func scoreArchetypes(p programText) []ArchetypeScore {
	beh := archetypeBehaviour(p)
	words := map[appkit.ArchetypeID]TemplateScore{}
	for _, s := range scoreEvidenceIn(archetypeSignalList, archetypeWeights, findEvidenceIn(archetypeSignalList, p)) {
		words[appkit.ArchetypeID(s.ID)] = s
	}
	rel := relatedArchetypes(p.Name, p.Related)
	var out []ArchetypeScore
	for _, def := range appkit.AllArchetypeDefs() {
		s := ArchetypeScore{ID: def.ID}
		if b := beh[def.ID]; b != nil {
			s.Score += b.Score
			s.Hits = append(s.Hits, b.Hits...)
		}
		if w, ok := words[def.ID]; ok {
			s.Score += w.Score
			s.Hits = append(s.Hits, w.Hits...)
		}
		for _, r := range rel[def.ID] {
			s.Score += 2
			s.Hits = append(s.Hits, "related: "+r)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// Confidence of a choice.
const (
	ConfidenceListed = "listed" // on an archetype list: exact
	ConfidenceClear  = "clear"  // led the next archetype clearly
	ConfidenceClose  = "close"  // chosen, but the next one was near
)

// clearLead is the lead over the next archetype a choice needs to be clear.
var clearLead = 4

func choiceConfidence(lead int) string {
	if lead >= clearLead {
		return ConfidenceClear
	}
	return ConfidenceClose
}

// ArchetypeChoice is the archetype chosen for a program, and why.
type ArchetypeChoice struct {
	ID           appkit.ArchetypeID
	Why          string
	Confidence   string           // ConfidenceListed, ConfidenceClear or ConfidenceClose
	Lead         int              // points over the next archetype
	Scores       []ArchetypeScore // best first (for --explain)
	Alternatives []appkit.ArchetypeID
}

// chooseArchetypeFor chooses from the help text alone, as for a program no
// list knows. It always chooses.
func chooseArchetypeFor(p programText) ArchetypeChoice {
	scores := scoreArchetypes(p)
	c := ArchetypeChoice{ID: scores[0].ID, Scores: scores, Lead: scores[0].Score - scores[1].Score}
	c.Why = fmt.Sprintf("best evidence (%d points, %d ahead of %s)", scores[0].Score, c.Lead, scores[1].ID)
	c.Confidence = choiceConfidence(c.Lead)
	for _, s := range scores[1:3] {
		if s.Score > 0 && scores[0].Score-s.Score < 2*clearLead {
			c.Alternatives = append(c.Alternatives, s.ID)
		}
	}
	return c
}

// chooseArchetype chooses a program's archetype: the archetype list first,
// else its help.
func chooseArchetype(bin string, manText, helpText string) ArchetypeChoice {
	name := strings.ToLower(toolBaseName(bin))
	if a, ok := appkit.ArchetypeForTool(name); ok {
		return ArchetypeChoice{ID: a.ID, Why: "listed for " + name + " in the archetype lists", Confidence: ConfidenceListed}
	}
	return chooseArchetypeFor(programTextFrom(name, manText, helpText))
}
