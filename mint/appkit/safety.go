package appkit

import (
	"path/filepath"
	"strings"
	"time"
)

// Safety defaults applied to every ToolSpec by New (and by mint-gen when it
// writes spec.go), so that a generated interface never runs a command the
// user did not ask for, and asks properly before one that cannot be undone.

// Timeouts for commands appkit runs itself rather than as a background job.
// Variables so tests can shorten them.
var (
	statusTimeout  = 15 * time.Second // reading state: status commands and polling
	actionTimeout  = 2 * time.Minute  // row actions such as "restart unit"
	choicesTimeout = 3 * time.Second  // WithDynamicChoices lookups at startup
)

// readOnlyTools only report state when run with their usual flags, so a
// monitor may run them on a timer.
var readOnlyTools = map[string]bool{
	"free": true, "vmstat": true, "iostat": true, "mpstat": true, "pidstat": true,
	"sensors": true, "acpi": true, "uptime": true, "df": true, "w": true, "who": true,
	"nvidia-smi": true, "ps": true, "pgrep": true, "pstree": true, "lsns": true,
	"ipcs": true, "lsof": true, "dmesg": true, "last": true, "lastlog": true,
	"ausearch": true, "tail": true, "ss": true, "netstat": true, "lsblk": true,
	"blkid": true, "findmnt": true, "du": true, "journalctl": true,
}

// statusArgsForTool are read-only commands that show the current state of
// tools whose plain invocation changes something.
var statusArgsForTool = map[string][]string{
	"sysctl":        {"-a"},
	"timedatectl":   {"status"},
	"hostnamectl":   {"status"},
	"localectl":     {"status"},
	"loginctl":      {"list-sessions", "--no-pager"},
	"resolvectl":    {"status"},
	"ufw":           {"status", "verbose"},
	"iptables":      {"-L", "-n", "-v"},
	"ip6tables":     {"-L", "-n", "-v"},
	"nft":           {"list", "ruleset"},
	"firewall-cmd":  {"--list-all"},
	"xrandr":        {"--query"},
	"amixer":        {"scontents"},
	"pactl":         {"list", "short", "sinks"},
	"wpctl":         {"status"},
	"brightnessctl": {"info"},
	"bluetoothctl":  {"show"},
	"nmcli":         {"general", "status"},
	"rfkill":        {"list"},
	"systemctl":     {"list-units", "--type=service", "--no-pager"},
	"supervisorctl": {"status"},
	"service":       {"--status-all"},
}

// irreversibleTools destroy data or processes; running them needs the user
// to type the target (or the tool's name) to confirm.
var irreversibleTools = map[string]bool{
	"dd": true, "shred": true, "wipefs": true, "mkswap": true, "blkdiscard": true,
	"sgdisk": true, "sfdisk": true, "truncate": true, "rm": true,
	"kill": true, "pkill": true, "killall": true, "fuser": true,
}

func toolBase(name string) string {
	return strings.ToLower(filepath.Base(strings.TrimSpace(name)))
}

// IsReadOnlyTool reports whether running bin only reports state.
func IsReadOnlyTool(bin string) bool { return readOnlyTools[toolBase(bin)] }

// IsIrreversibleTool reports whether bin destroys data or processes.
func IsIrreversibleTool(bin string) bool {
	b := toolBase(bin)
	return irreversibleTools[b] || strings.HasPrefix(b, "mkfs")
}

// ApplySafetyDefaults fills in ReadOnly, StatusArgs and Irreversible for
// tools it knows, without overriding values the spec already sets.
func ApplySafetyDefaults(spec *ToolSpec) {
	bin := spec.BinaryName()
	if IsReadOnlyTool(bin) {
		spec.ReadOnly = true
	}
	if len(spec.StatusArgs) == 0 {
		if args, ok := statusArgsForTool[toolBase(bin)]; ok {
			spec.StatusArgs = append([]string(nil), args...)
		}
	}
	if IsIrreversibleTool(bin) {
		spec.Irreversible = true
	}
}

// CanReadState reports whether the app has a command that only reads state:
// StatusArgs, or a tool that is read-only itself. Only then may it run
// without the user asking (monitor polling, refreshing on open).
func (s ToolSpec) CanReadState() bool {
	return len(s.StatusArgs) > 0 || s.ReadOnly
}

// secretWords mark an option whose value is a password, token or key.
var secretWords = []string{"password", "passwd", "passphrase", "secret", "token", "api key", "api-key", "apikey", "credential"}

// LooksSecret reports whether a flag, its label or hint names a secret value.
// mint-gen uses it to turn such flags into FlagSecret.
func LooksSecret(flag, label, hint string) bool {
	text := strings.ToLower(flag + " " + label + " " + hint)
	for _, w := range secretWords {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}
