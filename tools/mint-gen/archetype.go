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
//     weighed by where they were found (see archWeights); and broader
//     subject phrases (archetypeBroadPhrases) for a share of their points.
//
// It always chooses, and says how sure it is (see choiceConfidence).

// ArchetypeScore is how well an archetype fits a program, and why.
type ArchetypeScore struct {
	ID    appkit.ArchetypeID
	Score int
	Hits  []string
}

// archetypeSignals are each archetype's phrases, in the notation of
// phrases.go: "a|b" are ways of saying one idea, "~" marks a weak one (it
// counts in the name, summary, description and usage, not the body).
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

// How strong a behaviour cue is. Its points are the points of the part of
// the help it was found in (archWeights.CueZone), scaled by its strength
// (archWeights.CueStrength).
type cueLevel int

const (
	cueBase cueLevel = iota // counts a little for anything (General)
	cueWeak
	cueMedium
	cueStrong
)

// cueHit is one behaviour cue found for an archetype.
type cueHit struct {
	level cueLevel
	zone  int // the part of the help it was found in (see zoneLabels)
	why   string
}

var (
	// Usage arguments.
	reArgTargets = regexp.MustCompile(`(?i)^(?:hosts?|hostnames?|targets?|addresses|ips?|networks?|ranges?)$`)
	reArgOneName = regexp.MustCompile(`(?i)^(?:name|domain|hostname|host|address|ip|query|object)$`)
	reArgURL     = regexp.MustCompile(`(?i)^(?:urls?|uris?|links?|remote|source-url)$`)
	reArgPattern = regexp.MustCompile(`(?i)^(?:patterns?|regex|regexp|search|term|terms|keywords?|glob)$`)
	reArgQuery   = regexp.MustCompile(`(?i)^(?:filter|expression|expr|sql|query|statement|script-text|program-text)$`)
	// A placeholder for one of the program's own commands.
	reArgSubcommand = regexp.MustCompile(`(?i)^(?:command|commands|cmd|subcommand|subcommands|group|object|mode|action|verb)$`)
	reArgCommand    = regexp.MustCompile(`(?i)^(?:command|cmd|utility|program|prog)$`)
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
	reArgFile      = regexp.MustCompile(`(?i)^(?:files?|file\(s|filenames?|objfiles?|object-files?|paths?)$`)
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
	reSaysLists   = regexp.MustCompile(`(?i)^\s*(?:list|lists|show|shows|display|displays|print|prints|report|reports)\b(?:\W+\w+){0,5}?\W+\w+[^uis\W]s\b`)
	reSaysFiles   = regexp.MustCompile(`(?i)\bfiles?\b`)
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
	{"settings", []string{"get", "set", "unset", "reset", "read", "write", "list-keys", "list-schemas", "list-locales", "list-timezones"}},
	{"rules", []string{"append", "insert", "delete", "replace", "flush", "list-rules", "del", "check", "add-rule", "remove-rule"}},
	{"build", []string{"build", "test", "run", "bench", "compile", "clean", "vet", "fmt", "check", "doc"}},
}

// manySubcommands is how many subcommands of mixed kinds make a program
// a toolkit for certain.
const manySubcommands = 8

// minKindCommands is how many subcommands of one kind decide the kind.
const minKindCommands = 3

