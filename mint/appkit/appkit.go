// Package appkit provides a declarative runtime for turning command-line
// programs into Mint TUI applications using the 6 Base Archetypes, Composable
// Mixins, and 28 Template Types (T01..T28).
package appkit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
	"github.com/CurtisD4q/Mint-Interfaces/mint/secure"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// FlagKind identifies how a CLI option is edited in the TUI.
type FlagKind string

const (
	FlagBool   FlagKind = "bool"
	FlagText   FlagKind = "text"
	FlagString FlagKind = "text" // alias for FlagText
	FlagInt    FlagKind = "int"
	FlagChoice FlagKind = "choice"
	FlagFile   FlagKind = "file"
	FlagDir    FlagKind = "dir"
	// FlagSecret is a password, token or key. It is asked for with a
	// password dialog, kept in memory only (never saved to settings,
	// presets or history) and masked in the command preview.
	FlagSecret FlagKind = "secret"
)

// Choice represents a single selectable option for a FlagChoice flag.
type Choice struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// FlagSpec describes a single command-line option exposed in the TUI.
type FlagSpec struct {
	Flag       string   `json:"flag"`
	Join       string   `json:"join,omitempty"` // " " (default) or "="
	Kind       FlagKind `json:"kind"`
	Section    string   `json:"section,omitempty"`
	Label      string   `json:"label"`
	Hint       string   `json:"hint,omitempty"`
	Default    string   `json:"default,omitempty"`
	Choices    []Choice `json:"choices,omitempty"`
	ChoicesCmd []string `json:"choices_cmd,omitempty"` // WithDynamicChoices: runtime command to populate Choices
	Min        int      `json:"min,omitempty"`
	Max        int      `json:"max,omitempty"`
	Step       int      `json:"step,omitempty"`
	Advanced   bool     `json:"advanced,omitempty"`
	// SecretStdin (FlagSecret only): the program reads the secret from
	// standard input, as with --password-stdin. The flag is passed on its
	// own and the value goes to stdin, never onto the command line.
	SecretStdin bool `json:"secret_stdin,omitempty"`
	// SecretEnv (FlagSecret only): the program reads the secret from this
	// environment variable (RESTIC_PASSWORD, PGPASSWORD…). The value is set
	// in the program's environment, which only you and root can read, and
	// the option itself is not passed. Flag may be empty for a secret with
	// no option at all. SecretStdin, when also set, takes precedence.
	SecretEnv string `json:"secret_env,omitempty"`
	// EnvFlag is passed on its own when the secret goes through SecretEnv,
	// for programs that need to be told to read it (sshpass -e).
	EnvFlag string `json:"env_flag,omitempty"`
	// Role: what the value is for (an input or output file, a log, a
	// format…), from the help text. See ArgRole.
	Role ArgRole `json:"role,omitempty"`
	// Reveals: setting this option makes the tool print secrets
	// (--export-secret-keys, --decrypt…), so the run's results are secret.
	Reveals bool `json:"reveals,omitempty"`
}

// SecretKey identifies a FlagSecret option: its flag, or "$NAME" for a
// secret that has only an environment variable.
func (f FlagSpec) SecretKey() string {
	if f.Flag == "" && f.SecretEnv != "" {
		return "$" + f.SecretEnv
	}
	return f.Flag
}

// ProfileSpec defines a named preset of flag values and extra arguments.
type ProfileSpec struct {
	Name      string            `json:"name"`
	Desc      string            `json:"desc,omitempty"`
	Flags     map[string]string `json:"flags,omitempty"`
	ExtraArgs []string          `json:"extra_args,omitempty"`
}

// ToolSpec defines the metadata, template/archetype, mixins, flags, and presets
// for a wrapped CLI program.
type ToolSpec struct {
	Name              string           `json:"name"`
	Bin               string           `json:"bin,omitempty"`
	Title             string           `json:"title,omitempty"`
	Desc              string           `json:"desc,omitempty"`
	Version           string           `json:"version,omitempty"`
	Template          string           `json:"template,omitempty"`  // "T01".."T28" or template slug
	Archetype         Archetype        `json:"archetype,omitempty"` // config, runner, scanner, monitor, explorer, suite
	TargetLabel       string           `json:"target_label,omitempty"`
	TargetPlaceholder string           `json:"target_placeholder,omitempty"`
	TargetDefault     string           `json:"target_default,omitempty"`
	OutputsFile       bool             `json:"outputs_file,omitempty"`      // Mixin: WithFileOutput
	OutputFlag        string           `json:"output_flag,omitempty"`       // e.g. "-o" or "--output"; empty means stdout capture
	OutputPositional  bool             `json:"output_positional,omitempty"` // the tool writes its output to its last argument (ffmpeg)
	StdoutKind        string           `json:"stdout_kind,omitempty"`       // what the tool prints: StdoutResults, StdoutLog, StdoutBinary or "" (unknown)
	DefaultExt        string           `json:"default_ext,omitempty"`       // e.g. "txt", "json", "pcap"
	DefaultResultsDir string           `json:"default_results_dir,omitempty"`
	TableOutput       bool             `json:"table_output,omitempty"`       // Mixin: WithTableOutput
	TableColumns      []string         `json:"table_columns,omitempty"`      // Optional explicit column headers
	CollapsibleOutput bool             `json:"collapsible_output,omitempty"` // Mixin: WithCollapsibleOutput
	ChunkSize         int              `json:"chunk_size,omitempty"`         // Chunk size for collapsible output (default 100)
	NeedsRoot         bool             `json:"needs_root,omitempty"`         // Mixin: WithRootAuth
	ConfirmRun        bool             `json:"confirm_run,omitempty"`        // Mixin: WithConfirm
	ConfirmPrompt     string           `json:"confirm_prompt,omitempty"`     // Custom confirmation prompt
	KeepHistory       bool             `json:"keep_history,omitempty"`       // Mixin: WithHistory
	StatusArgs        []string         `json:"status_args,omitempty"`        // Read-only command that shows current state (Config/Monitor)
	ReadOnly          bool             `json:"read_only,omitempty"`          // Running the tool only reports state, so a monitor may poll it
	Irreversible      bool             `json:"irreversible,omitempty"`       // Destroys data or processes: running it needs typed confirmation
	Diff              *DiffSpec        `json:"diff,omitempty"`               // Mixin: WithDiff (Compare)
	Findings          *FindingsSpec    `json:"findings,omitempty"`           // Mixin: WithFindings (Audit)
	Preview           *PreviewSpec     `json:"preview,omitempty"`            // Mixin: WithPreview (Edit files)
	OperandSeparator  bool             `json:"operand_separator,omitempty"`  // The tool reads "--" as the end of its options: Mint puts it before the target, so "-f" there is a name, not an option
	ExitMeanings      map[int]string   `json:"exit_meanings,omitempty"`      // Exit codes besides 0 that are results, not failures (diff: 1 → "files differ")
	Data              string           `json:"data,omitempty"`               // How sensitive results are: "", DataSensitive or DataSecret
	DataReason        string           `json:"data_reason,omitempty"`        // Why, in a few words ("captures network traffic…")
	ApplyArgs         []string         `json:"apply_args,omitempty"`         // Prefix args when applying Config
	PollInterval      time.Duration    `json:"poll_interval,omitempty"`      // Poll interval for ArchetypeMonitor
	Subcommands       []SubcommandSpec `json:"subcommands,omitempty"`        // Subcommand tabs for ArchetypeSuite
	RowActions        []RowActionSpec  `json:"row_actions,omitempty"`        // Per-row actions for Monitor/Config tables
	Profiles          []ProfileSpec    `json:"profiles,omitempty"`           // Mixin: WithProfiles
	Flags             []FlagSpec       `json:"flags,omitempty"`
}

// BinaryName returns the executable name to invoke.
func (s ToolSpec) BinaryName() string {
	if s.Bin != "" {
		return s.Bin
	}
	return s.Name
}

// EnabledMixins returns which of the original 8 mixins are active on s.
// EnabledMixinNames also lists later add-ons.
func (s ToolSpec) EnabledMixins() Mixins {
	hasDyn := false
	for _, f := range s.Flags {
		if len(f.ChoicesCmd) > 0 {
			hasDyn = true
			break
		}
	}
	return Mixins{
		WithFileOutput:        s.OutputsFile,
		WithTableOutput:       s.TableOutput,
		WithCollapsibleOutput: s.CollapsibleOutput,
		WithProfiles:          len(s.Profiles) > 0,
		WithDynamicChoices:    hasDyn,
		WithRootAuth:          s.NeedsRoot,
		WithConfirm:           s.ConfirmRun,
		WithHistory:           s.KeepHistory,
	}
}

