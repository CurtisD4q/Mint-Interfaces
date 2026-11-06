package appkit

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

func suiteSpec() ToolSpec {
	return ToolSpec{
		Name: "gitx", Bin: "echo", ArchetypeID: "toolkit",
		Subcommands: []SubcommandSpec{
			{Name: "status", Label: "Status", Desc: "show the state", Args: []string{"status"}},
			{Name: "commit", Label: "Commit", Desc: "record changes", Args: []string{"commit"}},
		},
	}
}

func TestCommandSuiteLayout(t *testing.T) {
	a := newTestApp(t, suiteSpec())
	if got := a.InterfacePlan().Type; got != TypeCommandSuite {
		t.Fatalf("plan %s", got)
	}
	s := a.MainScreen()
	if s.TopTitle != titleCommands || s.TopWidth <= 0 {
		t.Fatalf("top %q width %v", s.TopTitle, s.TopWidth)
	}
	if len(s.Panes) < 2 || s.Panes[0].Title != titleOptions || !s.Panes[0].Beside || s.Panes[0].Width <= s.TopWidth {
		t.Fatalf("panes %+v", s.Panes)
	}
	if last := s.Panes[len(s.Panes)-1]; last.Title != titleOutput || last.Beside {
		t.Fatalf("output pane %+v", last)
	}
	// One row per command, in the list on the left.
	var ids []string
	for _, sec := range s.Sections {
		for _, r := range sec.Rows {
			ids = append(ids, rowField(r, "id"))
		}
	}
	if strings.Join(ids, ",") != "suite:command:status,suite:command:commit" {
		t.Fatalf("command rows %v", ids)
	}
}

func TestCommandSuiteFollowsTheSelectedCommand(t *testing.T) {
	a := newTestApp(t, suiteSpec())
	// The options pane describes the selected command and builds its line.
	if !strings.Contains(a.CommandPreview(), "status") || strings.Contains(a.CommandPreview(), "commit") {
		t.Fatalf("first command: preview %q", a.CommandPreview())
	}
	s := a.MainScreen()
	var found bool
	for _, r := range s.Sections[0].Rows {
		if rowField(r, "id") == "suite:command:commit" {
			activateRow(t, r)
			found = true
		}
	}
	if !found {
		t.Fatal("no row for the commit command")
	}
	if a.ActiveSubcommand != "commit" {
		t.Fatalf("active %q", a.ActiveSubcommand)
	}
	if !strings.Contains(a.CommandPreview(), "commit") || strings.Contains(a.CommandPreview(), "status") {
		t.Fatalf("second command: preview %q", a.CommandPreview())
	}
	// The screen is rebuilt on the next frame and still has the same shape.
	s = a.MainScreen()
	if s.TopTitle != titleCommands || len(s.Panes) == 0 || s.Panes[0].Title != titleOptions {
		t.Fatalf("after selecting: %q %+v", s.TopTitle, s.Panes)
	}
	grid := gridText(s.FrameText(32, 110), 32, 110)
	if !strings.Contains(grid, "record changes") || !strings.Contains(grid, "About this command") {
		t.Fatalf("the selected command's description is not shown\n%s", grid)
	}
	if strings.Contains(grid, "Mode description") || strings.Contains(grid, "┄ Toolkit") {
		t.Fatalf("stale wording from the picker layout\n%s", grid)
	}
}

func TestCommandSuiteDrawsBesideAndStacksWhenNarrow(t *testing.T) {
	a := newTestApp(t, suiteSpec())
	wide := gridText(a.MainScreen().FrameText(32, 110), 32, 110)
	if !hasLineWith(wide, "Commands", "Options") {
		t.Fatalf("wide: Commands and Options are not on one line\n%s", wide)
	}
	narrow := gridText(a.MainScreen().FrameText(40, 56), 40, 56)
	if hasLineWith(narrow, "Commands", "Options") {
		t.Fatalf("narrow: still side by side\n%s", narrow)
	}
	if !strings.Contains(narrow, "Options") || !strings.Contains(narrow, "commit") {
		t.Fatalf("narrow lost content\n%s", narrow)
	}
}

func TestClassicLayoutIsTheFallback(t *testing.T) {
	t.Setenv("MINT_CLASSIC_LAYOUT", "1")
	a := newTestApp(t, suiteSpec())
	s := a.MainScreen()
	if s.TopTitle != titleBasic || s.TopWidth != 0 {
		t.Fatalf("classic top %q width %v", s.TopTitle, s.TopWidth)
	}
	for _, p := range s.Panes {
		if p.Beside {
			t.Fatalf("classic screen has a pane beside another: %+v", p)
		}
	}
}

func TestOtherTypesAreNotChanged(t *testing.T) {
	for _, id := range []string{"inspect", "list", "search", "convert", "settings", "watch"} {
		s := newTestApp(t, ToolSpec{Name: id, Bin: "echo", ArchetypeID: id}).MainScreen()
		if s.TopTitle != titleBasic || s.TopWidth != 0 {
			t.Errorf("%s changed: top %q width %v", id, s.TopTitle, s.TopWidth)
		}
		for _, p := range s.Panes {
			if p.Beside {
				t.Errorf("%s has a pane beside another", id)
			}
		}
	}
}

