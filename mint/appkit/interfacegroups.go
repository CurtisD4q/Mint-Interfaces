package appkit

// Interface groups: an interface decided from how a program behaves, not
// from a label. mint-gen answers ten yes/no questions about a program from
// its own help (Answers); the answers place it in one of 22 groups by rule
// (PlaceInterface); and the answers, with the group's typical answers
// where the evidence is weak, switch features on (ApplyInterface).
//
// The groups come from a study of 1,076 command-line programs: a tree that
// splits on whichever answer removes the most disagreement, stopped at 22
// groups by cross-validation (most of the screen benefit, every group at
// least 14 programs). They are provisional until the study's answers have
// been checked by hand.

import "sort"

// Question is one yes/no question about how a program behaves.
type Question string

const (
	QKeepsRunning  Question = "keeps_running" // runs until stopped: refreshes, follows, serves or listens
	QChangesThings Question = "changes_state" // changes settings, the system, or something outside itself
	QTarget        Question = "takes_target"  // takes a file, host, name or other argument to work on
	QTwoInputs     Question = "two_inputs"    // needs two inputs, like a diff of two files
	QSubcommands   Question = "subcommands"   // has commands of its own (git commit, apt install)
	QWritesFile    Question = "writes_file"   // saves its result to a file
	QTable         Question = "table_output"  // prints rows and columns
	QNeedsRoot     Question = "needs_root"    // needs root for its main job
	QCanDestroy    Question = "destructive"   // can delete, overwrite or kill something
	QFullScreen    Question = "full_screen"   // draws its own full-screen terminal interface
)

// Questions are the ten questions, in order.
var Questions = []Question{QKeepsRunning, QChangesThings, QTarget, QTwoInputs, QSubcommands, QWritesFile, QTable, QNeedsRoot, QCanDestroy, QFullScreen}

// How sure an answer is.
const (
	SureStrong = "strong" // the help says so directly (an option, a usage line, the summary)
	SureWeak   = "weak"   // a hint, or no sign either way
	SureNone   = "none"   // nothing to go on (no usage line…)
)

// Answer is one question's answer, with how sure it is and why.
type Answer struct {
	Yes  bool   `json:"yes"`
	Sure string `json:"sure"`
	Why  string `json:"why,omitempty"`
}

// Answers are a program's answers to the questions.
type Answers map[Question]Answer

// InterfaceGroup is one of the 22 groups: the answers that place a program
// here (Rule), and what the other answers usually are (Typical).
type InterfaceGroup struct {
	ID      string
	Name    string
	Family  string // run once, live, changes things, live and changing, full-screen, commands
	Rule    map[Question]bool
	Typical map[Question]bool
}

// Families, from the questions that set how a screen runs.
const (
	FamilyOnce       = "Runs once"
	FamilyLive       = "Live"
	FamilyChanges    = "Changes things"
	FamilyLiveChange = "Live and changing"
	FamilyFullScreen = "Full-screen"
	FamilyCommands   = "Commands"
)

