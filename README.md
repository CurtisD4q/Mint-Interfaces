# Mint Interfaces (Go)

A zero-dependency terminal UI library, ported from the Lua original ([mint-interfaces-legacy](https://github.com/CurtisD4q/mint-interfaces-legacy)).

The library is the generic interface layer from [Terminal Settings Manager](https://github.com/CurtisD4q/Terminal-Settings-Manager) by Curtis (Apache-2.0). It draws a rounded purple frame with a title on the left and a breadcrumb on the right. Inside are a home list (icon, bold title, dim description), a selection bar, value rows, sliders, `‹ ›` pickers, toggles, actions, section headers, a scrollbar, and a key-hint footer. Keyboard and mouse both work: click selects, double-click opens, and the wheel scrolls. Redraws are dirty-gated, so an idle screen does not repaint.

On top of the original, it adds dividers and spacing, text styles, collapsible sections, slider ranges and units, progress bars, number steppers, checkboxes, radio groups, toasts, status badges, a loading spinner, background jobs with live output, and a catalogue for building interfaces for command-line programs. See [Additions](#additions). Every app also gets standard command-line flags, emacs and vim key profiles, and can be launched from Emacs, matching its theme. See [Standalone binaries and flags](#standalone-binaries-and-flags), [Keys](#keys) and [Running from Emacs](#running-from-emacs-emacsmintel).

There is no settings logic and no system backend in the library. An app describes screens as structs and handles its own data.

It uses only Go's standard library: no Charm, no tcell, and no `golang.org/x/...`. See [Prerequisites](#prerequisites) for what building, running and the Emacs integration need.

```
go run ./examples/example   # the program below
go run ./examples/demo      # every original widget and dialog
go run ./examples/features  # the additions, including a background job
```

## Quickstart: Wrap any CLI program in 3 steps

You can generate and compile a full Mint interface for any installed command-line program in three steps (see [Turnkey CLI apps and automated scaffolding](#turnkey-cli-apps-mintappkit-and-automated-scaffolding-toolsmint-gen) and [Standalone binaries and flags](#standalone-binaries-and-flags) below for full details):

1. **Check compatibility (optional):** Verify the program works in a pure TUI and inspect or cache your terminal's capability profile in `~/.config/mint-interfaces/termcaps.json` (`--list-templates` lists all 28 templates; `--probe-term` refreshes the terminal probe):
   ```sh
   go run ./tools/mint-gen --check nmap
   ```
2. **Generate and compile in one command:** Parse `man` + `--help`, auto-select a template (`T01`–`T28`), scaffold `apps/<tool>/`, and build the static binary:
   ```sh
   go run ./tools/mint-gen --build nmap
   ```
3. **Run your new interface:** Launch the standalone binary in your terminal (or via `M-x nmap-mint` in [Emacs](#running-from-emacs-emacsmintel)):
   ```sh
   ./apps/nmap/nmap-mint
   ```

### Global install: Run `mint-gen` from anywhere

Move the `mint-interfaces` folder to `/usr/local/share/mint-interfaces` and install `/usr/local/bin/mint-gen` so you can check, build, and run any tool's interface from any directory:

1. Move the repository to `/usr/local/share/mint-interfaces` and make `apps/` writable by your user so builds do not need `sudo`:
   ```sh
   sudo mv mint-interfaces /usr/local/share/mint-interfaces
   sudo mkdir -p /usr/local/share/mint-interfaces/apps
   sudo chown -R "$USER:$USER" /usr/local/share/mint-interfaces/apps
   ```
2. Install `/usr/local/bin/mint-gen` to invoke `mint-gen` from `/usr/local/share/mint-interfaces`:
   ```sh
   sudo tee /usr/local/bin/mint-gen >/dev/null << 'EOF'
   #!/usr/bin/env sh
   exec go -C /usr/local/share/mint-interfaces run ./tools/mint-gen "$@"
   EOF
   sudo chmod +x /usr/local/bin/mint-gen
   ```

Once installed, the steps from any directory are:

1. **Check compatibility (optional):** Verify the program works in a pure TUI and cache your terminal's capability profile.
   ```sh
   mint-gen --check nmap
   ```
2. **Generate and compile:** Parse `man` and `--help`, scaffold `apps/nmap/`, and compile the static binary.
   ```sh
   mint-gen --build nmap
   ```
3. **Run your new interface:** Launch the compiled `nmap` interface (building it first if needed).
   ```sh
   mint-gen --run nmap
   ```
4. **Remove a build (optional):** Delete the generated `apps/nmap/` folder and compiled binary.
   ```sh
   mint-gen --remove nmap
   ```
5. **List your apps:** Show the apps in the repository's `apps/` folder, one per line.
   ```sh
   mint-gen --list
   ```
6. **See what mint-gen understood (optional):** which template it chose, how sure it is, the archetype and the evidence that decided it, what the program reads and writes, where "Save output to file" will send the file, and the evidence behind every option's role.
   ```sh
   mint-gen --explain ffmpeg
   ```

## Prerequisites

**To run a Mint app:** nothing to install. Apps are single static binaries.

- **Systems:** Linux, macOS and the BSDs. Windows is not supported natively; apps work inside WSL (Windows Subsystem for Linux), for example in Windows Terminal.
- **Terminal:** any terminal with 256 colours and SGR mouse reporting, which is any modern terminal (GNOME Terminal, Konsole, kitty, Alacritty, iTerm2, Windows Terminal, tmux). 24-bit colour is used only when a theme asks for it and `$COLORTERM` is `truecolor` or `24bit`; if `$COLORTERM` is unset, RGB colours fall back to their nearest 256-colour approximation.
- **Font:** one with the box-drawing and symbol characters Mint draws (`╭─╮ ━ ● ▸ ☑ ◉`). Most monospace fonts have them.
- **Locale:** a UTF-8 locale, which is the default almost everywhere.
- **Programs the app wraps:** whatever it lists in `Require`. Missing ones are named on its home screen.

**To build apps:**

- **Go 1.21 or newer.** No C compiler or other tools: cross-compiling for another system or processor is just a setting.
- **Python 3**, only for `tools/check_catalogue.py`.

**For the Emacs integration (`emacs/mint.el`):**

- **Emacs 27.1 or newer.** Tested on Emacs 29.3.
- **Terminal Emacs** (`emacs -nw`) for the recommended handover mode. Graphical Emacs falls back to the built-in `term` mode, with the limits listed under [Limitations in Emacs](#limitations-in-emacs).
- **Go**, only if you use `M-x mint-run-source` or point `mint-define-app` at an app's source folder rather than a compiled binary.
- Finding the terminal device for handover uses `/proc` on Linux and `ps` on macOS and the BSDs.

## Build a screen

Import `mint`, describe a home list in a `mint.App`, and call `Run`. A home item's `Open` returns a `*mint.Screen`.

```go
package main

import (
	"fmt"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

var brightness = 60

func displayScreen() *mint.Screen {
	return &mint.Screen{
		Icon:  "▭",
		Title: "Display",
		Sections: []mint.Section{
			{Name: "Brightness", Rows: []mint.Row{
				mint.Note("How bright the panel should look."),
				mint.Slider(mint.SliderOpts{
					Label: "Adjust",
					Get:   func() int { return brightness },
					Set:   func(v int) { brightness = v },
				}),
				mint.Value(mint.ValueOpts{
					Label: "Level",
					Get:   func() string { return fmt.Sprintf("%d  (default 60)", brightness) },
				}),
				mint.Action(mint.ActionOpts{
					Label:      "Reset to default",
					Hint:       "reset",
					OnActivate: func(*mint.Ctx) { brightness = 60 },
				}),
			}},
		},
	}
}

func main() {
	app := &mint.App{
		Title: "◈ Example",
		Crumb: "Example", // breadcrumb shown on inner screens
		Note:  "Optional line under the home title.",
		Items: []mint.HomeItem{
			{Icon: "▭", Label: "Display", Desc: "Brightness", Open: displayScreen},
		},
	}
	app.Run()
}
```

`Sections` is a list of `{Name, Rows}`. The heading style the settings manager uses (`┄ Heading`) is drawn for you. Returning a screen from `Open` pushes it, `b` pops it, and `q` quits.

From a row callback, `ctx` can open another screen or a dialog:

```go
mint.Action(mint.ActionOpts{
	Label: "Rename",
	Hint:  "edit",
	OnActivate: func(ctx *mint.Ctx) {
		name, ok := ctx.Prompt("Name", "Shown in the title.", "untitled")
		if ok && ctx.Confirm("Use this name?", name) {
			ctx.Push(otherScreen(name))
		}
	},
})
```

Dialogs: `ctx.Confirm`, `ctx.Message`, `ctx.Prompt`, `ctx.Choose`, `ctx.ChooseItem`, `ctx.Password` and `ctx.TypeToConfirm`. They use the same frame. `ctx.Push`, `ctx.Pop` and `ctx.Quit` change screens.

### Row types

| Constructor | What it draws | Keys |
|---|---|---|
| `mint.Value(mint.ValueOpts{Label, Value})` or `Get` | Label on the left, muted value on the right | Enter runs `OnActivate` when set |
| `mint.Slider(mint.SliderOpts{Label, Get, Set})` | `━━━━●──── 60%`, range 0–100 unless set (see [More controls](#more-controls)) | ←→ by `Step` (default 5), click the track to jump |
| `mint.Picker(mint.PickerOpts{Label, Options, Get, Set})` | `‹ value ›` | ←→ cycles, Enter steps forward |
| `mint.Toggle(mint.ToggleOpts{Label, Get, Set})` | `● ON` or `OFF` | Space flips, → on, ← off |
| `mint.Action(mint.ActionOpts{Label, Hint, OnActivate})` | Hint on the right (`reset`, `save`, `open`) | Enter or double-click runs it |
| `mint.Text(mint.TextOpts{Label, Get, Set, Placeholder})` | Editable value | Enter edits, Enter saves, Esc cancels |
| `mint.Note("…")` | Dim line, not selectable | |
| `mint.Heading("…")` | Section header inside a row list | |
| `mint.Progress(mint.ProgressOpts{Label, Get})` | `━━━━━─────── 42%`, read-only | not selectable |
| `mint.Stepper(mint.StepperOpts{Label, Get, Set, Min, Max, Step})` | `− 8 +` | ←→ step, Enter types a number, click − or + |
| `mint.Checkbox(mint.CheckboxOpts{Label, Get, Set})` | `☑ Label` or `☐ Label` | Space flips, → on, ← off, click the box |
| `mint.Radio(mint.RadioOpts{Label, Options, Get, Set})` | one `◉`/`○` row per option | Space, Enter or → chooses, click the circle |
| `mint.Label(style, "…")` | Wrapped text in an emphasis style | not selectable |
| `mint.Divider()` / `mint.Divider("-")` | A line across the content | not selectable |
| `mint.Spacer()` | A blank line | not selectable |

`Get` and `Set` are ordinary functions. A slider's `Get` returns `0`–`100`. A picker's `Set` receives the next option string. Pass `Options` (or `OptionsFunc` for a list that changes), or leave them out and handle the raw `-1` / `0` / `1` direction yourself in `Step`. Every row also takes an optional `ID`.

If a `Get` panics, the row shows the message in amber, or the last good value if there was one. Panics in `Set`, `OnActivate`, `Enter` and `Poll` are recovered, so they never crash the app.

Override the footer with `Hints` on a screen, or `ScreenHints` on the app. The defaults match the settings manager: `↑↓ move ⏎ open q quit` on the home list, and `↑↓ move ←→/Space change b home q quit` on a screen. With the emacs or vim [key profile](#keys) they show that profile's keys instead, unless the app has set its own. They are also exported as `mint.HomeHints`, `mint.ScreenHints` and `mint.EditHints`.

### Live screens

`Refresh(screen)` runs before each draw and may replace `screen.Sections`. If it panics, the error appears in the screen's `Banner`. `Poll(screen)` runs on idle (about ten times a second) and should return `true` when the next frame must be painted. Keep `Poll` non-blocking: do the slow work in another process or goroutine, and only read a file or a variable here. `eventcache` is the same pattern the settings manager uses for background streams: start a command, watch its output, and mark slices dirty.

`Enter(screen)` runs once each time the screen is shown.

### Colour

The built-in palette is the original purple: 256-colour index 141 for the frame and titles, and a dim selection bar. `theme.Names` lists `default`, `terminal`, `nord`, `gruvbox`, `dracula`, `catppuccin` and `custom`. `mint.SetPalette(theme.Load(name))` applies one immediately. `theme.Set(name)` writes `~/.config/mint-interfaces/theme.conf`, the same file mint-interfaces-legacy reads, so both share a theme.

Truecolor (`CSI 38;2;r;g;b m` / `CSI 48;2;r;g;b m`) is gated on both the theme and the terminal: it is emitted only when a theme (or `MINT_PALETTE`) specifies RGB values **and** `$COLORTERM` is `truecolor` or `24bit`. When `$COLORTERM` is unset, any RGB colour falls back to the nearest 256-colour approximation (`CSI 38;5;n m` / `CSI 48;5;n m`).

The `MINT_PALETTE` environment variable overrides individual colours for one run, for example `MINT_PALETTE="accent=#ff5f87,selbg=236"`. Each value is a `#rrggbb` colour or a 256-colour number, and the roles are `accent`, `bright`, `text`, `muted`, `on`, `amber`, `err` and `selbg`. This is how Emacs passes its theme to the apps it launches.

### Layout of the library

```
mint/doc.go                    package docs
mint/home.go                   App, the home list, the screen stack, flags
mint/screen.go                 Screen, Ctx, row widgets, editing, mouse
mint/rows.go                   row constructors
mint/dialogs.go                confirm, message, choose, prompt, password
mint/footer.go                 status badges, toasts, the loading spinner
mint/log.go                    the output view and full-screen viewer
mint/keys.go                   key profiles (default, emacs, vim)
mint/render.go                 palette, frame, title, footer, scrollbar, wrapping
mint/term.go                   raw mode, terminal size, key and mouse decoding
mint/term_linux.go, term_bsd.go    the termios ioctls per platform
mint/theme/                    palettes and MINT_PALETTE
mint/util/                     quoting, process spawn, JSON, width helpers
mint/secure/                   secure store for passwords and keys while an app runs
mint/runner/                   background jobs, JobScreen
mint/eventcache/               values refreshed by a stream of events
mint/collapsible.go            inline foldable sections and large-text chunking
mint/table.go                  aligned multi-column tables and CLI table parser
mint/appkit/                   turnkey 4-screen CLI wrapper and output directory browser
templates/app/                 starter for a new interface (main.go, cli.go, app.el)
tools/mint-gen/                man-page & --help interface generator + TUI exclusion checker
emacs/mint.el                  launching apps from Emacs
emacs/mint-tests.el            tests for mint.el
CATALOGUE.md                   how to build an interface for a program
tools/check_catalogue.py       compiles every catalogue example
tools/build.sh                 builds a standalone static binary (also for apps with their own go.mod)
```

`mint/util` is there so apps can shell out safely and encode JSON without another dependency. `util.Run(program, args...)` passes each argument as data, so no shell ever parses it. `util.RunTimeout(limit, program, args...)` does the same with a time limit, detached from the terminal; `util.StopAll()` ends any still running when the app closes. It is not a settings backend.

## Additions

All of them are off unless you use them, so existing screens look exactly as before. `examples/features` shows every one.

### Dividers, spacing and text styles

`mint.Divider()` draws a dim line across the content area. Pass another character for a different line: `mint.Divider("-")` gives `-----`, and `mint.Divider("┄")` a dotted rule. `mint.Spacer()` is one blank line.

For a gap between every section, set `SectionSpacing` on the `App` (it applies to every screen) or on a single `Screen`. The value is the number of blank lines, and a negative value turns it off for one screen.

A terminal can't change font size, so `mint.Label` gives text a hierarchy through colour and weight instead. Labels wrap across as many lines as they need.

| Style | Looks like |
|---|---|
| `mint.StyleTitle` | accent colour, bold |
| `mint.StyleHeading` | bright, bold |
| `mint.StyleBody` | normal text |
| `mint.StyleCaption` | muted |
| `mint.StyleItalic` | italic |
| `mint.StyleUnderline` | underlined |

### Collapsible sections

Set `Collapsible: true` on a `Section`. Its heading becomes `▾ Name`, and the cursor can stop on it. Enter, Space or a click on the arrow folds the section away to `▸ Name` with a count such as `3 hidden` on the right. → always expands and ← always collapses. `Collapsed: true` makes a section start folded. After that, the screen remembers what the user chose, even when `Refresh` rebuilds the sections.

```go
{Name: "Advanced", Collapsible: true, Collapsed: true, Rows: []mint.Row{...}}
```

### More controls

**Sliders** take a range, a step and a unit. With none of them set, a slider behaves exactly as before (0–100, steps of 5, shown as a percentage).

```go
mint.Slider(mint.SliderOpts{Label: "Gain", Min: -24, Max: 12, Step: 3, Unit: "dB", Get: get, Set: set})
// ━━━━━━●────── -6 dB
```

`Format` gives you full control of the number, which is how to show decimals: store tenths as an `int` and format them.

```go
Format: func(v int) string { return fmt.Sprintf("%.1f GHz", float64(v)/10) }
```

**Progress bars** are read-only and use the same style as sliders: `mint.Progress(mint.ProgressOpts{Label: "Download", Get: percent})`. Return `true` from the screen's `Poll` while the work runs so the bar repaints.

**Steppers** are for exact whole numbers: `− 8 +`. ←→ change the value by `Step` (default 1), and clicking `−` or `+` does the same. Enter lets you type a value; only digits and a leading minus sign are accepted, and the result is clamped to `Min`–`Max`. Leave `Min` and `Max` at zero for no limits. At a limit, that end's `−` or `+` turns dim.

**Checkboxes** (`☑`/`☐`) are independent on/off rows; use several for a multi-select list. A **radio group** expands into one row per option (`◉` chosen, `○` not), with an optional caption above them. For both, clicking the box or circle acts immediately, and double-clicking anywhere on the row does too.

### Footer feedback

The left side of the footer is used for, in order of priority: a loading spinner, then a toast, then status badges. Whatever doesn't fit beside the key hints is left out.

**Loading spinner.** When a home item's `Open` or a screen's `Enter` takes longer than 150 ms, the footer shows `⠙ Loading Display…` until it finishes. Faster page changes show nothing, so there's no flicker. From a row callback, use:

```go
ctx.Open("Diagnostics", func() *mint.Screen { return buildSlowScreen() }) // spinner, then push
ok := ctx.Busy("Saving…", func() { save() })                            // spinner for any slow work
```

The function you pass runs on a background goroutine while the spinner animates, so it must not open dialogs. Keys pressed during loading are ignored. If the function panics, the panic message appears as an error toast. Tune the delay with `mint.LoadingDelay`.

**Toasts** flash a short message for two seconds (`mint.ToastDuration`): `ctx.Toast("Saved")` shows `✓ Saved`, `ctx.ToastWarn(...)` shows an amber `!`, and `ctx.ToastError(...)` a red `✕`. `mint.Toast` and its siblings are safe to call from any goroutine.

**Status badges** are short live indicators such as `● running` or `○ v2.1`. Return them from `App.Status`, which covers the home list and every screen, or from `Screen.Status`, which overrides it for that screen.

```go
Status: func() []mint.Badge {
	if running {
		return []mint.Badge{{Text: "service running", Kind: mint.BadgeOK}}
	}
	return []mint.Badge{{Text: "service stopped", Kind: mint.BadgeError}}
},
```

The kinds are `BadgeOK` (green), `BadgeWarn` (amber), `BadgeError` (red), `BadgeInfo` (accent) and `BadgeIdle` (muted `○`). The footer checks for changes about ten times a second and repaints only when a badge changes, so keep `Status` cheap: read a variable rather than running a command. Themes now include an optional red `Err` colour role; palettes without one use amber for errors.

### Running programs in the background

`mint/runner` runs a command-line program without freezing the interface. It captures the output line by line, tracks progress, state and elapsed time, and stops cleanly.

```go
job := runner.Start(runner.Cmd{
	Name:     "rsync",
	Args:     []string{"-a", "--info=progress2", src, dst},
	Progress: runner.Percent, // reads "45%" from the output
	Timeout:  time.Hour,
})
ctx.Push(runner.JobScreen("Sync", job)) // ready-made page: Stop, progress bar, live output, state in the footer
```

Or wire the pieces into your own screen:

```go
mint.Progress(mint.ProgressOpts{Label: "Copying", Get: job.Percent}),
mint.Log(mint.LogOpts{Label: "Output", Lines: job.Lines, Height: 8}),
Poll:   func(*mint.Screen) bool { return job.Changed() },
Status: job.Badges, // ● rsync 45% · 0:42 → ● rsync done in 1:03 / failed (exit 23) / stopped
```

What the runner takes care of:

- **Stopping everything.** `job.Stop()` and `Timeout` end the program *and every process it started*: SIGTERM first (or `Cmd.StopSignal`, such as `syscall.SIGINT` for programs that only clean up on Ctrl+C), then SIGKILL three seconds later for anything still alive. Call `runner.StopAll()` after `app.Run()` so nothing outlives the interface.
- **Progress lines that redraw in place.** Programs like curl and rsync rewrite one line with a carriage return. The runner keeps that as a single updating line, shows each update immediately, and strips colour codes and other control characters.
- **No terminal access.** Each job runs detached from the terminal, so it can never read the keyboard or draw over the interface. A program that insists on asking a question fails, rather than hanging the screen. Answer prompts with `Cmd.Input: true` and `job.SendLine(...)`, which is also how to pass a password (for example to a `--password-stdin` option) without it appearing on the command line.
- **Bounded memory.** It keeps the newest 50,000 lines (`MaxLines`, configurable per `Cmd`), with `Job.TailLines(n)` and `Job.Slice(start, count)` for windowed inspection.
- **Results.** `job.Output()` returns stdout once the job is done; `job.State()`, `ExitCode()`, `Err()` and `Elapsed()` describe how it ended; `OnExit` runs when it finishes (toasts are safe from there).

`runner.Percent` and `runner.Fraction` (`12/80`, `3 of 7`) cover most progress formats; any `func(line string) (int, bool)` works.

**The output view.** `mint.Log` shows the newest lines in a fixed-height box under a heading row such as `Output  128 lines · view`. Enter or a double-click on the heading opens the full output in a viewer that keeps following new lines until you scroll up. Errors are shown in amber. The viewer is also available directly as `mint.ViewLog(title, lines)` or `mint.ViewLogWith(mint.ViewLogOpts{...})` and supports horizontal panning (`←`/`→`, `0` to reset), case-insensitive search (`/`, `n`, `N`), and saving the full output to a named file (`s`). `mint.CleanLine` also resolves backspace overstrikes (`\b`) from `man` pages while preserving single-width Unicode box-drawing and Braille characters.

**Slow work without a ctx.** `mint.Busy("Applying…", fn)` is `ctx.Busy` for code that has no `ctx`, such as a row's `Set`.

### Building an interface for a program

`CATALOGUE.md` is the guide for wrapping a command-line program. It covers how to read a program's help, the five common program shapes (settings, job, monitor, multi-command, lookup), which component fits each kind of option, and the rules every interface follows, such as never running a command inside a row's `Get` and sending secrets through standard input. `templates/app` is the starter every new interface copies.

Every code example in the catalogue compiles. Check them after changing the library with:

```
python3 tools/check_catalogue.py
```

### Inline collapsible sections, large text, and formatted tables

- **`mint.Collapsible` and `mint.CollapsibleText`**: place foldable groups (`▸ Title` / `▾ Title`) directly inside any `Section.Rows` slice. When `Lines` and `ChunkSize` are set, massive text outputs automatically split into nested collapsible chunks (`Lines 1–100`, `Lines 101–200`, …) alongside an action to open the full text in `mint.ViewLog`.
- **`mint.Table` and `mint.ParseTable` (`mint.ParseColumns`)**: format multi-column data into aligned, selectable Mint rows, or parse whitespace-, tab-, pipe-, and box-drawing-aligned CLI tables (`ps`, `df`, `ss`, `ls -l`, `nmap`) automatically.

### Turnkey CLI apps (`mint/appkit`) and automated scaffolding (`tools/mint-gen`)

`mint/appkit` provides a declarative runtime built on **6 Base Archetypes**, **Composable Mixins** (add-ons), and **28 Template Types (`T01`–`T28`)** (see [`templates/README.md`](templates/README.md)):

- **6 Base Archetypes** (`templates/config`, `templates/runner`, `templates/scanner`, `templates/monitor`, `templates/explorer`, `templates/suite`):
  - `config` (`ArchetypeConfig` — `T01`–`T04`): System, dotfile, hardware, and firewall rule-table configurators.
  - `runner` (`ArchetypeRunner` — `T05`–`T07`, `T10`–`T11`, `T15`–`T16`): Batch runners, media transcoders, downloaders, build/test runners, benchmarks, security auditors, and crypto/hash utilities.
  - `scanner` (`ArchetypeScanner` — `T12`–`T14`): Network/port scanners, packet analysers, and OSINT/recon gatherers with live parsed tables + collapsible logs.
  - `monitor` (`ArchetypeMonitor` — `T17`–`T20`): Live system resource monitors, service/daemon controllers, process inspectors, and journal/log analysers.
  - `explorer` (`ArchetypeExplorer` — `T24`–`T27`): Disk/filesystem inspectors, file/content searchers, structured data (`jq`/`yq`/`sqlite3`) explorers, and documentation browsers.
  - `suite` (`ArchetypeSuite` — `T08`–`T09`, `T21`–`T23`, `T28`): Multi-subcommand suites for archives, backups, VCS (`git`/`gh`), containers (`docker`/`kubectl`), package managers, and cron/timers.
- **Composable Mixins** (15; `go run ./tools/mint-gen --list-templates` lists them):
  - The original 8: `WithFileOutput`, `WithTableOutput`, `WithCollapsibleOutput`, `WithProfiles`, `WithDynamicChoices`, `WithRootAuth`, `WithConfirm`, and `WithHistory`.
  - 7 for the program shapes the base layouts don't cover: `WithDiff` (Compare: `diff`, `cmp`), `WithFindings` (Audit: `shellcheck`, `lynis`, SARIF), `WithPreview` (Edit files: `rm`, `cp`, `mv`, `rsync`), `WithFollow` (Follow: `tail -F`, `journalctl -f`), `WithService` (Serve: `python -m http.server`, `nc -l`), `WithTranscript` (Session: `psql`, `sqlite3`, `gdb`, `redis-cli`) and `WithReorder` (Rules: `iptables`, `nft`, `ufw`, `ip rule`). mint-gen attaches the ones that are safe to offer by itself; [`CATALOGUE.md`](CATALOGUE.md) Part E describes each.

`tools/mint-gen` checks whether a command can run inside a pure detached TUI, auto-classifies any CLI tool into `T01`–`T28`, and scaffolds a complete `apps/<tool>/` module (`go.mod`, `main.go`, `spec.go`, and `<tool>-mint.el`) from its `man` page and `--help` output.

#### Step-by-step: generating and running a Mint interface for any installed CLI program

1. **Check compatibility (optional)**
   Verify that the program is installed and works inside a pure detached TUI (this also probes and caches your terminal's capability profile in `~/.config/mint-interfaces/termcaps.json` on first run; pass `--probe-term` anytime to force a fresh probe):
   ```bash
   go run ./tools/mint-gen --check nmap
   ```
   *(You can also run `go run ./tools/mint-gen --list-templates` to see all 28 available templates `T01`–`T28`, 6 base archetypes, and the composable mixins.)*

2. **Generate and compile the interface in one command**
   Run `mint-gen` with `--build` (or pass `--template T12` if you want to override the auto-detected template):
   ```bash
   go run ./tools/mint-gen --build nmap
   ```
   Behind the scenes, this automatically:
   - Ensures your terminal capability profile is cached in `~/.config/mint-interfaces/termcaps.json` (keyed by `$TERM`, `$TERM_PROGRAM`, and `$TMUX`).
   - Reads `man nmap` and `nmap --help` to extract its summary, synopsis, target input, file-output flags (`-oN`, `-oX`, `-o`, etc.), subcommands, and option flags.
   - Auto-selects the matching template from `T01`–`T28` (for `nmap`, `T12` `network-scanner` using `ArchetypeScanner` + `WithTableOutput`, `WithCollapsibleOutput`, `WithFileOutput`, `WithProfiles`, `WithRootAuth`, and `WithHistory`). A program that no template lists gets its archetype first, from how it works (subcommands or modes, repeating or following, hosts, patterns or devices, getting and setting values), then the template within that archetype from the words in its summary, description, usage and help. It always gets a template, marked as a clear choice or a close call; for a close call the build output names the likely alternatives and the `--template` command to switch. On 123 everyday programs never used for tuning, 73% get the right template; 60% are clear choices, right 9 times in 10 (`MINT_TEMPLATE_BENCH=1 go test -run TestTemplateBenchmark -v ./tools/mint-gen`).
   - Maps every flag to the right Mint control (`Checkbox`, `Stepper`, `Select`, `Text`, or interactive path/repeatable browsers) and groups secondary flags into collapsible sections (`mint.Collapsible`).
   - Generates a self-contained folder at `apps/nmap/` (`go.mod`, `main.go`, `spec.go`, and `nmap-mint.el`) and compiles the static binary at `apps/nmap/nmap-mint` via `tools/build.sh`.

3. **Run your new interface immediately**
   Launch the compiled binary directly in your terminal (or load `apps/nmap/nmap-mint.el` and run `M-x nmap-mint` in Emacs):
   ```bash
   ./apps/nmap/nmap-mint
   ```
   You immediately get the full multi-screen interface: **Configure & Run** (target, presets, flags, collapsible sections, and output file naming), **Live / Last Output** (progress bar, live parsed `mint.Table`, collapsible large-text chunks, and full-screen `ViewLog` with `←`/`→` horizontal pan, `/` search, and `s` save), **Output Directory Browser**, and **Saved Runs History**.

### Icons

The library counts every character as one column, as the original does. Emoji such as `⚡` take two columns in most terminals and push the right border out of line, so use single-width symbols like `◆ ▤ ◧ ◷ ≡` for icons.

## Security

Generated interfaces are built to keep passwords, tokens and keys away from other users on the computer, off the disk and off the screen, and to stop commands that change or destroy things from running by accident. The rules below apply to every app built with `appkit` and `mint-gen`. The secure store underneath them can also be used on its own.

### Safety rules every generated interface follows

- **Nothing runs unless you ask, except commands that only read.** A monitor polls, and a settings screen shows "Refresh live state", only when the tool has a read-only way to show its state: `StatusArgs` (for example `systemctl list-units`, `ufw status`, `sysctl -a`), or a tool that only reports (`ReadOnly`, such as `ps`, `df` or `journalctl`). Common tools get these automatically. For anything else, such as `pkill` or `kill`, the monitor becomes an ordinary screen with a Run action and the usual confirmations.
- **Commands appkit runs itself have time limits and run in the background.** Polling never freezes the screen (a `tail -f` that never ends is stopped after 15 seconds), row actions and refreshes show the loading spinner, and anything still running when the app closes is stopped.
- **Irreversible tools need the target typed to confirm.** `dd`, `shred`, `rm`, `wipefs`, `mkfs.*`, `kill`, `pkill`, `killall` and similar are marked `Irreversible`.
- **Secrets stay secret.** Options whose value is a password, token or key are `FlagSecret`: asked for with a password dialog, kept for the session only (never in settings, presets or history), and masked in the command preview. Each secret takes the safest route the tool offers: standard input for a `--password-stdin` style option (never visible), then an environment variable such as `RESTIC_PASSWORD`, `PGPASSWORD` or `GH_TOKEN` (only you and root can read a program's environment), and only as a last resort the command line, where other users can see it with `ps`; then the app says so and asks before running. Environment variables come from a built-in list of well-known tools (restic, borg, kopia, duplicity, rclone, psql and the pg_ tools, gh, glab, sshpass) and from the man page's ENVIRONMENT section. Settings and history files are readable only by you.
- **Secrets are hidden in the output.** Every secret a run uses, and any typed into a text box, is replaced by `••••••` in the program's output before it is shown, saved or used in a message. So are well-known token formats (GitHub, GitLab, AWS, Slack, Google, `sk-…` keys, private keys), `Authorization:` and API-key header values (such as the `Basic` header `curl -v` prints), and passwords in URLs. Mint keeps only a salted fingerprint of each secret for this, not the secret itself. Secrets shorter than 4 characters are not hidden, since they would blank out ordinary words. A file the program writes itself (through its own output option) is not changed.
- **Sensitive results are kept private.** Each tool has a data level, shown as a badge: *sensitive* for packet captures, logs, security audits, database dumps and tracers, and *secret* for password managers and key and crypto tools. The level comes from the tool, its template, or `ToolSpec.Data`. Options that print secrets (`--export-secret-keys`, `-d` "Decrypt data", `--show-password`) make a run secret, and so does output that contained a private key, a login token (JWT), a password hash or another recognised secret. For sensitive results, new results folders are private (0700) and result files, including ones the tool writes itself, are readable only by you. Secret tools keep results in a private in-memory folder by default ("Keep results in memory only"): `$XDG_RUNTIME_DIR`, cleared at logout, or `/dev/shm`, cleared at restart. "Encrypt the output" seals the output file (or the captured output) with a passphrase and removes the original. The Output Directory opens sealed files after asking for the passphrase, decrypted in memory only. `mint-gen`'s safety report shows the level and the revealing options.
- **Passwords typed into text boxes are caught.** Before a run, the Extra arguments and Target boxes are checked for passwords in URLs (`https://user:pass@host`), `--password=…` / `--token …` style options, the tool's own secret options typed by hand, `--user name:pass`, `Authorization:` and API-key headers, `PASSWORD=…` assignments and well-known token formats. If one is found, the app explains that it will be visible on the command line, points to the tool's own secret option if there is one, and asks before running. Such values are hidden in the command preview, settings and history and are never written to disk; in the next session the box shows `••••••` and the app asks you to type it again rather than running with it.
- **Mint never asks for your sudo password.** A tool that needs root runs when the whole app is started with sudo (`sudo apps/tcpdump/tcpdump`), so the password goes to sudo's own prompt before Mint starts. `mint-gen --run` asks "Start it with sudo? [y/N]" for tools that usually need root (answer in advance with `--sudo` or `--no-sudo`). The app's "Root access" section shows whether it runs as root and, if not, the exact command to restart it with sudo; when a run fails with "permission denied" or similar, the app says the same. Started with sudo, the app still keeps its settings, history and results in your home and makes them belong to you, not to root.
- **`mint-gen` reads a program's help without trusting it.** To learn a program's options it runs `man` and the program's `--help` (or `-h`), plus fuller help such as curl's `--help all` or the `dig -h` that dig's `--help` points to, but only when the program's own help mentions it, only with help arguments from a fixed list, and at most six runs in all. Each run is passed as a list of arguments (never through a shell), in its own session with no terminal and input closed, inside an empty temporary folder that is also its `HOME`, with a cleaned environment (your tokens and passwords are not passed on), a 5-second limit and a 2 MB output cap. Anything it leaves running afterwards is stopped. It never runs as root: under sudo it runs as you, and as plain root it runs as `nobody`.
- **The app writes only where mint-gen is sure.** For each tool, mint-gen works out what it reads and writes, from three sources: the options' descriptions, its usage line, and the known-tools list. "Save output to file" then works one of three ways. The tool may write the file itself through its output option (`curl -o`) or as its last argument (`ffmpeg … out.mkv`). Otherwise Mint saves what the tool prints. A path is only passed to an option or argument when that's certain: a listed option, a clearly described one, or an output argument confirmed by the known-tools list. Anything unclear is shown as a path the app never writes to. For tools that only print progress messages (gzip, tar, ffmpeg), saving those messages is labelled as such and off by default. The output file's extension comes from the known-tools list, a format named in the output option's description, or the template. Text that Mint saves is never given a binary extension like `.pcap`. When you choose an output format in the app (pandoc `-t docx`, a `--json` switch, or the same typed into Extra arguments), the automatically named file follows it (`.docx`). A file name you type yourself is never changed, and only extensions from a fixed list are used. `mint-gen --explain <program>` shows every decision and its reason. On a benchmark of 151 programs, no app passes its output file to the wrong option or argument, with or without the known-tools list (`MINT_IO_BENCH=1 go test -run TestIOBenchmark -v ./tools/mint-gen`).
- **Facts about tools you add are checked before use.** mint-gen has a built-in list of what common tools read and write (tcpdump's `-w` capture file, nmap's `-oN`/`-oX`/`-oA` outputs, wget's log options…). You can add or correct entries in JSON files in `~/.config/mint-interfaces/tools.d/` or `/etc/mint-interfaces/tools.d/`. Such a file is read only if it and every folder above it belong to you or root and nobody else can change them, and only if it isn't a link and is under 256 KB. Its content must have exactly the expected shape, with option names, extensions and roles checked. Any problem is reported with `--check` and the file or entry is skipped. The files can describe options only; they can't change any safety setting.
- **`mint-gen` only touches folders it created.** It refuses to scaffold into a folder that already has other files, and `--remove` deletes only the files it generated (listed in the folder's `.mint-gen` file), leaving anything else in place. Options may come before or after the program name.
- **Check before you build.** `mint-gen --check <program>`, the summary after scaffolding, and the Safety section of `mint-gen --tui` all show a safety report: when the tool will run (automatically, only when asked, or only after the target is typed), each secret option and how its value is passed (standard input, which no one else can see; an environment variable, which only you and root can see, and whether it came from the built-in list or the man page; or the command line, which other users can see with `ps`), and the options that look destructive (`--force`, `--delete`, `-f` "never prompt"…). For example:

  ```
  Safety:
    Running:     destroys data or processes: runs only when asked, and the target must be typed to confirm
    Secrets:     none found
    Destructive: 3 options to review (they delete, overwrite or skip a prompt)
      -f                   Ignore nonexistent files and arguments, never prompt
      --no-preserve-root   Do not treat '/' specially
      -r                   Remove directories and their contents recursively
  ```

### Secure store (`mint/secure`)

`mint/secure` keeps passwords, tokens and keys safe while an app runs:

- Each secret is encrypted (AES-256-GCM) and held in **locked memory** outside Go's heap: never written to swap, **excluded from core dumps**, and wiped when removed.
- The encryption key lives in the app's own **kernel keyring** on Linux: only this process can use it, the programs it starts do not inherit it, and the kernel deletes it when the app closes or the **idle timeout** passes.
- Opening the store also **hardens the app**: core dumps off, and on Linux not dumpable, so other programs running as you cannot read its memory.
- Secrets are decrypted only into locked memory, for the moment they are used, then wiped. Keep them as `[]byte` all the way to the program: a Go string makes copies nothing can wipe.

```go
st, _ := secure.Open()
defer st.Close()
st.Put("db-password", typed) // wipes typed
st.Use("db-password", func(pw []byte) error {
	_, err := stdin.Write(append(pw, '\n'))
	return err
})
```

**Idle timeout.** Each computer chooses its own: `$MINT_KEY_TIMEOUT` (`"5m"`, or `"0"` to keep the key until the app closes), then `~/.config/mint-interfaces/security.json`, then the administrator's `/etc/mint-interfaces/security.json` (both `{"key_timeout": "5m"}`), then 15 minutes. Each use restarts the countdown. When it passes, every secret is wiped; `Use` returns `ErrExpired` and the app asks again (`Renew` starts a fresh key).

**Fallbacks.** `st.Level()` says how well secrets are protected, and `st.Notes()` why:

| Level | When | What changes |
|---|---|---|
| full | Linux with the kernel keyring | nothing |
| no kernel keyring | containers that block it (Docker's default profile does), macOS, the BSDs | the key is kept in its own locked page and the store enforces the timeout itself |
| memory cannot be locked | the locked-memory limit is too low | secrets may reach swap; use encrypted swap |

**In generated apps.** `appkit` keeps every secret option in this store from the moment it is entered. A secret read from standard input is written to the program straight from the store's protected memory. One passed as an environment variable or on the command line has to become an ordinary Go string at the moment the program starts, because that is the only form those take. When the idle timeout wipes a secret, its row shows "expired, enter again" and a run refuses to start without it rather than leaving it out silently.

**Encrypting files, with your own algorithm.** `secure.SealBytes`, `secure.OpenSealed` and `secure.SealFile` encrypt data with a passphrase into a `.mintseal` file. Mint derives a 32-byte key from the passphrase (PBKDF2-HMAC-SHA256, 600,000 rounds, random salt) and records which *sealer* made the file, so a file always opens with the right one. The built-in sealer is AES-256-GCM. To use your own encryption, implement `secure.Sealer` and register it:

```go
type mySealer struct{}

func (mySealer) Name() string                              { return "my-scheme-v1" }
func (mySealer) Seal(key, plain []byte) ([]byte, error)    { /* encrypt */ }
func (mySealer) Open(key, sealed []byte) ([]byte, error)   { /* decrypt; fail if key or data is wrong */ }

func init() { secure.RegisterSealer(mySealer{}) }
```

Choose it for new files with `$MINT_SEALER=my-scheme-v1` or `{"sealer": "my-scheme-v1"}` in `security.json`. `secure.PrivateDir` gives the private in-memory folder generated apps use for results.

### Limits

- **Root can see everything.** Root, and anything with root's power, can read the app's memory, its kernel keyring and the environment of the programs it starts.
- **Some routes are visible to other users.** A secret that a tool accepts only on the command line, or one you type into Extra arguments or Target, can be seen by other users (for example with `ps`) while the program runs. Mint warns and asks first.
- **Some copies sit in ordinary memory for a moment.** The password box hands over what was typed as a Go string, and a secret passed as an environment variable or argument must become one. Those copies stay in ordinary memory until they are garbage-collected. While a secret is encrypted or decrypted, Go's AES keeps the expanded key there too. Core dumps are off, so this matters only if the system swaps those pages.
- **Weaker without the kernel keyring or locked memory.** In containers, on macOS and on the BSDs the key is kept in locked memory instead of the keyring. Without enough locked memory, secrets may reach swap, so use encrypted swap (see Fallbacks above).
- **Masking has gaps.** Secrets shorter than 4 characters are not hidden in output. Files a tool writes through its own output option are not changed; they are made private, and "Encrypt the output" protects them. The Extra arguments box still shows what you typed while you edit it.
- **Memory and deleted files are not absolute.** The in-memory folder can be swapped to disk like any memory, so use encrypted swap. When a file is encrypted, the original is overwritten and removed, but journaling and copy-on-write file systems and SSDs may keep old copies. A decrypted file shown in the viewer is held as ordinary Go strings while it is open.
- **Data levels come from lists.** A tool, template or option that is not on the lists, or a secret in a format Mint does not recognise, is not detected. Set `ToolSpec.Data` yourself when you know better.
- **Option detection depends on the help text.** `mint-gen` finds secret and destructive options from the man page and `--help`. A tool whose short help leaves options out (such as `curl`) may have secret options it does not know about. The text-box check still catches values typed by hand.

### Conclusion

On an ordinary Linux desktop, secrets typed into a Mint app stay in protected memory, are never saved, and are hidden wherever they would be shown, while sensitive results stay private, in memory or encrypted with your own scheme. What remains open is anything with root's power, the routes a tool itself forces onto the command line, and copies the disk or swap may keep, which Mint warns about or documents above.

## Standalone binaries and flags

Every Mint app compiles into a single, statically linked binary that runs on any Linux machine without Go or Emacs installed:

```sh
./tools/build.sh ./templates/app ./mytool
./mytool
```

Apps with their own `go.mod` under `apps/<tool>` are built the same way: `./tools/build.sh ./apps/<tool> ./<tool>-mint`.

`mint.App` handles a few things at startup, before taking over the terminal:

- **Flags.** The first argument may be one of Mint's flags:
  - `--help` (`-h`) lists the app's screens and usage.
  - `--version` (`-v`) prints `mytool 1.2.0 (Mint-Interfaces 0.1.0)`, using `App.Version`.
  - `--mint-info` prints a JSON description (title, version, screens, required and missing tools) for launchers such as Emacs.
  - `--screen NAME` opens that home item directly, and Back returns to the home list.

  Only the first argument is checked, so `mytool somefile -v` is left alone. An app that parses its own arguments sets `NoFlags: true` to turn these off.
- **Required programs.** `Require: []string{"rsync"}` checks `PATH`, and any missing program is named at the top of the home list, ahead of the app's own `Note`.
- **No terminal.** Started without one (input redirected, or launched from a non-terminal tool), the app explains that it needs a terminal and exits with status 2, instead of spinning.
- **Lost terminal.** If the terminal disappears while the app runs (window closed, Emacs killed), `Run` returns so the app can clean up (for example `runner.StopAll()`), rather than lingering.

## Keys

Arrow keys, Enter, Space, Esc, `b` and `q` always work. Home, End, Page Up and Page Down jump and page through lists; other keys Mint doesn't use, such as Delete or F5, are ignored.

A key profile adds a familiar set on top:

| Profile | Move | Change | Back | First / last | Page |
|---|---|---|---|---|---|
| `default` | ↑ ↓ | ← → Space | Esc, b | Home, End | PgUp, PgDn |
| `emacs` | `C-n` `C-p` | `C-f` `C-b` | `C-g` | `C-a`/`M-<`, `C-e`/`M->` | `M-v`, `C-v` |
| `vim` | `j` `k` | `h` `l` | Esc, b | `g`, `G` | `C-u`/`C-b`, `C-d`/`C-f` |

The person running the app chooses with the `MINT_KEYS` environment variable (`export MINT_KEYS=vim`). An app can set its own default with `App.Keys`. In the vim profile, letters only act as keys outside text fields, so typing still works. The footer hints follow the profile.

## Running from Emacs (`emacs/mint.el`)

`mint.el` launches Mint apps from Emacs using only what's built into Emacs.

In terminal Emacs, the default is **handover**. Emacs steps aside, the app runs on the real terminal Emacs is using, and Emacs redraws itself when you quit the app. Nothing is emulated, so the app looks and behaves exactly as it does outside Emacs: colours, mouse, a single Esc to go back, and live animation. Emacs keeps running in the background meanwhile.

Emacs also sets up each app it launches:

- **Keys:** `mint-key-profile` (default `emacs`) gives every app Emacs keys. Set it to `vim` or `default`, or to `nil` to leave `MINT_KEYS` alone.
- **Colours:** with `mint-match-theme` on (the default), apps take their colours from the current Emacs theme. `mint-theme-faces` says which face each colour comes from. On terminals without 24-bit colour, Emacs passes the same 256-colour approximations it uses itself, so the app matches what Emacs shows.

Setup:

```elisp
(add-to-list 'load-path "/path/to/Mint-Interfaces/emacs")
(require 'mint)
(setq mint-app-directories '("~/mint-apps"))   ; optional: folders of compiled Mint apps
```

Commands:

- `M-x mint` chooses an app, showing each one's description. With `C-u`, it also asks which screen to open.
- `M-x mint-run` runs any compiled Mint binary.
- `M-x mint-run-source` builds an app's Go folder if needed, then runs it. It rebuilds only when a `.go` file has changed.
- `(mint-define-app mytool "mytool" "/path/to/mytool-ui/")` defines `M-x mytool` and adds the app to `M-x mint`. The starter's `templates/app/app.el` does this for a new interface.

Hooks: `mint-before-launch-functions` runs with the app's name, and `mint-after-exit-functions` runs with the name and exit status. The exit hook is useful for refreshing buffers the app may have changed.

**The other display mode.** `mint-display` can also be `term`, which runs the app in an Emacs buffer using the built-in terminal emulator, so it can sit beside other windows. Graphical Emacs has no terminal to hand over, so `auto` (the default) uses `term` there.

Tests: `emacs -Q --batch -L emacs -l emacs/mint-tests.el -f ert-run-tests-batch-and-exit` (needs Go).

### Limitations in Emacs

**Handover (the default in terminal Emacs)**

- The app takes over the whole terminal until you quit it. You can't see it beside other Emacs windows; use `term` mode or a terminal multiplexer such as tmux for that.
- Emacs keeps running in the background (timers, processes, the Emacs server), but you can't see or type into Emacs until the app exits.
- Terminal Emacs only, since graphical Emacs has no terminal to hand over.
- Emacs's own key bindings don't apply while the app is open: every key goes to the app. With `mint-key-profile` set to `emacs`, the app understands the common movement keys and `C-g`.

**`term` mode (built in; the default in graphical Emacs)**

- Emacs's built-in terminal emulator has bugs that affect full-screen programs. `mint.el` works around the ones found in testing (characters split between chunks of output, and sideways scrolling), but very busy screens, such as fast-moving progress bars, can still occasionally draw a frame wrongly. Moving to another row redraws it.
- It is slower than a real terminal on busy screens.
- `C-x` is passed to Emacs (so window commands work) and never reaches the app.

**General**

- Tested on Emacs 29.3 in a terminal. Graphical Emacs has not been tested.
- Themes are matched from the faces named in `mint-theme-faces`. If your theme leaves a face's colour unset (common for the default text colour in terminal Emacs), the app keeps its own colour for that role.

## Running in Vim, Neovim and tmux

Mint apps are ordinary terminal programs, so they work inside the terminals built into Vim and Neovim and inside tmux. Each of these was tested with Vim 9.1, Neovim 0.9.5 and tmux 3.4: drawing, colours, every key including a single Esc, mouse clicks, animation, resizing, and quitting.

| Where | How | Notes |
|---|---|---|
| Neovim | `:terminal mytool`, then `i` (or `+startinsert`) | Keys go to the app in Terminal mode; `C-\ C-n` leaves it. |
| Neovim, beside your work | `:vsplit \| terminal mytool` | The app redraws to fit the split. |
| Neovim, floating | `termopen` in a floating window (below) | |
| Vim | `:terminal mytool` (a split) or `:terminal ++curwin mytool` | `C-w` is Vim's terminal prefix and never reaches the app. |
| Vim, full screen | `:!mytool` | Vim hands over the real terminal, like Emacs's handover. |
| tmux | a pane, a window, or `tmux display-popup -E -w 80% -h 80% mytool` | |

A floating Neovim window that closes when the app quits:

```lua
vim.keymap.set("n", "<leader>m", function()
  local buf = vim.api.nvim_create_buf(false, true)
  local w, h = math.floor(vim.o.columns * 0.8), math.floor(vim.o.lines * 0.8)
  vim.api.nvim_open_win(buf, true, { relative = "editor", width = w, height = h,
    row = math.floor((vim.o.lines - h) / 2), col = math.floor((vim.o.columns - w) / 2), border = "rounded" })
  vim.wo.winhighlight = "NormalFloat:Normal"   -- keep the app's background, not the float colour
  vim.fn.termopen("mytool", { on_exit = function() vim.cmd("close") end })
  vim.cmd.startinsert()
end)
```

Settings that matter:

- **tmux: `set -g mouse on`** for mouse clicks to reach the app (and the popup), and **`set -sg escape-time 10`**, because tmux otherwise holds every Esc for half a second before passing it on.
- **Vim: `set ttimeout ttimeoutlen=50`** (already set by Vim's `defaults.vim`, but not with `vim -u NONE`), or Esc waits a second. `set mouse=a` for clicks.
- **Neovim** needs nothing: Esc is passed on at once, and mouse clicks work in Terminal mode.
- **Key profiles:** tmux's prefix is `C-b`, which the emacs profile uses for ← and the vim profile for Page Up. Press `C-b C-b` to send it through, use the arrow keys, or change the tmux prefix.

Limitations:

- `:!mytool` in Neovim doesn't work: Neovim runs `:!` commands without a terminal, so the app says it needs one and exits. Use `:terminal`.
- In a terminal split, the app gets only the split's size. Below about 48 columns or 12 rows it shows "Terminal too small" until the split is made bigger, as it would anywhere.

## Licence

The UI layer is derived from Terminal Settings Manager by Curtis and is under the Apache License 2.0. See `LICENSE`.