// RunRecord stores a completed run in history.
type RunRecord struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Target     string    `json:"target"`
	Args       []string  `json:"args"`
	CmdLine    string    `json:"cmd_line"`
	OutputFile string    `json:"output_file,omitempty"`
	State      string    `json:"state"`
	DurationMs int64     `json:"duration_ms"`
	LineCount  int       `json:"line_count"`
}

// FileEntry represents an entry inside ResultsDir.
type FileEntry struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// ToolApp holds the live runtime state for a ToolSpec.
type ToolApp struct {
	Spec             ToolSpec
	Target           string
	ExtraArgs        string
	Profile          string
	ActiveSubcommand string
	Values           map[string]string
	SaveToFile       bool
	ResultsDir       string
	OutputFile       string
	ConfigDir        string
	DataDir          string
	MonitorPaused    bool
	LastRefresh      time.Time
	SnapshotLines    []string
	InMemory         bool // results go to a private in-memory folder (see SetInMemory)

	mu        sync.Mutex
	activeJob *runner.Job
	lastOut   string
	history   []RunRecord

	owner      *invoker        // the user behind `sudo app`, who gets the files
	store      *secure.Store   // FlagSecret values: encrypted, locked memory, never on disk
	secretSet  map[string]bool // which secrets the user entered (names only), to notice expiry
	argvOK     bool            // user accepted secrets on the command line this session
	freeTextOK string          // fingerprint of free-text secrets the user accepted
	refreshing bool            // a background state refresh is running
	refreshed  bool            // a refresh finished since the screen last looked
	refreshErr string

	memSetting   *bool           // InMemory as saved in settings (nil: never chosen)
	diskDir      string          // the results folder to return to when InMemory is turned off
	memBacked    bool            // the in-memory folder really is in memory (tmpfs)
	lastData     string          // level the last run's output turned out to have
	lastFindings []string        // kinds of secret found in the last run's output
	diffOK       bool            // WithDiff: the last run printed a format the differences view reads
	findings     *findingsReport // WithFindings: what the last run found
	findingsOK   bool            // WithFindings: the last run printed a format the findings view reads
}

// New creates and initializes a ToolApp from a ToolSpec, applying its Template
// (T01..T28) and Archetype defaults automatically.
func New(spec ToolSpec) *ToolApp {
	if spec.Template != "" {
		if tpl, ok := LookupTemplate(spec.Template); ok {
			tpl.ApplyTo(&spec)
		}
	} else if tpl, ok := TemplateForTool(spec.BinaryName()); ok {
		tpl.ApplyTo(&spec)
	}

	ApplySafetyDefaults(&spec)
	if spec.Archetype == "" {
		spec.Archetype = ArchetypeRunner
	}
	if spec.Title == "" {
		spec.Title = "◈ " + spec.Name
	}
	if spec.Desc == "" {
		spec.Desc = "Configure and run " + spec.Name
	}
	if spec.TargetLabel == "" {
		spec.TargetLabel = "Target / arguments"
	}
	if spec.TargetPlaceholder == "" {
		spec.TargetPlaceholder = "arguments, file, or host"
	}
	if spec.DefaultExt == "" {
		spec.DefaultExt = "txt"
	}
	if spec.ChunkSize <= 0 {
		spec.ChunkSize = 100
	}

	home, _ := os.UserHomeDir()
	owner := sudoInvoker()
	if owner != nil {
		// Started with sudo: keep settings, history and results in the
		// user's own home, not root's.
		home = owner.home
	}
	if home == "" {
		home = "."
	}
	resDir := spec.DefaultResultsDir
	if resDir == "" {
		resDir = filepath.Join(home, "mint-results", spec.Name)
	}
	cfgDir := filepath.Join(home, ".config", "mint-"+spec.Name)
	dataDir := filepath.Join(home, ".local", "share", "mint-"+spec.Name)

	values := make(map[string]string, len(spec.Flags))
	for _, f := range spec.Flags {
		if f.Default != "" {
			values[f.Flag] = f.Default
		} else if f.Kind == FlagBool {
			values[f.Flag] = "false"
		}
	}

	activeSub := ""
	if len(spec.Subcommands) > 0 {
		activeSub = spec.Subcommands[0].Name
	}

	a := &ToolApp{
		Spec:             spec,
		Target:           spec.TargetDefault,
		ActiveSubcommand: activeSub,
		Values:           values,
		SaveToFile:       spec.OutputsFile && (spec.StdoutKind != StdoutLog || spec.OutputFlag != "" || spec.OutputPositional),
		ResultsDir:       resDir,
		OutputFile:       DefaultOutputFilename(spec.Name, spec.DefaultExt),
		ConfigDir:        cfgDir,
		DataDir:          dataDir,
		secretSet:        map[string]bool{},
		owner:            owner,
	}

	if len(spec.Profiles) > 0 {
		a.ApplyProfile(spec.Profiles[0].Name)
	}
	a.ResolveDynamicChoices()
	a.memSetting = nil
	_ = a.LoadSettings()
	switch {
	case a.memSetting != nil && *a.memSetting:
		if err := a.SetInMemory(true); err != nil { // recreate it after a reboot
			a.InMemory = false
			if a.diskDir != "" {
				a.ResultsDir = a.diskDir
			}
		}
	case a.memSetting == nil && spec.Data == DataSecret:
		_ = a.SetInMemory(true)
	}
	a.loadHistory()
	return a
}

// DefaultOutputFilename returns a clean timestamped filename for tool outputs.
func DefaultOutputFilename(toolName, ext string) string {
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "txt"
	}
	return fmt.Sprintf("%s-%s.%s", outputNameBase(toolName), time.Now().Format("20060102-150405"), ext)
}

// outputNameBase is the tool name as it starts an automatic file name.
func outputNameBase(toolName string) string {
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.ToLower(toolName))
	if clean == "" {
		clean = "mint-output"
	}
	return clean
}

// ApplyProfile applies the named preset to the current flag values and ExtraArgs.
func (a *ToolApp) ApplyProfile(name string) bool {
	for _, p := range a.Spec.Profiles {
		if p.Name == name {
			a.Profile = p.Name
			for _, f := range a.Spec.Flags {
				if f.Kind == FlagBool {
					a.Values[f.Flag] = "false"
				}
			}
			for k, v := range p.Flags {
				a.Values[k] = v
			}
			if len(p.ExtraArgs) > 0 {
				a.ExtraArgs = strings.Join(p.ExtraArgs, " ")
			} else {
				a.ExtraArgs = ""
			}
			return true
		}
	}
	return false
}

// ResolvedOutputPath returns the full output file path when file output is enabled.
func (a *ToolApp) ResolvedOutputPath() string {
	if !a.Spec.OutputsFile || !a.SaveToFile {
		return ""
	}
	name := strings.TrimSpace(a.OutputFile)
	if name == "" {
		name = DefaultOutputFilename(a.Spec.Name, a.Spec.DefaultExt)
	}
	name = a.withOutputExt(name)
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	dir := strings.TrimSpace(a.ResultsDir)
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, name)
}

func (a *ToolApp) buildFlagAndTargetArgs() (args []string, outPath string) {
	args, outPath, _ = a.composeFlagArgs(false)
	return args, outPath
}

// secretMask stands in for secret values wherever a command line is shown
// or recorded.
const secretMask = "••••••"

