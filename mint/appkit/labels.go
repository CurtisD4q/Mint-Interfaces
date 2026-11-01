package appkit

// ArchetypeLabels is the wording a screen uses for one of the 36 archetypes:
// the action button, the input section, and the output section.
type ArchetypeLabels struct {
	Verb   string // "Compare", "Record", "Probe"
	Input  string // "Inputs", "Source", "Targets"
	Output string // "Differences", "Capture Output", "Discovered Targets"
	Title  string // screen title, after the tool name
	Home   string // the home-screen item for this archetype
}

// ArchetypeLabelsFor returns the screen wording for an archetype id or name.
// ok is false when id is empty or unknown, so the caller keeps the layout's
// generic label.
func ArchetypeLabelsFor(idOrName string) (ArchetypeLabels, bool) {
	if idOrName == "" {
		return ArchetypeLabels{}, false
	}
	l, ok := archetypeLabels[archetypeKey(idOrName)]
	return l, ok
}

// archetypeLabels maps each archetype (via archetypeKey) to its screen wording.
// The layout's own labels are the fallback when ArchetypeID is empty.
var archetypeLabels = map[string]ArchetypeLabels{
	"ask":       {Verb: "Ask", Input: "Question", Output: "Answer", Title: "Ask", Home: "Ask"},
	"lookup":    {Verb: "Look up", Input: "Name", Output: "Records", Title: "Look up", Home: "Look up"},
	"inspect":   {Verb: "Inspect", Input: "Item", Output: "Report", Title: "Inspect", Home: "Inspect"},
	"list":      {Verb: "List", Input: "Filter", Output: "Items", Title: "List", Home: "List"},
	"search":    {Verb: "Search", Input: "Pattern", Output: "Matches", Title: "Search", Home: "Search"},
	"query":     {Verb: "Query", Input: "Expression", Output: "Results", Title: "Query", Home: "Query"},
	"compare":   {Verb: "Compare", Input: "Inputs", Output: "Differences", Title: "Compare", Home: "Compare"},
	"read":      {Verb: "Read", Input: "Topic", Output: "Page", Title: "Read", Home: "Read"},
	"watch":     {Verb: "Watch", Input: "Sample", Output: "Live values", Title: "Watch", Home: "Watch"},
	"follow":    {Verb: "Follow", Input: "Source", Output: "Live log", Title: "Follow", Home: "Follow"},
	"record":    {Verb: "Record", Input: "Source", Output: "Capture Output", Title: "Record", Home: "Record"},
	"probe":     {Verb: "Probe", Input: "Targets", Output: "Discovered Targets", Title: "Probe", Home: "Probe"},
	"audit":     {Verb: "Audit", Input: "What to check", Output: "Findings", Title: "Audit", Home: "Audit"},
	"measure":   {Verb: "Measure", Input: "Workload", Output: "Statistics", Title: "Measure", Home: "Measure"},
	"convert":   {Verb: "Convert", Input: "Input", Output: "Output", Title: "Convert", Home: "Convert"},
	"filter":    {Verb: "Filter", Input: "Input", Output: "Output", Title: "Filter", Home: "Filter"},
	"pack":      {Verb: "Pack", Input: "Files", Output: "Listing", Title: "Pack", Home: "Pack"},
	"generate":  {Verb: "Generate", Input: "Settings", Output: "Result", Title: "Generate", Home: "Generate"},
	"build":     {Verb: "Build", Input: "Project", Output: "Build log", Title: "Build", Home: "Build"},
	"transfer":  {Verb: "Transfer", Input: "Inputs", Output: "Progress", Title: "Transfer", Home: "Transfer"},
	"present":   {Verb: "Present", Input: "What to present", Output: "Result", Title: "Present", Home: "Present"},
	"editfiles": {Verb: "Preview", Input: "Files", Output: "Changes", Title: "Edit files", Home: "Edit files"},
	"mirror":    {Verb: "Mirror", Input: "Inputs", Output: "Changes", Title: "Mirror", Home: "Mirror"},
	"settings":  {Verb: "Apply", Input: "Settings", Output: "Current values", Title: "Settings", Home: "Settings"},
	"device":    {Verb: "Apply", Input: "Device", Output: "Current state", Title: "Device", Home: "Device"},
	"rules":     {Verb: "Apply", Input: "Rules", Output: "Rules", Title: "Rules", Home: "Rules"},
	"control":   {Verb: "Control", Input: "Items", Output: "Running items", Title: "Control", Home: "Control"},
	"signal":    {Verb: "Signal", Input: "Processes", Output: "Result", Title: "Signal", Home: "Signal"},
	"records":   {Verb: "Manage", Input: "Commands", Output: "Items", Title: "Records", Home: "Records"},
	"history":   {Verb: "Show", Input: "Commands", Output: "History", Title: "History", Home: "History"},
	"toolkit":   {Verb: "Run", Input: "Commands", Output: "Output", Title: "Toolkit", Home: "Toolkit"},
	"wrap":      {Verb: "Run", Input: "Command", Output: "Output", Title: "Wrap", Home: "Wrap"},
	"schedule":  {Verb: "Schedule", Input: "Commands", Output: "Jobs", Title: "Schedule", Home: "Schedule"},
	"serve":     {Verb: "Serve", Input: "Listen", Output: "Log", Title: "Serve", Home: "Serve"},
	"session":   {Verb: "Start", Input: "Commands", Output: "Transcript", Title: "Session", Home: "Session"},
	"general":   {Verb: "Run", Input: "Arguments", Output: "Output", Title: "General", Home: "Run"},
}
