package main

import (
	"regexp"
	"sort"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// contrastLinks join archetypes that are easily mistaken for each other,
// each side with words that tell it from the other. A link acts only when
// its two archetypes are the top two choices: then each side gains points
// for its words (see applyContrast). The words say what such programs
// usually say about themselves, from general knowledge, not from the
// benchmark programs' help.
var contrastLinks = []struct {
	a, b           appkit.ArchetypeID
	aWords, bWords []string
}{
	{"record", "convert", []string{"capture", "captures", "capturing", "record from", "recording", "recorder", "microphone", "sniff", "dump traffic"},
		[]string{"convert", "converts", "conversion", "transcode", "encode to", "decode to", "output format", "into another format"}},
	{"present", "convert", []string{"play", "plays", "playback", "player", "speaker", "speakers", "sound output", "display on the screen", "announce", "speak"},
		[]string{"convert", "converts", "conversion", "transcode", "encode to", "decode to", "output format", "into another format"}},
	{"read", "pack", []string{"pager", "view", "viewer", "screenful", "page through", "browse", "one screen at a time"},
		[]string{"create an archive", "create archives", "extract", "compress files", "decompress files", "archive files"}},
	{"watch", "list", []string{"real time", "real-time", "refresh", "refreshes", "every second", "continuously", "live", "updates", "interactive"},
		[]string{"list", "lists", "listing", "snapshot", "once"}},
	{"follow", "watch", []string{"log", "logs", "messages", "events", "as they arrive", "as they happen", "tail", "journal"},
		[]string{"statistics", "usage", "utilisation", "utilization", "load", "performance"}},
	{"follow", "list", []string{"monitor", "as they arrive", "as they happen", "events", "follow", "until interrupted"},
		[]string{"list", "lists", "snapshot", "once"}},
	{"inspect", "search", []string{"information about", "information on", "details", "describe", "properties", "metadata", "header"},
		[]string{"search for", "searches for", "find", "locate", "matching", "pattern", "regular expression"}},
	{"inspect", "convert", []string{"information", "info", "details", "properties", "metadata", "header", "summary of"},
		[]string{"convert", "converts", "conversion", "render", "encode", "output format"}},
	{"inspect", "record", []string{"information", "info", "details", "statistics about", "summary of", "properties"},
		[]string{"capture", "captures", "capturing", "record from", "recording", "sniff"}},
	{"device", "settings", []string{"device", "devices", "hardware", "interface", "interfaces", "driver", "card", "ethernet", "link"},
		[]string{"configuration", "preferences", "settings database", "key", "keys", "value", "values"}},
	{"look-up", "settings", []string{"lookup", "look up", "query", "resolve", "resolves", "entries from", "address", "addresses", "geolocation"},
		[]string{"configure", "configuration", "preferences", "set the", "change the"}},
	{"control", "toolkit", []string{"start", "stop", "restart", "service", "services", "unit", "units", "daemon"},
		[]string{"containers", "images", "virtual machines", "resources", "manage", "subcommands"}},
	{"records", "toolkit", []string{"package", "packages", "install", "uninstall", "upgrade", "repository", "repositories"},
		[]string{"containers", "images", "virtual machines", "resources", "subcommands"}},
	{"history", "mirror", []string{"backup", "backups", "snapshot", "snapshots", "revision", "revisions", "version", "versions", "restore"},
		[]string{"synchronize", "synchronise", "sync", "mirror", "replicate"}},
	{"list", "records", []string{"list", "lists", "show", "display", "currently loaded", "loaded"},
		[]string{"install", "remove", "add", "manage", "repository"}},
	{"search", "read", []string{"search", "searches", "locate", "locates", "find", "where"},
		[]string{"read", "view", "viewer", "page", "pager", "documentation"}},
	{"edit-files", "probe", []string{"link", "links", "copy", "copies", "move", "rename", "remove", "file", "files", "directory"},
		[]string{"scan", "scans", "ping", "pings", "port", "ports", "reachable"}},
	{"generate", "settings", []string{"generate", "generates", "create a new", "new key", "key pair", "random"},
		[]string{"configure", "configuration", "preferences", "settings"}},
	{"toolkit", "convert", []string{"subcommands", "commands", "manage", "create", "check", "info", "resize"},
		[]string{"convert", "converts", "conversion", "transcode", "render"}},
	{"serve", "history", []string{"server", "serve", "serves", "daemon", "listen", "listens", "network"},
		[]string{"commit", "commits", "branch", "revision", "snapshot", "backup"}},
	{"session", "transfer", []string{"interactive", "shell", "prompt", "commands", "session"},
		[]string{"download", "downloads", "upload", "uploads", "transfer", "fetch"}},
	{"rules", "list", []string{"rule", "rules", "partition", "partitions", "table", "chain", "chains", "policy"},
		[]string{"list", "lists", "show", "display"}},
	{"signal", "list", []string{"signal", "signals", "kill", "kills", "terminate", "renice"},
		[]string{"list", "lists", "listing", "display", "show", "report"}},
	{"signal", "search", []string{"signal", "signals", "kill", "kills"},
		[]string{"search for", "searches for", "find files", "matching lines", "regular expression"}},
	{"signal", "control", []string{"signal", "signals", "kill", "kills", "processes by name", "priority", "pid", "process id"},
		[]string{"service", "services", "unit", "units", "daemon", "start", "stop", "restart", "enable", "disable"}},
	{"wrap", "control", []string{"run a command", "runs a command", "run a program", "transient", "scope", "with a time limit"},
		[]string{"start", "stop", "restart", "status", "enable", "disable"}},
}

// contrastPoints are the points a contrast word gives its side in the
// name, summary, description or usage; in the rest of the help it gives a
// third of that. contrastCap is the most one side can gain. 60 was chosen
// by 5-fold cross-validation over 0-200 (MINT_ARCHETYPE_BENCH=1 with each
// value): the gain levels off from 60.
var contrastPoints, contrastCap = 60.0, 240.0

var contrastRes = map[string]*regexp.Regexp{}

func contrastRe(w string) *regexp.Regexp {
	if re, ok := contrastRes[w]; ok {
		return re
	}
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `\b`)
	contrastRes[w] = re
	return re
}

