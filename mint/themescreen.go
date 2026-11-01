package mint

// The Theme item every app's home list ends with, and the --theme flag.
//
// Choosing a theme on the Theme screen shows it straight away, across the
// whole interface, so each one can be tried. "Use this theme" saves it to
// theme.conf for every Mint app; leaving without saving puts the old
// colours back. --theme NAME uses a theme for one run and saves nothing.

import (
	"fmt"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

// themeItemLabel is the built-in item's label. An app with its own item of
// that name keeps its own.
const themeItemLabel = "Theme"

// themeDescs say what each theme looks like.
var themeDescs = map[string]string{
	"default":    "Mint's purple",
	"terminal":   "your terminal's own 16 colours",
	"nord":       "cool blues",
	"gruvbox":    "warm, retro",
	"dracula":    "dark purple and pink",
	"catppuccin": "soft pastels",
	"custom":     "your RGB colours in theme.conf",
}

// knownTheme reports whether name is one of theme.Names.
func knownTheme(name string) bool {
	for _, n := range theme.Names {
		if n == name {
			return true
		}
	}
	return false
}

// shownTheme is the theme on screen when it is not the saved one (set by
// --theme or a preview), or "".
var shownTheme string

// applyTheme shows the named theme now, without saving it.
func applyTheme(name string) bool {
	if !knownTheme(name) || !SetPalette(theme.Load(name)) {
		return false
	}
	shownTheme = name
	return true
}

// items is the home list: the app's items, then the Theme item unless the
// app turned it off or has its own.
func (a *App) items() []HomeItem {
	if a.NoThemeItem {
		return a.Items
	}
	for _, it := range a.Items {
		if strings.EqualFold(strings.TrimSpace(it.Label), themeItemLabel) {
			return a.Items
		}
	}
	out := append(append([]HomeItem(nil), a.Items...), HomeItem{
		Icon:  "◐",
		Label: themeItemLabel,
		Desc:  "Choose the colours for every Mint app",
		Open:  ThemeScreen,
	})
	return out
}

// ThemeScreen lists the themes. Choosing one shows it at once; "Use this
// theme" saves it. Leaving without saving restores the colours in use when
// the screen opened.
func ThemeScreen() *Screen {
	before := C
	saved := theme.Current()
	if !knownTheme(saved) {
		saved = "default"
	}
	showing := saved
	if shownTheme != "" {
		showing = shownTheme
	}
	beforeName := shownTheme

	options := make([]string, len(theme.Names))
	byLabel := map[string]string{}
	for i, n := range theme.Names {
		label := n
		if d := themeDescs[n]; d != "" {
			label = n + " — " + d
		}
		options[i] = label
		byLabel[label] = n
	}
	labelOf := func(name string) string {
		for _, o := range options {
			if byLabel[o] == name {
				return o
			}
		}
		return name
	}

	scr := &Screen{
		Icon:  "◐",
		Title: "Theme",
		Sections: []Section{
			{Rows: []Row{
				Note("Choose a theme to see it now. Use this theme saves it for every Mint app."),
				Radio(RadioOpts{
					ID:      "theme",
					Label:   "Themes",
					Options: options,
					Get:     func() string { return labelOf(showing) },
					Set: func(label string) {
						name := byLabel[label]
						if applyTheme(name) {
							showing = name
						}
					},
				}),
			}},
			{Rows: []Row{
				Action(ActionOpts{
					ID:    "save",
					Label: "Use this theme",
					Get: func() string {
						if showing == saved {
							return "saved"
						}
						return "not saved"
					},
					OnActivate: func(c *Ctx) {
						if err := theme.Set(showing); err != nil {
							ToastError("Could not save the theme: " + err.Error())
							return
						}
						saved = showing
						before, beforeName = C, showing
						Toast(fmt.Sprintf("Theme %s saved", showing))
					},
				}),
				Action(ActionOpts{
					ID:    "undo",
					Label: "Undo preview",
					OnActivate: func(c *Ctx) {
						C, shownTheme = before, beforeName
						showing = saved
						if beforeName != "" {
							showing = beforeName
						}
					},
				}),
			}},
		},
	}
	scr.Sections = append(scr.Sections, Section{Name: "Theme file", Rows: themeFileRows()})
	// A file that names a theme Mint does not have says so.
	scr.Banner = theme.Problem()
	scr.onLeave = func() { C, shownTheme = before, beforeName }
	return scr
}

// themeFlag reads --theme NAME or --theme=NAME at the start of args. It
// returns the name (empty when args do not start with it) and the
// arguments after it.
func themeFlag(args []string) (name string, rest []string, ok bool) {
	if len(args) == 0 {
		return "", args, false
	}
	switch arg := args[0]; {
	case arg == "--theme":
		if len(args) < 2 {
			return "", nil, true
		}
		return args[1], args[2:], true
	case strings.HasPrefix(arg, "--theme="):
		return strings.TrimPrefix(arg, "--theme="), args[1:], true
	}
	return "", args, false
}

// themeFileRows say where the theme is saved, and where it was read from
// when that is somewhere else (a sudo run reads the user's file).
func themeFileRows() []Row {
	rows := []Row{Note("Saved in " + theme.ConfigPath)}
	if rp := theme.ReadPath(); rp != theme.ConfigPath {
		rows = append(rows, Note("Running as root: using the colours in "+rp+" until you save one here"))
	}
	return rows
}
