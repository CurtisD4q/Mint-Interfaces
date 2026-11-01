package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// attachParsedScreens gives an unlisted follow, serve, or session tool its
// interactive screen when the help actually named what that screen needs.
// It also infers ReadOnly, StatusArgs, and RowActions from that help when
// safety.go has no curated answer. A tool Mint already configured (Follow,
// Service, Transcript, RowActions, or a curated ReadOnly / StatusArgs) is
// left alone.
func attachParsedScreens(spec *appkit.ToolSpec, help string) {
	if spec == nil || spec.ArchetypeID == "" {
		return
	}
	def, ok := appkit.LookupArchetype(spec.ArchetypeID)
	if !ok {
		return
	}
	b := def.Blueprint()
	if spec.Follow == nil && (spec.ArchetypeID == "follow" || b.Has(appkit.FeatFollow)) {
		if flag := parsedFollowFlag(spec.Flags); flag != "" {
			spec.Follow = &appkit.FollowSpec{Flag: flag}
		}
	}
	if spec.Service == nil && spec.ArchetypeID == "serve" {
		if flag, port := parsedListenFlag(spec.Flags); flag != "" {
			if port == 0 {
				port = 8080
			}
			spec.Service = &appkit.ServiceSpec{
				Adapter:     appkit.ServiceGeneric,
				PortFlag:    flag,
				DefaultPort: port,
			}
		}
	}
	if spec.Transcript == nil && spec.ArchetypeID == "session" && sessionCuesMatch(help) {
		tr := appkit.TranscriptSpec{Adapter: "lines", QuitWords: []string{"quit", "exit"}}
		if known, ok := appkit.KnownTranscriptSpec(spec.BinaryName()); ok {
			tr.Adapter = known.Adapter
		}
		spec.Transcript = &tr
	}
	attachInferredSafety(spec, help)
}

// parsedFollowFlag is the -f or --follow option in flags, preferring the
// long form. Empty when the help did not list one.
func parsedFollowFlag(flags []appkit.FlagSpec) string {
	short := ""
	for _, f := range flags {
		name := flagName(f.Flag)
		switch name {
		case "--follow":
			return name
		case "-f":
			short = name
		}
	}
	return short
}

// parsedListenFlag is a port or listen option (-p, --port, --listen),
// preferring --port, then -p, then --listen. port is a numeric default
// written on that option, or 0.
func parsedListenFlag(flags []appkit.FlagSpec) (flag string, port int) {
	rank := map[string]int{"--port": 3, "-p": 2, "--listen": 1}
	best := 0
	for _, f := range flags {
		name := flagName(f.Flag)
		s := rank[name]
		if s <= best {
			continue
		}
		best = s
		flag = name
		port = 0
		if n, err := strconv.Atoi(strings.TrimSpace(f.Default)); err == nil && n > 0 && n <= 65535 {
			port = n
		}
	}
	return flag, port
}

// applyChosenArchetype makes def the spec's archetype: layout, target box,
// and the interactive screens that the help can support. A label this
// archetype itself applied is cleared first, so the new one can hide the
// target box; a label taken from the usage line is kept.
func applyChosenArchetype(spec *appkit.ToolSpec, a appkit.ArchetypeDef, help string) {
	if prev, ok := appkit.LookupArchetype(spec.ArchetypeID); ok && prev.ID != a.ID {
		if spec.TargetLabel == prev.TargetLabel {
			spec.TargetLabel = ""
		}
		if spec.TargetPlaceholder == prev.TargetPlaceholder {
			spec.TargetPlaceholder = ""
		}
		if spec.SecondTargetLabel == prev.SecondTargetLabel {
			spec.SecondTargetLabel = ""
		}
		if spec.SecondTargetPlaceholder == prev.SecondTargetPlaceholder {
			spec.SecondTargetPlaceholder = ""
		}
	}
	spec.TwoInputs = false
	spec.WantTarget = false
	spec.HideTarget = false
	clearParsedScreens(spec)
	a.ApplyTo(spec)
	if spec.TargetLabel == "" && !spec.HideTarget {
		spec.TargetLabel = "Target / arguments"
		if spec.TargetPlaceholder == "" {
			spec.TargetPlaceholder = "arguments"
		}
	}
	attachParsedScreens(spec, help)
}

