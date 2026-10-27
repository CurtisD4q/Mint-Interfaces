package appkit

import (
	"sort"
	"strings"
	"time"
)

// The 35 archetypes say what kind of program a tool is: what it takes in,
// how it runs, and what it gives back. Each is a definition with guide
// fields, and its interface preferences are weights from 0 to 1, so that a
// design style can later be blended in by weight (see Blueprint). There are
// no sub-archetypes: a program is exactly one of these.
//
// The 6 base layouts (Archetype: config, runner, scanner, monitor,
// explorer, suite) are what an app is built from; an archetype prefers one
// or more of them.

// ArchetypeID names one of the 35 archetypes: "probe", "look-up"…
type ArchetypeID string

// Feature is one thing an interface can have: a table of results, a live
// log, a preview of file changes… Archetypes want each by a weight.
type Feature string

const (
	FeatTable          Feature = "table"           // results as a table (WithTableOutput)
	FeatCollapsible    Feature = "collapsible"     // long output in folding chunks (WithCollapsibleOutput)
	FeatFileOutput     Feature = "file-output"     // save the result to a file (WithFileOutput)
	FeatProfiles       Feature = "profiles"        // named presets of options (WithProfiles)
	FeatDynamicChoices Feature = "dynamic-choices" // choices read from the system (WithDynamicChoices)
	FeatRoot           Feature = "root"            // may need root (WithRootAuth)
	FeatConfirm        Feature = "confirm"         // ask before running (WithConfirm)
	FeatHistory        Feature = "history"         // keep past runs (WithHistory)
	FeatDiff           Feature = "diff"            // two sides in columns (WithDiff)
	FeatFindings       Feature = "findings"        // findings by severity (WithFindings)
	FeatPreview        Feature = "preview"         // what will change, before it changes (WithPreview)
	FeatFollow         Feature = "follow"          // a live log, newest at the bottom (WithFollow)
	FeatService        Feature = "service"         // start, stop, and where it listens (WithService)
	FeatTranscript     Feature = "transcript"      // a command box and transcript (WithTranscript)
	FeatReorder        Feature = "reorder"         // move rules, review, apply (WithReorder)
	FeatLiveRefresh    Feature = "live-refresh"    // refresh on an interval
	FeatProgress       Feature = "progress"        // a long job with progress
	FeatTarget         Feature = "target"          // a main target box (a host, file, expression…)
	FeatTwoInputs      Feature = "two-inputs"      // two inputs side by side (left, right)
	FeatRowActions     Feature = "row-actions"     // act on one row of a table
)

// AllFeatures lists the features in a fixed order.
func AllFeatures() []Feature {
	return []Feature{FeatTable, FeatCollapsible, FeatFileOutput, FeatProfiles, FeatDynamicChoices, FeatRoot,
		FeatConfirm, FeatHistory, FeatDiff, FeatFindings, FeatPreview, FeatFollow, FeatService, FeatTranscript,
		FeatReorder, FeatLiveRefresh, FeatProgress, FeatTarget, FeatTwoInputs, FeatRowActions}
}

// Preferences are weights from 0 (not wanted) to 1 (essential).
type Preferences struct {
	Layouts  map[Archetype]float64 `json:"layouts"`
	Features map[Feature]float64   `json:"features"`
}

// ArchetypeDef is one archetype.
type ArchetypeDef struct {
	ID    ArchetypeID `json:"id"`
	Name  string      `json:"name"`
	Group string      `json:"group"` // what it does and how it runs: "Answer once", "Observe over time"…

	Is    string `json:"is"`    // what kind of program it is, in one sentence
	Takes string `json:"takes"` // its main input
	Runs  string `json:"runs"`  // how it runs: once, as a job, until stopped…
	Shows string `json:"shows"` // how its results are shown

	Prefer Preferences `json:"prefer"`

	TargetLabel       string        `json:"target_label,omitempty"`
	TargetPlaceholder string        `json:"target_placeholder,omitempty"`
	PollInterval      time.Duration `json:"poll_interval,omitempty"`

	// Data is how sensitive this kind of program's results usually are
	// ("", DataSensitive or DataSecret), for programs not classified on
	// their own (see ClassifyData).
	Data       string `json:"data,omitempty"`
	DataReason string `json:"data_reason,omitempty"`
}