// applyContrast settles a close call between linked neighbours: when the
// top two archetypes are linked, each gains points for its contrast words,
// and the list is ranked again. Nothing changes otherwise.
func applyContrast(p programText, scores []ArchetypeScore) []ArchetypeScore {
	if len(scores) < 2 || contrastPoints <= 0 {
		return scores
	}
	top, second := scores[0].ID, scores[1].ID
	front := strings.Join([]string{p.Name, p.Summary, p.Description, strings.Join(p.Usage, " ")}, "\n")
	body := p.ArchBody
	gain := func(words []string) (pts float64, hits []string) {
		for _, w := range words {
			re := contrastRe(w)
			switch {
			case re.MatchString(front):
				pts += contrastPoints
				hits = append(hits, w)
			case re.MatchString(body):
				pts += contrastPoints / 3
				hits = append(hits, w+" (help)")
			}
		}
		if pts > contrastCap {
			pts = contrastCap
		}
		return
	}
	for _, l := range contrastLinks {
		var aWords, bWords []string
		switch {
		case l.a == top && l.b == second, l.a == second && l.b == top:
			aWords, bWords = l.aWords, l.bWords
		default:
			continue
		}
		ga, ha := gain(aWords)
		gb, hb := gain(bWords)
		out := append([]ArchetypeScore(nil), scores...)
		for i := range out[:2] {
			switch out[i].ID {
			case l.a:
				out[i].Score += ga
				out[i].Words += ga
				if ga > 0 {
					out[i].Hits = append(out[i].Hits, "rather than "+string(l.b)+": "+strings.Join(ha, ", "))
				}
			case l.b:
				out[i].Score += gb
				out[i].Words += gb
				if gb > 0 {
					out[i].Hits = append(out[i].Hits, "rather than "+string(l.a)+": "+strings.Join(hb, ", "))
				}
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
		return out
	}
	return scores
}