// clearParsedScreens drops follow, service, and transcript values that
// attachParsedScreens added, and puts ReadOnly, StatusArgs, and RowActions
// back to the safety.go baseline, so a later --archetype choice can attach
// its own. Specs Mint already knew (follow args, a named service adapter, a
// transcript without quit words, a curated read-only tool or status command)
// stay.
func clearParsedScreens(spec *appkit.ToolSpec) {
	if spec.Follow != nil && spec.Follow.Flag != "" && len(spec.Follow.FollowArgs) == 0 && spec.Follow.Format == "" {
		spec.Follow = nil
	}
	if spec.Service != nil && spec.Service.Adapter == appkit.ServiceGeneric && spec.Service.BindFlag == "" && spec.Service.DirFlag == "" {
		spec.Service = nil
	}
	if spec.Transcript != nil && len(spec.Transcript.QuitWords) > 0 {
		spec.Transcript = nil
	}
	resetInferredSafety(spec)
}

func flagName(flag string) string {
	if i := strings.IndexByte(flag, '='); i >= 0 {
		return flag[:i]
	}
	return flag
}

// attachInferredSafety reads ReadOnly, StatusArgs, and RowActions out of
// help. Tools safety.go already describes keep the curated ReadOnly and
// StatusArgs; everything else is filled only when those fields are empty.
func attachInferredSafety(spec *appkit.ToolSpec, help string) {
	lines := splitLines(help)
	if !safetyCurated(spec) {
		inferReadOnly(spec, lines)
		inferStatusArgs(spec, lines)
	}
	inferRowActions(spec, lines)
}

// resetInferredSafety drops parser-inferred safety fields back to the
// safety.go baseline. A later archetype can infer its own, and a tool
// listed in safety.go keeps the curated command.
func resetInferredSafety(spec *appkit.ToolSpec) {
	spec.RowActions = nil
	bare := appkit.ToolSpec{Name: spec.Name, Bin: spec.Bin}
	appkit.ApplySafetyDefaults(&bare)
	if bare.ReadOnly || len(bare.StatusArgs) > 0 {
		spec.ReadOnly = bare.ReadOnly
		spec.StatusArgs = append([]string(nil), bare.StatusArgs...)
		return
	}
	spec.ReadOnly = false
	spec.StatusArgs = nil
}

// safetyCurated reports whether safety.go already set ReadOnly or
// StatusArgs for this tool. Inference must not replace that.
func safetyCurated(spec *appkit.ToolSpec) bool {
	bare := appkit.ToolSpec{Name: spec.Name, Bin: spec.Bin}
	appkit.ApplySafetyDefaults(&bare)
	return bare.ReadOnly || len(bare.StatusArgs) > 0
}

func splitLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return cleanLines(s)
}

// Verbs that change something. A tool that offers one is not a pure observer.
var mutatingSubcommands = map[string]bool{
	"set": true, "add": true, "delete": true, "remove": true,
	"start": true, "stop": true, "enable": true, "disable": true,
	"install": true, "uninstall": true, "write": true, "create": true,
	"kill": true, "flush": true,
}

func isMutatingSubcommand(name string) bool {
	name = strings.ToLower(name)
	if mutatingSubcommands[name] {
		return true
	}
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' }) {
		if mutatingSubcommands[part] {
			return true
		}
	}
	return false
}

func hasMutatingSubcommand(lines []string) bool {
	for _, n := range discoveredSubcommands(lines) {
		if isMutatingSubcommand(n) {
			return true
		}
	}
	return false
}

func flagsLookDestructive(flags []appkit.FlagSpec) bool {
	for _, f := range flags {
		if appkit.LooksDestructive(f.Flag, strings.TrimSpace(f.Label+" "+f.Hint)) {
			return true
		}
	}
	return false
}

