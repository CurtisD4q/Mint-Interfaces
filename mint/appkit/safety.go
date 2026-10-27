package appkit

import (
	"path/filepath"
	"regexp"
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

// separatorTools take file operands and read "--" as the end of their
// options (getopt), so the target can always be passed after "--".
var separatorTools = map[string]bool{
	"rm": true, "cp": true, "mv": true, "ln": true, "mkdir": true, "rmdir": true,
	"chmod": true, "chown": true, "chgrp": true, "touch": true, "truncate": true,
	"shred": true, "install": true, "cat": true, "head": true, "wc": true,
	"stat": true, "du": true, "ls": true, "file": true, "diff": true, "cmp": true,
	"comm": true, "sort": true, "uniq": true, "md5sum": true, "sha1sum": true,
	"sha256sum": true, "sha512sum": true, "b2sum": true, "wipefs": true,
	"realpath": true, "readlink": true, "unlink": true, "split": true,
}

// UsesOperandSeparator reports whether bin is known to accept "--" before
// its file operands.
func UsesOperandSeparator(bin string) bool { return separatorTools[toolBase(bin)] }

// ApplySafetyDefaults fills in ReadOnly, StatusArgs, Irreversible and
// OperandSeparator for tools it knows, without overriding values the spec
// already sets.
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
	if UsesOperandSeparator(bin) {
		spec.OperandSeparator = true
	}
	applyKnownSecretEnvs(spec)
	applyDataDefaults(spec)
}

// knownSecretEnv is a secret a known tool reads from an environment
// variable, and the options it replaces.
type knownSecretEnv struct {
	Env      string
	Label    string
	Replaces []string // options carrying the same secret on the command line
	EnvFlag  string   // switch telling the tool to read the variable
}

// knownSecretEnvs lists tools that read a secret from the environment, so
// the secret never has to go on the command line. Only variables the tools
// document for this purpose are listed.
var knownSecretEnvs = map[string][]knownSecretEnv{
	"restic":     {{Env: "RESTIC_PASSWORD", Label: "Repository password"}},
	"borg":       {{Env: "BORG_PASSPHRASE", Label: "Repository passphrase"}},
	"kopia":      {{Env: "KOPIA_PASSWORD", Label: "Repository password", Replaces: []string{"--password"}}},
	"duplicity":  {{Env: "PASSPHRASE", Label: "Encryption passphrase"}},
	"rclone":     {{Env: "RCLONE_CONFIG_PASS", Label: "Config password"}},
	"psql":       {{Env: "PGPASSWORD", Label: "Database password"}},
	"pg_dump":    {{Env: "PGPASSWORD", Label: "Database password"}},
	"pg_dumpall": {{Env: "PGPASSWORD", Label: "Database password"}},
	"pg_restore": {{Env: "PGPASSWORD", Label: "Database password"}},
	"gh":         {{Env: "GH_TOKEN", Label: "GitHub token"}},
	"glab":       {{Env: "GITLAB_TOKEN", Label: "GitLab token"}},
	"sshpass":    {{Env: "SSHPASS", Label: "SSH password", Replaces: []string{"-p"}, EnvFlag: "-e"}},
}

// KnownSecretEnv reports whether bin is known to read its secret from the
// environment variable env.
func KnownSecretEnv(bin, env string) bool {
	for _, k := range knownSecretEnvs[toolBase(bin)] {
		if k.Env == env {
			return true
		}
	}
	return false
}

// applyKnownSecretEnvs routes a known tool's secrets through its environment
// variable: options carrying the same secret are switched to it, and a
// secret with no option is added as its own entry.
func applyKnownSecretEnvs(spec *ToolSpec) {
	for _, k := range knownSecretEnvs[toolBase(spec.BinaryName())] {
		if spec.hasSecretEnv(k.Env) {
			continue
		}
		replaced := false
		for i := range spec.Flags {
			f := &spec.Flags[i]
			for _, r := range k.Replaces {
				if f.Flag == r && !f.SecretStdin {
					f.Kind, f.SecretEnv, f.EnvFlag = FlagSecret, k.Env, k.EnvFlag
					f.Choices, f.Min, f.Max = nil, 0, 0
					replaced = true
				}
			}
		}
		if !replaced {
			spec.Flags = append(spec.Flags, FlagSpec{Kind: FlagSecret, Section: "Credentials",
				Label: k.Label, SecretEnv: k.Env, EnvFlag: k.EnvFlag})
		}
	}
}

