package mint

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/theme"
)

// Version of the library.
const Version = "0.1.0"

// HomeItem is one entry on the home list: an icon, a bold label, and a dim
// description. Open returns the screen to show (nil stays on the list).
type HomeItem struct {
	Icon  string
	Label string
	Desc  string
	Open  func() *Screen
}

// App is the whole program: a home list and a stack of screens.
type App struct {
	Title       string // default "◈ App"
	Crumb       string // breadcrumb on inner screens; default Title
	Note        string // optional amber line under the home title
	Items       []HomeItem
	ScreenHints string // footer override for every screen

	// SectionSpacing puts that many blank lines between named sections on
	// every screen (a screen's own setting wins).
	SectionSpacing int

	// Status returns the badges shown on the left of the footer on the home
	// list and on every screen without its own Status. It is called about
	// ten times a second; keep it cheap.
	Status func() []Badge

	// Version is the app's own version, reported by --version and
	// --mint-info.
	Version string

	// Require lists external programs this interface expects on PATH. Any
	// that are missing are named at the top of the home list.
	Require []string

	// Keys is the app's default key profile: "default", "emacs" or "vim".
	// The MINT_KEYS environment variable overrides it.
	Keys string

	// NoFlags turns off Mint's own command-line flags (--help, --man,
	// --version, --mint-info, --screen), for apps that parse their
	// arguments themselves.
	NoFlags bool

	// NoThemeItem leaves out the Theme item Mint adds at the end of the
	// home list (see ThemeScreen).
	NoThemeItem bool

	all []HomeItem // Items plus the Theme item, set by Run
}

const homeBlock = 3