// readOnlyArchetype is a pure observer shape: watch, list, inspect, or any
// other answer-once tool that can run with no positional target. Look-ups
// are not: they ask other computers (DNS, whois), which is not reading this
// computer's state.
func readOnlyArchetype(spec *appkit.ToolSpec, lines []string) bool {
	switch spec.ArchetypeID {
	case "watch", "list", "inspect":
		return true
	case "look-up":
		return false
	}
	def, ok := appkit.LookupArchetype(spec.ArchetypeID)
	if !ok || def.Group != appkit.GroupAnswer {
		return false
	}
	return runnableWithoutTarget(spec, lines)
}

// runnableWithoutTarget reports whether the tool can be polled with no
// positional argument. A required word in the first synopsis form is a
// target even when the archetype hides the box. HideTarget, or a synopsis
// whose positionals are all optional, means there is nothing to pass.
func runnableWithoutTarget(spec *appkit.ToolSpec, lines []string) bool {
	known, required := synopsisRequiresPositional(spec, lines)
	if known && required {
		return false
	}
	if spec.HideTarget {
		return true
	}
	return known && !required
}

// synopsisRequiresPositional reports whether the first usage form was found
// and whether it has a positional that is not in brackets.
func synopsisRequiresPositional(spec *appkit.ToolSpec, lines []string) (known, required bool) {
	name := toolBaseName(spec.Name)
	if name == "" {
		name = toolBaseName(spec.BinaryName())
	}
	syn := ParseSynopsis(name, lines, lines)
	if len(syn.Forms) == 0 {
		return false, false
	}
	for _, p := range syn.Forms[0].Positionals {
		if !p.Optional {
			return true, true
		}
	}
	return true, false
}

func inferReadOnly(spec *appkit.ToolSpec, lines []string) {
	if spec.ReadOnly || len(spec.StatusArgs) > 0 || spec.Irreversible {
		return
	}
	if !readOnlyArchetype(spec, lines) || !runnableWithoutTarget(spec, lines) {
		return
	}
	if flagsLookDestructive(spec.Flags) || hasMutatingSubcommand(lines) || helpNamesChangingOption(lines) {
		return
	}
	spec.ReadOnly = true
}

// reChangingOption is an option, anywhere in the help, whose name says it
// changes things: xrdb's -load and -merge, faillock's --reset, find's
// -delete and -exec. Options mint-gen did not parse (find's actions,
// faillock's) are caught too.
var reChangingOption = regexp.MustCompile(`(?i)(?:^|[\s\[,|(])--?(?:[a-z0-9]+-)*(?:reset|load|merge|remove|clear|delete|set|write|kill|flush|apply|import|purge|wipe|erase|unlock|lock|replace|install|uninstall|update|upgrade|edit|modify|create|add|exec|execdir|ok|okdir|fix|repair|restore|rename|move|truncate|format|mount|umount|unmount|enable|disable|start|stop|restart|reload|signal|send|commit|push|apply)\b`)

// helpNamesChangingOption reports whether the help lists an option that
// changes things. Such a tool is not guessed to be read-only, so Mint never
// runs it unasked.
func helpNamesChangingOption(lines []string) bool {
	for _, l := range lines {
		if reChangingOption.MatchString(l) {
			return true
		}
	}
	return false
}

func namedStateArchetype(id string) bool {
	switch id {
	case "settings", "device", "rules", "control", "records", "schedule", "history", "backup", "watch", "list":
		return true
	default:
		return false
	}
}

// stateReadingArchetype is a screen that shows state worth reading back:
// the named archetypes, or any blueprint that refreshes live or acts on rows.
func stateReadingArchetype(spec *appkit.ToolSpec) bool {
	if namedStateArchetype(spec.ArchetypeID) {
		return true
	}
	def, ok := appkit.LookupArchetype(spec.ArchetypeID)
	if !ok {
		return false
	}
	b := def.Blueprint()
	return b.Has(appkit.FeatLiveRefresh) || b.Has(appkit.FeatRowActions)
}

