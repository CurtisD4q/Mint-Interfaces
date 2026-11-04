package mint

import (
	"strings"
	"testing"
)

func TestDimFrame(t *testing.T) {
	in := "\x1b[2J\x1b[1;1H\x1b[38;5;99m╭─╮\x1b[0m\x1b[2;3H\x1b[1mTitle\x1b[22m plain\x1b[m end\x1b]8;;https://x.org/\x1b\\link\x1b]8;;\x1b\\"
	got := dimFrame(in)
	want := "\x1b[2m\x1b[2J\x1b[2m\x1b[1;1H\x1b[2m\x1b[38;5;99m╭─╮\x1b[0m\x1b[2m\x1b[2;3H\x1b[2m\x1b[1mTitle\x1b[22m\x1b[2m plain\x1b[m\x1b[2m end\x1b]8;;https://x.org/\x1b\\link\x1b]8;;\x1b\\"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	// The text is unchanged: only faint is added.
	if stripFrame(strings.ReplaceAll(got, "\x1b[2m", "")) != stripFrame(in) {
		t.Fatal("dimming changed the text")
	}
}

// Losing focus draws the last frame again faint; getting it back, or any
// key, draws it as it was. Nothing is drawn when dimming is off.
func TestFocusRepaints(t *testing.T) {
	var sent []string
	oldW, oldF, oldA, oldS := writeFrame, rt.profile.FocusEvents, screenActive, rt.profile.SyncOutput
	writeFrame = func(s string) { sent = append(sent, s) }
	rt.profile.FocusEvents, screenActive, rt.profile.SyncOutput = true, true, false
	t.Setenv("MINT_DIM_UNFOCUSED", "")
	t.Cleanup(func() {
		writeFrame, rt.profile.FocusEvents, screenActive, rt.profile.SyncOutput = oldW, oldF, oldA, oldS
		rt.focused.Store(true)
		lastFrame.s = ""
	})
	rt.focused.Store(true)

	frame := "\x1b[2J\x1b[1;1Hhello"
	showFrame(frame)
	if len(sent) != 1 || sent[0] != frame {
		t.Fatalf("focused frame: %q", sent)
	}
	setFocused(false)
	if len(sent) != 2 || sent[1] != dimFrame(frame) {
		t.Fatalf("unfocused repaint: %q", sent)
	}
	// A frame drawn while unfocused is faint too.
	showFrame(frame + "!")
	if sent[2] != dimFrame(frame+"!") {
		t.Fatalf("unfocused frame: %q", sent[2])
	}
	setFocused(false) // no change, no repaint
	if len(sent) != 3 {
		t.Fatalf("repainted without a change: %d", len(sent))
	}
	setFocused(true)
	if len(sent) != 4 || sent[3] != frame+"!" {
		t.Fatalf("refocused repaint: %q", sent[3])
	}

	// Off: focus changes draw nothing.
	t.Setenv("MINT_DIM_UNFOCUSED", "0")
	setFocused(false)
	showFrame(frame)
	if len(sent) != 5 || sent[4] != frame {
		t.Fatalf("dimmed with MINT_DIM_UNFOCUSED=0: %q", sent[len(sent)-1])
	}
	setFocused(true)
}

// Focus reports are decoded, and drop the hover.
func TestFocusReports(t *testing.T) {
	resetPointer(t)
	oldW := writeFrame
	writeFrame = func(string) {}
	defer func() { writeFrame = oldW; rt.focused.Store(true) }()
	mouseKey("35;5;5", true)
	setFocused(false)
	if rt.focused.Load() || hoverAt(5) {
		t.Fatal("focus-out not recorded")
	}
	setFocused(true)
	if !rt.focused.Load() {
		t.Fatal("focus-in not recorded")
	}
}