func runHome(items []HomeItem, startSel int, title, breadcrumb, note, hints string, statusFn func() []Badge) (int, bool) {
	sel := startSel
	if sel < 1 {
		sel = 1
	}
	if sel > len(items) {
		sel = max(1, len(items))
	}
	scroll := 0
	var sb *scrollInfo
	hit := map[int]int{}
	lastClickIdx, lastClickTime := 0, time.Time{}
	footSig := ""
	pageItems := 1

	draw := func(h, w int) {
		cv := begin(h, w)
		if w < 40 || h < 12 {
			cv.frame(h, w)
			cv.put(2, 3, C.Amber+"Terminal too small"+Reset)
			cv.put(3, 3, C.Muted+fmt.Sprintf("need 40x12, have %dx%d", w, h)+Reset)
			cv.paint()
			return
		}
		cv.frame(h, w)
		cv.chromeTitle(w, title, breadcrumb)
		cv.titleRule(w)
		var noteLines []string
		if note != "" {
			nl := wrap2(note, max(8, w-6))
			noteLines = append(noteLines, nl[0])
			if nl[1] != "" {
				noteLines = append(noteLines, nl[1])
			}
		}
		// Row 3 is the rule. Notes move down onto the blank row that
		// used to sit under them, so the list still starts where it did.
		contentTop := 4
		if len(noteLines) > 0 && noteLines[0] != "" {
			cv.put(4, 3, C.Amber+noteLines[0]+Reset)
			contentTop = 5
			if len(noteLines) > 1 {
				cv.put(5, 3, C.Amber+noteLines[1]+Reset)
				contentTop = 6
			}
		}
		contentBottom := h - 2
		avail := contentBottom - contentTop + 1
		vis := max(1, floorf(float64(avail)/homeBlock))
		pageItems = vis
		n := len(items)
		if sel < 1 {
			sel = 1
		}
		if n > 0 && sel > n {
			sel = n
		}
		if sel < scroll+1 {
			scroll = sel - 1
		}
		if sel > scroll+vis {
			scroll = sel - vis
		}
		if scroll < 0 {
			scroll = 0
		}
		maxscroll := max(0, n-vis)
		if scroll > maxscroll {
			scroll = maxscroll
		}
		textw := max(8, w-8)
		hit = map[int]int{}
		if n == 0 {
			cv.put(contentTop, 3, C.Dim+"No entries"+Reset)
		}
		for idx := scroll + 1; idx <= min(scroll+vis, n); idx++ {
			cat := items[idx-1]
			y := contentTop + (idx-scroll-1)*homeBlock
			if y > contentBottom {
				continue
			}
			label := cat.Label
			if cat.Icon != "" {
				label = cat.Icon + " " + label
			}
			label = Truncate(label, textw)
			if idx == sel {
				cv.selBar(y, w)
				cv.put(y, 3, C.SelBg+C.Accent+C.Bold+"▸ "+C.Bright+label+Reset)
			} else {
				cv.put(y, 3, C.Text+"  "+label+Reset)
			}
			lines := wrap2(cat.Desc, textw)
			if lines[0] != "" && y+1 <= contentBottom {
				cv.put(y+1, 5, C.Dim+lines[0]+Reset)
			}
			if lines[1] != "" && y+2 <= contentBottom {
				cv.put(y+2, 5, C.Dim+lines[1]+Reset)
			}
			for dy := 0; dy < homeBlock; dy++ {
				if y+dy <= contentBottom {
					hit[y+dy] = idx
				}
			}
		}
		sb = nil
		if n > vis {
			sbx := w - 1
			barrows := min(vis*homeBlock, avail)
			thumb := min(barrows, max(1, floorf(float64(barrows*vis)/float64(n)+0.5)))
			maxtop := barrows - thumb
			tpos := 0
			if maxscroll > 0 {
				tpos = floorf(float64(scroll)/float64(maxscroll)*float64(maxtop) + 0.5)
			}
			sb = &scrollInfo{col: sbx, top: contentTop, rows: barrows, maxoff: maxscroll, vis: vis}
			cv.scrollbar(sbx, contentTop, barrows, thumb, tpos)
		}
		badges := callStatus(statusFn)
		cv.footer(h, w, hints, badges)
		footSig = footerSig(badges)
		cv.paint()
	}

	h, w := TermSize()
	draw(h, w)
	sc := 0
	for {
		k := readNav(false)
		dirty := false
		switch {
		case k == nil:
			sc++
			if rt.consumeResize() || sc >= 2 {
				sc = 0
				if nh, nw := TermSize(); nh != h || nw != w {
					h, w = nh, nw
					dirty = true
				}
			}
			if footerSig(callStatus(statusFn)) != footSig {
				dirty = true
			}
		case k.Mouse != nil && (k.Mouse.WheelUp || k.Mouse.WheelDown):
			if k.Mouse.WheelUp && sel > 1 {
				sel--
				dirty = true
			}
			if k.Mouse.WheelDown && sel < len(items) {
				sel++
				dirty = true
			}
		case k.Mouse != nil && k.Mouse.Click:
			m := k.Mouse
			if sb != nil && m.Col == sb.col && m.Row >= sb.top && m.Row <= sb.top+sb.rows-1 {
				frac := float64(m.Row-sb.top) / float64(max(1, sb.rows-1))
				scroll = clampInt(int(math.Floor(frac*float64(sb.maxoff)+0.5)), 0, sb.maxoff)
				if sel < scroll+1 {
					sel = scroll + 1
				}
				if sel > scroll+sb.vis {
					sel = scroll + sb.vis
				}
				dirty = true
			} else if idx, ok := hit[m.Row]; ok {
				now := time.Now()
				if idx == sel && lastClickIdx == idx && now.Sub(lastClickTime) <= 450*time.Millisecond {
					return sel, true
				}
				sel = idx
				dirty = true
				lastClickIdx, lastClickTime = idx, now
			}
		case k.Name == "up":
			if sel > 1 {
				sel--
				dirty = true
			}
		case k.Name == "down":
			if sel < len(items) {
				sel++
				dirty = true
			}
		case k.Name == "home" || k.Name == "end" || k.Name == "pgup" || k.Name == "pgdn":
			switch k.Name {
			case "home":
				sel = 1
			case "end":
				sel = len(items)
			case "pgup":
				sel = max(1, sel-pageItems)
			case "pgdn":
				sel = min(len(items), sel+pageItems)
			}
			dirty = true
		case k.Name == " " || k.Name == "\r" || k.Name == "\n":
			if sel >= 1 && sel <= len(items) {
				return sel, true
			}
		case k.Name == "q" || k.Name == "\x03":
			return 0, false
		}
		if dirty {
			draw(h, w)
		}
	}
}

// homeItems is the home list Run shows: the app's items and the Theme
// item.
func (a *App) homeItems() []HomeItem {
	if a.all != nil {
		return a.all
	}
	return a.items()
}