// gridText replays a frame (cursor moves and text) into rows of text, so a
// test can see what shares a row on the screen.
func gridText(frame string, h, w int) string {
	grid := make([][]rune, h)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", w))
	}
	r, c := 1, 1
	for i := 0; i < len(frame); {
		if frame[i] == 0x1b && i+1 < len(frame) && frame[i+1] == '[' {
			j := i + 2
			for j < len(frame) && !(frame[j] >= '@' && frame[j] <= '~') {
				j++
			}
			if j < len(frame) && frame[j] == 'H' {
				r, c = 1, 1
				if parts := strings.Split(frame[i+2:j], ";"); len(parts) == 2 {
					r, c = atoiTest(parts[0]), atoiTest(parts[1])
				}
			}
			i = j + 1
			continue
		}
		ru, size := utf8.DecodeRuneInString(frame[i:])
		if r >= 1 && r <= h && c >= 1 && c <= w {
			grid[r-1][c-1] = ru
		}
		c++
		i += size
	}
	lines := make([]string, h)
	for i := range grid {
		lines[i] = strings.TrimRight(string(grid[i]), " ")
	}
	return strings.Join(lines, "\n")
}

func atoiTest(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func hasLineWith(text string, parts ...string) bool {
	for _, line := range strings.Split(text, "\n") {
		ok := true
		for _, p := range parts {
			if !strings.Contains(line, p) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// activateRow runs a row's OnActivate, as Enter on it would.
func activateRow(t *testing.T, r mint.Row) {
	t.Helper()
	f := reflect.ValueOf(&r).Elem().FieldByName("onActivate")
	fn, ok := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Interface().(func(*mint.Ctx))
	if !ok || fn == nil {
		t.Fatal("the row has no action")
	}
	fn(nil)
}

func monitorSpec() ToolSpec {
	return ToolSpec{
		Name: "free", Bin: "echo", ArchetypeID: "watch", ReadOnly: true,
		StatusArgs: []string{"-m"}, PollInterval: 2000000000, TableOutput: true,
		Flags: []FlagSpec{{Flag: "-h", Kind: FlagBool, Label: "Human readable", Section: "Display"}},
	}
}

func TestLiveMonitorLayout(t *testing.T) {
	a := newTestApp(t, monitorSpec())
	if got := a.InterfacePlan().Type; got != TypeLiveMonitor {
		t.Fatalf("plan %s", got)
	}
	s := a.MainScreen()
	if s.TopTitle != titleOptions || s.TopWidth <= 0 {
		t.Fatalf("top %q width %v", s.TopTitle, s.TopWidth)
	}
	if len(s.Panes) != 1 || s.Panes[0].Title != titleLive || !s.Panes[0].Beside || s.Panes[0].Width <= s.TopWidth {
		t.Fatalf("panes %+v", s.Panes)
	}
	// The live controls stay docked under both.
	if len(s.Dock) == 0 || s.Dock[0].Name != "Live" {
		t.Fatalf("dock %+v", s.Dock)
	}
	wide := gridText(s.FrameText(32, 110), 32, 110)
	if !hasLineWith(wide, "Options", "Live values") {
		t.Fatalf("wide: Options and Live values are not on one line\n%s", wide)
	}
	narrow := gridText(a.MainScreen().FrameText(40, 56), 40, 56)
	if hasLineWith(narrow, "Options", "Live values") || !strings.Contains(narrow, "Live values") {
		t.Fatalf("narrow\n%s", narrow)
	}
}

func TestMonitorWithAdvancedOptionsStacksThemUnderBoth(t *testing.T) {
	spec := monitorSpec()
	spec.Flags = append(spec.Flags, manyAdvanced()...)
	s := newTestApp(t, spec).MainScreen()
	var titles []string
	for _, p := range s.Panes {
		titles = append(titles, p.Title)
	}
	if strings.Join(titles, "|") != titleLive+"|"+titleAdvanced {
		t.Fatalf("panes %v", titles)
	}
	if !s.Panes[0].Beside || s.Panes[1].Beside {
		t.Fatalf("beside: live %v, advanced %v", s.Panes[0].Beside, s.Panes[1].Beside)
	}
}

func TestMonitorThatCannotReadItsStateKeepsTheClassicScreen(t *testing.T) {
	spec := monitorSpec()
	spec.ReadOnly, spec.StatusArgs = false, nil
	s := newTestApp(t, spec).MainScreen()
	if s.TopTitle != titleBasic || s.TopWidth != 0 {
		t.Fatalf("top %q width %v", s.TopTitle, s.TopWidth)
	}
	t.Setenv("MINT_CLASSIC_LAYOUT", "1")
	s = newTestApp(t, monitorSpec()).MainScreen()
	if s.TopTitle != titleBasic {
		t.Fatalf("classic override ignored: %q", s.TopTitle)
	}
}

// git has a known status command, which makes a plain app a state editor;
// as a command suite it runs the selected command instead.
func TestCommandSuiteRunsTheSelectedCommand(t *testing.T) {
	spec := suiteSpec()
	spec.StatusArgs = []string{"status"}
	a := newTestApp(t, spec)
	if a.executionMode() != execOnce {
		t.Fatalf("mode %v", a.executionMode())
	}
	s := a.MainScreen()
	if len(s.Dock) == 0 || s.Dock[0].Name != "Run" || s.Icon != "❖" {
		t.Fatalf("dock %+v icon %q", s.Dock, s.Icon)
	}
	if labels := strings.Join(screenLabelsOf(s.Dock), "|"); !strings.Contains(labels, "Run (gitx status)") {
		t.Fatalf("run button %q", labels)
	}
	t.Setenv("MINT_CLASSIC_LAYOUT", "1")
	if newTestApp(t, spec).executionMode() != execState {
		t.Fatal("the classic screen changed")
	}
}

func screenLabelsOf(secs []mint.Section) []string {
	var out []string
	for _, sec := range secs {
		for _, r := range sec.Rows {
			out = append(out, rowField(r, "label"))
		}
	}
	return out
}
