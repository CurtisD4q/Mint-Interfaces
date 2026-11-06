// Panes side by side: a command list on the left, the selected command's
// form on the right, and a result pane under both. On a terminal narrower
// than two panes need (60 columns), the panes stack instead.
//
//	go run ./examples/columns
package main

import (
	"fmt"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

var (
	commands = []string{"status", "commit", "push", "log"}
	selected = 0
	message  = "update notes"
	amend    = false
	pushed   = 0
)

func columnsScreen() *mint.Screen {
	list := make([]mint.Row, len(commands))
	for i := range commands {
		i := i
		list[i] = mint.Action(mint.ActionOpts{
			Label: commands[i],
			Get: func() string {
				if selected == i {
					return "selected"
				}
				return ""
			},
			OnActivate: func(*mint.Ctx) { selected = i },
		})
	}
	return &mint.Screen{
		Icon:     "▤",
		Title:    "Side by side",
		TopTitle: "Commands",
		TopWidth: 1,
		Sections: []mint.Section{{Name: "Commands", Rows: list}},
		Panes: []*mint.Pane{
			{
				Title: "Options", Beside: true, Width: 2,
				Refresh: func(p *mint.Pane) {
					p.Sections = []mint.Section{{Name: commands[selected], Rows: []mint.Row{
						mint.Text(mint.TextOpts{Label: "Message", Get: func() string { return message }, Set: func(v string) { message = v }}),
						mint.Checkbox(mint.CheckboxOpts{Label: "Amend", Get: func() bool { return amend }, Set: func(v bool) { amend = v }}),
					}}}
				},
			},
			{
				Title: "Result",
				Refresh: func(p *mint.Pane) {
					p.Sections = []mint.Section{{Name: "Output", Rows: []mint.Row{
						mint.Note(fmt.Sprintf("git %s -m %q (amend: %v)", commands[selected], message, amend)),
					}}}
				},
			},
		},
	}
}

func main() {
	app := &mint.App{
		Title: "◈ Columns",
		Items: []mint.HomeItem{{Icon: "▤", Label: "Side by side", Desc: "Panes beside each other", Open: columnsScreen}},
	}
	app.Run()
}