// archetypeCues finds the behaviour cues for each archetype: how the
// program works.
func archetypeCues(p programText) map[appkit.ArchetypeID][]cueHit {
	out := map[appkit.ArchetypeID][]cueHit{}
	add := func(id appkit.ArchetypeID, zone int, level cueLevel, why string) {
		for _, h := range out[id] {
			if h.why == why {
				return
			}
		}
		out[id] = append(out[id], cueHit{level, zone, why})
	}

	// The usage lines' arguments.
	args := usageArgs(p.Usage)
	wrapCmd := "" // a COMMAND followed by its own arguments
	for _, a := range args {
		w := a.word
		switch {
		case reArgTargets.MatchString(w) || (a.repeat && reArgOneName.MatchString(w)):
			add("probe", zoneUsage, cueMedium, "takes many targets ("+a.raw+")")
		case reArgURL.MatchString(w):
			add("transfer", zoneUsage, cueMedium, "takes a URL ("+a.raw+")")
		case reArgPattern.MatchString(w):
			add("search", zoneUsage, cueMedium, "takes a pattern ("+a.raw+")")
		case reArgQuery.MatchString(w):
			add("query", zoneUsage, cueMedium, "takes an expression or program ("+a.raw+")")
		case reArgCommand.MatchString(w):
			if a.withArgs {
				wrapCmd = a.raw
			}
		case reArgPair.MatchString(w):
			add("compare", zoneUsage, cueMedium, "takes two inputs ("+a.raw+")")
		case reArgTopic.MatchString(w):
			add("read", zoneUsage, cueMedium, "takes a topic or page ("+a.raw+")")
		case reArgDevice.MatchString(w):
			add("device", zoneUsage, cueWeak, "takes a device ("+a.raw+")")
			add("inspect", zoneUsage, cueWeak, "takes a device ("+a.raw+")")
		case reArgKV.MatchString(w):
			add("settings", zoneUsage, cueMedium, "takes a key or key=value ("+a.raw+")")
		case reArgTime.MatchString(w):
			add("schedule", zoneUsage, cueMedium, "takes a time ("+a.raw+")")
		case reArgRepeat.MatchString(w):
			add("watch", zoneUsage, cueMedium, "repeats on an interval ("+a.raw+")")
		case reArgOneName.MatchString(w):
			add("look-up", zoneUsage, cueMedium, "takes a name ("+a.raw+")")
		case reArgAsk.MatchString(w):
			add("ask", zoneUsage, cueWeak, "takes a number or question ("+a.raw+")")
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
		add("mirror", zoneUsage, cueWeak, "takes a source and a destination")
		add("edit-files", zoneUsage, cueWeak, "takes a source and a destination")
	case has(reArgInput) && has(reArgOutput):
		add("convert", zoneUsage, cueMedium, "takes an input and an output")
	}

	// Subcommands, and what they are called. A COMMAND in the usage of a
	// program with subcommands is one of them, not a command it runs.
	subs := suiteCommandNames(p.manLines, p.helpLines)
	if wrapCmd != "" && len(subs) < 2 {
		add("wrap", zoneUsage, cueStrong, "runs a command it is given ("+wrapCmd+" with its arguments)")
	}
	if len(subs) >= 2 {
		// A name counts as itself and by its first part: "list-keys" is
		// also "list".
		names := map[string]bool{}
		for _, s := range subs {
			names[s] = true
			if first, _, ok := strings.Cut(s, "-"); ok {
				names[first] = true
			}
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
		// The subcommands count as part of the usage when a usage line
		// has a place for them ("git [options] <command>"), and as part of
		// the rest of the help when only a list names them.
		subZone := zoneOf(secCommands)
		if has(reArgSubcommand) {
			subZone = zoneUsage
		}
		// A kind needs several of the commands and a real share of them,
		// not two of sixty.
		if bestN >= minKindCommands && bestN*6 >= len(subs) {
			add(best, subZone, cueStrong, fmt.Sprintf("its subcommands manage %s (%d of %d)", best, bestN, len(subs)))
		} else if len(subs) >= manySubcommands {
			add("toolkit", subZone, cueStrong, fmt.Sprintf("has %d subcommands of many kinds", len(subs)))
		} else {
			add("toolkit", subZone, cueMedium, fmt.Sprintf("has %d subcommands", len(subs)))
		}
	}

	// Options, read in the whole help.
	body := p.Whole
	if reOptFollow.MatchString(body) {
		add("follow", zoneOf(secOptions), cueMedium, "it can follow new output")
	}
	if reOptListen.MatchString(body) {
		add("serve", zoneOf(secOptions), cueMedium, "it listens for connections")
	}
	if reOptCommand.MatchString(body) || reOptBatch.MatchString(body) {
		add("session", zoneOf(secOptions), cueMedium, "it takes commands to run in its session")
	}
	if reOptRepeat.MatchString(body) {
		add("watch", zoneOf(secOptions), cueWeak, "an option repeats it")
	}
	if reOptDryRun.MatchString(body) {
		add("edit-files", zoneOf(secOptions), cueWeak, "it has a dry run")
		add("mirror", zoneOf(secOptions), cueWeak, "it has a dry run")
	}
	if n := len(reOptLoad.FindAllString(body, 4)); n >= 2 {
		add("measure", zoneOf(secOptions), cueMedium, "it sets a load: concurrency, requests, duration or runs")
	}
	if reOptFormat.MatchString(body) {
		add("convert", zoneOf(secOptions), cueWeak, "it chooses a format")
	}
	if len(args) == 0 && reOptLength.MatchString(body) {
		add("generate", zoneOf(secOptions), cueWeak, "it takes only settings (length, count…)")
	}

	// What it says it does.
	about := p.Summary + "\n" + p.Description
	// Where it says it: the summary, else the description.
	saidIn := func(re *regexp.Regexp) int {
		if re.MatchString(p.Summary) {
			return zoneSummary
		}
		return zoneDescription
	}
	// "Display information about logical volumes" lists; "display
	// information about a file" inspects.
	// What it lists or reports is the files it is given when its usage
	// takes files: "print the strings in files" inspects them.
	switch {
	case (reSaysLists.MatchString(p.Summary) || reSaysReports.MatchString(about)) && has(reArgFile) && reSaysFiles.MatchString(p.Summary):
		add("inspect", zoneSummary, cueMedium, "it reports on the files it is given")
	case reSaysLists.MatchString(p.Summary):
		add("list", zoneSummary, cueMedium, "it lists things")
	case reSaysReports.MatchString(about) && len(args) > 0:
		add("inspect", saidIn(reSaysReports), cueMedium, "it reports on its input")
	case reSaysReports.MatchString(about):
		// Nothing goes in: it reports on the system.
		add("inspect", saidIn(reSaysReports), cueWeak, "it reports on the system")
	}
	if reSaysStdin.MatchString(about) {
		add("filter", saidIn(reSaysStdin), cueMedium, "it reads standard input and writes standard output")
	}

	// General: anything, a little.
	add("general", zoneName, cueBase, "it runs and prints (the usual kind)")
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
			// "[FILE]..." is the word "file".
			w := strings.ToLower(strings.Trim(strings.TrimRight(bare, ".…"), "[]{}()<>|,'\""))
			if w == "" || w == "option" || w == "options" {
				continue
			}
			if seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, usageArg{raw: strings.Trim(tok, "[]{}()|,"), word: w, repeat: repeat, withArgs: withArgs})
		}
	}
	return out
}