// composeFlagArgs builds the flag, output and target arguments. With mask,
// secret values are replaced by secretMask (for previews and history).
// stdin lists the secrets (by key) the program reads from standard input;
// StartJob writes them straight from the secure store.
func (a *ToolApp) composeFlagArgs(mask bool) (args []string, outPath string, stdin []string) {
	for _, f := range a.Spec.Flags {
		val := strings.TrimSpace(a.Values[f.Flag])
		switch f.Kind {
		case FlagSecret:
			key := f.SecretKey()
			if !a.hasSecret(key) {
				continue
			}
			switch f.SecretRoute() {
			case RouteStdin:
				if f.Flag != "" {
					args = append(args, f.Flag)
				}
				stdin = append(stdin, key)
				continue
			case RouteEnv:
				// The value goes in the environment (secretEnv); only a
				// "read it from the environment" switch is passed.
				if f.EnvFlag != "" {
					args = append(args, f.EnvFlag)
				}
				continue
			}
			// The command line can only take a string: decrypt just now.
			sec := secretMask
			if !mask {
				sec = a.reveal(key)
			}
			if f.Join == "=" || strings.HasSuffix(f.Flag, "=") {
				args = append(args, strings.TrimSuffix(f.Flag, "=")+"="+sec)
			} else {
				args = append(args, f.Flag, sec)
			}
		case FlagBool:
			if val == "true" || val == "1" || val == "on" || val == "yes" {
				args = append(args, f.Flag)
			}
		default:
			if val != "" {
				parts := []string{val}
				if strings.Contains(val, "\n") {
					parts = strings.Split(val, "\n")
				}
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					if f.Join == "=" || strings.HasSuffix(f.Flag, "=") {
						flagTok := strings.TrimSuffix(f.Flag, "=")
						args = append(args, flagTok+"="+part)
					} else {
						args = append(args, f.Flag, part)
					}
				}
			}
		}
	}

	outPath = a.ResolvedOutputPath()
	if outPath != "" && a.Spec.OutputFlag != "" {
		if strings.HasSuffix(a.Spec.OutputFlag, "=") {
			args = append(args, a.Spec.OutputFlag+outPath)
		} else {
			args = append(args, a.Spec.OutputFlag, outPath)
		}
	}

	if extra := strings.TrimSpace(a.ExtraArgs); extra != "" {
		if mask {
			extra = maskFreeText(extra, a.secretFlagNames())
		}
		args = append(args, splitShellWords(extra)...)
	}

	if target := strings.TrimSpace(a.Target); target != "" {
		if mask {
			target = maskFreeText(target, a.secretFlagNames())
		}
		if a.Spec.OperandSeparator {
			// Without "--", a file named "-rf" (or a target typed as
			// "-rf x") reaches rm as options. After it, every word is
			// a name.
			args = append(args, "--")
		}
		args = append(args, splitShellWords(target)...)
	}
	if outPath != "" && a.Spec.OutputPositional {
		// The tool writes to its last argument (ffmpeg … outfile). A path
		// that starts with "-" would read as an option, so it never does.
		if strings.HasPrefix(outPath, "-") {
			outPath = "./" + outPath
		}
		args = append(args, outPath)
	}
	return args, outPath, stdin
}

// What a tool prints (ToolSpec.StdoutKind).
const (
	StdoutResults = "results" // its results: worth saving
	StdoutLog     = "log"     // progress and messages; the real output goes to a file
	StdoutBinary  = "binary"  // binary data, not text
)

// toolWritesOutput reports whether the tool writes the output file itself
// (through its output option or its last argument), rather than Mint
// saving what it prints.
func (a *ToolApp) toolWritesOutput() bool {
	return a.Spec.OutputFlag != "" || a.Spec.OutputPositional
}

// saveToggleLabel names the "save to file" switch for what will be saved.
func (a *ToolApp) saveToggleLabel() string {
	if !a.toolWritesOutput() && a.Spec.StdoutKind == StdoutLog {
		return "Save the program's messages to a file"
	}
	return "Save output to file"
}

// secretFlagNames lists the tool's secret options, such as "-p".
func (a *ToolApp) secretFlagNames() []string {
	var out []string
	for _, f := range a.Spec.Flags {
		if f.Kind == FlagSecret && f.Flag != "" {
			out = append(out, f.Flag)
		}
	}
	return out
}

// FreeTextSecrets lists passwords, tokens and keys typed into the Extra
// arguments and Target boxes, where they reach the command line.
func (a *ToolApp) FreeTextSecrets() []FreeTextSecret {
	flags := a.secretFlagNames()
	found := FindFreeTextSecrets(a.ExtraArgs, flags)
	seen := map[string]bool{}
	for _, s := range found {
		seen[s.Value] = true
	}
	for _, s := range FindFreeTextSecrets(a.Target, flags) {
		if !seen[s.Value] {
			found = append(found, s)
		}
	}
	return found
}

// freeTextFingerprint identifies a set of free-text secrets without keeping
// them, so an accepted warning is not asked again for the same values.
func freeTextFingerprint(found []FreeTextSecret) string {
	h := sha256.New()
	for _, s := range found {
		h.Write([]byte(s.Value))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// staleMaskedText reports which free-text box still holds a value hidden
// when settings were saved (it has to be typed again).
func (a *ToolApp) staleMaskedText() string {
	switch {
	case strings.Contains(a.ExtraArgs, secretMask):
		return "Extra arguments"
	case strings.Contains(a.Target, secretMask):
		return a.Spec.TargetLabel
	}
	return ""
}

// newRunMasker hides every secret this run uses, and those typed into the
// free-text boxes, in the program's output.
func (a *ToolApp) newRunMasker() *outputMasker {
	m := newOutputMasker()
	a.mu.Lock()
	st := a.store
	a.mu.Unlock()
	if st != nil {
		for _, f := range a.Spec.Flags {
			if f.Kind == FlagSecret && st.Has(f.SecretKey()) {
				_ = st.Use(f.SecretKey(), func(b []byte) error { m.add(b); return nil })
			}
		}
	}
	for _, s := range a.FreeTextSecrets() {
		m.add([]byte(s.Value))
	}
	return m
}

// secretEnv returns NAME=value for every secret passed through the
// environment (NAME=•••••• with mask, for previews).
func (a *ToolApp) secretEnv(mask bool) []string {
	var env []string
	for _, f := range a.Spec.Flags {
		if f.Kind != FlagSecret || f.SecretRoute() != RouteEnv {
			continue
		}
		if key := f.SecretKey(); a.hasSecret(key) {
			sec := secretMask
			if !mask {
				sec = a.reveal(key) // an environment entry is a string
			}
			env = append(env, f.SecretEnv+"="+sec)
		}
	}
	return env
}

// hasSecret reports whether a secret is stored (and not expired).
func (a *ToolApp) hasSecret(key string) bool {
	a.mu.Lock()
	st := a.store
	a.mu.Unlock()
	return st != nil && st.Has(key)
}

// secretExpired reports whether the user entered this secret but the
// store's idle timeout has since wiped it.
func (a *ToolApp) secretExpired(key string) bool {
	a.mu.Lock()
	set := a.secretSet[key]
	a.mu.Unlock()
	return set && !a.hasSecret(key)
}

// reveal decrypts a secret for a command-line argument or an environment
// entry, which Go can only pass as strings. Stdin secrets never use it.
func (a *ToolApp) reveal(key string) string {
	a.mu.Lock()
	st := a.store
	a.mu.Unlock()
	var out string
	if st != nil {
		_ = st.Use(key, func(b []byte) error { out = string(b); return nil })
	}
	return out
}

// secureStore opens the secure store the first time a secret is entered.
func (a *ToolApp) secureStore() (*secure.Store, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store == nil {
		st, err := secure.Open()
		if err != nil {
			return nil, err
		}
		a.store = st
	}
	return a.store, nil
}

// SecretProtection describes how well this app can protect secrets here,
// once one has been entered ("" before that).
func (a *ToolApp) SecretProtection() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store == nil {
		return ""
	}
	return a.store.Level().String()
}

// SetSecret stores the value of a FlagSecret option for this session only,
// encrypted in the secure store (an empty value removes it).
func (a *ToolApp) SetSecret(flag, value string) error {
	if value == "" {
		a.mu.Lock()
		st := a.store
		delete(a.secretSet, flag)
		a.mu.Unlock()
		if st != nil {
			st.Delete(flag)
		}
		return nil
	}
	st, err := a.secureStore()
	if err != nil {
		return err
	}
	err = st.Put(flag, []byte(value))
	if errors.Is(err, secure.ErrExpired) {
		// The idle timeout wiped the old key: start a fresh one.
		if err = st.Renew(); err == nil {
			err = st.Put(flag, []byte(value))
		}
	}
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.secretSet[flag] = true
	a.mu.Unlock()
	return nil
}

// expiredSecrets lists the labels of secrets the user entered that the
// idle timeout has wiped, and forgets them, so a run never goes ahead
// silently without them.
func (a *ToolApp) expiredSecrets() []string {
	var out []string
	for _, f := range a.Spec.Flags {
		if f.Kind == FlagSecret && a.secretExpired(f.SecretKey()) {
			label := f.Label
			if label == "" {
				label = f.SecretKey()
			}
			out = append(out, label)
			a.mu.Lock()
			delete(a.secretSet, f.SecretKey())
			a.mu.Unlock()
		}
	}
	return out
}

// Close wipes every secret. appkit.Run calls it when the interface closes.
func (a *ToolApp) Close() {
	a.mu.Lock()
	st := a.store
	a.store = nil
	a.mu.Unlock()
	if st != nil {
		st.Close()
	}
}

func pronoun(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// argvSecrets lists the labels of secrets that are set and can only be
// passed on the command line, where other users can see them in ps.
func (a *ToolApp) argvSecrets() []string {
	var out []string
	for _, f := range a.Spec.Flags {
		if f.Kind == FlagSecret && f.SecretRoute() == RouteArgument && a.hasSecret(f.SecretKey()) {
			label := f.Label
			if label == "" {
				label = f.Flag
			}
			out = append(out, label)
		}
	}
	return out
}

// BuildArgs constructs the CLI argument list and resolved output path from the
// current archetype, subcommand, flag values, output file settings, extra args, and target.
func (a *ToolApp) BuildArgs() (args []string, outPath string) {
	args, outPath, _ = a.buildArgs(false)
	return args, outPath
}

func (a *ToolApp) buildArgs(mask bool) (args []string, outPath string, stdin []string) {
	if a.Spec.Archetype == ArchetypeSuite && len(a.Spec.Subcommands) > 0 {
		if sub := a.activeSubcommandSpec(); sub != nil {
			args = append(args, sub.Args...)
		}
	} else if a.Spec.Archetype == ArchetypeConfig && len(a.Spec.ApplyArgs) > 0 {
		args = append(args, a.Spec.ApplyArgs...)
	}
	prefix := len(args) // subcommand or apply words
	flagArgs, outPath, stdin := a.composeFlagArgs(mask)
	args = append(args, flagArgs...)
	args, _ = a.diffArgs(args, prefix)
	args, _ = a.findingsArgs(args, prefix)
	return args, outPath, stdin
}

// argPrefixLen is how many leading words of the argument list are the
// subcommand (or apply) words, before any options.
func (a *ToolApp) argPrefixLen() int {
	if a.Spec.Archetype == ArchetypeSuite && len(a.Spec.Subcommands) > 0 {
		if sub := a.activeSubcommandSpec(); sub != nil {
			return len(sub.Args)
		}
	} else if a.Spec.Archetype == ArchetypeConfig && len(a.Spec.ApplyArgs) > 0 {
		return len(a.Spec.ApplyArgs)
	}
	return 0
}

// CommandPreview returns the full shell-like command preview string.
func (a *ToolApp) CommandPreview() string {
	args, outPath, _ := a.buildArgs(true)
	parts := make([]string, 0, len(args)+3)
	parts = append(parts, a.secretEnv(true)...)
	parts = append(parts, a.Spec.BinaryName())
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\"'") {
			parts = append(parts, strconv.Quote(arg))
		} else {
			parts = append(parts, arg)
		}
	}
	if outPath != "" && !a.toolWritesOutput() {
		parts = append(parts, ">", outPath)
	}
	return strings.Join(parts, " ")
}

