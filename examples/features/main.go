// A tour of the additions: dividers, spacing, text styles, collapsible
// sections, slider ranges, progress bars, steppers, checkboxes, radio
// groups, toasts, status badges, and the loading spinner.
//
//	go run ./examples/features
package main

import (
	"fmt"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

var (
	gain      = -6
	cpu       = 25
	threads   = 8
	timeout   = 30
	bluetooth = true
	location  = false
	updates   = true
	quality   = "High"
	downloadP = 0
	running   = true
	saves     = 0
)

func layoutScreen() *mint.Screen {
	return &mint.Screen{
		Icon:  "▤",
		Title: "Layout & text",
		Sections: []mint.Section{
			{Name: "Text styles", Rows: []mint.Row{
				mint.Label(mint.StyleTitle, "Title text"),
				mint.Label(mint.StyleHeading, "Heading text"),
				mint.Label(mint.StyleBody, "Body text wraps onto as many lines as it needs, so longer explanations stay readable instead of being cut off at the edge of the frame."),
				mint.Label(mint.StyleCaption, "Caption text for small print."),
				mint.Label(mint.StyleItalic, "Italic text for emphasis."),
				mint.Label(mint.StyleUnderline, "Underlined text."),
			}},
			{Name: "Dividers", Rows: []mint.Row{
				mint.Value(mint.ValueOpts{Label: "Above the line", Value: "one"}),
				mint.Divider(),
				mint.Value(mint.ValueOpts{Label: "Between lines", Value: "two"}),
				mint.Divider("-"),
				mint.Value(mint.ValueOpts{Label: "After a spacer", Value: "three"}),
				mint.Spacer(),
				mint.Divider("┄"),
			}},
		},
	}
}

func controlsScreen() *mint.Screen {
	return &mint.Screen{
		Icon:  "◧",
		Title: "Controls",
		Sections: []mint.Section{
			{Name: "Sliders", Rows: []mint.Row{
				mint.Slider(mint.SliderOpts{Label: "Gain", Min: -24, Max: 12, Step: 3, Unit: "dB",
					Get: func() int { return gain }, Set: func(v int) { gain = v }}),
				mint.Slider(mint.SliderOpts{Label: "CPU clock", Min: 8, Max: 50, Step: 1,
					Format: func(v int) string { return fmt.Sprintf("%.1f GHz", float64(v)/10) },
					Get:    func() int { return cpu }, Set: func(v int) { cpu = v }}),
				mint.Progress(mint.ProgressOpts{Label: "Download", Get: func() int { return downloadP }}),
			}},
			{Name: "Steppers", Rows: []mint.Row{
				mint.Stepper(mint.StepperOpts{Label: "Threads", Min: 1, Max: 64,
					Get: func() int { return threads }, Set: func(v int) { threads = v }}),
				mint.Stepper(mint.StepperOpts{Label: "Timeout", Min: 5, Max: 300, Step: 5, Unit: "s",
					Get: func() int { return timeout }, Set: func(v int) { timeout = v }}),
			}},
			{Name: "Permissions", Collapsible: true, Rows: []mint.Row{
				mint.Checkbox(mint.CheckboxOpts{Label: "Bluetooth",
					Get: func() bool { return bluetooth }, Set: func(v bool) { bluetooth = v }}),
				mint.Checkbox(mint.CheckboxOpts{Label: "Location",
					Get: func() bool { return location }, Set: func(v bool) { location = v }}),
				mint.Checkbox(mint.CheckboxOpts{Label: "Automatic updates",
					Get: func() bool { return updates }, Set: func(v bool) { updates = v }}),
			}},
			{Name: "Video quality", Collapsible: true, Collapsed: true, Rows: []mint.Row{
				mint.Radio(mint.RadioOpts{Label: "Used for streaming and recording.",
					Options: []string{"Low", "Medium", "High", "Ultra"},
					Get:     func() string { return quality }, Set: func(v string) { quality = v }}),
			}},
		},
		Poll: func(*mint.Screen) bool {
			if downloadP < 100 {
				downloadP++
				return true
			}
			return false
		},
	}
}

func actionsScreen() *mint.Screen {
	return &mint.Screen{
		Icon:  "◆",
		Title: "Feedback",
		Sections: []mint.Section{
			{Name: "Toasts", Rows: []mint.Row{
				mint.Action(mint.ActionOpts{Label: "Save settings", Hint: "save", OnActivate: func(c *mint.Ctx) {
					if c.Busy("Saving…", func() { time.Sleep(900 * time.Millisecond) }) {
						saves++
						c.Toast(fmt.Sprintf("Saved (%d)", saves))
					}
				}}),
				mint.Action(mint.ActionOpts{Label: "Warn me", Hint: "warn", OnActivate: func(c *mint.Ctx) {
					c.ToastWarn("Disk is 90% full")
				}}),
				mint.Action(mint.ActionOpts{Label: "Fail", Hint: "error", OnActivate: func(c *mint.Ctx) {
					c.ToastError("Could not reach the server")
				}}),
			}},
			{Name: "Service", Rows: []mint.Row{
				mint.Toggle(mint.ToggleOpts{Label: "Running (see the footer)",
					Get: func() bool { return running }, Set: func(v bool) { running = v }}),
				mint.Action(mint.ActionOpts{Label: "Open a slow page", Hint: "open", OnActivate: func(c *mint.Ctx) {
					c.Open("Diagnostics", func() *mint.Screen {
						time.Sleep(1200 * time.Millisecond)
						return layoutScreen()
					})
				}}),
			}},
		},
	}
}

var job *runner.Job

// fakeUpdate stands in for a real program: it prints progress lines,
// redraws a percentage in place, and writes one warning to stderr.
const fakeUpdate = `for i in $(seq 1 40); do
  printf 'Fetching package %d of 40\n' $i
  printf '%d%%\r' $((i*100/40))
  [ $i -eq 20 ] && echo 'warning: mirror is slow' >&2
  sleep 0.1
done
echo 'All packages up to date'`

func jobsScreen() *mint.Screen {
	return &mint.Screen{
		Icon:  "≡",
		Title: "Jobs",
		Sections: []mint.Section{
			{Name: "Update", Rows: []mint.Row{
				mint.Action(mint.ActionOpts{Label: "Start update", Hint: "run", OnActivate: func(c *mint.Ctx) {
					if job != nil && job.Running() {
						c.ToastWarn("Already running")
						return
					}
					job = runner.Start(runner.Cmd{Shell: fakeUpdate, Label: "update", Progress: runner.Percent,
						Timeout: 2 * time.Minute,
						OnExit: func(j *runner.Job) {
							if j.State() == runner.Done {
								mint.Toast("Update finished")
							}
						}})
				}}),
				mint.Action(mint.ActionOpts{Label: "Stop", Hint: "stop", OnActivate: func(c *mint.Ctx) {
					if job != nil && job.Running() {
						job.Stop()
					}
				}}),
				mint.Progress(mint.ProgressOpts{Label: "Progress", Get: func() int {
					if job == nil {
						return 0
					}
					return job.Percent()
				}}),
				mint.Log(mint.LogOpts{Label: "Output", Height: 6, Lines: func() []mint.LogLine {
					if job == nil {
						return nil
					}
					return job.Lines()
				}}),
			}},
		},
		Poll: func(*mint.Screen) bool { return job != nil && job.Changed() },
		Status: func() []mint.Badge {
			if job == nil {
				return []mint.Badge{{Text: "no job yet", Kind: mint.BadgeIdle}}
			}
			return job.Badges()
		},
	}
}

func main() {
	app := &mint.App{
		Title:          "◈ Mint features",
		Crumb:          "Features",
		Note:           "New in this version: layout, text styles, controls, and footer feedback.",
		SectionSpacing: 1,
		Status: func() []mint.Badge {
			svc := mint.Badge{Text: "service running", Kind: mint.BadgeOK}
			if !running {
				svc = mint.Badge{Text: "service stopped", Kind: mint.BadgeError}
			}
			return []mint.Badge{svc, {Text: "v2.1", Kind: mint.BadgeIdle}}
		},
		Items: []mint.HomeItem{
			{Icon: "▤", Label: "Layout & text", Desc: "Dividers, spacing and emphasis styles", Open: layoutScreen},
			{Icon: "◧", Label: "Controls", Desc: "Ranges, progress, steppers, checkboxes, radio groups, collapsible sections", Open: controlsScreen},
			{Icon: "◆", Label: "Feedback", Desc: "Toasts, status badges and loading spinners", Open: actionsScreen},
			{Icon: "≡", Label: "Jobs", Desc: "Run a command in the background with progress, output and Stop", Open: jobsScreen},
			{Icon: "◷", Label: "Slow page", Desc: "Takes a second to open, so the footer shows a spinner", Open: func() *mint.Screen {
				time.Sleep(1500 * time.Millisecond)
				return layoutScreen()
			}},
		},
	}
	app.Run()
	runner.StopAll() // don't leave jobs running after the interface closes
}