// The groups, in order.
const (
	GroupAnswer   = "Answer once"
	GroupObserve  = "Observe over time"
	GroupTargets  = "Act on targets"
	GroupMake     = "Make something"
	GroupChange   = "Change things"
	GroupManage   = "Manage a collection"
	GroupRunOther = "Run other things"
	GroupOther    = "Anything else"
)

// lay and feat build preference maps tersely.
func lay(pairs ...any) map[Archetype]float64 {
	m := map[Archetype]float64{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(Archetype)] = pairs[i+1].(float64)
	}
	return m
}

func feat(pairs ...any) map[Feature]float64 {
	m := map[Feature]float64{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(Feature)] = pairs[i+1].(float64)
	}
	return m
}

var archetypeRegistry = []ArchetypeDef{
	// ---- Answer once: it reports, quickly ----
	{ID: "ask", Name: "Ask", Group: GroupAnswer,
		Is: "Answers a question or works out an expression.", Takes: "One expression or question.",
		Runs: "Once, at once.", Shows: "A single answer, kept in a history.",
		Prefer:      Preferences{lay(ArchetypeExplorer, 1.0, ArchetypeRunner, 0.4), feat(FeatTarget, 1.0, FeatHistory, 0.8, FeatCollapsible, 0.2)},
		TargetLabel: "Question or expression", TargetPlaceholder: "e.g. 2^10, now, 3 miles in km"},
	{ID: "look-up", Name: "Look up", Group: GroupAnswer,
		Is: "Looks a name up somewhere else and shows its records.", Takes: "A name, address or key, and what kind of record.",
		Runs: "Once.", Shows: "The records as a table.",
		Prefer:      Preferences{lay(ArchetypeExplorer, 1.0, ArchetypeScanner, 0.3), feat(FeatTarget, 1.0, FeatTable, 0.8, FeatHistory, 0.6, FeatProfiles, 0.3)},
		TargetLabel: "Name to look up", TargetPlaceholder: "e.g. example.com",
		Data: DataSensitive, DataReason: "lookups can gather information about people and organisations"},
	{ID: "inspect", Name: "Inspect", Group: GroupAnswer,
		Is: "Describes one item in detail.", Takes: "One file, device or object.",
		Runs: "Once.", Shows: "A report in folding sections.",
		Prefer:      Preferences{lay(ArchetypeExplorer, 1.0, ArchetypeRunner, 0.3), feat(FeatTarget, 1.0, FeatCollapsible, 0.9, FeatTable, 0.4, FeatFileOutput, 0.3)},
		TargetLabel: "Item to inspect", TargetPlaceholder: "e.g. /usr/bin/ls"},
	{ID: "list", Name: "List", Group: GroupAnswer,
		Is: "Lists the items of a collection.", Takes: "Optionally a filter and the columns to show.",
		Runs: "Once, refreshed on demand.", Shows: "A sortable table; a row opens its detail.",
		Prefer: Preferences{lay(ArchetypeExplorer, 1.0, ArchetypeMonitor, 0.4), feat(FeatTable, 1.0, FeatDynamicChoices, 0.5, FeatRowActions, 0.3, FeatFileOutput, 0.3)}},
	{ID: "search", Name: "Search", Group: GroupAnswer,
		Is: "Finds what matches a pattern.", Takes: "A pattern and where to look.",
		Runs: "As a job that can take a while.", Shows: "The matches; each opens where it was found.",
		Prefer:      Preferences{lay(ArchetypeExplorer, 1.0), feat(FeatTarget, 1.0, FeatTable, 0.7, FeatCollapsible, 0.6, FeatHistory, 0.5, FeatProgress, 0.4, FeatFileOutput, 0.3)},
		TargetLabel: "Pattern", TargetPlaceholder: "e.g. TODO"},
	{ID: "query", Name: "Query", Group: GroupAnswer,
		Is: "Runs a query language over data.", Takes: "An expression or program, and the data.",
		Runs: "Once.", Shows: "The result as a table or text.",
		Prefer:      Preferences{lay(ArchetypeExplorer, 1.0, ArchetypeRunner, 0.3), feat(FeatTarget, 1.0, FeatTable, 0.7, FeatProfiles, 0.6, FeatHistory, 0.6, FeatFileOutput, 0.5)},
		TargetLabel: "Expression", TargetPlaceholder: "e.g. .items[] | .name"},
	{ID: "compare", Name: "Compare", Group: GroupAnswer,
		Is: "Shows the differences between inputs.", Takes: "Two inputs: left and right.",
		Runs: "Once.", Shows: "The two sides in columns, change by change.",
		Prefer: Preferences{lay(ArchetypeExplorer, 1.0), feat(FeatTwoInputs, 1.0, FeatDiff, 1.0, FeatCollapsible, 0.4, FeatFileOutput, 0.3)}},
	{ID: "read", Name: "Read", Group: GroupAnswer,
		Is: "Browses documents or manuals.", Takes: "A topic, page or section.",
		Runs: "Once.", Shows: "A page to read and search.",
		Prefer:      Preferences{lay(ArchetypeExplorer, 1.0), feat(FeatTarget, 1.0, FeatCollapsible, 0.8, FeatHistory, 0.7)},
		TargetLabel: "Topic or page", TargetPlaceholder: "e.g. tar"},

	// ---- Observe over time ----
	{ID: "watch", Name: "Watch", Group: GroupObserve,
		Is: "Samples values again and again.", Takes: "What to sample, and how often.",
		Runs: "Repeats until stopped.", Shows: "Live numbers and a table.",
		Prefer:       Preferences{lay(ArchetypeMonitor, 1.0), feat(FeatLiveRefresh, 1.0, FeatTable, 0.8, FeatHistory, 0.3)},
		PollInterval: 2 * time.Second},
	{ID: "follow", Name: "Follow", Group: GroupObserve,
		Is: "Shows events as they happen.", Takes: "A source and a filter.",
		Runs: "Until stopped.", Shows: "A growing log, newest at the bottom.",
		Prefer: Preferences{lay(ArchetypeMonitor, 1.0), feat(FeatFollow, 1.0, FeatCollapsible, 0.5, FeatFileOutput, 0.3)},
		Data:   DataSensitive, DataReason: "logs and events can include user names, addresses and sometimes passwords"},
	{ID: "record", Name: "Record", Group: GroupObserve,
		Is: "Records a live source to a file.", Takes: "A source (interface, device, screen) and a file.",
		Runs: "Until stopped or for a set time.", Shows: "How long and how much so far; the file at the end.",
		Prefer: Preferences{lay(ArchetypeScanner, 0.7, ArchetypeRunner, 0.6), feat(FeatFileOutput, 1.0, FeatProgress, 0.8, FeatRoot, 0.5, FeatDynamicChoices, 0.5, FeatTable, 0.3)},
		Data:   DataSensitive, DataReason: "recordings can capture passwords, private messages or what is on screen"},

	// ---- Act on targets ----
	{ID: "probe", Name: "Probe", Group: GroupTargets,
		Is: "Checks targets and gives one result for each.", Takes: "A list of hosts or targets, and options.",
		Runs: "As a job.", Shows: "One row per target, filling in live.",
		Prefer:      Preferences{lay(ArchetypeScanner, 1.0), feat(FeatTarget, 1.0, FeatTable, 1.0, FeatProfiles, 0.7, FeatFileOutput, 0.6, FeatProgress, 0.5, FeatRoot, 0.5, FeatHistory, 0.5)},
		TargetLabel: "Targets", TargetPlaceholder: "e.g. 192.168.1.0/24"},
	{ID: "audit", Name: "Audit", Group: GroupTargets,
		Is: "Runs checks and lists findings.", Takes: "Something to check (a path, a system) and a rule level.",
		Runs: "As a job.", Shows: "Findings grouped by severity; each opens its detail.",
		Prefer:      Preferences{lay(ArchetypeRunner, 1.0, ArchetypeScanner, 0.4), feat(FeatFindings, 1.0, FeatTarget, 0.7, FeatTable, 0.6, FeatCollapsible, 0.6, FeatFileOutput, 0.5, FeatHistory, 0.5)},
		TargetLabel: "What to check", TargetPlaceholder: "e.g. ./src",
		Data: DataSensitive, DataReason: "its findings describe weaknesses of this computer or a target"},
	{ID: "measure", Name: "Measure", Group: GroupTargets,
		Is: "Runs a workload and reports numbers.", Takes: "What to measure, how many runs or how long.",
		Runs: "As a job.", Shows: "A table of statistics, compared with earlier runs.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatTable, 0.9, FeatHistory, 0.8, FeatProgress, 0.6, FeatProfiles, 0.5, FeatFileOutput, 0.4)}},

	// ---- Make something ----
	{ID: "convert", Name: "Convert", Group: GroupMake,
		Is: "Turns an input into an output of another form.", Takes: "An input file, an output file, and a format.",
		Runs: "As a job with progress.", Shows: "Progress, then the output file.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatFileOutput, 1.0, FeatProgress, 0.7, FeatProfiles, 0.6, FeatCollapsible, 0.3)}},
	{ID: "filter", Name: "Filter", Group: GroupMake,
		Is: "Transforms text: input in, output out.", Takes: "Text or a file, and options.",
		Runs: "Once.", Shows: "The output text.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0, ArchetypeExplorer, 0.3), feat(FeatCollapsible, 0.7, FeatFileOutput, 0.6, FeatProfiles, 0.3)}},
	{ID: "pack", Name: "Pack", Group: GroupMake,
		Is: "Packs many files into one, or unpacks them.", Takes: "Create, extract or list, and the files.",
		Runs: "As a job.", Shows: "Progress; a listing as a table.",
		Prefer: Preferences{lay(ArchetypeSuite, 1.0, ArchetypeRunner, 0.5), feat(FeatFileOutput, 0.8, FeatTable, 0.5, FeatConfirm, 0.5, FeatProgress, 0.5)}},
	{ID: "generate", Name: "Generate", Group: GroupMake,
		Is: "Makes something new from settings alone.", Takes: "Settings: length, count, style.",
		Runs: "Once.", Shows: "The result, ready to copy or save.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatFileOutput, 0.7, FeatHistory, 0.6, FeatProfiles, 0.4)}},
	{ID: "build", Name: "Build", Group: GroupMake,
		Is: "Builds or tests a project.", Takes: "A project folder and a target.",
		Runs: "As a job.", Shows: "The log, then pass or fail.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatCollapsible, 0.9, FeatHistory, 0.7, FeatProgress, 0.6, FeatProfiles, 0.6)}},
	{ID: "transfer", Name: "Transfer", Group: GroupMake,
		Is: "Fetches or sends something over the network.", Takes: "A URL or remote, and where it goes.",
		Runs: "As a job with progress.", Shows: "Progress and speed, then the saved file.",
		Prefer:      Preferences{lay(ArchetypeRunner, 1.0), feat(FeatTarget, 1.0, FeatFileOutput, 0.9, FeatProgress, 0.8, FeatProfiles, 0.5, FeatHistory, 0.4)},
		TargetLabel: "URL or remote", TargetPlaceholder: "e.g. https://example.com/file.iso"},
	{ID: "present", Name: "Present", Group: GroupMake,
		Is: "Shows, plays, prints or announces something; that is its effect.", Takes: "What to present, and on which device.",
		Runs: "Once, or until it finishes.", Shows: "Only whether it worked.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatTarget, 0.6, FeatDynamicChoices, 0.5)}},

	// ---- Change things ----
	{ID: "edit-files", Name: "Edit files", Group: GroupChange,
		Is: "Changes many files in place: copies, moves, renames, removes.", Takes: "The files, and what to do.",
		Runs: "Once.", Shows: "What will change first, then what changed.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatPreview, 1.0, FeatConfirm, 0.9, FeatTable, 0.4)}},
	{ID: "mirror", Name: "Mirror", Group: GroupChange,
		Is: "Makes a destination match a source.", Takes: "A source, a destination, and rules.",
		Runs: "As a job.", Shows: "What will change, then progress.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatPreview, 0.9, FeatConfirm, 0.8, FeatProgress, 0.7, FeatProfiles, 0.6)}},
	{ID: "settings", Name: "Settings", Group: GroupChange,
		Is: "Reads and sets named values.", Takes: "The current values.",
		Runs: "Once.", Shows: "A table of keys and values to edit and apply.",
		Prefer:      Preferences{lay(ArchetypeConfig, 1.0), feat(FeatTable, 0.9, FeatRowActions, 0.5, FeatRoot, 0.4, FeatConfirm, 0.3)},
		TargetLabel: "Key", TargetPlaceholder: "e.g. net.ipv4.ip_forward"},
	{ID: "device", Name: "Device", Group: GroupChange,
		Is: "Adjusts a device or session live.", Takes: "The device's current state.",
		Runs: "Once.", Shows: "A control for each setting; apply.",
		Prefer: Preferences{lay(ArchetypeConfig, 1.0), feat(FeatDynamicChoices, 0.9, FeatRoot, 0.4, FeatTable, 0.3)}},
	{ID: "rules", Name: "Rules", Group: GroupChange,
		Is: "Edits an ordered list of rules or entries.", Takes: "The current rules.",
		Runs: "Once.", Shows: "The rules in order: add, move, delete, then apply.",
		Prefer: Preferences{lay(ArchetypeConfig, 1.0), feat(FeatReorder, 1.0, FeatTable, 0.9, FeatConfirm, 0.8, FeatRoot, 0.8, FeatRowActions, 0.6)}},
	{ID: "control", Name: "Control", Group: GroupChange,
		Is: "Lists running things and acts on each.", Takes: "The running items.",
		Runs: "Refreshes.", Shows: "A table of items with actions for each.",
		Prefer:       Preferences{lay(ArchetypeMonitor, 1.0), feat(FeatTable, 1.0, FeatRowActions, 1.0, FeatConfirm, 0.7, FeatLiveRefresh, 0.6, FeatRoot, 0.6)},
		PollInterval: 3 * time.Second},

	// ---- Manage a collection ----
	{ID: "records", Name: "Records", Group: GroupManage,
		Is: "Adds, lists, changes and removes items in a store.", Takes: "The store.",
		Runs: "Once per action.", Shows: "A table of items to add to, edit and remove from.",
		Prefer: Preferences{lay(ArchetypeSuite, 1.0, ArchetypeExplorer, 0.4), feat(FeatTable, 1.0, FeatRowActions, 0.8, FeatConfirm, 0.6)}},
	{ID: "history", Name: "History", Group: GroupManage,
		Is: "Keeps versions or snapshots of something.", Takes: "A repository.",
		Runs: "Once per action.", Shows: "The log or snapshots, with actions on each.",
		Prefer: Preferences{lay(ArchetypeSuite, 1.0), feat(FeatTable, 0.9, FeatRowActions, 0.7, FeatConfirm, 0.6, FeatHistory, 0.5, FeatCollapsible, 0.4)}},
	{ID: "toolkit", Name: "Toolkit", Group: GroupManage,
		Is: "Many unrelated commands under one name.", Takes: "A command, then its own options.",
		Runs: "Depends on the command.", Shows: "Depends on the command.",
		Prefer: Preferences{lay(ArchetypeSuite, 1.0), feat(FeatCollapsible, 0.6, FeatFileOutput, 0.4, FeatHistory, 0.4)}},

	// ---- Run other things ----
	{ID: "wrap", Name: "Wrap", Group: GroupRunOther,
		Is: "Runs another command with changes: a limit, a priority, a timer.", Takes: "The command to run, and the changes.",
		Runs: "As long as the command runs.", Shows: "The command's own output.",
		Prefer:      Preferences{lay(ArchetypeRunner, 1.0), feat(FeatTarget, 1.0, FeatCollapsible, 0.8, FeatConfirm, 0.4)},
		TargetLabel: "Command to run", TargetPlaceholder: "e.g. make -j4"},
	{ID: "schedule", Name: "Schedule", Group: GroupRunOther,
		Is: "Runs something later or on a timetable.", Takes: "When, and what.",
		Runs: "Once per action.", Shows: "The scheduled jobs, each removable.",
		Prefer: Preferences{lay(ArchetypeSuite, 1.0), feat(FeatTable, 0.9, FeatRowActions, 0.7, FeatConfirm, 0.7)}},
	{ID: "serve", Name: "Serve", Group: GroupRunOther,
		Is: "Runs as a service until stopped.", Takes: "A port, a folder, and options.",
		Runs: "Until stopped.", Shows: "Running or not, the address it listens on, and its log.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0, ArchetypeMonitor, 0.4), feat(FeatService, 1.0, FeatCollapsible, 0.6)}},
	{ID: "session", Name: "Session", Group: GroupRunOther,
		Is: "An interactive prompt, driven one command at a time.", Takes: "Commands, or a script.",
		Runs: "One command at a time.", Shows: "A transcript of commands and what they returned.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatTranscript, 1.0, FeatHistory, 0.6, FeatCollapsible, 0.4)}},

	// ---- Anything else ----
	{ID: "general", Name: "General", Group: GroupOther,
		Is: "Anything else: a command and its output.", Takes: "Arguments and options.",
		Runs: "Once.", Shows: "The output.",
		Prefer: Preferences{lay(ArchetypeRunner, 1.0), feat(FeatCollapsible, 0.6, FeatFileOutput, 0.4, FeatHistory, 0.3)}},
}

// AllArchetypeDefs returns the 35 archetypes in order.
func AllArchetypeDefs() []ArchetypeDef {
	return append([]ArchetypeDef(nil), archetypeRegistry...)
}

// LookupArchetype finds an archetype by ID ("look-up") or name ("Look up"),
// ignoring case, spaces and dashes.
func LookupArchetype(idOrName string) (ArchetypeDef, bool) {
	key := archetypeKey(idOrName)
	for _, a := range archetypeRegistry {
		if archetypeKey(string(a.ID)) == key || archetypeKey(a.Name) == key {
			return a, true
		}
	}
	return ArchetypeDef{}, false
}

func archetypeKey(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(s)))
}

// wantThreshold is the weight at which a feature is switched on today, with
// no design style blended in.
const wantThreshold = 0.5

// Blueprint is the concrete plan an app is built from: a base layout and
// the features switched on. Today it is the archetype's own preferences;
// a design style will later be blended in by weight.
type Blueprint struct {
	Layout   Archetype
	Features []Feature // in AllFeatures order
}

// Has reports whether the blueprint switches f on.
func (b Blueprint) Has(f Feature) bool {
	for _, x := range b.Features {
		if x == f {
			return true
		}
	}
	return false
}

// BlendPreferences mixes two sets of preferences: weight w of a, 1-w of b.
func BlendPreferences(a, b Preferences, w float64) Preferences {
	out := Preferences{Layouts: map[Archetype]float64{}, Features: map[Feature]float64{}}
	for _, l := range AllArchetypes() {
		if v := w*a.Layouts[l] + (1-w)*b.Layouts[l]; v > 0 {
			out.Layouts[l] = v
		}
	}
	for _, f := range AllFeatures() {
		if v := w*a.Features[f] + (1-w)*b.Features[f]; v > 0 {
			out.Features[f] = v
		}
	}
	return out
}

// BlueprintFrom turns preferences into a blueprint: the most wanted base
// layout (the first in AllArchetypes order on a tie) and every feature
// wanted at wantThreshold or more.
func BlueprintFrom(p Preferences) Blueprint {
	var b Blueprint
	best := -1.0
	for _, l := range AllArchetypes() {
		if v := p.Layouts[l]; v > best {
			b.Layout, best = l, v
		}
	}
	for _, f := range AllFeatures() {
		if p.Features[f] >= wantThreshold {
			b.Features = append(b.Features, f)
		}
	}
	return b
}

// Blueprint is the archetype's blueprint, from its own preferences.
func (a ArchetypeDef) Blueprint() Blueprint { return BlueprintFrom(a.Prefer) }

// ApplyTo sets spec up as this archetype, without overwriting what the
// caller has already set. The add-ons that need a program's own details
// (WithDiff, WithFollow…) are attached by whoever knows the program
// (appkit.Known…Spec); the blueprint only says they are wanted.
func (a ArchetypeDef) ApplyTo(spec *ToolSpec) {
	if spec == nil {
		return
	}
	b := a.Blueprint()
	spec.ArchetypeID = string(a.ID)
	if spec.Archetype == "" {
		spec.Archetype = b.Layout
	}
	if spec.TargetLabel == "" && a.TargetLabel != "" {
		spec.TargetLabel = a.TargetLabel
	}
	if spec.TargetPlaceholder == "" && a.TargetPlaceholder != "" {
		spec.TargetPlaceholder = a.TargetPlaceholder
	}
	if spec.PollInterval <= 0 && a.PollInterval > 0 {
		spec.PollInterval = a.PollInterval
	}
	if b.Has(FeatFileOutput) {
		spec.OutputsFile = true
	}
	if b.Has(FeatTable) {
		spec.TableOutput = true
	}
	if b.Has(FeatCollapsible) {
		spec.CollapsibleOutput = true
		if spec.ChunkSize <= 0 {
			spec.ChunkSize = 100
		}
	}
	if b.Has(FeatRoot) {
		spec.NeedsRoot = true
	}
	if b.Has(FeatConfirm) {
		spec.ConfirmRun = true
	}
	if b.Has(FeatHistory) {
		spec.KeepHistory = true
	}
	if b.Has(FeatProfiles) && len(spec.Profiles) == 0 {
		spec.Profiles = defaultProfilesForArchetype(a, spec.Flags)
	}
}

func defaultProfilesForArchetype(a ArchetypeDef, flags []FlagSpec) []ProfileSpec {
	base := map[string]string{}
	for _, f := range flags {
		if f.Kind == FlagBool {
			base[f.Flag] = "false"
		}
	}
	if a.Prefer.Features[FeatProgress] >= wantThreshold || a.Prefer.Features[FeatProfiles] >= 0.6 {
		return []ProfileSpec{
			{Name: "Default", Desc: "Standard invocation", Flags: base},
			{Name: "Fast / Quiet", Desc: "Minimal output", Flags: base},
			{Name: "Verbose", Desc: "Detailed diagnostic output", Flags: base},
		}
	}
	return []ProfileSpec{{Name: "Default", Flags: base}}
}

// ArchetypeForTool finds a program on the archetype lists (see
// archetypeTools): an exact answer, checked by a person.
func ArchetypeForTool(bin string) (ArchetypeDef, bool) {
	id, ok := archetypeTools[toolBase(bin)]
	if !ok {
		return ArchetypeDef{}, false
	}
	return LookupArchetype(string(id))
}

// ArchetypeToolNames lists the programs on the archetype lists, sorted.
func ArchetypeToolNames() []string {
	var out []string
	for n := range archetypeTools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// templateArchetype is the archetype an old template (T01..T28) stands
// for, so --template T12 and apps generated with a template keep working.
var templateArchetype = map[TemplateID]ArchetypeID{
	T01: "settings", T02: "settings", T03: "device", T04: "rules",
	T05: "edit-files", T06: "convert", T07: "transfer", T08: "pack",
	T09: "history", T10: "build", T11: "measure", T12: "probe",
	T13: "record", T14: "look-up", T15: "audit", T16: "convert",
	T17: "watch", T18: "control", T19: "list", T20: "follow",
	T21: "history", T22: "toolkit", T23: "records", T24: "list",
	T25: "search", T26: "query", T27: "read", T28: "schedule",
}

// ArchetypeForTemplate is the archetype an old template ID or slug stands for.
func ArchetypeForTemplate(idOrSlug string) (ArchetypeDef, bool) {
	tpl, ok := LookupTemplate(idOrSlug)
	if !ok {
		return ArchetypeDef{}, false
	}
	return LookupArchetype(string(templateArchetype[tpl.ID]))
}