// Run takes over the terminal until the user quits. A panic anywhere in the
// app restores the terminal, prints the error and exits with status 1.
//
// Unless NoFlags is set, the first command-line argument may be one of
// Mint's flags: --help (-h), --man, --version (-v), --mint-info, or
// --screen NAME, which opens that home item straight away. --theme NAME
// may come before any of them and uses that theme for this run only. --help prints
// usage. --man opens the Manual screen, which is the full manual viewer.
func (a *App) Run() {
	title := a.Title
	if title == "" {
		title = "◈ App"
	}
	crumb := a.Crumb
	if crumb == "" {
		crumb = a.Title
	}
	if crumb == "" {
		crumb = "App"
	}
	note := a.homeNote()
	a.all = a.items()

	if a.Keys != "" {
		SetKeyProfile(a.Keys)
	}
	profileFromEnv()

	start := 0 // 1-based home item to open first
	if !a.NoFlags && len(os.Args) > 1 {
		done, item := a.handleFlags(title, crumb, note)
		if done {
			return
		}
		start = item
	}

	if !isTerminal() {
		fmt.Fprintf(os.Stderr, "%s needs an interactive terminal. Run it from a terminal, or see %s --help.\n",
			crumb, os.Args[0])
		os.Exit(2)
	}

	ScreenEnter()
	failed := func() (msg string) {
		defer func() {
			if p := recover(); p != nil && p != errHangup {
				msg = fmt.Sprintf("%v\n%s", p, debug.Stack())
			}
		}()
		var stack []*Screen
		sel := 1
		open := func(choice int) {
			sel = choice
			if item := a.all[choice-1]; item.Open != nil {
				var scr *Screen
				if p := withLoading("Loading "+item.Label+"…", func() { scr = item.Open() }); p != nil {
					panic(p)
				}
				if scr != nil {
					stack = append(stack, scr)
				}
			}
		}
		if start > 0 {
			open(start)
		}
		for {
			if len(stack) == 0 {
				choice, ok := runHome(a.all, sel, title, "Home", note, HomeHints, a.Status)
				if !ok {
					return ""
				}
				open(choice)
				continue
			}
			scr := stack[len(stack)-1]
			if scr.Breadcrumb == "" {
				scr.Breadcrumb = crumb
			}
			if scr.Hints == "" && a.ScreenHints != "" {
				scr.Hints = a.ScreenHints
			}
			if scr.SectionSpacing == 0 {
				scr.SectionSpacing = a.SectionSpacing
			}
			scr.appStatus = a.Status
			res := scr.run()
			switch res.op {
			case "quit":
				return ""
			case "back":
				if scr.onLeave != nil {
					scr.onLeave()
				}
				stack = stack[:len(stack)-1]
			case "push":
				if res.screen != nil {
					stack = append(stack, res.screen)
				}
			}
		}
	}()
	ScreenLeave()
	if failed != "" {
		fmt.Fprint(os.Stderr, "error: "+strings.TrimRight(failed, "\n")+"\n")
		os.Exit(1)
	}
}

