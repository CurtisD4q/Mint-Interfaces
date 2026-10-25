package mint

import "testing"

// feed makes the next reads return data, then report "no input".
func feed(data string) {
	inPos, inLen = 0, 0
	pending := []byte(data)
	readInput = func(b []byte) (int, error) {
		if len(pending) == 0 {
			return 0, nil
		}
		n := copy(b, pending)
		pending = pending[n:]
		return n, nil
	}
}

func describe(k *Key) string {
	switch {
	case k == nil:
		return "<nil>"
	case k.Mouse != nil:
		if k.Mouse.Click {
			return "click"
		}
		return "mouse"
	case k.Special:
		return "[" + k.Name + "]"
	}
	return k.Name
}

func TestDecodeKeys(t *testing.T) {
	cases := map[string]string{
		"\x1b[A": "[up]", "\x1b[B": "[down]", "\x1bOC": "[right]", "\x1b[1;5D": "[left]",
		"\x1b[H": "[home]", "\x1bOF": "[end]", "\x1b[1~": "[home]", "\x1b[4~": "[end]",
		"\x1b[5~": "[pgup]", "\x1b[6~": "[pgdn]", "\x1b[3~": "[delete]", "\x1b[2~": "[insert]",
		"\x1b[15~": "[f5]", "\x1bOP": "[f1]", "\x1b[24~": "[f12]", "\x1b[Z": "[btab]",
		"\x1b": "[esc]", "\x1b\x1b": "[esc]", "\x1bx": "[M-x]", "\x1b<": "[M-<]",
		"\x1b[<0;10;5M": "click", "\x1b[<64;10;5M": "mouse", "\x1b[<0;10;5m": "<nil>",
		"\x1b[99X": "<nil>", "\x1b[9~": "<nil>", "\x1b[": "[esc]",
		"a": "a", "é": "é", "\r": "\r", "\x0e": "\x0e",
	}
	for in, want := range cases {
		feed(in)
		if got := describe(ReadKey()); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestUnknownSequenceLeavesNothingBehind(t *testing.T) {
	// Before, Delete was read as Esc and its "~" leaked out as a typed key.
	feed("\x1b[3~x")
	if k := ReadKey(); describe(k) != "[delete]" {
		t.Fatalf("got %s", describe(k))
	}
	if k := ReadKey(); describe(k) != "x" {
		t.Fatalf("next key got %s", describe(k))
	}
}

func TestIsText(t *testing.T) {
	for _, k := range []*Key{char("a"), char("é"), char(" ")} {
		if !k.IsText() {
			t.Errorf("%q should be text", k.Name)
		}
	}
	for _, k := range []*Key{named("home"), named("M-x"), char("\x7f"), char("\r"), char("\x0e"), nil} {
		if k.IsText() {
			t.Errorf("%s should not be text", describe(k))
		}
	}
}

func TestProfiles(t *testing.T) {
	defer SetKeyProfile("default")
	HomeHints, ScreenHints = defaultHomeHints, defaultScreenHints

	SetKeyProfile("emacs")
	for in, want := range map[string]string{"\x0e": "[down]", "\x10": "[up]", "\x06": "[right]",
		"\x02": "[left]", "\x07": "[esc]", "\x16": "[pgdn]", "j": "j"} {
		if got := describe(navKey(char(in), false)); got != want {
			t.Errorf("emacs %q: got %s want %s", in, got, want)
		}
	}
	if got := describe(navKey(named("M-v"), false)); got != "[pgup]" {
		t.Errorf("emacs M-v: %s", got)
	}
	if HomeHints != profileHints["emacs"][0] {
		t.Errorf("hints not switched: %q", HomeHints)
	}

	SetKeyProfile("vim")
	if got := describe(navKey(char("j"), false)); got != "[down]" {
		t.Errorf("vim j: %s", got)
	}
	if got := describe(navKey(char("j"), true)); got != "j" {
		t.Errorf("vim j while typing should stay j: %s", got)
	}
	if got := describe(navKey(char("\x04"), true)); got != "[pgdn]" {
		t.Errorf("vim C-d: %s", got)
	}
	if ScreenHints != profileHints["vim"][1] {
		t.Errorf("hints not switched: %q", ScreenHints)
	}

	SetKeyProfile("default")
	if got := describe(navKey(char("j"), false)); got != "j" {
		t.Errorf("default j: %s", got)
	}
	if HomeHints != defaultHomeHints {
		t.Errorf("hints not restored: %q", HomeHints)
	}

	// An app's own footer is left alone.
	HomeHints = " custom "
	SetKeyProfile("vim")
	if HomeHints != " custom " {
		t.Errorf("custom hints overwritten")
	}
	HomeHints = defaultHomeHints
	if SetKeyProfile("nonsense") {
		t.Error("unknown profile accepted")
	}
}

func TestHomeNoteAndFindItem(t *testing.T) {
	a := &App{Note: "Hello.", Require: []string{"sh", "definitely-missing-xyz"},
		Items: []HomeItem{{Label: "Settings"}, {Label: "Run backup"}}}
	if got := a.homeNote(); got != "definitely-missing-xyz is not installed or not on PATH; actions that use it will fail. Hello." {
		t.Errorf("note %q", got)
	}
	a.Require = []string{"sh"}
	if got := a.homeNote(); got != "Hello." {
		t.Errorf("note %q", got)
	}
	if a.findItem("  run BACKUP ") != 2 || a.findItem("nope") != 0 {
		t.Error("findItem")
	}
}
