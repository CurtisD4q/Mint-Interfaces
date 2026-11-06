package appkit

import "strings"

// Interface types say how a program's screen is laid out and how you move
// around it. Archetypes say what a program is for; they are the back end.
// An interface type is the front end: it is chosen from facts about how the
// program runs (the ten answers mint-gen detects), and an archetype only
// breaks a tie where the answers cannot tell two types apart.
//
// The choice is a pipeline of plain data, with no terminal in any step:
//
//	Facts  ->  Plan  ->  screen
//
// FactsFromSpec reads the facts from a ToolSpec, PlanFor chooses the type
// from them by an ordered rule table (the first rule that matches wins), and
// the screen is built from the plan. This file is the first two steps. Every
// type still draws through the current MainScreen, so nothing on screen
// changes yet; later steps give each type a layout of its own.

// InterfaceTypeID names one interface type.
type InterfaceTypeID string

const (
	TypeLauncher       InterfaceTypeID = "launcher"        // the program draws its own full screen
	TypeSession        InterfaceTypeID = "session"         // a command box and a transcript
	TypeServiceControl InterfaceTypeID = "service-control" // start, stop and status of something that keeps running
	TypeLiveMonitor    InterfaceTypeID = "live-monitor"    // values or rows that refresh
	TypeLogFollower    InterfaceTypeID = "log-follower"    // a log that grows, newest at the bottom
	TypeCommandSuite   InterfaceTypeID = "command-suite"   // a program made of commands, one form each
	TypeCompare        InterfaceTypeID = "compare"         // two inputs and the differences
	TypeStateEditor    InterfaceTypeID = "state-editor"    // read state, edit it, apply
	TypeAction         InterfaceTypeID = "action"          // do something to a target, with a preview and a confirm
	TypeResultTable    InterfaceTypeID = "result-table"    // rows you can sort and open
	TypeReport         InterfaceTypeID = "report"          // a report in folding sections
	TypeQuickAnswer    InterfaceTypeID = "quick-answer"    // one input, one result
)

// Facts are what the choice of type is made from: the ten answers, and the
// features the program's archetype wants, used only as tie-breaks.
type Facts struct {
	KeepsRunning bool // runs until stopped: refreshes, follows, serves or listens
	ChangesState bool // changes settings, the system or something outside itself
	TakesTarget  bool // takes a file, host, name or other argument to work on
	TwoInputs    bool // needs two inputs, like a diff of two files
	Subcommands  bool // has commands of its own
	WritesFile   bool // saves its result to a file
	TableOutput  bool // prints rows and columns
	NeedsRoot    bool // needs root for its main job
	Destructive  bool // can delete, overwrite or kill something
	FullScreen   bool // a full-screen terminal app

	// Hints are the features the program's archetypes want together (the
	// blended blueprint, see ToolSpec.Preferences). The
	// answers decide the type; a hint is read only where a rule says so.
	Hints []Feature
	// Weights are the archetype's weights (0 to 1) for those features, for
	// the rules that need a strong preference and not just a wanted one.
	Weights map[Feature]float64
}

// Strong is the weight at which a hint counts as a firm preference.
const strongHint = 0.8

// Firmly reports whether the archetype wants feature f at strongHint or more.
func (f Facts) Firmly(ft Feature) bool { return f.Weights[ft] >= strongHint }

// Wants reports whether the archetype wants feature f.
func (f Facts) Wants(ft Feature) bool {
	for _, h := range f.Hints {
		if h == ft {
			return true
		}
	}
	return false
}

// InterfaceType is one entry of the rule table.
type InterfaceType struct {
	ID   InterfaceTypeID
	Name string
	Is   string // what the screen is, in one sentence
	// Why is the reason shown by an explanation when this rule matched.
	Why string
	// When reports whether the facts fit this type. Rules are tried in
	// table order and the first match wins, so a rule may rely on every
	// rule above it having failed. The last rule matches everything.
	When func(Facts) bool
}

// interfaceTypes is the rule table, in the order the rules are tried. The
// order is the design: what most changes the screen comes first.
var interfaceTypes = []InterfaceType{
	{ID: TypeLauncher, Name: "Launcher",
		Is:   "Starts the program, which draws its own full screen.",
		Why:  "it is a full-screen program",
		When: func(f Facts) bool { return f.FullScreen }},
	{ID: TypeSession, Name: "Session",
		Is:   "A command box and a transcript of what was run.",
		Why:  "its archetype is a session (a command box and a transcript)",
		When: func(f Facts) bool { return f.Wants(FeatTranscript) }},
	{ID: TypeServiceControl, Name: "Service control",
		Is:   "Start, stop and see the status of something that keeps running.",
		Why:  "it keeps running and changes things, or serves",
		When: func(f Facts) bool { return f.KeepsRunning && (f.ChangesState || f.Wants(FeatService)) }},
	{ID: TypeLiveMonitor, Name: "Live monitor",
		Is:  "Values or rows that refresh on their own.",
		Why: "it keeps running and prints rows or refreshes",
		// A program that firmly wants a live log and does not refresh is a
		// log follower, even when it prints rows (journalctl).
		When: func(f Facts) bool {
			return f.KeepsRunning && (f.TableOutput || f.Wants(FeatLiveRefresh)) && !(f.Firmly(FeatFollow) && !f.Wants(FeatLiveRefresh))
		}},
	{ID: TypeLogFollower, Name: "Log follower",
		Is:   "A log that grows, newest at the bottom.",
		Why:  "it keeps running and writes a stream",
		When: func(f Facts) bool { return f.KeepsRunning }},
	{ID: TypeCommandSuite, Name: "Command suite",
		Is:   "A program made of commands, with a form for each one.",
		Why:  "it has commands of its own",
		When: func(f Facts) bool { return f.Subcommands }},
	{ID: TypeCompare, Name: "Compare",
		Is:   "Two inputs side by side and the differences between them.",
		Why:  "it needs two inputs",
		When: func(f Facts) bool { return f.TwoInputs }},
	{ID: TypeStateEditor, Name: "State editor",
		Is:   "Read the current state, edit it and apply the change.",
		Why:  "it changes things and takes no target",
		When: func(f Facts) bool { return f.ChangesState && !f.TakesTarget }},
	{ID: TypeAction, Name: "Action",
		Is:   "Do something to a target, with a preview and a confirm.",
		Why:  "it changes things and takes a target",
		When: func(f Facts) bool { return f.ChangesState }},
	{ID: TypeResultTable, Name: "Result table",
		Is:   "Rows you can sort, filter and open.",
		Why:  "it prints rows and columns",
		When: func(f Facts) bool { return f.TableOutput }},
	{ID: TypeReport, Name: "Report",
		Is:   "A report in folding sections.",
		Why:  "its archetype firmly reports in sections or by severity",
		When: func(f Facts) bool { return f.Firmly(FeatCollapsible) || f.Firmly(FeatFindings) }},
	{ID: TypeQuickAnswer, Name: "Quick answer",
		Is:   "One input, one result.",
		Why:  "nothing else about it changes the screen",
		When: func(Facts) bool { return true }},
}