// missingTools lists the Require entries that are not on PATH.
func (a *App) missingTools() []string {
	var missing []string
	for _, bin := range a.Require {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	return missing
}

// homeNote is the app's Note, preceded by a warning about missing tools.
func (a *App) homeNote() string {
	var warn string
	switch m := a.missingTools(); len(m) {
	case 0:
	case 1:
		warn = m[0] + " is not installed or not on PATH; actions that use it will fail."
	default:
		warn = "Not installed or not on PATH: " + strings.Join(m, ", ") + "."
	}
	switch {
	case warn == "":
		return a.Note
	case a.Note == "":
		return warn
	}
	return warn + " " + a.Note
}

// findItem returns the 1-based home item whose label matches name
// (ignoring case and surrounding space), or 0.
func (a *App) findItem(name string) int {
	name = strings.TrimSpace(name)
	for i, it := range a.homeItems() {
		if strings.EqualFold(strings.TrimSpace(it.Label), name) {
			return i + 1
		}
	}
	return 0
}

// handleFlags looks at the first argument only. It returns done=true when
// a flag printed something and the app should exit, or the home item that
// --screen asked for.
func (a *App) handleFlags(title, crumb, note string) (done bool, item int) {
	return a.handleArgs(os.Args[1:], title, crumb, note)
}

// handleArgs is handleFlags for args (the arguments after the program
// name). --theme NAME may come first, before one of the other flags.
func (a *App) handleArgs(args []string, title, crumb, note string) (done bool, item int) {
	prog := filepath.Base(os.Args[0])
	if name, rest, ok := themeFlag(args); ok {
		if !applyTheme(name) {
			if name == "" {
				fmt.Fprintf(os.Stderr, "%s: --theme needs a name. Themes: %s\n", prog, strings.Join(theme.Names, ", "))
			} else {
				fmt.Fprintf(os.Stderr, "%s: no theme named %q. Themes: %s\n", prog, name, strings.Join(theme.Names, ", "))
			}
			os.Exit(2)
		}
		args = rest
	}
	if len(args) == 0 {
		return false, 0
	}
	switch arg := args[0]; {
	case arg == "-v" || arg == "--version":
		if a.Version != "" {
			fmt.Printf("%s %s (Mint-Interfaces %s)\n", crumb, a.Version, Version)
		} else {
			fmt.Printf("%s (Mint-Interfaces %s)\n", crumb, Version)
		}
		return true, 0
	case arg == "-h" || arg == "--help":
		fmt.Println(title)
		if note != "" {
			fmt.Println(note)
		}
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %-28s open the interface\n", prog)
		fmt.Printf("  %-28s open one screen directly\n", prog+" --screen NAME")
		fmt.Printf("  %-28s use a theme for this run (%s)\n", prog+" --theme NAME", strings.Join(theme.Names, ", "))
		fmt.Printf("  %-28s print the version\n", prog+" --version")
		fmt.Printf("  %-28s print a description as JSON, for launchers\n", prog+" --mint-info")
		fmt.Printf("  %-28s show this help\n", prog+" --help")
		fmt.Printf("  %-28s open the manual\n", prog+" --man")
		if len(a.homeItems()) > 0 {
			fmt.Println()
			fmt.Println("Screens:")
			for _, it := range a.homeItems() {
				fmt.Printf("  %-26s %s\n", it.Label, it.Desc)
			}
		}
		fmt.Println()
		fmt.Println("Keys: set MINT_KEYS to default, emacs or vim (now: " + keyProfile + ").")
		fmt.Println("See `mint-gen --help` for a guide.")
		return true, 0
	case arg == "--man":
		if i := a.findItem("Manual"); i > 0 {
			return false, i
		}
		fmt.Fprintf(os.Stderr, "%s: no Manual screen. Screens:", prog)
		for _, it := range a.homeItems() {
			fmt.Fprintf(os.Stderr, " %q", it.Label)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(2)
	case arg == "--mint-info":
		type itemInfo struct {
			Icon  string `json:"icon,omitempty"`
			Label string `json:"label"`
			Desc  string `json:"desc,omitempty"`
		}
		items := make([]itemInfo, len(a.homeItems()))
		for i, it := range a.homeItems() {
			items[i] = itemInfo{Icon: it.Icon, Label: it.Label, Desc: it.Desc}
		}
		info := struct {
			Title   string     `json:"title"`
			Crumb   string     `json:"crumb"`
			Version string     `json:"version,omitempty"`
			Mint    string     `json:"mint"`
			Note    string     `json:"note,omitempty"`
			Require []string   `json:"require,omitempty"`
			Missing []string   `json:"missing,omitempty"`
			Items   []itemInfo `json:"items"`
		}{title, crumb, a.Version, Version, note, a.Require, a.missingTools(), items}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(info)
		return true, 0
	case arg == "--screen" || strings.HasPrefix(arg, "--screen="):
		name := strings.TrimPrefix(strings.TrimPrefix(arg, "--screen"), "=")
		if name == "" && len(args) > 1 {
			name = args[1]
		}
		if i := a.findItem(name); i > 0 {
			return false, i
		}
		fmt.Fprintf(os.Stderr, "%s: no screen named %q. Screens:", prog, name)
		for _, it := range a.homeItems() {
			fmt.Fprintf(os.Stderr, " %q", it.Label)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(2)
	}
	return false, 0
}