// interfaceGroups are the 22 groups, largest first in the study. Their
// rules divide every possible set of answers between them (see
// TestInterfaceGroupsCoverEveryAnswerSet).
var interfaceGroups = []InterfaceGroup{
	{ID: "once", Name: "Run once", Family: FamilyOnce,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: false, QTable: false, QCanDestroy: false, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QWritesFile: false, QNeedsRoot: false}},
	{ID: "once-target", Name: "Run once on a target", Family: FamilyOnce,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: true, QSubcommands: false, QWritesFile: false, QTable: false, QCanDestroy: false, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QNeedsRoot: false}},
	{ID: "live", Name: "Live view", Family: FamilyLive,
		Rule:    map[Question]bool{QKeepsRunning: true, QChangesThings: false, QTarget: false, QWritesFile: false, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QTable: false, QNeedsRoot: false, QCanDestroy: false}},
	{ID: "live-target", Name: "Live view of a target", Family: FamilyLive,
		Rule:    map[Question]bool{QKeepsRunning: true, QChangesThings: false, QTarget: true, QCanDestroy: false, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QWritesFile: false, QTable: false, QNeedsRoot: false}},
	{ID: "once-target-table", Name: "Table about a target", Family: FamilyOnce,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: true, QSubcommands: false, QWritesFile: false, QTable: true, QCanDestroy: false, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QNeedsRoot: false}},
	{ID: "once-table", Name: "Table", Family: FamilyOnce,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: false, QTable: true, QCanDestroy: false, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QWritesFile: false, QNeedsRoot: false}},
	{ID: "once-target-destroy", Name: "Risky run on a target", Family: FamilyOnce,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: true, QSubcommands: false, QCanDestroy: true, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QWritesFile: false, QTable: false, QNeedsRoot: false}},
	{ID: "screen", Name: "Full-screen app", Family: FamilyFullScreen,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: false, QFullScreen: true},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QCanDestroy: false}},
	{ID: "change-target", Name: "Change a target", Family: FamilyChanges,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: true, QTarget: true, QSubcommands: false, QCanDestroy: false},
		Typical: map[Question]bool{QTwoInputs: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QFullScreen: false}},
	{ID: "live-screen", Name: "Full-screen live view", Family: FamilyFullScreen,
		Rule:    map[Question]bool{QKeepsRunning: true, QChangesThings: false, QFullScreen: true},
		Typical: map[Question]bool{QTarget: false, QTwoInputs: false, QSubcommands: false, QWritesFile: false, QTable: true, QNeedsRoot: false, QCanDestroy: false}},
	{ID: "change", Name: "Change things", Family: FamilyChanges,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: true, QTarget: false, QSubcommands: false},
		Typical: map[Question]bool{QTwoInputs: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QCanDestroy: false, QFullScreen: false}},
	{ID: "live-change", Name: "Live and changing", Family: FamilyLiveChange,
		Rule:    map[Question]bool{QKeepsRunning: true, QChangesThings: true, QFullScreen: false},
		Typical: map[Question]bool{QTarget: true, QTwoInputs: false, QSubcommands: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QCanDestroy: false}},
	{ID: "commands-target", Name: "Commands on a target", Family: FamilyCommands,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: true, QSubcommands: true, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QCanDestroy: false}},
	{ID: "change-commands-target", Name: "Change things with commands", Family: FamilyChanges,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: true, QTarget: true, QSubcommands: true},
		Typical: map[Question]bool{QTwoInputs: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QCanDestroy: false, QFullScreen: false}},
	{ID: "once-destroy", Name: "Risky run", Family: FamilyOnce,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: false, QCanDestroy: true, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QWritesFile: false, QTable: false, QNeedsRoot: false}},
	{ID: "once-target-file", Name: "Make a file from a target", Family: FamilyOnce,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: true, QSubcommands: false, QWritesFile: true, QCanDestroy: false, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QTable: false, QNeedsRoot: false}},
	{ID: "screen-target", Name: "Full-screen app for a target", Family: FamilyFullScreen,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: false, QTarget: true, QFullScreen: true},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QCanDestroy: false}},
	{ID: "live-target-destroy", Name: "Risky long run on a target", Family: FamilyLive,
		Rule:    map[Question]bool{QKeepsRunning: true, QChangesThings: false, QTarget: true, QCanDestroy: true, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QWritesFile: false, QTable: false, QNeedsRoot: false}},
	{ID: "change-target-destroy", Name: "Risky change to a target", Family: FamilyChanges,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: true, QTarget: true, QSubcommands: false, QCanDestroy: true},
		Typical: map[Question]bool{QTwoInputs: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QFullScreen: false}},
	{ID: "live-screen-change", Name: "Full-screen manager", Family: FamilyFullScreen,
		Rule:    map[Question]bool{QKeepsRunning: true, QChangesThings: true, QFullScreen: true},
		Typical: map[Question]bool{QTarget: false, QTwoInputs: false, QSubcommands: true, QWritesFile: false, QTable: true, QNeedsRoot: false, QCanDestroy: false}},
	{ID: "change-commands", Name: "Commands that change things", Family: FamilyChanges,
		Rule:    map[Question]bool{QKeepsRunning: false, QChangesThings: true, QTarget: false, QSubcommands: true},
		Typical: map[Question]bool{QTwoInputs: false, QWritesFile: false, QTable: false, QNeedsRoot: false, QCanDestroy: false, QFullScreen: false}},
	{ID: "live-file", Name: "Long run saving a file", Family: FamilyLive,
		Rule:    map[Question]bool{QKeepsRunning: true, QChangesThings: false, QTarget: false, QWritesFile: true, QFullScreen: false},
		Typical: map[Question]bool{QTwoInputs: false, QSubcommands: false, QTable: false, QNeedsRoot: false, QCanDestroy: false}},
}

// InterfaceGroups returns the 22 groups.
func InterfaceGroups() []InterfaceGroup { return append([]InterfaceGroup(nil), interfaceGroups...) }

// LookupInterfaceGroup finds a group by ID.
func LookupInterfaceGroup(id string) (InterfaceGroup, bool) {
	for _, g := range interfaceGroups {
		if g.ID == id {
			return g, true
		}
	}
	return InterfaceGroup{}, false
}

// Matches reports whether yes (the answers, as plain yes/no) fits the
// group's rule.
func (g InterfaceGroup) Matches(yes map[Question]bool) bool {
	for q, want := range g.Rule {
		if yes[q] != want {
			return false
		}
	}
	return true
}

// Plain is the answers as plain yes/no.
func (a Answers) Plain() map[Question]bool {
	out := make(map[Question]bool, len(Questions))
	for _, q := range Questions {
		out[q] = a[q].Yes
	}
	return out
}