func (s *ToolSpec) hasSecretEnv(env string) bool {
	for _, f := range s.Flags {
		if f.SecretEnv == env {
			return true
		}
	}
	return false
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

// How a secret option's value reaches the program, from safest to least safe.
const (
	RouteStdin    = "standard input"
	RouteEnv      = "environment variable"
	RouteArgument = "command line"
)

// SecretRoute says how a FlagSecret value is passed to the program:
// standard input when the program reads it there, else an environment
// variable when it has one, else the command line.
func (f FlagSpec) SecretRoute() string {
	switch {
	case f.SecretStdin:
		return RouteStdin
	case f.SecretEnv != "":
		return RouteEnv
	}
	return RouteArgument
}

// RouteNote explains a route in a few words, for reports.
func RouteNote(route string) string {
	switch route {
	case RouteStdin:
		return "never visible to other users"
	case RouteEnv:
		return "only you and root can see it"
	default:
		return "visible to other users (ps) while it runs; the app asks first"
	}
}

var (
	destructiveFlagWords = map[string]bool{
		"force": true, "delete": true, "remove": true, "overwrite": true, "wipe": true,
		"erase": true, "destroy": true, "purge": true, "prune": true,
		"kill": true, "unlink": true, "shred": true,
	}
	// "force" is left out of descriptions: there it usually means "set"
	// ("force NAME as owner"), unlike a --force option.
	destructiveLabel = regexp.MustCompile(`\b(delete|deletes|deleting|overwrite|overwrites|wipe|wipes|erase|erases|destroy|destroys|purge|purges|shred|never prompt|do not prompt|don't prompt|without (asking|confirmation|prompting))\b|\bremoves? (it|files|directories|the|all|everything)\b`)
	// Options that make a tool more careful mention overwriting too.
	saferLabel = regexp.MustCompile(`\b(do not|don't|never|no) (overwrite|clobber|delete|remove)|^(interactive[,:]? )?(prompt|ask) before\b`)
)

// LooksDestructive reports whether an option deletes, overwrites or forces
// something, judged from its name (--force, --remove-files, --hard,
// --no-preserve-root) and description. Reports list these for review.
func LooksDestructive(flag, label string) bool {
	name := strings.ToLower(strings.TrimLeft(flag, "-"))
	switch {
	case name == "no-preserve-root" || name == "hard":
		return true
	case strings.HasPrefix(name, "no-"):
		return false // --no-overwrite-dir and friends make things safer
	}
	for _, tok := range strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '=' }) {
		if destructiveFlagWords[tok] {
			return true
		}
	}
	l := strings.ToLower(label)
	return destructiveLabel.MatchString(l) && !saferLabel.MatchString(l)
}

// SecretOption is one secret option in a SafetyReport.
type SecretOption struct {
	Flag, Label, Route string
	Env                string // the variable, when Route is RouteEnv
}

// SafetyReport summarises how an interface for the tool behaves: whether it
// can run without being asked, how it confirms, which options are secrets
// and how each is passed, and which options look destructive.
type SafetyReport struct {
	Running     string         // when the tool runs, and how it is confirmed
	Secrets     []SecretOption // FlagSecret options, in spec order
	Destructive []FlagSpec     // options that delete, overwrite or force
	Data        string         // DataNormal, DataSensitive or DataSecret
	DataReason  string
	Revealing   []FlagSpec // options that make the tool print secrets
}

// SafetyReport describes s after ApplySafetyDefaults.
func (s ToolSpec) SafetyReport() SafetyReport {
	ApplySafetyDefaults(&s)
	var r SafetyReport
	switch {
	case s.Irreversible:
		r.Running = "destroys data or processes: runs only when asked, and the target must be typed to confirm"
	case s.ReadOnly:
		r.Running = "only reports state, so it may run automatically (monitor polling, refresh on open)"
	case len(s.StatusArgs) > 0:
		r.Running = "changes things, so it runs only when asked; its state is read with: " + s.BinaryName() + " " + strings.Join(s.StatusArgs, " ")
	case s.ConfirmRun:
		r.Running = "changes things, so it runs only when asked, after a Yes/No confirmation"
	default:
		r.Running = "runs only when asked"
	}
	flags := append([]FlagSpec(nil), s.Flags...)
	for _, sub := range s.Subcommands {
		flags = append(flags, sub.Flags...)
	}
	for _, f := range flags {
		if f.Kind == FlagSecret {
			r.Secrets = append(r.Secrets, SecretOption{Flag: f.Flag, Label: f.Label, Route: f.SecretRoute(), Env: f.SecretEnv})
		} else if LooksDestructive(f.Flag, f.Label) {
			r.Destructive = append(r.Destructive, f)
		}
		if f.Reveals {
			r.Revealing = append(r.Revealing, f)
		}
	}
	r.Data, r.DataReason = s.Data, s.DataReason
	return r
}
