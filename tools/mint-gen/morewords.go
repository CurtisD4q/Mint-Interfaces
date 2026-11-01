package main

// moreArchetypeSignals are further phrases for each archetype, written from
// its definition (appkit/archetypedefs.go) and what such programs usually
// say, not from the benchmark programs' help. They count like
// archetypeSignals. A phrase an archetype already has is not repeated.
var moreArchetypeSignals = map[string][]string{
	"ask":        {"evaluate an expression|evaluates expressions|mathematical expression", "~arithmetic", "base conversion|number base", "~compute|~computes", "day of the week", "answer a question"},
	"look-up":    {"reverse lookup|reverse dns", "query dns servers|query a dns server", "ip address of|address of a host", "~resolve|~resolves", "name resolution", "name service|name service switch|nss databases", "~lookups"},
	"inspect":    {"~report on", "~describe|~describes", "~properties", "header information|file header|headers", "information on", "module information|kernel module information", "document information", "~identify|~identifies", "hardware information"},
	"list":       {"~lists", "running processes|process list|process status", "~open files", "~entries", "~enumerate|~enumerates", "loaded modules|loaded kernel modules", "~mounted"},
	"search":     {"~search for|~searches for", "locate files|files matching", "~occurrences", "~locates", "search the database|searches a database"},
	"query":      {"database queries|query a database", "jq|xpath|jsonpath", "~fields"},
	"compare":    {"compare files|compares files|compare two", "~differences between", "~changes between", "byte by byte"},
	"read":       {"~reading", "~viewer|~viewing", "screen at a time|one screenful|screenful", "~browser", "info pages|manual pages"},
	"watch":      {"~monitors", "realtime|real-time", "~refreshes|updates every", "~periodically", "interactive process viewer|process viewer", "~utilisation"},
	"follow":     {"~following", "~tail", "as they happen|as they occur", "~streaming", "~messages", "watch for changes"},
	"record":     {"~captures|~capturing", "~dumps", "audio input|microphone", "screen recording|session recording|terminal session"},
	"probe":      {"~scans|~scanning", "~probes", "~pings", "~reachable|~reachability", "network discovery"},
	"audit":      {"~check for|~checks for", "~weakness|~weaknesses"},
	"measure":    {"~measures"},
	"convert":    {"~converts", "from one format to another|into another format|to another format", "~export", "~render|~rendering", "output format"},
	"filter":     {"standard input to standard output|stdin to stdout", "each line|lines of", "~squeeze", "~substitute", "~sorts", "duplicate lines|adjacent lines", "tabs to spaces|spaces to tabs", "format paragraphs|fold long lines"},
	"generate":   {"~generates", "~creates a new", "key pair|key pairs", "temporary file|temporary directory|unique name"},
	"pack":       {"~archives", "~unzip", "tar", "compressed file|compressed files"},
	"build":      {"~compiles", "run tests|run the tests"},
	"transfer":   {"~downloads", "~uploads", "~transfers", "remote server|from a remote", "secure copy|over ssh|over the network"},
	"present":    {"~plays|~player", "display an image|show an image|image viewer", "sound output|audio output|~speakers"},
	"edit-files": {"make directories|create directories", "change permissions|change ownership|file attributes", "symbolic link|hard link"},
	"mirror":     {"~synchronise", "~mirroring", "~replicate|~replication", "keep in sync|kept in sync", "source and destination", "two-way|bidirectional"},
	"wrap":       {"~niceness", "as another user|as a different user", "in a namespace|new namespace", "modified environment", "immune to hangups"},
	"serve":      {"~listens", "accept connections|incoming connections", "share files"},
	"session":    {"connect to|connection to", "enter commands|commands at the prompt", "~console"},
	"settings":   {"~configure", "system settings", "terminal settings|terminal line settings", "resource database"},
	"device":     {"~hardware", "network interfaces", "~driver", "audio device", "~battery", "~usb"},
	"rules":      {"~policy|~policies", "packet filtering", "~nat|~forwarding", "~routes", "~allow|~deny|~reject"},
	"control":    {"~systemd|~init", "~shutdown|~reboot", "~transient"},
	"records":    {"~repository|~repositories", "add and remove|add or remove", "~catalogue|~catalog|~inventory"},
	"history":    {"~branches", "~revisions"},
	"toolkit":    {"~sub-commands", "virtual machines|~domains", "~suite", "~cluster"},
	"schedule":   {"at a later time|at a specified time", "~timers", "~wake|~alarm", "~recurring"},
}
