# Mint catalogue

How to turn a command-line program into a Mint interface: which program shapes exist, which component fits each kind of option or action, and the rules every interface follows.

It is written for two readers: a person building an interface by hand, and the generator that will build them automatically. Every code example compiles. `tools/check_catalogue.py` builds each one on top of the starter app's helpers (`templates/app/cli.go`), so the examples can use these freely:

| Helper | Does |
|---|---|
| `tool` | the program being wrapped |
| `query(args...)` | run it and return trimmed output (`""` on failure), for reading values |
| `queryJSON(&v, args...)` | run it and decode JSON output into `v` |
| `apply(args...)` | run it to change something; the error carries the program's own message |
| `applyShown(doing, done, args...)` | `apply` behind the footer spinner, with a toast for the result |

The process is: read the program (below), pick its shape (Part A), map each option and action to a component (Part B), check the rules (Part C), and assemble it from the starter (Part D).

## Reading a program

Facts come from these sources, most reliable first:

1. `tool --help`, `tool help <command>` and `tool <command> --help`.
2. `man tool`.
3. Shell completions (`/usr/share/bash-completion/completions/tool`, or zsh's `_tool`). They list sub-commands and the allowed values of options reliably.
4. `tool --version`. Record it: the interface is built against this version.
5. Machine-readable output. Look for `--json`, `-o json`, `--output json`, `--format`, `--porcelain`, `--machine`, `-z`, `--null`.
6. The web. It's good for what the program is for and what people mostly do with it. Don't trust it for exact flags, because it is often about a different version.

For each option worth exposing, note:

- its type: on/off, one of a set of values, a number (with its range and unit), a path or free text, or a secret;
- its default;
- whether it changes the system now (a setting) or only affects the next run (an option);
- whether it destroys anything.

## Part A: Program shapes

Most programs are one of these, and big ones are several (A4).

| Shape | Signs in the help | Layout |
|---|---|---|
| **A1 Settings** | `get`, `set`, `show`, `config`, `list`; `key = value` output; changes persist | one screen per group of settings, values loaded once per visit |
| **A2 Job** | `sync`, `backup`, `build`, `install`, `update`, `download`, `convert`; progress output; runs for minutes | options screen → Run → job screen |
| **A3 Monitor** | `status`, `list`, `ps`, `top`, `watch`, `stats`, `events`, `--follow` | a snapshot reloaded on a timer or on events; status badges |
| **A4 Multi-command** | `Usage: tool <command>` with many commands | home list with one item per everyday command, each an A1, A2, A3 or A5 screen |
| **A5 Lookup** | takes input, prints a result, exits (`dig`, `whois`, `ping -c`) | input rows, a Run action, the result below |

### A1 Settings

Load every value with one command when the screen opens, keep the values in a struct, and have the rows read from the struct. After a change, apply it, then reload so the screen shows what the program actually stored.

```go
var settings struct {
	Verbose bool   `json:"verbose"`
	Level   int    `json:"level"`
	Format  string `json:"format"`
}

func loadSettings() {
	if err := queryJSON(&settings, "config", "show", "--json"); err != nil {
		mint.ToastError("Could not read settings: " + err.Error())
	}
}

// change applies one setting, then reloads.
func change(key, value string) {
	applyShown("Saving…", "", "config", "set", key, value)
	loadSettings()
}

func settingsScreen() *mint.Screen {
	return &mint.Screen{
		Title: "Settings",
		Enter: func(*mint.Screen) { loadSettings() }, // shows the loading spinner if slow
		Sections: []mint.Section{{Name: "General", Rows: []mint.Row{
			mint.Toggle(mint.ToggleOpts{Label: "Verbose logging",
				Get: func() bool { return settings.Verbose },
				Set: func(v bool) { change("verbose", strconv.FormatBool(v)) }}),
			mint.Slider(mint.SliderOpts{Label: "Compression level", Min: 0, Max: 9, Step: 1,
				Get: func() int { return settings.Level },
				Set: func(v int) { change("level", strconv.Itoa(v)) }}),
			mint.Picker(mint.PickerOpts{Label: "Output format", Options: []string{"text", "json", "yaml"},
				Get: func() string { return settings.Format },
				Set: func(v string) { change("format", v) }}),
		}}},
	}
}
```

Each arrow press on a slider or picker applies the change at once. If applying is slow or disruptive, keep the edits in the struct instead and add an "Apply" action, as in A2.

### A2 Job

Options are kept in a struct and turned into arguments only when the user runs the job. `runner.JobScreen` is the standard running page: Stop, a progress bar, live output, and the job's state in the footer.

```go
var backup = struct {
	Dest     string
	Compress bool
	Verify   bool
	Threads  int
}{Dest: "/mnt/backup", Compress: true, Threads: 4}

func backupArgs() []string {
	args := []string{"backup", "--dest", backup.Dest, "--threads", strconv.Itoa(backup.Threads), "--progress"}
	if backup.Compress {
		args = append(args, "--compress")
	}
	if backup.Verify {
		args = append(args, "--verify")
	}
	return args
}

func backupScreen() *mint.Screen {
	return &mint.Screen{
		Title: "Backup",
		Sections: []mint.Section{
			{Name: "Options", Rows: []mint.Row{
				mint.Text(mint.TextOpts{Label: "Destination",
					Get: func() string { return backup.Dest }, Set: func(v string) { backup.Dest = v }}),
				mint.Checkbox(mint.CheckboxOpts{Label: "Compress",
					Get: func() bool { return backup.Compress }, Set: func(v bool) { backup.Compress = v }}),
				mint.Checkbox(mint.CheckboxOpts{Label: "Verify afterwards",
					Get: func() bool { return backup.Verify }, Set: func(v bool) { backup.Verify = v }}),
				mint.Stepper(mint.StepperOpts{Label: "Threads", Min: 1, Max: 32,
					Get: func() int { return backup.Threads }, Set: func(v int) { backup.Threads = v }}),
			}},
			{Rows: []mint.Row{
				mint.Action(mint.ActionOpts{Label: "Run backup", Hint: "run", OnActivate: func(c *mint.Ctx) {
					job := runner.Start(runner.Cmd{Name: tool, Args: backupArgs(),
						Progress: runner.Percent, Timeout: 6 * time.Hour})
					c.Push(runner.JobScreen("Backup", job))
				}}),
			}},
		},
	}
}
```

Choosing a progress parser:

- `runner.Percent` reads the last `NN%` on a line. It covers curl, wget, `rsync --info=progress2`, pv, and most others.
- `runner.Fraction` reads `12/80` or `3 of 7`.
- For anything else, write your own `func(line string) (int, bool)`.

If the program has no progress output at all, leave `Progress` unset. The job screen then shows elapsed time instead of a bar.

**Stopping.** Stop sends SIGTERM, then SIGKILL three seconds later. Some programs only tidy up on Ctrl+C (Python scripts, ffmpeg, many scanners), and leave a half-written result or a record stuck at "running" otherwise. Set `StopSignal` for those.

**Results and messages.** Many programs print their results on stdout and progress or log lines on stderr. Each line carries `Err` for stderr, so the two can be shown apart instead of in one `Log`.

```go
func scanScreen(target string) *mint.Screen {
	job := runner.Start(runner.Cmd{Name: tool, Args: []string{"scan", target},
		StopSignal: syscall.SIGINT, Timeout: 12 * time.Hour})
	pick := func(errs bool) func() []mint.LogLine {
		return func() []mint.LogLine {
			var out []mint.LogLine
			for _, l := range job.Lines() {
				if l.Err == errs {
					out = append(out, l)
				}
			}
			return out
		}
	}
	return &mint.Screen{
		Title: "Scan " + target,
		Sections: []mint.Section{{Rows: []mint.Row{
			mint.Action(mint.ActionOpts{Label: "Stop", Hint: "stop", OnActivate: func(c *mint.Ctx) { job.Stop() }}),
			mint.Log(mint.LogOpts{Label: "Results", Height: 10, Lines: pick(false)}),
			mint.Log(mint.LogOpts{Label: "Activity", Height: 5, Lines: pick(true)}),
		}}},
		Poll:   func(*mint.Screen) bool { return job.Changed() },
		Status: job.Badges,
	}
}
```

Filtering every line on each repaint is fine for a few thousand lines. For long runs, sort new lines into two slices from `Poll`, using `job.Total()` and `job.Tail(n)` to fetch only what is new.

### A3 Monitor

**On a timer.** `Poll` runs about ten times a second, so check the clock before reloading. `Refresh` rebuilds the rows from the latest snapshot before each draw.

```go
type service struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

var (
	services   []service
	lastLoaded time.Time
)

func loadServices() {
	var s []service
	if queryJSON(&s, "status", "--json") == nil {
		services = s
	}
	lastLoaded = time.Now()
}

func statusScreen() *mint.Screen {
	return &mint.Screen{
		Title: "Status",
		Enter: func(*mint.Screen) { loadServices() },
		Refresh: func(s *mint.Screen) {
			var rows []mint.Row
			for _, svc := range services {
				state := "stopped"
				if svc.Active {
					state = "running"
				}
				rows = append(rows, mint.Value(mint.ValueOpts{Label: svc.Name, Value: state}))
			}
			s.Sections = []mint.Section{{Name: "Services", Rows: rows}}
		},
		Poll: func(*mint.Screen) bool {
			if time.Since(lastLoaded) < 5*time.Second {
				return false
			}
			loadServices()
			return true
		},
		Status: func() []mint.Badge {
			down := 0
			for _, svc := range services {
				if !svc.Active {
					down++
				}
			}
			if down > 0 {
				return []mint.Badge{{Text: fmt.Sprintf("%d down", down), Kind: mint.BadgeWarn}}
			}
			return []mint.Badge{{Text: "all running", Kind: mint.BadgeOK}}
		},
	}
}
```

A reload inside `Poll` briefly pauses the interface while the command runs. That's fine for commands that answer in a few milliseconds. For slower ones, start a `runner` job from `Poll` and parse `job.Output()` once it is done.

**On events.** If the program can print a line whenever something changes (`monitor`, `events`, `--follow`, `watch`), use `eventcache`. Values are fetched again only when an event says they changed, which makes this the one place a row's `Get` may lead to running a command.

```go
var live = eventcache.New(eventcache.Options{
	Stream: tool + " monitor", // prints a line whenever something changes
	Fetchers: map[string]func() any{
		"volume": func() any { return query("get", "volume") },
	},
})

func liveScreen() *mint.Screen {
	return &mint.Screen{
		Title: "Live",
		Enter: func(*mint.Screen) { live.Start() },
		Sections: []mint.Section{{Rows: []mint.Row{
			mint.Value(mint.ValueOpts{Label: "Volume", Get: func() string {
				v, _ := live.Get("volume").(string)
				return v
			}}),
		}}},
		Poll: func(*mint.Screen) bool { return live.Poll() },
	}
}
```

### A4 Multi-command

One home item per command people use day to day, most used first. Leave out plumbing and debugging commands, or group them under a single "Advanced" item. Each item opens a screen of the shape that command has.

```go
func multiApp() *mint.App {
	return &mint.App{
		Title: "◈ " + tool,
		Crumb: tool,
		Items: []mint.HomeItem{
			{Icon: "◧", Label: "Settings", Desc: "View and change configuration", Open: settingsScreen},
			{Icon: "≡", Label: "Backup", Desc: "Back up now, with progress", Open: backupScreen},
			{Icon: "◷", Label: "Status", Desc: "Services and their health", Open: statusScreen},
		},
	}
}

func settingsScreen() *mint.Screen { return &mint.Screen{Title: "Settings"} } // A1
func backupScreen() *mint.Screen   { return &mint.Screen{Title: "Backup"} }   // A2
func statusScreen() *mint.Screen   { return &mint.Screen{Title: "Status"} }   // A3
```

### A5 Lookup

Input rows, a Run action, and the result underneath. A short result can go into Value rows; anything longer goes into a Log.

```go
var lookup = struct{ Host, Kind string }{Kind: "A"}

func lookupScreen() *mint.Screen {
	var result []mint.LogLine
	return &mint.Screen{
		Title: "Lookup",
		Sections: []mint.Section{{Name: "Query", Rows: []mint.Row{
			mint.Text(mint.TextOpts{Label: "Host name", Placeholder: "example.com",
				Get: func() string { return lookup.Host }, Set: func(v string) { lookup.Host = v }}),
			mint.Radio(mint.RadioOpts{Label: "Record type", Options: []string{"A", "AAAA", "MX", "TXT"},
				Get: func() string { return lookup.Kind }, Set: func(v string) { lookup.Kind = v }}),
			mint.Action(mint.ActionOpts{Label: "Look up", Hint: "run", OnActivate: func(c *mint.Ctx) {
				if lookup.Host == "" {
					c.ToastWarn("Enter a host name first")
					return
				}
				job := runner.Start(runner.Cmd{Name: tool, Args: []string{lookup.Host, lookup.Kind},
					Timeout: 30 * time.Second})
				c.Busy("Looking up "+lookup.Host+"…", job.Wait)
				result = job.Lines()
			}}),
			mint.Log(mint.LogOpts{Label: "Result", Height: 6, Lines: func() []mint.LogLine { return result }}),
		}}},
	}
}
```

## Part B: Options and actions

| In the program | Use | Notes |
|---|---|---|
| on/off flag that changes the system now | `Toggle` | A1 |
| on/off flag for the next run | `Checkbox` | A2 |
| one of 2–4 values | `Radio` | all choices visible at once |
| one of 5 or more values | `Picker` | |
| values that depend on the machine (disks, interfaces, profiles) | `Picker` with `OptionsFunc`; the `Choose` dialog for 20 or more | B1 |
| a number with a natural range, where roughly is fine | `Slider` with `Min`, `Max`, `Step`, `Unit` | A1 |
| an exact count, port or size | `Stepper` | A2 |
| a decimal | `Slider` or `Stepper` with `Format`, stored ×10 or ×100 | B2 |
| a path, name, URL or pattern | `Text` | A2 |
| a password, token or passphrase | the `Password` dialog, sent through the runner's input | B3 |
| a read-only fact | `Value`; `Label` for longer text; `Note` for one line of small print | `Note` is cut off at the frame; use `Label(StyleCaption, …)` for small print that may wrap |
| records the program keeps (scans, backups, containers) | one `Value` row each, with `OnActivate` opening a detail screen | B8 |
| many items to choose several from (plugins, modules) | `Checkbox` rows in collapsed sections, one per group, plus a search | B9 |
| a program that is a script, not a command | a helper that fills in the interpreter and folder | B7 |
| a quick action (under a second or so) | `Action` + `applyShown` | B5 |
| a slow action | `runner.Start` + `runner.JobScreen` | A2 |
| disruptive but reversible (restart, reload) | `Confirm` first | B5 |
| irreversible (delete, wipe, purge, `--force`) | `TypeToConfirm` | B4 |
| long or growing output | `Log`; `ViewLog` for the full text | A5 |
| options few people need | a collapsed "Advanced" section | B6 |
| the program's overall state | status badges | A3 |

### B1 Choices that depend on the machine

`OptionsFunc` runs every time the user steps through the picker, so if listing the choices is slow, cache the list.

```go
var iface string

func interfaces() []string { return strings.Fields(query("list-interfaces")) }

func interfaceRow() mint.Row {
	return mint.Picker(mint.PickerOpts{Label: "Interface", OptionsFunc: interfaces,
		Get: func() string { return iface }, Set: func(v string) { iface = v }})
}
```

For long lists, the `Choose` dialog is filterable by typing:

```go
var pkg string

func packageRow() mint.Row {
	return mint.Action(mint.ActionOpts{Label: "Package", Get: func() string { return pkg },
		OnActivate: func(c *mint.Ctx) {
			var all []string
			c.Busy("Loading packages…", func() { all = strings.Fields(query("list", "--names")) })
			if v, ok := c.Choose("Package", all, "Type to filter"); ok {
				pkg = v
			}
		}})
}
```

### B2 Decimals

Sliders and steppers hold whole numbers, so store the value scaled up and format it for display.

```go
var timeoutTenths = 25 // 2.5 seconds

func timeoutRow() mint.Row {
	return mint.Slider(mint.SliderOpts{Label: "Timeout", Min: 1, Max: 100, Step: 5,
		Format: func(v int) string { return fmt.Sprintf("%.1f s", float64(v)/10) },
		Get:    func() int { return timeoutTenths }, Set: func(v int) { timeoutTenths = v }})
}

func timeoutArg() string { return fmt.Sprintf("%.1f", float64(timeoutTenths)/10) }
```

### B3 Secrets

Ask with the `Password` dialog and pass the answer on the program's standard input, never as an argument: arguments are visible to every user on the machine (`ps`). Look for flags like `--password-stdin`, `--passphrase-fd 0`, or `sudo -S`.

```go
func signInRow() mint.Row {
	return mint.Action(mint.ActionOpts{Label: "Sign in", Hint: "sign in", OnActivate: func(c *mint.Ctx) {
		user, ok := c.Prompt("User name", "", "")
		if !ok {
			return
		}
		pass, ok := c.Password("Password", "For "+user)
		if !ok {
			return
		}
		job := runner.Start(runner.Cmd{Name: tool,
			Args: []string{"login", "--username", user, "--password-stdin"}, Input: true})
		job.SendLine(pass)
		job.CloseInput()
		c.Busy("Signing in…", job.Wait)
		if job.State() == runner.Done {
			c.Toast("Signed in as " + user)
		} else {
			c.ToastError(lastLine(job))
		}
	}})
}

// lastLine is the job's final non-empty line of output, usually the error.
func lastLine(j *runner.Job) string {
	lines := j.Lines()
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Text != "" {
			return lines[i].Text
		}
	}
	return j.Badge().Text
}
```

### B4 Irreversible actions

Make the user type the name of the thing being destroyed, and pass the program's own "yes" flag, because it has no terminal to ask on.

```go
func deleteRow(name string) mint.Row {
	return mint.Action(mint.ActionOpts{Label: "Delete " + name, Hint: "delete", OnActivate: func(c *mint.Ctx) {
		if !c.TypeToConfirm("Delete "+name, "This permanently deletes "+name+" and everything in it.", name) {
			return
		}
		if applyShown("Deleting…", "Deleted "+name, "delete", "--yes", name) {
			c.Pop()
		}
	}})
}
```

### B5 Quick and disruptive actions

```go
func reloadRow() mint.Row {
	return mint.Action(mint.ActionOpts{Label: "Reload configuration", Hint: "reload", OnActivate: func(c *mint.Ctx) {
		applyShown("Reloading…", "Reloaded", "reload")
	}})
}

func restartRow() mint.Row {
	return mint.Action(mint.ActionOpts{Label: "Restart service", Hint: "restart", OnActivate: func(c *mint.Ctx) {
		if c.Confirm("Restart the service?", "Connected clients will drop for a few seconds.") {
			applyShown("Restarting…", "Restarted", "restart")
		}
	}})
}
```

### B6 Advanced options

Expose the options most people use. Put the rest in a section that starts folded, rather than leaving them out or crowding the screen.

```go
var dryRun, keepPartial bool

func advancedSection() mint.Section {
	return mint.Section{Name: "Advanced", Collapsible: true, Collapsed: true, Rows: []mint.Row{
		mint.Checkbox(mint.CheckboxOpts{Label: "Dry run (show what would change)",
			Get: func() bool { return dryRun }, Set: func(v bool) { dryRun = v }}),
		mint.Checkbox(mint.CheckboxOpts{Label: "Keep partially transferred files",
			Get: func() bool { return keepPartial }, Set: func(v bool) { keepPartial = v }}),
	}}
}
```

### B7 Programs that aren't one command

Some programs are a script run from their own folder with a particular interpreter (`python3 sf.py` inside a checkout, with its own venv). Find the folder once (an environment variable, the script on PATH, the usual install places, or a setting the user can change), then build every command through one function instead of using `tool`.

```go
var home, python = "/opt/scanner", "/opt/scanner/venv/bin/python"

func scriptCmd(args ...string) runner.Cmd {
	return runner.Cmd{Name: python, Args: append([]string{"scan.py"}, args...), Dir: home}
}

func scriptQuery(args ...string) string {
	cmd := exec.Command(python, append([]string{"scan.py"}, args...)...)
	cmd.Dir = home
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}
```

`App.Require` can't check for such a program, so check in `main` and say what's missing in the app's `Note`, with a Settings screen to fix it.

If the program keeps its data in a database and has no command to export it, don't read the database yourself: its schema changes between versions. Embed a short helper script (`//go:embed helper.py`) that reads the data through the program's own code and prints JSON, write it out once, and run it with the program's interpreter.

### B8 Records

Rebuild the list in `Refresh` from a slice loaded in `Enter`. Put the record's state in the row's value, and open a detail screen on Enter. Hints are short, so use `Value` when the right-hand text is more than a word.

```go
type record struct{ ID, Name, State string }

var records []record

func recordsScreen() *mint.Screen {
	return &mint.Screen{
		Title: "Backups",
		Enter: func(*mint.Screen) { _ = queryJSON(&records, "list", "--json") },
		Refresh: func(s *mint.Screen) {
			var rows []mint.Row
			for _, r := range records {
				r := r
				rows = append(rows, mint.Value(mint.ValueOpts{ID: r.ID, Label: r.Name, Value: r.State,
					OnActivate: func(c *mint.Ctx) {
						c.Open(r.Name, func() *mint.Screen { return recordScreen(r) })
					}}))
			}
			if len(rows) == 0 {
				rows = append(rows, mint.Label(mint.StyleBody, "Nothing here yet."))
			}
			s.Sections = []mint.Section{{Name: fmt.Sprintf("Backups (%d)", len(records)), Rows: rows}}
		},
	}
}

func recordScreen(r record) *mint.Screen {
	return &mint.Screen{Title: r.Name, Sections: []mint.Section{{Rows: []mint.Row{
		mint.Value(mint.ValueOpts{Label: "State", Value: r.State}),
	}}}}
}
```

Give each row an `ID` so the selection stays on the same record when the list reloads. After deleting a record, reload the list before `Pop`.

### B9 Choosing several from many

Group the items, one collapsed section per group with the count in its name, so a long list opens as a short one. Add a search with `ChooseItem` for people who know what they want. Keep the choices in a map, so they survive the rows being rebuilt.

```go
type plugin struct{ ID, Name, Group string }

var (
	plugins []plugin
	picked  = map[string]bool{}
)

func pluginSections() []mint.Section {
	groups := map[string][]mint.Row{}
	var order []string
	for _, p := range plugins {
		id := p.ID
		if groups[p.Group] == nil {
			order = append(order, p.Group)
		}
		groups[p.Group] = append(groups[p.Group], mint.Checkbox(mint.CheckboxOpts{ID: id, Label: p.Name,
			Get: func() bool { return picked[id] }, Set: func(v bool) { picked[id] = v }}))
	}
	var out []mint.Section
	for _, g := range order {
		out = append(out, mint.Section{Name: fmt.Sprintf("%s (%d)", g, len(groups[g])),
			Collapsible: true, Collapsed: true, Rows: groups[g]})
	}
	return out
}
```

### B10 Collapsible sections & large text chunks (`mint.Collapsible`)

When a screen has inline option groups or a command produces hundreds or thousands of lines of text, fold them with `mint.Collapsible` or `mint.CollapsibleText` so the screen stays compact and fast. Setting `ChunkSize` splits large text into nested foldable chunks (`Lines 1–100`, `Lines 101–200`, …) plus an action to open the full output in `mint.ViewLog`.

```go
var showAdvanced bool

func collapsibleExample(lines []string) []mint.Row {
	return []mint.Row{
		mint.Collapsible(mint.CollapsibleOpts{
			ID:    "adv",
			Title: "Advanced Options",
			Open:  &showAdvanced,
			Rows: []mint.Row{
				mint.Note("Rarely needed flags live here."),
			},
		}),
		mint.CollapsibleLines("Captured Output", func() []string { return lines }, 100),
	}
}
```

### B11 Formatted tables (`mint.Table` and `mint.ParseTable`)

Use `mint.Table` to render multi-column data as aligned, navigable Mint rows, and `mint.ParseTable` (or `mint.ParseColumns`) to turn whitespace-, tab-, pipe-, or box-drawing-aligned CLI output (`ps`, `df`, `ss`, `ls -l`, `nmap`) directly into table headers and rows.

```go
func tableExample(rawLines []string) mint.Row {
	headers, rows := mint.ParseTable(rawLines)
	return mint.Table(mint.TableOpts{
		ID:      "ports",
		Headers: headers,
		Rows:    rows,
		Empty:   "No rows parsed.",
		OnSelect: func(ctx *mint.Ctx, idx int, row []string) {
			ctx.Toast(strings.Join(row, " · "))
		},
	})
}
```

## Part E: Automated Scaffolding with `mint/appkit` and `tools/mint-gen` (6 Archetypes · 8 Mixins · 28 Templates)

For rapid wrapping of compatible CLI programs, `mint/appkit` provides a declarative runtime built on **6 Base Archetypes**, **8 Composable Mixins**, and **28 Template Types (`T01`–`T28`)**:

- **6 Base Archetypes (`appkit.Archetype`)**:
  - `ArchetypeConfig` (`"config"` — State Editor: reads live state, renders controls/rules, applies changes).
  - `ArchetypeRunner` (`"runner"` — Task & Pipeline: target + flags + presets $\rightarrow$ background `runner.Job` with `Progress` bar $\rightarrow$ output + history).
  - `ArchetypeScanner` (`"scanner"` — Target $\rightarrow$ Live Table + Log: target/interface + preset $\rightarrow$ live parsed `mint.Table` alongside collapsible raw logs).
  - `ArchetypeMonitor` (`"monitor"` — Live Poller & Controller: periodic `Poll` command $\rightarrow$ gauges & live `mint.Table` $\rightarrow$ per-row actions).
  - `ArchetypeExplorer` (`"explorer"` — Query $\rightarrow$ Results $\rightarrow$ Detail: search/query bar $\rightarrow$ parsed `mint.Table` or `mint.Collapsible` sections).
  - `ArchetypeSuite` (`"suite"` — Multi-Subcommand Manager: top `SubcommandSpec` selector where each mode runs its own subcommand, table view, or action form).
- **8 Composable Mixins (`appkit.Mixins`)**:
  - `WithFileOutput` (`OutputsFile`, `OutputFlag`, `DefaultExt`), `WithTableOutput` (`TableOutput`, `TableColumns`), `WithCollapsibleOutput` (`CollapsibleOutput`, `ChunkSize`), `WithProfiles` (`Profiles`), `WithDynamicChoices` (`FlagSpec.ChoicesCmd`), `WithRootAuth` (`NeedsRoot`), `WithConfirm` (`ConfirmRun`), and `WithHistory` (`KeepHistory`).
- **28 Template Types (`T01`–`T28`)**:
  - Setting `Template: "T12"` (or `"network-scanner"`) on `appkit.ToolSpec` automatically applies its archetype, mixins, target labels, subcommands, and default profiles (see `templates/README.md` or run `go run ./tools/mint-gen --list-templates`).

```go
func appkitExample() appkit.ToolSpec {
	return appkit.ToolSpec{
		Name:              "nmap",
		Title:             "◈ Nmap",
		Desc:              "Network exploration tool and security / port scanner",
		Template:          "T12",
		Archetype:         appkit.ArchetypeScanner,
		TargetLabel:       "Target host / network",
		OutputsFile:       true,
		OutputFlag:        "-oN",
		DefaultExt:        "txt",
		TableOutput:       true,
		CollapsibleOutput: true,
		NeedsRoot:         true,
		KeepHistory:       true,
		Flags: []appkit.FlagSpec{
			{Flag: "-sV", Kind: appkit.FlagBool, Section: "Scan Techniques", Label: "Probe open ports for service/version info"},
			{Flag: "-p", Kind: appkit.FlagText, Section: "Port Specification", Label: "Port ranges", Hint: "1-1024"},
		},
	}
}
```

`tools/mint-gen` checks whether a target binary can run inside a pure detached TUI (`--check <program>`), auto-classifies any CLI tool into `T01`–`T28` (`DetectTemplate`, overridable via `--template <T01..T28|slug>`), and parses its `man` page and `--help` output into a standalone `apps/<tool>/` module (`go.mod`, `main.go`, `spec.go`, `<tool>-mint.el`).

## Part C: Rules

1. **Never run a command inside a row's `Get`.** Getters run on every repaint, many times a second while anything is moving. Load values in `Enter`, after a change, or from `Poll` on a timer, and read them from variables. `eventcache` is the one exception, because it only fetches again after an event.
2. **Prefer machine-readable output.** Use `--json` (or `--porcelain`, `--format`, `-o json`) and `queryJSON`. Parsing human-readable text breaks when a new version rewords it. If there is no such output but the program has a library or API, read through it with an embedded helper (B7) rather than reading its files directly.
3. **Pass arguments as data.** Use `query`, `apply`, `util.Run` and `runner.Cmd{Name, Args}`, which never go through a shell. Use `runner.Cmd{Shell}` only for fixed command lines, and quote anything the user typed with `util.ShQuote`.
4. **Secrets go through standard input** (B3), never into arguments or environment variables.
5. **Irreversible actions need `TypeToConfirm`** with a phrase naming the target (B4). Disruptive but reversible ones need `Confirm`, which defaults to No.
6. **Nothing may wait for an answer.** Jobs have no terminal, so a program that stops to ask "[y/N]" will hang until its timeout. Pass `-y`, `--yes`, `--assume-yes`, `--non-interactive`, `--batch` or `--no-input`, or answer through `Input` and `SendLine`.
7. **Every job gets a timeout and a way to stop.** `runner.JobScreen` provides Stop, and the starter calls `runner.StopAll()` on exit. If the program only cleans up on Ctrl+C, set `StopSignal: syscall.SIGINT` (A2). A background service may have no timeout, but needs an off switch.
8. **Report failures with the program's own words.** `apply` returns the program's last message line; show it with `mint.ToastError`. After any change, reload, so the screen shows what the program actually did.
9. **Label things in plain words**, such as "Show hidden files" rather than `--all`. The flag can go in a `Note` underneath if it helps.
10. **Expose the common tasks, not every flag.** Rarely used options go in a collapsed "Advanced" section (B6), and plumbing commands are left out.
11. **Declare what the interface needs.** List every program it runs in `App.Require` (missing ones are named on the home list; for a script, see B7), and set `App.Version` so `--version` and `--mint-info` report it. Note the wrapped program's version in the app's `Note` or a status badge.
12. **Use single-width icons**, such as `◆ ▤ ◧ ◷ ≡`. Emoji take two columns and push the border out of line.

## Part D: Assembling an interface

1. Copy `templates/app` to a new folder and set `tool` in `cli.go`. For Emacs users, rename `app.el` to `<tool>-mint.el` and replace `mytool` in it.
2. Give the new folder its own module and point it at the hub:

   ```
   go mod init example.com/mytool-ui
   go get github.com/CurtisD4q/Mint-Interfaces@latest
   ```

   Until the hub is published, add `replace github.com/CurtisD4q/Mint-Interfaces => ../path/to/hub` to the new `go.mod` instead.
3. Replace the example home item with the program's screens (Part A), one file per screen group, built from the components in Part B.
4. Check the result against the rules in Part C.
5. `go build`, then open every screen and try every action before handing it over.