func splitShellWords(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

type persistedSettings struct {
	Target           string            `json:"target"`
	ExtraArgs        string            `json:"extra_args"`
	Profile          string            `json:"profile"`
	ActiveSubcommand string            `json:"active_subcommand,omitempty"`
	Values           map[string]string `json:"values"`
	SaveToFile       bool              `json:"save_to_file"`
	ResultsDir       string            `json:"results_dir"`
	OutputFile       string            `json:"output_file"`
	InMemory         *bool             `json:"in_memory,omitempty"`
	DiskResultsDir   string            `json:"disk_results_dir,omitempty"`
}

func (a *ToolApp) settingsPath() string {
	return filepath.Join(a.ConfigDir, "settings.json")
}

// SaveSettings writes current flag values and output paths to ConfigDir/settings.json.
func (a *ToolApp) SaveSettings() error {
	if a.ConfigDir == "" {
		return nil
	}
	if err := util.Mkdir(a.ConfigDir); err != nil {
		return err
	}
	// Passwords typed into the free-text boxes are never written to disk.
	st := persistedSettings{
		Target:           maskFreeText(a.Target, a.secretFlagNames()),
		ExtraArgs:        maskFreeText(a.ExtraArgs, a.secretFlagNames()),
		Profile:          a.Profile,
		ActiveSubcommand: a.ActiveSubcommand,
		Values:           a.Values,
		SaveToFile:       a.SaveToFile,
		ResultsDir:       a.ResultsDir,
		OutputFile:       a.OutputFile,
		InMemory:         &a.InMemory,
		DiskResultsDir:   a.diskDir,
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	// Settings hold targets and option values, so keep them private.
	if err := os.WriteFile(a.settingsPath(), b, 0o600); err != nil {
		return err
	}
	a.giveBack(a.settingsPath())
	return nil
}

// LoadSettings restores saved settings from ConfigDir/settings.json if present.
func (a *ToolApp) LoadSettings() error {
	b, err := os.ReadFile(a.settingsPath())
	if err != nil {
		return err
	}
	var st persistedSettings
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.Target != "" {
		a.Target = st.Target
	}
	a.ExtraArgs = st.ExtraArgs
	if st.Profile != "" {
		a.Profile = st.Profile
	}
	if st.ActiveSubcommand != "" {
		a.ActiveSubcommand = st.ActiveSubcommand
	}
	for k, v := range st.Values {
		a.Values[k] = v
	}
	a.SaveToFile = st.SaveToFile
	if st.ResultsDir != "" {
		a.ResultsDir = st.ResultsDir
	}
	if st.OutputFile != "" {
		a.OutputFile = st.OutputFile
	}
	if st.InMemory != nil {
		on := *st.InMemory
		a.memSetting = &on
		a.diskDir = st.DiskResultsDir
		if on {
			a.InMemory = true // SetInMemory re-checks the folder
		}
	}
	return nil
}

func (a *ToolApp) historyPath() string {
	return filepath.Join(a.DataDir, "runs.json")
}

func (a *ToolApp) loadHistory() {
	b, err := os.ReadFile(a.historyPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &a.history)
}

func (a *ToolApp) saveHistoryLocked() {
	if a.DataDir == "" {
		return
	}
	_ = util.Mkdir(a.DataDir)
	b, err := json.MarshalIndent(a.history, "", "  ")
	if err != nil {
		return
	}
	if os.WriteFile(a.historyPath(), b, 0o600) == nil {
		a.giveBack(a.historyPath())
	}
}

// History returns a copy of the recorded runs for this tool.
func (a *ToolApp) History() []RunRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]RunRecord, len(a.history))
	copy(out, a.history)
	return out
}