// InterfaceTypes lists the types in rule order.
func InterfaceTypes() []InterfaceType {
	return append([]InterfaceType(nil), interfaceTypes...)
}

// LookupInterfaceType finds a type by id.
func LookupInterfaceType(id InterfaceTypeID) (InterfaceType, bool) {
	for _, t := range interfaceTypes {
		if t.ID == id {
			return t, true
		}
	}
	return InterfaceType{}, false
}

// Plan is what the rules decide for one program: the type, why, and the
// optional extras that wrap whatever the type draws.
type Plan struct {
	Type InterfaceTypeID
	Why  string
	// Extras wrap the type's screen (a file to save to, root, a confirm…),
	// in AllFeatures order. They never change which type it is.
	Extras []Feature
}

// extraFromHints are the archetype's features that stay extras.
var extraFromHints = map[Feature]bool{FeatHistory: true, FeatProfiles: true, FeatDynamicChoices: true, FeatProgress: true}

// PlanFor chooses the interface type for facts.
func PlanFor(f Facts) Plan {
	p := Plan{Type: TypeQuickAnswer, Why: interfaceTypes[len(interfaceTypes)-1].Why}
	for _, t := range interfaceTypes {
		if t.When(f) {
			p.Type, p.Why = t.ID, t.Why
			break
		}
	}
	want := map[Feature]bool{}
	if f.WritesFile {
		want[FeatFileOutput] = true
	}
	if f.NeedsRoot {
		want[FeatRoot] = true
	}
	if f.Destructive {
		want[FeatConfirm] = true
	}
	for _, h := range f.Hints {
		if extraFromHints[h] {
			want[h] = true
		}
	}
	for _, ft := range AllFeatures() {
		if want[ft] {
			p.Extras = append(p.Extras, ft)
		}
	}
	return p
}

// Signature is a stable one-line form of the plan, for tests and review.
func (p Plan) Signature() string {
	parts := make([]string, len(p.Extras))
	for i, e := range p.Extras {
		parts[i] = string(e)
	}
	return string(p.Type) + "+" + strings.Join(parts, ",")
}

// FactsFromSpec reads the facts from a tool spec, with the features its
// archetype wants as hints.
func FactsFromSpec(spec ToolSpec) Facts {
	f := Facts{
		KeepsRunning: spec.PollInterval > 0 || spec.Follow != nil || spec.Service != nil,
		// A read-only program may have status arguments (a monitor reads its
		// state with them) and still change nothing.
		ChangesState: !spec.ReadOnly && (len(spec.ApplyArgs) > 0 || len(spec.StatusArgs) > 0),
		TakesTarget:  spec.TargetLabel != "",
		TwoInputs:    spec.TwoInputs,
		Subcommands:  len(spec.Subcommands) > 0,
		WritesFile:   spec.OutputsFile,
		TableOutput:  spec.TableOutput,
		NeedsRoot:    spec.NeedsRoot,
		Destructive:  spec.ConfirmRun,
		FullScreen:   spec.FullScreen,
	}
	// The hints are all the program's archetypes together, by weight.
	if len(spec.Memberships()) > 0 {
		prefs := spec.Preferences()
		b := BlueprintFrom(prefs)
		f.Hints = append(f.Hints, b.Features...)
		f.Weights = prefs.Features
		if b.Has(FeatLiveRefresh) || b.Has(FeatFollow) || b.Has(FeatService) {
			f.KeepsRunning = true
		}
		if b.Has(FeatStateEdit) {
			f.ChangesState = true
		}
		if b.Has(FeatTarget) {
			f.TakesTarget = true
		}
		if b.Has(FeatTwoInputs) {
			f.TwoInputs = true
		}
		if b.Has(FeatTable) {
			f.TableOutput = true
		}
	}
	return f
}

// InterfacePlan is the plan for this app. It only describes the choice:
// MainScreen does not read it yet.
func (a *ToolApp) InterfacePlan() Plan { return PlanFor(FactsFromSpec(a.Spec)) }
