// The "Build a screen" program from the README.
//
//	go run ./example
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