// archWeights weigh the evidence for archetypes.
type archWeights struct {
	CueZone     [numZones]int // points for a behaviour cue by where it was found (see zoneLabels)
	CueStrength [4]int        // percent of those points for a base, weak, medium and strong cue
	Words       scoreWeights  // the archetype phrases
	Broad       scoreWeights  // the broader phrases of each archetype (archetypeBroadPhrases)
	BroadShare  int           // percent of an archetype's broad-phrase score that is added to it
	Related     int           // per related program (SEE ALSO) on the archetype lists, at most two per archetype
	ClearLead   int           // the lead over the next archetype a clear choice needs
}

// splitZones gives points to the name, summary, description and usage, and
// the same to each help section.
func splitZones(name, summary, description, usage, section int) [numZones]int {
	var z [numZones]int
	z[zoneName], z[zoneSummary], z[zoneDescription], z[zoneUsage] = name, summary, description, usage
	for i := zoneSection; i < numZones; i++ {
		z[i] = section
	}
	return z
}

// defaultArchWeights give behaviour cues and phrases alike their points
// by where they were found: usage 30, summary 25, description 20, name 15,
// and 10 for each help section read (sectionsRead). A cue's strength is a percent of its part's
// points; the strengths, the weak-word points, the section limit and the
// share of the broad subject phrases were chosen by a cross-validated
// search within that split (MINT_ARCHETYPE_BENCH=tune).
var defaultArchWeights = archWeights{
	CueZone:     splitZones(15, 25, 20, 30, 10),
	CueStrength: [4]int{cueBase: 15, cueWeak: 75, cueMedium: 127, cueStrong: 150},
	Words: scoreWeights{
		Name: 15, Summary: 25, Description: 20, Usage: 30, Section: 10,
		WeakName: 8, WeakSummary: 13, WeakDescription: 11, WeakUsage: 16,
		Hits: 2, SectionCap: 30,
	},
	Broad:      defaultWeights,
	BroadShare: 60,
	Related:    10, // SEE ALSO is in the rest of the help
	ClearLead:  32, // calibrated: clear choices are right about 85% of the time
}

