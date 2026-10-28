package appkit

import "strings"

// Apps made before archetypes name one of 28 old templates (T01..T28) in
// their spec's Template field. Mint no longer uses those templates: such an
// app is converted to the archetype its template stood for when it starts,
// and keeps the data level its template gave it, so an older app's results
// are never less protected than before.

type legacyTemplate struct {
	id, slug, name string
	archetype      ArchetypeID
	data, reason   string // the data level the template gave its tools
}

var legacyTemplates = []legacyTemplate{
	{"T01", "system-config", "System & Daemon Configurator", "settings", "", ""},
	{"T02", "dotfile-config", "Dotfile / App Config Editor", "settings", "", ""},
	{"T03", "hardware-controller", "Hardware & Device Controller", "device", "", ""},
	{"T04", "rule-table-editor", "Firewall & Rule-Table Editor", "rules", "", ""},
	{"T05", "batch-runner", "Single-Target Batch Runner", "edit-files", "", ""},
	{"T06", "media-transcoder", "Media & Document Transcoder", "convert", "", ""},
	{"T07", "downloader", "Downloader & Stream Fetcher", "transfer", "", ""},
	{"T08", "archive-manager", "Archive & Compression Tool", "pack", "", ""},
	{"T09", "backup-manager", "Backup & Snapshot Manager", "history", "", ""},
	{"T10", "build-test-runner", "Build, Test & Compiler Runner", "build", "", ""},
	{"T11", "benchmark-tester", "Benchmark & Stress Tester", "measure", "", ""},
	{"T12", "network-scanner", "Network & Port Scanner", "probe", "", ""},
	{"T13", "packet-analyser", "Packet & Traffic Analyser", "record", "sensitive", "captures network traffic, which can include passwords and private messages"},
	{"T14", "osint-recon", "OSINT & Recon Gatherer", "look-up", "sensitive", "gathers information about people and organisations"},
	{"T15", "security-auditor", "Security & Vulnerability Auditor", "audit", "sensitive", "its findings describe weaknesses of this computer or a target"},
	{"T16", "crypto-hash-tool", "Password, Hash & Crypto Utility", "convert", "secret", "works with keys, passwords and decrypted data"},
	{"T17", "resource-monitor", "Live System Resource Monitor", "watch", "", ""},
	{"T18", "service-controller", "Service & Daemon Controller", "control", "", ""},
	{"T19", "process-inspector", "Process & Open-File Inspector", "list", "", ""},
	{"T20", "log-analyser", "Log Streamer & Journal Analyser", "follow", "sensitive", "logs can include user names, addresses and sometimes passwords"},
	{"T21", "vcs-suite", "Version Control & Repo Suite", "history", "", ""},
	{"T22", "container-orchestrator", "Container & VM Orchestrator", "toolkit", "", ""},
	{"T23", "package-manager", "Package & Plugin Manager", "records", "", ""},
	{"T24", "disk-inspector", "Disk, Partition & Filesystem Inspector", "list", "", ""},
	{"T25", "content-searcher", "File & Content Searcher", "search", "", ""},
	{"T26", "data-query-explorer", "Structured Data & Query Explorer", "query", "", ""},
	{"T27", "doc-browser", "Reference & Documentation Browser", "read", "", ""},
	{"T28", "schedule-manager", "Schedule, Cron & Timer Manager", "schedule", "", ""},
}

func lookupLegacyTemplate(idOrSlug string) (legacyTemplate, bool) {
	q := strings.TrimSpace(idOrSlug)
	if q == "" {
		return legacyTemplate{}, false
	}
	for _, t := range legacyTemplates {
		if strings.EqualFold(t.id, q) || strings.EqualFold(t.slug, q) || strings.EqualFold(t.name, q) {
			return t, true
		}
	}
	return legacyTemplate{}, false
}

// ArchetypeForLegacyTemplate is the archetype an old template ID, slug or
// name ("T12", "network-scanner") stands for.
func ArchetypeForLegacyTemplate(idOrSlug string) (ArchetypeDef, bool) {
	t, ok := lookupLegacyTemplate(idOrSlug)
	if !ok {
		return ArchetypeDef{}, false
	}
	return LookupArchetype(string(t.archetype))
}
