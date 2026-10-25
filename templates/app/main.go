// Starter for a new Mint interface. Copy templates/app, set tool in cli.go,
// and replace the example home item with the program's screens, following
// CATALOGUE.md.
package main

import (
	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

func main() {
	app := &mint.App{
		Title:          "◈ " + tool,
		Crumb:          tool,
		Require:        []string{tool},
		Version:        "0.1.0",
		SectionSpacing: 1,
		Items: []mint.HomeItem{
			{Icon: "◧", Label: "Example", Desc: "Replace with the program's screens", Open: exampleScreen},
		},
	}
	app.Run()
	runner.StopAll() // don't leave background jobs running after the interface closes
}

func exampleScreen() *mint.Screen {
	return &mint.Screen{
		Title: "Example",
		Sections: []mint.Section{{Name: "About", Rows: []mint.Row{
			mint.Value(mint.ValueOpts{Label: "Version", Get: func() string { return version }}),
		}}},
		Enter: func(*mint.Screen) { version = query("--version") },
	}
}

var version string