// archEvidence is what was found in a program's help, before weighing.
type archEvidence struct {
	cues    map[appkit.ArchetypeID][]cueHit
	words   phraseEvidence // archetype phrases
	broad   phraseEvidence // broader archetype phrases
	related map[appkit.ArchetypeID][]string
}

func findArchEvidence(p programText) archEvidence {
	return archEvidence{
		cues:    archetypeCues(p),
		words:   findEvidenceIn(archetypeSignalList, p),
		broad:   findEvidenceIn(broadSignalList, p),
		related: relatedArchetypes(p.Name, p.Related),
	}
}

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

// scoreArchEvidence weighs evidence with w: the 35 archetypes, best first;
// ties go to the order of appkit.AllArchetypeDefs, so every run is the same.
func scoreArchEvidence(w archWeights, ev archEvidence) []ArchetypeScore {
	words := map[appkit.ArchetypeID]PhraseScore{}
	for _, s := range scoreEvidenceIn(archetypeSignalList, w.Words, ev.words) {
		words[appkit.ArchetypeID(s.ID)] = s
	}
	broad := map[appkit.ArchetypeID]PhraseScore{}
	if w.BroadShare > 0 {
		for _, s := range scoreEvidenceIn(broadSignalList, w.Broad, ev.broad) {
			broad[appkit.ArchetypeID(s.ID)] = s
		}
	}
	var out []ArchetypeScore
	for _, def := range appkit.AllArchetypeDefs() {
		s := ArchetypeScore{ID: def.ID}
		for _, h := range ev.cues[def.ID] {
			s.Score += w.CueZone[h.zone] * w.CueStrength[h.level] / 100
			s.Hits = append(s.Hits, h.why)
		}
		if ws, ok := words[def.ID]; ok {
			s.Score += ws.Score
			s.Hits = append(s.Hits, ws.Hits...)
		}
		if bs, ok := broad[def.ID]; ok {
			if pts := bs.Score * w.BroadShare / 100; pts > 0 {
				s.Score += pts
				s.Hits = append(s.Hits, fmt.Sprintf("its subject (%d): %s", pts, strings.Join(bs.Hits, ", ")))
			}
		}
		for _, r := range ev.related[def.ID] {
			s.Score += w.Related
			s.Hits = append(s.Hits, "related: "+r)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// scoreArchetypes ranks the 35 archetypes for a program, best first.
func scoreArchetypes(p programText) []ArchetypeScore {
	return scoreArchEvidence(defaultArchWeights, findArchEvidence(p))
}

// Confidence of a choice.
const (
	ConfidenceListed = "listed" // on an archetype list: exact
	ConfidenceClear  = "clear"  // led the next archetype clearly
	ConfidenceClose  = "close"  // chosen, but the next one was near
)

// clearLead is the lead over the next archetype a choice needs to be clear.
var clearLead = defaultArchWeights.ClearLead

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
	return chooseFromScores(scoreArchetypes(p))
}

// chooseFromScores is the choice from ranked scores.
func chooseFromScores(scores []ArchetypeScore) ArchetypeChoice {
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