// StartJob launches the configured command as a background runner.Job.
func (a *ToolApp) StartJob() (*runner.Job, error) {
	a.mu.Lock()
	if a.activeJob != nil && a.activeJob.Running() {
		a.activeJob.Stop()
	}
	a.mu.Unlock()

	if labels := a.expiredSecrets(); len(labels) > 0 {
		return nil, fmt.Errorf("%s expired after a period of inactivity; enter %s again", strings.Join(labels, ", "), pronoun(len(labels)))
	}
	if box := a.staleMaskedText(); box != "" {
		return nil, fmt.Errorf("%s has a hidden value (%s) that was not saved; type it again", box, secretMask)
	}
	args, outPath, stdinSecrets := a.buildArgs(false)
	shownArgs, _, _ := a.buildArgs(true)
	if outPath != "" {
		if err := a.makeResultsDir(filepath.Dir(outPath)); err != nil {
			return nil, err
		}
		a.giveBack(filepath.Dir(outPath))
	}
	_ = a.SaveSettings()

	bin := a.Spec.BinaryName()
	cmdArgs := args
	env := a.secretEnv(false)
	useStdin := len(stdinSecrets) > 0

	_, findingsOK := a.findingsArgs(args, a.argPrefixLen())
	var scope scanScope
	if a.Spec.Findings != nil {
		wd, _ := os.Getwd()
		scope = newScanScope(wd, splitShellWords(a.Target))
	}
	started := time.Now()
	cmdLine := a.CommandPreview()
	shownTarget := maskFreeText(a.Target, a.secretFlagNames())
	masker := a.newRunMasker()
	runLevel, _ := a.DataLevel()

	job := runner.Start(runner.Cmd{
		Name:      bin,
		Args:      cmdArgs,
		Env:       env,
		Input:     useStdin,
		Redact:    masker.Redact,
		OKExits:   a.Spec.ExitMeanings,
		RawStdout: a.findingsRawLimit(),
		OnExit: func(j *runner.Job) {
			a.readFindings(j, findingsRun{readable: findingsOK, scope: scope, key: targetKey(a.Spec.Name, args)})
			lines := j.Lines()
			if outPath != "" && !a.toolWritesOutput() {
				_ = mint.SaveLogLines(outPath, lines)
			}
			level := runLevel
			found := masker.Findings()
			if len(found) > 0 {
				level = DataSecret
			}
			a.protectFile(level, outPath)
			a.giveBack(outPath) // the tool, running as root, may have written it
			a.mu.Lock()
			a.lastData, a.lastFindings = "", nil
			if len(found) > 0 {
				a.lastData, a.lastFindings = DataSecret, found
			}
			a.mu.Unlock()
			if len(found) > 0 {
				msg := "The output contained " + strings.Join(found, ", ") + "; it is hidden here."
				if outPath != "" && a.toolWritesOutput() {
					msg += " " + filepath.Base(outPath) + " may still hold it: it is readable only by you, and \"Encrypt the output\" protects it."
				}
				mint.ToastWarn(msg)
			}
			if j.State() == runner.Failed && !runningAsRoot() && looksLikeNeedsRoot(j.LastError()) {
				mint.ToastError(needsRootMessage(a.Spec.Name))
			}
			textLines := make([]string, len(lines))
			for i, l := range lines {
				textLines[i] = l.Text
			}
			rec := RunRecord{
				ID:         started.Format("20060102-150405"),
				Time:       started,
				Target:     shownTarget,
				Args:       shownArgs,
				CmdLine:    cmdLine,
				OutputFile: outPath,
				State:      j.State().String(),
				DurationMs: j.Elapsed().Milliseconds(),
				LineCount:  len(lines),
			}
			a.mu.Lock()
			a.SnapshotLines = textLines
			a.history = append([]RunRecord{rec}, a.history...)
			if len(a.history) > 50 {
				a.history = a.history[:50]
			}
			a.saveHistoryLocked()
			a.mu.Unlock()
		},
	})

	if useStdin {
		// Secrets go from the store's protected memory straight to the
		// program's standard input, without an ordinary copy.
		a.mu.Lock()
		st := a.store
		a.mu.Unlock()
		for _, key := range stdinSecrets {
			if st != nil {
				_ = st.Use(key, func(b []byte) error {
					if _, err := job.Write(b); err != nil {
						return err
					}
					_, err := job.Write([]byte{'\n'})
					return err
				})
			}
		}
		job.CloseInput()
	}

	_, diffOK := a.diffArgs(args, a.argPrefixLen())
	a.mu.Lock()
	a.activeJob = job
	a.lastOut = outPath
	a.diffOK = diffOK
	a.mu.Unlock()

	return job, nil
}

// ActiveJob returns the currently running or most recently launched job.
func (a *ToolApp) ActiveJob() *runner.Job {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.activeJob
}