// PlaceInterface is the group the answers fall in. The rules divide every
// set of answers between the groups, so there is always exactly one.
func PlaceInterface(a Answers) InterfaceGroup {
	yes := a.Plain()
	for _, g := range interfaceGroups {
		if g.Matches(yes) {
			return g
		}
	}
	return interfaceGroups[0] // unreachable: see TestInterfaceGroupsCoverEveryAnswerSet
}

// Effective is the answers the interface is built from: the program's own
// answer wherever its help gave any sign (a yes, even a hint, or a clear
// no), and the group's typical answer only where the help said nothing.
// So a group can only add to what the help says, never take a yes away;
// in particular a yes to a caution question (changes things, can destroy,
// needs root) always stands.
func (a Answers) Effective(g InterfaceGroup) map[Question]bool {
	out := a.Plain()
	for q, typical := range g.Typical {
		if ans := a[q]; ans.Yes || ans.Sure == SureStrong {
			continue
		}
		out[q] = out[q] || typical
	}
	return out
}

// BlueprintFromAnswers turns effective answers into features.
//
//   - A program that keeps running gets a live refresh only when it
//     prints a table (a snapshot worth redrawing) and its purpose is not
//     to change or destroy things (destroysByPurpose); otherwise it is a
//     long run with progress. A risky option alone does not stop a
//     refresh: repeated runs never pass destructive options.
//   - Changing or destroying things asks before running.
//   - Every interface keeps past runs and folds long output.
func BlueprintFromAnswers(yes map[Question]bool, destroysByPurpose bool) Blueprint {
	on := map[Feature]bool{FeatCollapsible: true, FeatHistory: true}
	if yes[QTarget] {
		on[FeatTarget] = true
	}
	if yes[QTwoInputs] {
		on[FeatTwoInputs] = true
		on[FeatTarget] = true
	}
	if yes[QWritesFile] {
		on[FeatFileOutput] = true
	}
	if yes[QTable] {
		on[FeatTable] = true
	}
	if yes[QNeedsRoot] {
		on[FeatRoot] = true
	}
	risky := yes[QChangesThings] || yes[QCanDestroy]
	if risky {
		on[FeatConfirm] = true
	}
	if yes[QKeepsRunning] && !yes[QFullScreen] {
		if !yes[QChangesThings] && !destroysByPurpose && yes[QTable] {
			on[FeatLiveRefresh] = true
		} else {
			on[FeatProgress] = true
		}
	}
	var b Blueprint
	for _, f := range AllFeatures() {
		if on[f] {
			b.Features = append(b.Features, f)
		}
	}
	return b
}

// ApplyInterface sets spec up from a program's answers: it places the
// program in a group and switches on the features the answers call for.
// Like ArchetypeDef.ApplyTo it never overwrites what the caller set
// (an explicit target label, a known status command), and it only ever
// adds caution. It returns the group.
func ApplyInterface(spec *ToolSpec, a Answers) InterfaceGroup {
	g := PlaceInterface(a)
	if spec == nil {
		return g
	}
	yes := a.Effective(g)
	b := BlueprintFromAnswers(yes, a[QCanDestroy].Yes && a[QCanDestroy].Sure == SureStrong)
	// Reading state, then editing and applying, needs a way to read the
	// state: only then does a program that changes things get that screen.
	if yes[QChangesThings] && !yes[QKeepsRunning] && len(spec.StatusArgs) > 0 {
		b.Features = append(b.Features, FeatStateEdit)
		sort.SliceStable(b.Features, func(i, j int) bool { return featureIndex(b.Features[i]) < featureIndex(b.Features[j]) })
	}
	spec.InterfaceGroup = g.ID
	spec.Features = append([]Feature{}, b.Features...)
	spec.FullScreen = yes[QFullScreen]
	explicitTarget := spec.TargetLabel != ""
	if b.Has(FeatTwoInputs) {
		spec.TwoInputs = true
		if spec.SecondTargetLabel == "" {
			spec.SecondTargetLabel = "Right"
		}
		if spec.SecondTargetPlaceholder == "" {
			spec.SecondTargetPlaceholder = "second input"
		}
	}
	if !explicitTarget {
		spec.WantTarget = b.Has(FeatTarget)
		spec.HideTarget = !spec.WantTarget
	} else {
		spec.WantTarget, spec.HideTarget = true, false
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
	// A program whose purpose is to change or destroy things is not
	// read-only, whatever a guess elsewhere said: nothing polls it or runs
	// it unasked. (A risky option alone is left to the status runs, which
	// never pass one.)
	if yes[QChangesThings] || (a[QCanDestroy].Yes && a[QCanDestroy].Sure == SureStrong) {
		spec.ReadOnly = false
	}
	return g
}

func featureIndex(f Feature) int {
	for i, x := range AllFeatures() {
		if x == f {
			return i
		}
	}
	return len(AllFeatures())
}