var statusSubcommandGroups = []struct {
	names    []string
	checkArg bool
}{
	{[]string{"status"}, false},
	{[]string{"list-units", "list-rules", "list-keys", "list-sessions", "list-timers", "list-sockets", "list-sinks", "list-sources", "list-cards", "list"}, false},
	{[]string{"show", "info", "get-all", "dump"}, true},
}

func inferStatusArgs(spec *appkit.ToolSpec, lines []string) {
	if spec.ReadOnly || len(spec.StatusArgs) > 0 {
		return
	}
	if stateReadingArchetype(spec) {
		if sub := pickStatusSubcommand(discoveredSubcommands(lines), lines); sub != "" {
			spec.StatusArgs = []string{sub}
			return
		}
	}
	if !namedStateArchetype(spec.ArchetypeID) {
		return
	}
	if flag := pickStatusFlag(spec); flag != "" {
		spec.StatusArgs = []string{flag}
	}
}

func pickStatusSubcommand(names []string, lines []string) string {
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, g := range statusSubcommandGroups {
		for _, n := range g.names {
			if !have[n] {
				continue
			}
			if g.checkArg && subcommandRequiresArg(lines, n) {
				continue
			}
			return n
		}
	}
	return ""
}

var (
	reStatusDesc    = regexp.MustCompile(`\b(status|state|lists?|listing)\b`)
	reListDesc      = regexp.MustCompile(`\b(lists?|listing|display all|show all|rules?|jobs?|entries|entry)\b`)
	reQueryDesc     = regexp.MustCompile(`\b(query|queries|current state|connected|configuration|config)\b`)
	reAllDesc       = regexp.MustCompile(`\b(display all|show all|print all|all values|all settings)\b`)
	reWideGap       = regexp.MustCompile(`\s{2,}`)
	reOptionalGroup = regexp.MustCompile(`\[[^\[\]]*\]`)
	reAngleArg      = regexp.MustCompile(`<[^\s>]+>`)
	reCommandToken  = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,30}$`)
	reDeleteVerb    = regexp.MustCompile(`\b(delete|deletes|deleting|deleted|remove|removes|removing|removed)\b`)
	reDeleteNoun    = regexp.MustCompile(`\b(entr(?:y|ies)|rules?|jobs?|items?)\b`)
	reKillVerb      = regexp.MustCompile(`\b(kills?|killing|killed|stops?|stopping|stopped)\b`)
	reKillNoun      = regexp.MustCompile(`\b(process(?:es)?|jobs?|services?|units?)\b`)
	reRecursive     = regexp.MustCompile(`\brecurs(?:ive|ively|e|ion)?\b`)
	reTraversal     = regexp.MustCompile(`\b(?:director(?:y|ies)|folders?|trees?|traversal)\b`)
)

func flagBlurb(f appkit.FlagSpec) string {
	return strings.ToLower(strings.TrimSpace(f.Label + " " + f.Hint))
}

// blurbOK matches re against the flag's description. A long flag with no
// description is accepted when emptyLong is set; a short flag is not,
// because -S or -l can mean something else.
func blurbOK(name string, f appkit.FlagSpec, re *regexp.Regexp, emptyLong bool) bool {
	text := flagBlurb(f)
	if text == "" {
		return emptyLong && strings.HasPrefix(name, "--")
	}
	return re.MatchString(text)
}

func statusFlagOK(name string, f appkit.FlagSpec) bool {
	return blurbOK(name, f, reStatusDesc, true)
}

func listFlagOK(name string, f appkit.FlagSpec) bool {
	return blurbOK(name, f, reListDesc, true)
}

func queryFlagOK(f appkit.FlagSpec) bool {
	text := flagBlurb(f)
	if text == "" || strings.Contains(text, "quiet") {
		return false
	}
	return reQueryDesc.MatchString(text)
}

func allFlagOK(id string, f appkit.FlagSpec) bool {
	switch id {
	case "settings", "device", "watch", "list":
	default:
		return false
	}
	return reAllDesc.MatchString(flagBlurb(f))
}

func boolFlags(flags []appkit.FlagSpec) map[string]appkit.FlagSpec {
	out := map[string]appkit.FlagSpec{}
	for _, f := range flags {
		if f.Kind != appkit.FlagBool {
			continue
		}
		name := flagName(f.Flag)
		if _, ok := out[name]; ok || appkit.LooksDestructive(name, f.Label) {
			continue
		}
		out[name] = f
	}
	return out
}

func pickStatusFlag(spec *appkit.ToolSpec) string {
	flags := boolFlags(spec.Flags)
	groups := []struct {
		names []string
		ok    func(string, appkit.FlagSpec) bool
	}{
		{[]string{"--status-all", "--status", "-S"}, statusFlagOK},
		{[]string{"--list-all", "--list", "--list-rules", "-L", "-l"}, listFlagOK},
		{[]string{"--query", "-q"}, func(_ string, f appkit.FlagSpec) bool { return queryFlagOK(f) }},
		{[]string{"--show", "--info", "--get-all"}, func(string, appkit.FlagSpec) bool { return true }},
		{[]string{"--all", "-a"}, func(_ string, f appkit.FlagSpec) bool { return allFlagOK(spec.ArchetypeID, f) }},
	}
	for _, g := range groups {
		for _, name := range g.names {
			f, ok := flags[name]
			if ok && g.ok(name, f) {
				return name
			}
		}
	}
	return ""
}

func wantsRowActions(spec *appkit.ToolSpec) bool {
	switch spec.ArchetypeID {
	case "control", "records", "schedule", "rules", "history", "backup", "settings", "list", "device":
		return true
	}
	def, ok := appkit.LookupArchetype(spec.ArchetypeID)
	return ok && def.Blueprint().Has(appkit.FeatRowActions)
}

type rowVerb struct {
	name    string
	confirm bool
}

var (
	controlRowVerbs = []rowVerb{
		{"status", false}, {"start", false}, {"stop", true}, {"restart", true},
		{"reload", false}, {"enable", false}, {"disable", true}, {"kill", true}, {"logs", false},
	}
	recordsRowVerbs = []rowVerb{
		{"show", false}, {"info", false}, {"inspect", false}, {"status", false},
		{"update", true}, {"upgrade", true},
		{"remove", true}, {"uninstall", true}, {"delete", true}, {"purge", true},
	}
	scheduleRowVerbs = []rowVerb{
		{"show", false}, {"info", false}, {"status", false},
		{"run", false}, {"start", false},
		{"remove", true}, {"delete", true}, {"cancel", true}, {"rm", true},
	}
	rulesRowVerbs = []rowVerb{
		{"show", false}, {"status", false},
		{"enable", false}, {"allow", false},
		{"disable", true}, {"deny", true}, {"reject", true},
		{"delete", true}, {"del", true}, {"remove", true}, {"remove-rule", true},
	}
	historyRowVerbs = []rowVerb{
		{"show", false}, {"diff", false}, {"inspect", false}, {"log", false},
		{"checkout", true}, {"restore", true}, {"revert", true},
	}
	backupRowVerbs = []rowVerb{
		{"ls", false}, {"list", false}, {"show", false}, {"info", false}, {"diff", false}, {"stats", false}, {"check", false},
		{"restore", true}, {"rollback", true}, {"forget", true}, {"prune", true}, {"delete", true}, {"remove", true},
	}
	// device, settings, list, and any other row-action archetype.
	collectionRowVerbs = []rowVerb{
		{"show", false}, {"info", false}, {"status", false}, {"get", false},
		{"enable", false}, {"connect", false}, {"up", false}, {"unblock", false},
		{"disable", true}, {"disconnect", true}, {"down", true}, {"block", true},
		{"remove", true}, {"delete", true},
	}
)

// readingRowVerbs only read: an inferred row action with any other verb
// (start, enable, up, allow, run…) changes things, so it asks first.
// systemctl enable, ufw enable (which can cut off a remote session),
// virsh start and nmcli up all confirm.
var readingRowVerbs = map[string]bool{
	"status": true, "show": true, "info": true, "inspect": true, "get": true,
	"diff": true, "log": true, "logs": true,
}

func rowVerbTable(id string) []rowVerb {
	switch id {
	case "control":
		return controlRowVerbs
	case "records":
		return recordsRowVerbs
	case "schedule":
		return scheduleRowVerbs
	case "rules":
		return rulesRowVerbs
	case "history":
		return historyRowVerbs
	case "backup":
		return backupRowVerbs
	default:
		return collectionRowVerbs
	}
}

func rowActionLabel(verb string) string {
	return titleWords(strings.NewReplacer("-", " ", "_", " ").Replace(verb))
}

func inferRowActions(spec *appkit.ToolSpec, lines []string) {
	if len(spec.RowActions) > 0 || !wantsRowActions(spec) {
		return
	}
	if actions := rowActionsFromSubcommands(spec.ArchetypeID, lines); len(actions) > 0 {
		spec.RowActions = actions
		return
	}
	if actions := rowActionsFromFlags(spec.Flags); len(actions) > 0 {
		spec.RowActions = actions
	}
}

func rowActionsFromSubcommands(id string, lines []string) []appkit.RowActionSpec {
	have := map[string]bool{}
	for _, n := range discoveredSubcommands(lines) {
		have[n] = true
	}
	var out []appkit.RowActionSpec
	for _, v := range rowVerbTable(id) {
		if !have[v.name] {
			continue
		}
		out = append(out, appkit.RowActionSpec{
			Label:      rowActionLabel(v.name),
			Hint:       v.name,
			Subcommand: []string{v.name},
			Column:     0,
			Confirm:    v.confirm || !readingRowVerbs[v.name],
		})
	}
	return out
}

func rowActionsFromFlags(flags []appkit.FlagSpec) []appkit.RowActionSpec {
	var out []appkit.RowActionSpec
	seen := map[string]bool{}
	add := func(flag, hint, label string, confirm bool) {
		flag = flagName(flag)
		if flag == "" || seen[flag] {
			return
		}
		seen[flag] = true
		out = append(out, appkit.RowActionSpec{
			Label: label, Hint: hint, Subcommand: []string{flag}, Column: 0, Confirm: confirm,
		})
	}
	for _, f := range flags {
		switch flagName(f.Flag) {
		case "--show":
			add(f.Flag, "show", "Show", false)
		case "--info":
			add(f.Flag, "info", "Info", false)
		case "--status":
			add(f.Flag, "status", "Status", false)
		}
	}
	for _, f := range flags {
		if !deleteActionFlag(f) {
			continue
		}
		name := flagName(f.Flag)
		label, hint := "Delete", "delete"
		if name == "--remove" || name == "-r" {
			label, hint = "Remove", "remove"
		}
		add(f.Flag, hint, label, true)
	}
	for _, f := range flags {
		if !killActionFlag(f) {
			continue
		}
		name := flagName(f.Flag)
		label, hint := "Kill", "kill"
		if name == "--stop" {
			label, hint = "Stop", "stop"
		}
		add(f.Flag, hint, label, true)
	}
	return out
}

func isRecursiveTraversal(text string) bool {
	return reRecursive.MatchString(text) && reTraversal.MatchString(text)
}

func deleteActionFlag(f appkit.FlagSpec) bool {
	switch flagName(f.Flag) {
	case "-d", "--delete", "-D", "--remove", "-r":
	default:
		return false
	}
	text := flagBlurb(f)
	if isRecursiveTraversal(text) || !reDeleteVerb.MatchString(text) || !reDeleteNoun.MatchString(text) {
		return false
	}
	return true
}

func killActionFlag(f appkit.FlagSpec) bool {
	switch flagName(f.Flag) {
	case "-k", "--kill", "--stop":
	default:
		return false
	}
	text := flagBlurb(f)
	return reKillVerb.MatchString(text) && reKillNoun.MatchString(text)
}

// discoveredSubcommands is the union of the suite-name reader, the tab
// reader, and command lines whose usage sits on the same line
// ("show <name>", "show [controller]"). The first two keep a name only
// when a gap follows it, so a placeholder would hide it.
func discoveredSubcommands(lines []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.ToLower(strings.Trim(strings.TrimSpace(n), ":"))
		if !reCommandToken.MatchString(n) || cmdFiller[n] || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, n := range suiteCommandNames(lines, lines) {
		add(n)
	}
	for _, s := range extractSubcommandsN(lines, lines, 30) {
		add(s.Name)
	}
	in := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if commandHeading(line, t) {
			in = true
			continue
		}
		if in && commandSectionEnds(t) {
			in = false
			continue
		}
		if !in {
			continue
		}
		if name, ok := looseCommandName(line); ok {
			add(name)
		}
	}
	return out
}

func commandHeading(line, trimmed string) bool {
	head := !strings.HasPrefix(trimmed, "-") && len(strings.Fields(trimmed)) <= 10 && len(trimmed) < 80
	return head && reCmdHeading.MatchString(trimmed) && (!strings.HasPrefix(line, " ") || strings.HasSuffix(trimmed, ":") || reCmdAllCaps.MatchString(trimmed))
}

func commandSectionEnds(trimmed string) bool {
	return reOptsHeading.MatchString(trimmed) || (reCmdAllCaps.MatchString(trimmed) && !reCmdHeading.MatchString(trimmed))
}

// looseCommandName is the command at the start of an indented definition:
// "status", "status - show it", "show <name>", "show [controller]".
func looseCommandName(line string) (string, bool) {
	if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
		return "", false
	}
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "-") {
		return "", false
	}
	fields := strings.Fields(trim)
	if len(fields) == 0 {
		return "", false
	}
	name := strings.TrimRight(fields[0], ":")
	if !reCommandToken.MatchString(strings.ToLower(name)) {
		return "", false
	}
	if len(fields) == 1 || strings.HasSuffix(fields[0], ":") {
		return name, true
	}
	after := trim[len(fields[0]):]
	lead := strings.TrimLeft(after, " \t")
	switch {
	case strings.HasPrefix(lead, "- "), strings.HasPrefix(lead, "<"), strings.HasPrefix(lead, "["):
		return name, true
	case gapAtLeast(after, 2):
		return name, true
	default:
		return "", false
	}
}

func gapAtLeast(after string, n int) bool {
	c := 0
	for _, r := range after {
		if r != ' ' && r != '\t' {
			break
		}
		c++
		if c >= n {
			return true
		}
	}
	return false
}

// subcommandRequiresArg reports whether a command's own line demands a
// positional, written as <name> or <key>. "show [controller]" and a bare
// "show" do not.
func subcommandRequiresArg(lines []string, name string) bool {
	name = strings.ToLower(name)
	for _, line := range lines {
		got, ok := looseCommandName(line)
		if !ok || strings.ToLower(got) != name {
			continue
		}
		if usageHasRequiredArg(commandUsageHead(line)) {
			return true
		}
	}
	return false
}

func commandUsageHead(line string) string {
	trim := strings.TrimSpace(line)
	fields := strings.Fields(trim)
	if len(fields) == 0 {
		return ""
	}
	rest := strings.TrimSpace(trim[len(fields[0]):])
	rest = strings.TrimPrefix(rest, ":")
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "- ") {
		return ""
	}
	if i := strings.Index(rest, " - "); i >= 0 {
		rest = strings.TrimSpace(rest[:i])
	}
	if loc := reWideGap.FindStringIndex(rest); loc != nil {
		rest = strings.TrimSpace(rest[:loc[0]])
	}
	var toks []string
	for _, f := range strings.Fields(rest) {
		if strings.HasPrefix(f, "<") || strings.HasPrefix(f, "[") || strings.HasPrefix(f, "-") {
			toks = append(toks, f)
			continue
		}
		break
	}
	return strings.Join(toks, " ")
}

func usageHasRequiredArg(usage string) bool {
	return reAngleArg.MatchString(reOptionalGroup.ReplaceAllString(usage, " "))
}