// ListResultsDir returns all files and subdirectories inside ResultsDir.
func (a *ToolApp) ListResultsDir() ([]FileEntry, error) {
	dir := strings.TrimSpace(a.ResultsDir)
	if dir == "" {
		dir = "."
	}
	defer a.giveBack(dir)
	if err := a.makeResultsDir(dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	eagerStat := len(entries) <= 256
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		var sz int64
		var mod time.Time
		isDir := e.IsDir()
		if eagerStat {
			if info, ierr := e.Info(); ierr == nil {
				sz = info.Size()
				mod = info.ModTime()
			}
		}
		out = append(out, FileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			Size:    sz,
			ModTime: mod,
			IsDir:   isDir,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		if eagerStat && !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (a *ToolApp) statusBadges() []mint.Badge {
	var data []mint.Badge
	if level, _ := a.DataLevel(); level != DataNormal {
		b, _ := dataBadge(level)
		data = append(data, b)
	}
	return append(data, a.jobBadges()...)
}

func (a *ToolApp) jobBadges() []mint.Badge {
	job := a.ActiveJob()
	if job == nil {
		return nil
	}
	if job.Running() {
		return []mint.Badge{{Text: "running", Kind: mint.BadgeOK}}
	}
	switch job.State() {
	case runner.Done:
		if meaning := job.ExitMeaning(); meaning != "" {
			return []mint.Badge{{Text: meaning, Kind: mint.BadgeOK}}
		}
		return []mint.Badge{{Text: "done", Kind: mint.BadgeOK}}
	case runner.Failed:
		return []mint.Badge{{Text: "failed", Kind: mint.BadgeError}}
	default:
		return []mint.Badge{{Text: job.State().String(), Kind: mint.BadgeIdle}}
	}
}

func (a *ToolApp) targetAndPresetSection(secName string) mint.Section {
	rows := []mint.Row{
		mint.Text(mint.TextOpts{
			ID:          "target",
			Label:       a.Spec.TargetLabel,
			Placeholder: a.Spec.TargetPlaceholder,
			Get:         func() string { return a.Target },
			Set:         func(v string) { a.Target = strings.TrimSpace(v) },
		}),
	}
	if len(a.Spec.Profiles) > 0 {
		opts := make([]string, len(a.Spec.Profiles))
		for i, p := range a.Spec.Profiles {
			opts[i] = p.Name
		}
		rows = append(rows, mint.Picker(mint.PickerOpts{
			ID:      "profile",
			Label:   "Preset profile",
			Options: opts,
			Get:     func() string { return a.Profile },
			Set:     func(v string) { a.ApplyProfile(v) },
		}))
	}
	rows = append(rows, mint.Text(mint.TextOpts{
		ID:          "extra_args",
		Label:       "Extra arguments",
		Placeholder: "optional CLI flags",
		Get:         func() string { return a.ExtraArgs },
		Set:         func(v string) { a.ExtraArgs = v },
	}))
	return mint.Section{
		Name: secName,
		Rows: rows,
	}
}

func (a *ToolApp) flagSections() []mint.Section {
	var sections []mint.Section
	sectionOrder := []string{}
	primary := map[string][]mint.Row{}
	advanced := map[string][]mint.Row{}

	for _, f := range a.Spec.Flags {
		secName := f.Section
		if secName == "" {
			secName = "Options"
		}
		if _, seen := primary[secName]; !seen {
			if _, seenAdv := advanced[secName]; !seenAdv {
				sectionOrder = append(sectionOrder, secName)
			}
		}
		row := a.flagRow(f)
		if f.Advanced {
			advanced[secName] = append(advanced[secName], row)
		} else {
			primary[secName] = append(primary[secName], row)
		}
	}

	for _, secName := range sectionOrder {
		pRows := primary[secName]
		aRows := advanced[secName]
		if len(pRows) == 0 && len(aRows) > 0 {
			sections = append(sections, mint.CollapsibleSection(secName+" (Advanced)", true, aRows...))
			continue
		}
		rows := append([]mint.Row(nil), pRows...)
		if len(aRows) > 0 {
			rows = append(rows, mint.Collapsible(mint.CollapsibleOpts{
				ID:        "adv:" + secName,
				Title:     "More " + secName + " options",
				Badge:     fmt.Sprintf("%d options", len(aRows)),
				Collapsed: true,
				Rows:      aRows,
			}))
		}
		sections = append(sections, mint.Section{
			Name: secName,
			Rows: rows,
		})
	}
	return sections
}

func (a *ToolApp) outputFileSection() mint.Section {
	rows := []mint.Row{
		mint.Toggle(mint.ToggleOpts{
			ID:    "out:enable",
			Label: a.saveToggleLabel(),
			Get:   func() bool { return a.SaveToFile },
			Set:   func(v bool) { a.SaveToFile = v },
		}),
	}
	rows = append(rows, a.dataRows()...)
	return mint.Section{
		Name: "Output File (WithFileOutput)",
		Rows: append(rows,
			mint.Text(mint.TextOpts{
				ID:          "out:dir",
				Label:       "Save directory",
				Placeholder: "/path/to/results",
				Get:         func() string { return a.ResultsDir },
				Set: func(v string) {
					if strings.TrimSpace(v) != "" && strings.TrimSpace(v) != a.ResultsDir {
						a.ResultsDir = strings.TrimSpace(v)
						a.InMemory, a.diskDir = false, "" // a folder of your choosing
					}
				},
			}),
			mint.Text(mint.TextOpts{
				ID:          "out:file",
				Label:       "File name",
				Placeholder: "output." + a.Spec.DefaultExt,
				Get:         func() string { return a.OutputFile },
				Set: func(v string) {
					if strings.TrimSpace(v) != "" {
						a.OutputFile = strings.TrimSpace(v)
					}
				},
			}),
			mint.Value(mint.ValueOpts{
				Label: "Full output path",
				Get: func() string {
					p := a.ResolvedOutputPath()
					if p == "" {
						return "(disabled)"
					}
					return p
				},
			}),
			mint.Action(mint.ActionOpts{
				Label: "Generate fresh timestamped filename",
				Hint:  "rename",
				OnActivate: func(c *mint.Ctx) {
					a.OutputFile = DefaultOutputFilename(a.Spec.Name, a.outputExt())
					c.Toast("Output filename: " + a.OutputFile)
				},
			}),
			mint.Action(mint.ActionOpts{
				Label: "Browse output directory…",
				Hint:  "files",
				OnActivate: func(c *mint.Ctx) {
					c.Push(a.DirectoryScreen())
				},
			}),
		),
	}
}

func (a *ToolApp) flagRow(f FlagSpec) mint.Row {
	key := f.Flag
	if f.Kind == FlagSecret {
		key = f.SecretKey()
	}
	label := f.Label
	if label == "" {
		label = f.Flag
	}

	switch f.Kind {
	case FlagBool:
		return mint.Checkbox(mint.CheckboxOpts{
			ID:    "flag:" + key,
			Label: label,
			Get:   func() bool { return a.Values[key] == "true" },
			Set: func(v bool) {
				if v {
					a.Values[key] = "true"
				} else {
					a.Values[key] = "false"
				}
			},
		})
	case FlagChoice:
		opts := make([]string, len(f.Choices))
		for i, c := range f.Choices {
			opts[i] = c.Value
		}
		if len(opts) == 0 {
			opts = []string{""}
		}
		return mint.Picker(mint.PickerOpts{
			ID:      "flag:" + key,
			Label:   label,
			Options: opts,
			Get:     func() string { return a.Values[key] },
			Set:     func(v string) { a.Values[key] = v },
		})
	case FlagInt:
		step := f.Step
		if step <= 0 {
			step = 1
		}
		return mint.Stepper(mint.StepperOpts{
			ID:    "flag:" + key,
			Label: label,
			Min:   f.Min,
			Max:   f.Max,
			Step:  step,
			Get: func() int {
				v, _ := strconv.Atoi(a.Values[key])
				return v
			},
			Set: func(v int) {
				a.Values[key] = strconv.Itoa(v)
			},
		})
	case FlagSecret:
		return mint.Action(mint.ActionOpts{
			ID:    "flag:" + key,
			Label: label,
			Get: func() string {
				if a.secretExpired(key) {
					return "expired, enter again"
				}
				if a.hasSecret(key) {
					return "set (not saved)"
				}
				return "not set"
			},
			OnActivate: func(c *mint.Ctx) {
				if v, ok := c.Password(label, "Kept for this session only; empty clears it"); ok {
					if err := a.SetSecret(key, v); err != nil {
						c.ToastError("Could not store the secret: " + err.Error())
						return
					}
					if lvl := a.SecretProtection(); lvl != "" && lvl != "full" {
						c.ToastWarn("Secret protection " + lvl)
					}
				}
			},
		})
	case FlagFile, FlagDir:
		dirsOnly := f.Kind == FlagDir
		return mint.Action(mint.ActionOpts{
			Label: label,
			Hint:  a.pathFlagSummary(key, dirsOnly),
			OnActivate: func(c *mint.Ctx) {
				startDir := a.ResultsDir
				if cur := strings.TrimSpace(a.Values[key]); cur != "" {
					if dirsOnly {
						startDir = cur
					} else {
						startDir = filepath.Dir(cur)
					}
				}
				if chosen, ok := browsePathPopup(c, label, startDir, a.Values[key], dirsOnly); ok {
					a.Values[key] = chosen
				}
			},
		})
	default:
		if isRepeatableKVFlag(f) {
			return mint.Action(mint.ActionOpts{
				Label: label,
				Hint:  a.kvFlagSummary(key, f.Hint),
				OnActivate: func(c *mint.Ctx) {
					if updated, ok := editRepeatableValuesPopup(c, label, a.Values[key], f.Hint); ok {
						a.Values[key] = updated
					}
				},
			})
		}
		hint := f.Hint
		if hint == "" {
			hint = "value"
		}
		return mint.Text(mint.TextOpts{
			ID:          "flag:" + key,
			Label:       label,
			Placeholder: hint,
			Get:         func() string { return a.Values[key] },
			Set:         func(v string) { a.Values[key] = strings.TrimSpace(v) },
		})
	}
}

func (a *ToolApp) pathFlagSummary(key string, dirsOnly bool) string {
	v := strings.TrimSpace(a.Values[key])
	if v != "" {
		return v
	}
	if dirsOnly {
		return "browse dir…"
	}
	return "browse file…"
}

func isRepeatableKVFlag(f FlagSpec) bool {
	combined := strings.ToLower(f.Hint + " " + f.Label)
	return strings.Contains(combined, "key=val") ||
		strings.Contains(combined, "k=v") ||
		strings.Contains(combined, "header") ||
		strings.Contains(combined, "repeatable") ||
		strings.Contains(combined, "multiple")
}

func (a *ToolApp) kvFlagSummary(key, hint string) string {
	v := strings.TrimSpace(a.Values[key])
	if v == "" {
		if hint != "" {
			return hint
		}
		return "key=value…"
	}
	lines := strings.Split(v, "\n")
	if len(lines) == 1 {
		return lines[0]
	}
	return fmt.Sprintf("%s (+%d more)", lines[0], len(lines)-1)
}

func editRepeatableValuesPopup(c *mint.Ctx, title, current, hint string) (string, bool) {
	if hint == "" {
		hint = "KEY=VALUE"
	}
	var entries []string
	for _, line := range strings.Split(current, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			entries = append(entries, t)
		}
	}
	for {
		items := []mint.Choice{
			{Value: "__add__", Label: "+ Add entry (" + hint + ")…"},
		}
		if len(entries) > 0 {
			items = append(items,
				mint.Choice{Value: "__done__", Label: "✓ Done (" + strconv.Itoa(len(entries)) + " configured)"},
				mint.Choice{Value: "__clear__", Label: "✕ Clear all entries"},
			)
		}
		for i, e := range entries {
			items = append(items, mint.Choice{
				Value: fmt.Sprintf("idx:%d", i),
				Label: e,
			})
		}
		pickChoice, ok := c.ChooseItem(title, items, "No matching entries")
		if !ok {
			return strings.Join(entries, "\n"), true
		}
		pick := pickChoice.Value
		switch pick {
		case "__done__":
			return strings.Join(entries, "\n"), true
		case "__clear__":
			entries = nil
		case "__add__":
			val, pok := c.Prompt(title, "Add "+hint, "")
			if pok && strings.TrimSpace(val) != "" {
				entries = append(entries, strings.TrimSpace(val))
			}
		default:
			if strings.HasPrefix(pick, "idx:") {
				idx, err := strconv.Atoi(strings.TrimPrefix(pick, "idx:"))
				if err == nil && idx >= 0 && idx < len(entries) {
					val, pok := c.Prompt(title, "Edit entry (empty to remove)", entries[idx])
					if pok {
						val = strings.TrimSpace(val)
						if val == "" {
							entries = append(entries[:idx], entries[idx+1:]...)
						} else {
							entries[idx] = val
						}
					}
				}
			} else if strings.Contains(pick, ",") {
				return strings.Join(entries, "\n"), true
			}
		}
	}
}

func browsePathPopup(c *mint.Ctx, title, startDir, currentVal string, dirsOnly bool) (string, bool) {
	dir := strings.TrimSpace(startDir)
	if dir == "" {
		dir = "."
	}
	if abs, err := filepath.Abs(util.ExpandUser(dir)); err == nil {
		dir = abs
	}
	showHidden := false
	for {
		rawEntries, err := os.ReadDir(dir)
		items := []mint.Choice{
			{Value: "__manual__", Label: "✎ Type path manually…"},
		}
		if dirsOnly {
			items = append(items, mint.Choice{
				Value: "__select_dir__",
				Label: "✓ Use this directory: " + dir,
			})
		}
		items = append(items,
			mint.Choice{Value: "__up__", Label: "↑ .. (Parent directory: " + filepath.Dir(dir) + ")"},
			mint.Choice{Value: "__hidden__", Label: fmt.Sprintf("⊙ Toggle hidden dotfiles (hidden=%v)", showHidden)},
		)
		if currentVal != "" {
			items = append(items, mint.Choice{Value: "__clear__", Label: "✕ Clear value"})
		}
		if err == nil {
			var dirs, files []mint.Choice
			for _, e := range rawEntries {
				name := e.Name()
				if !showHidden && strings.HasPrefix(name, ".") {
					continue
				}
				full := filepath.Join(dir, name)
				isDir := e.IsDir()
				if e.Type()&os.ModeSymlink != 0 && len(rawEntries) <= 2048 {
					if st, serr := os.Stat(full); serr == nil && st.IsDir() {
						isDir = true
					}
				}
				if isDir {
					dirs = append(dirs, mint.Choice{
						Value: "dir:" + full,
						Label: "▸ " + name + "/",
					})
				} else if !dirsOnly {
					files = append(files, mint.Choice{
						Value: "file:" + full,
						Label: "  " + name,
					})
				}
			}
			items = append(items, dirs...)
			items = append(items, files...)
		}
		pickChoice, ok := c.ChooseItem(title+" — "+dir, items, "No matching files")
		if !ok {
			return "", false
		}
		pick := pickChoice.Value
		switch pick {
		case "__manual__":
			def := currentVal
			if def == "" {
				def = dir
			}
			v, pok := c.Prompt(title, "Enter path", def)
			if pok {
				return strings.TrimSpace(v), true
			}
		case "__select_dir__":
			return dir, true
		case "__up__":
			dir = filepath.Dir(dir)
		case "__hidden__":
			showHidden = !showHidden
		case "__clear__":
			return "", true
		default:
			if strings.HasPrefix(pick, "dir:") {
				dir = strings.TrimPrefix(pick, "dir:")
			} else if strings.HasPrefix(pick, "file:") {
				return strings.TrimPrefix(pick, "file:"), true
			}
		}
	}
}

// MainScreen dispatches to the primary screen for the tool's configured Archetype
// (ConfigScreen, RunnerScreen, ScannerScreen, MonitorScreen, ExplorerScreen, or SuiteScreen).
func (a *ToolApp) MainScreen() *mint.Screen {
	switch a.Spec.Archetype {
	case ArchetypeConfig:
		return a.ConfigScreen()
	case ArchetypeScanner:
		return a.ScannerScreen()
	case ArchetypeMonitor:
		return a.MonitorScreen()
	case ArchetypeExplorer:
		return a.ExplorerScreen()
	case ArchetypeSuite:
		return a.SuiteScreen()
	default:
		return a.RunnerScreen()
	}
}

// RunnerScreen builds the ArchetypeRunner ("runner") Task & Pipeline screen.
func (a *ToolApp) RunnerScreen() *mint.Screen {
	s := &mint.Screen{
		Icon:  "▸",
		Title: a.Spec.Name,
		Status: func() []mint.Badge {
			return a.statusBadges()
		},
	}
	s.Refresh = func(scr *mint.Screen) {
		var sections []mint.Section
		sections = append(sections, a.targetAndPresetSection("Target & Preset"))
		sections = append(sections, a.flagSections()...)
		if a.Spec.OutputsFile {
			sections = append(sections, a.outputFileSection())
		}
		if rootSec := a.rootAuthSection(); rootSec != nil {
			sections = append(sections, *rootSec)
		}
		sections = append(sections, mint.Section{
			Name: "Execute",
			Rows: []mint.Row{
				mint.Value(mint.ValueOpts{
					Label: "Command preview",
					Get:   func() string { return a.CommandPreview() },
				}),
				mint.Action(mint.ActionOpts{
					Label: "Run " + a.Spec.Name,
					Hint:  "run",
					OnActivate: func(c *mint.Ctx) {
						a.executeFromUI(c)
					},
				}),
			},
		})
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}

// RunScreen builds the live job monitor, progress bar, table/collapsible output, and file actions screen.
func (a *ToolApp) RunScreen() *mint.Screen {
	s := &mint.Screen{
		Icon:  "◧",
		Title: a.Spec.Name + " — Live Run",
		Status: func() []mint.Badge {
			return a.statusBadges()
		},
		Poll: func(scr *mint.Screen) bool {
			job := a.ActiveJob()
			return job != nil && job.Changed()
		},
	}
	s.Refresh = func(scr *mint.Screen) {
		job := a.ActiveJob()
		var statusRows []mint.Row
		statusRows = append(statusRows,
			mint.Value(mint.ValueOpts{
				Label: "Command",
				Get:   func() string { return a.CommandPreview() },
			}),
			mint.Progress(mint.ProgressOpts{
				ID:    "run:progress",
				Label: "Job progress",
				Get: func() int {
					j := a.ActiveJob()
					if j == nil {
						return 0
					}
					if p := j.Percent(); p >= 0 {
						return p
					}
					if j.Running() {
						return 50
					}
					return 100
				},
				Format: func(pct int) string {
					j := a.ActiveJob()
					if j == nil {
						return "idle"
					}
					_, total := j.TailLines(1)
					return fmt.Sprintf("%s · %s · %d lines", j.State(), j.Elapsed().Round(100*time.Millisecond), total)
				},
			}),
			mint.Value(mint.ValueOpts{
				Label: "Output file",
				Get: func() string {
					a.mu.Lock()
					out := a.lastOut
					a.mu.Unlock()
					if out == "" {
						return "(not saving to file)"
					}
					if info, err := os.Stat(out); err == nil {
						return fmt.Sprintf("%s (%d bytes)", out, info.Size())
					}
					return out + " (pending)"
				},
			}),
		)

		var actRows []mint.Row
		if row, ok := a.diffActionRow(); ok {
			actRows = append(actRows, row)
		}
		if row, ok := a.findingsActionRow(); ok {
			actRows = append(actRows, row)
		}
		actRows = append(actRows,
			mint.Action(mint.ActionOpts{
				Label: "Open full log viewer (search / horizontal scroll / save)…",
				Hint:  "log",
				OnActivate: func(c *mint.Ctx) {
					j := a.ActiveJob()
					if j == nil {
						c.ToastWarn("No job output yet")
						return
					}
					c.Push(mint.ViewLog(a.Spec.Name+" output", j.Lines))
				},
			}),
			mint.Action(mint.ActionOpts{
				Label: "Save captured output to file…",
				Hint:  "save",
				OnActivate: func(c *mint.Ctx) {
					j := a.ActiveJob()
					if j == nil {
						c.ToastWarn("No job output yet")
						return
					}
					def := a.ResolvedOutputPath()
					if def == "" {
						def = filepath.Join(a.ResultsDir, DefaultOutputFilename(a.Spec.Name, a.outputExt()))
					}
					dest, ok := c.Prompt("Save Output", "Destination file path", def)
					if !ok || strings.TrimSpace(dest) == "" {
						return
					}
					if err := mint.SaveLogLines(strings.TrimSpace(dest), j.Lines()); err != nil {
						c.ToastError("Save failed: " + err.Error())
						return
					}
					level, _ := a.DataLevel()
					a.protectFile(level, util.ExpandUser(strings.TrimSpace(dest)))
					a.giveBack(util.ExpandUser(strings.TrimSpace(dest)))
					c.Toast("Saved to " + dest)
				},
			}),
			mint.Action(mint.ActionOpts{
				Label:      "Encrypt the output (passphrase)…",
				Hint:       "encrypt",
				OnActivate: a.encryptOutput,
			}),
			mint.Action(mint.ActionOpts{
				Label: "Browse output directory…",
				Hint:  "files",
				OnActivate: func(c *mint.Ctx) {
					c.Push(a.DirectoryScreen())
				},
			}),
		)
		if job != nil && job.Running() {
			actRows = append(actRows, mint.Action(mint.ActionOpts{
				Label: "Stop running job",
				Hint:  "stop",
				OnActivate: func(c *mint.Ctx) {
					if j := a.ActiveJob(); j != nil {
						j.Stop()
						c.ToastWarn("Stopped job")
					}
				},
			}))
		}

		scr.Sections = []mint.Section{
			{Name: "Run Status", Rows: statusRows},
			{Name: "Actions", Rows: actRows},
			a.buildOutputDisplaySection("Captured Output"),
		}
	}
	s.Refresh(s)
	return s
}

// DirectoryScreen builds an interactive browser for ResultsDir so users can
// verify output files, switch directories, or open any saved file inside ViewLog.
func (a *ToolApp) DirectoryScreen() *mint.Screen {
	s := &mint.Screen{
		Icon:  "▤",
		Title: "Output Directory Browser",
	}
	s.Refresh = func(scr *mint.Screen) {
		entries, err := a.ListResultsDir()
		dirRows := []mint.Row{
			mint.Text(mint.TextOpts{
				ID:          "browser:dir",
				Label:       "Current directory",
				Placeholder: "/path/to/results",
				Get:         func() string { return a.ResultsDir },
				Set: func(v string) {
					if strings.TrimSpace(v) != "" {
						a.ResultsDir = strings.TrimSpace(v)
						_ = a.SaveSettings()
					}
				},
			}),
			mint.Action(mint.ActionOpts{
				Label: "Parent directory (..)",
				Hint:  "up",
				OnActivate: func(c *mint.Ctx) {
					a.ResultsDir = filepath.Dir(a.ResultsDir)
				},
			}),
			mint.Action(mint.ActionOpts{
				Label: "Quick filter & jump (100k+ entries)…",
				Hint:  "search",
				OnActivate: func(c *mint.Ctx) {
					if chosen, ok := browsePathPopup(c, "Browse Files", a.ResultsDir, "", false); ok && chosen != "" {
						info, err := os.Stat(chosen)
						if err == nil && info.IsDir() {
							a.ResultsDir = chosen
							return
						}
						a.ResultsDir = filepath.Dir(chosen)
						b, rerr := os.ReadFile(chosen)
						if rerr != nil {
							c.ToastError("Cannot read: " + rerr.Error())
							return
						}
						if secure.IsSealed(b) {
							openSealed(c, filepath.Base(chosen), b)
							return
						}
						raw := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
						lines := make([]mint.LogLine, len(raw))
						for i, l := range raw {
							lines[i] = mint.LogLine{Text: mint.CleanLine(l)}
						}
						c.Push(mint.ViewLog(filepath.Base(chosen), func() []mint.LogLine { return lines }))
					}
				},
			}),
		}

		var fileRows []mint.Row
		if err != nil {
			fileRows = append(fileRows, mint.Note("  Cannot read directory: "+err.Error()))
		} else if len(entries) == 0 {
			fileRows = append(fileRows, mint.Note("  No files in "+a.ResultsDir))
		} else {
			limit := len(entries)
			if limit > 200 {
				limit = 200
			}
			for _, e := range entries[:limit] {
				entry := e
				if entry.IsDir {
					fileRows = append(fileRows, mint.Action(mint.ActionOpts{
						Label: "▸ " + entry.Name + "/",
						Hint:  "dir",
						OnActivate: func(c *mint.Ctx) {
							a.ResultsDir = entry.Path
						},
					}))
				} else {
					fileRows = append(fileRows, mint.Action(mint.ActionOpts{
						Label: entry.Name,
						Hint:  fmt.Sprintf("%d B", entry.Size),
						OnActivate: func(c *mint.Ctx) {
							b, err := os.ReadFile(entry.Path)
							if err != nil {
								c.ToastError("Cannot read: " + err.Error())
								return
							}
							if secure.IsSealed(b) {
								openSealed(c, entry.Name, b)
								return
							}
							raw := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
							lines := make([]mint.LogLine, len(raw))
							for i, l := range raw {
								lines[i] = mint.LogLine{Text: mint.CleanLine(l)}
							}
							c.Push(mint.ViewLog(entry.Name, func() []mint.LogLine { return lines }))
						},
					}))
				}
			}
		}

		scr.Sections = []mint.Section{
			{Name: "Directory", Rows: dirRows},
			{Name: fmt.Sprintf("Files (%d)", len(entries)), Rows: fileRows},
		}
	}
	s.Refresh(s)
	return s
}

// HistoryScreen displays past runs recorded for this tool.
func (a *ToolApp) HistoryScreen() *mint.Screen {
	s := &mint.Screen{
		Icon:  "◷",
		Title: "Run History",
	}
	s.Refresh = func(scr *mint.Screen) {
		hist := a.History()
		var rows []mint.Row
		if len(hist) == 0 {
			rows = append(rows, mint.Note("  No runs recorded yet."))
		} else {
			for _, rec := range hist {
				r := rec
				rows = append(rows, mint.Action(mint.ActionOpts{
					Label: fmt.Sprintf("[%s] %s", r.Time.Format("15:04:05"), r.CmdLine),
					Hint:  fmt.Sprintf("%s · %d lines", r.State, r.LineCount),
					OnActivate: func(c *mint.Ctx) {
						if r.OutputFile != "" {
							if b, err := os.ReadFile(r.OutputFile); err == nil {
								raw := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
								lines := make([]mint.LogLine, len(raw))
								for i, l := range raw {
									lines[i] = mint.LogLine{Text: mint.CleanLine(l)}
								}
								c.Push(mint.ViewLog(filepath.Base(r.OutputFile), func() []mint.LogLine { return lines }))
								return
							}
						}
						c.Message("Run Details", fmt.Sprintf("Command: %s\nState: %s\nLines: %d", r.CmdLine, r.State, r.LineCount))
					},
				}))
			}
		}
		scr.Sections = []mint.Section{{Name: "Recent Runs", Rows: rows}}
	}
	s.Refresh(s)
	return s
}

// BuildApp returns a configured *mint.App ready for Run().
func (a *ToolApp) BuildApp() *mint.App {
	primaryLabel := "Configure & Run"
	primaryIcon := "▸"
	switch a.Spec.Archetype {
	case ArchetypeConfig:
		primaryLabel = "State & Configuration"
		primaryIcon = "⚙"
	case ArchetypeScanner:
		primaryLabel = "Target & Live Scanner"
		primaryIcon = "◎"
	case ArchetypeMonitor:
		primaryLabel = "Live Monitor & Control"
		primaryIcon = "◧"
	case ArchetypeExplorer:
		primaryLabel = "Search & Query Explorer"
		primaryIcon = "⌕"
	case ArchetypeSuite:
		primaryLabel = "Subcommand Suite"
		primaryIcon = "❖"
	}

	items := []mint.HomeItem{
		{
			Icon:  primaryIcon,
			Label: primaryLabel,
			Desc:  a.Spec.Desc,
			Open:  a.MainScreen,
		},
		{
			Icon:  "◧",
			Label: "Live Run & Output",
			Desc:  "Monitor active job, search output, or save log",
			Open:  a.RunScreen,
		},
	}

	if a.Spec.OutputsFile {
		items = append(items, mint.HomeItem{
			Icon:  "▤",
			Label: "Output Directory",
			Desc:  "Browse saved output files and configure results folder",
			Open:  a.DirectoryScreen,
		})
	}

	items = append(items, mint.HomeItem{
		Icon:  "◷",
		Label: "Run History",
		Desc:  "Inspect previous commands and saved output files",
		Open:  a.HistoryScreen,
	})

	return &mint.App{
		Title:          a.Spec.Title,
		Crumb:          a.Spec.Name,
		Require:        []string{a.Spec.BinaryName()},
		Version:        a.Spec.Version,
		SectionSpacing: 1,
		Items:          items,
	}
}

// MintApp is an alias for BuildApp.
func (a *ToolApp) MintApp() *mint.App {
	return a.BuildApp()
}

// Run creates a ToolApp from spec and runs the interactive Mint interface.
func Run(spec ToolSpec) {
	app := New(spec)
	app.BuildApp().Run()
	app.Close() // wipe every secret
	runner.StopAll()
	util.StopAll() // status commands still running in the background
}
