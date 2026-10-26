// A tour of every row type and dialog.
//
//	go run ./examples/demo
package main

import (
	"fmt"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

var (
	volume   = 35
	wifi     = true
	mode     = "Balanced"
	hostname = "mint-box"
	nickname = ""
	accent   = 2
	lastPick = "none"
	fails    = true
)

func widgetsScreen() *mint.Screen {
	return &mint.Screen{
		Icon:  "◧",
		Title: "Widgets",
		Sections: []mint.Section{
			{Name: "Sound", Rows: []mint.Row{
				mint.Note("Every row type the library draws."),
				mint.Slider(mint.SliderOpts{Label: "Volume",
					Get: func() int { return volume }, Set: func(v int) { volume = v }}),
				mint.Toggle(mint.ToggleOpts{Label: "Wi-Fi",
					Get: func() bool { return wifi }, Set: func(v bool) { wifi = v }}),
				mint.Picker(mint.PickerOpts{Label: "Power mode",
					Options: []string{"Quiet", "Balanced", "Performance"},
					Get:     func() string { return mode }, Set: func(v string) { mode = v }}),
				mint.Picker(mint.PickerOpts{Label: "Accent index",
					Get:  func() string { return fmt.Sprint(accent) },
					Step: func(d int) { accent += d }}),
			}},
			{Name: "Identity", Rows: []mint.Row{
				mint.Text(mint.TextOpts{Label: "Hostname",
					Get: func() string { return hostname }, Set: func(v string) { hostname = v }}),
				mint.Text(mint.TextOpts{Label: "Nickname", Placeholder: "unset",
					Get: func() string { return nickname }, Set: func(v string) { nickname = v }}),
				mint.Value(mint.ValueOpts{Label: "Static value", Value: "42"}),
				mint.Value(mint.ValueOpts{Label: "Empty value"}),
				mint.Value(mint.ValueOpts{Label: "Broken getter", Get: func() string {
					if fails {
						panic("backend unavailable")
					}
					return "ok"
				}}),
				mint.Heading("Inline heading"),
				mint.Action(mint.ActionOpts{Label: "Last pick", Get: func() string { return lastPick }}),
				mint.Action(mint.ActionOpts{Label: "Open nothing"}),
			}},
		},
	}
}

func longScreen() *mint.Screen {
	rows := []mint.Row{}
	for i := 1; i <= 30; i++ {
		rows = append(rows, mint.Value(mint.ValueOpts{
			Label: fmt.Sprintf("Entry %02d with a fairly long label to test truncation behaviour", i),
			Value: strings.Repeat("v", i),
		}))
	}
	return &mint.Screen{Title: "Long list", Banner: "Thirty rows, so the scrollbar shows.",
		Hints: " ↑↓ scroll   b back ", Sections: []mint.Section{{Name: "Rows", Rows: rows}}}
}

func dialogsScreen() *mint.Screen {
	return &mint.Screen{
		Icon:  "◇",
		Title: "Dialogs",
		Sections: []mint.Section{
			{Name: "Modal", Rows: []mint.Row{
				mint.Action(mint.ActionOpts{Label: "Message", Hint: "show", OnActivate: func(c *mint.Ctx) {
					c.Message("Heads up", "This is a message dialog.\n\nIt wraps long text across the width of the frame so that nothing is cut off at the edge.")
				}}),
				mint.Action(mint.ActionOpts{Label: "Confirm", Hint: "ask", OnActivate: func(c *mint.Ctx) {
					if c.Confirm("Delete cache?", "The cache will be rebuilt on next start.") {
						lastPick = "confirmed"
					} else {
						lastPick = "declined"
					}
				}}),
				mint.Action(mint.ActionOpts{Label: "Prompt", Hint: "edit", OnActivate: func(c *mint.Ctx) {
					if v, ok := c.Prompt("Name", "Shown in the title.", "untitled"); ok {
						lastPick = v
					}
				}}),
				mint.Action(mint.ActionOpts{Label: "Choose", Hint: "pick", OnActivate: func(c *mint.Ctx) {
					if v, ok := c.Choose("Pick a fruit", []string{"Apple", "Banana", "Cherry", "Damson", "Elderberry"}, "Type to narrow the list."); ok {
						lastPick = v
					}
				}}),
				mint.Action(mint.ActionOpts{Label: "Password", Hint: "enter", OnActivate: func(c *mint.Ctx) {
					if v, ok := c.Password("Unlock", "Your account password."); ok {
						lastPick = "pw:" + fmt.Sprint(len(v))
					}
				}}),
				mint.Action(mint.ActionOpts{Label: "Type to confirm", Hint: "danger", OnActivate: func(c *mint.Ctx) {
					if c.TypeToConfirm("Wipe disk", "This erases everything on the selected disk.", "wipe") {
						lastPick = "wiped"
					}
				}}),
				mint.Action(mint.ActionOpts{Label: "Push widgets", OnActivate: func(c *mint.Ctx) {
					c.Push(widgetsScreen())
				}}),
				mint.Action(mint.ActionOpts{Label: "Last pick", Get: func() string { return lastPick }}),
			}},
		},
	}
}

func main() {
	items := []mint.HomeItem{
		{Icon: "◧", Label: "Widgets", Desc: "Sliders, toggles, pickers, text fields and values", Open: widgetsScreen},
		{Icon: "◇", Label: "Dialogs", Desc: "Message, confirm, prompt, choose, password and type-to-confirm", Open: dialogsScreen},
		{Icon: "≡", Label: "Long list", Desc: "A screen with enough rows to scroll, and a description long enough that it has to wrap onto a second line and then get truncated", Open: longScreen},
	}
	for i := 1; i <= 5; i++ {
		items = append(items, mint.HomeItem{Icon: "·", Label: fmt.Sprintf("Filler %d", i), Desc: "Extra entry so the home list scrolls"})
	}
	app := &mint.App{
		Title: "◈ Mint demo",
		Crumb: "Demo",
		Note:  "A tour of every widget and dialog. This note is long enough to wrap onto a second line on narrow terminals.",
		Items: items,
	}
	app.Run()
}
