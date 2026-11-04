package mint

import (
	"os"
	"strings"
	"sync"
)

// Dimming when the terminal loses focus.
//
// Mint turns on focus reporting (mode 1004) where the terminal has it.
// When the window, or the tmux pane, the app is in loses focus, the last
// frame is drawn again faint (SGR 2), so the app you are typing in stands
// out from the ones you are not. When focus comes back, or any key
// arrives, the frame is drawn as it was. Nothing is laid out again: the
// frame is the one already on screen, so this costs one write.
//
// Set MINT_DIM_UNFOCUSED=0 to keep apps bright when unfocused.

var lastFrame struct {
	sync.Mutex
	s string // the last frame painted, before dimming
}

// dimWanted reports whether an unfocused app is drawn faint.
func dimWanted() bool {
	return rt.profile.FocusEvents && os.Getenv("MINT_DIM_UNFOCUSED") != "0"
}

// showFrame writes a frame, faint when the terminal has lost focus, and
// keeps it to draw again when focus changes.
func showFrame(frame string) {
	lastFrame.Lock()
	lastFrame.s = frame
	lastFrame.Unlock()
	if !rt.focused.Load() && dimWanted() {
		frame = dimFrame(frame)
	}
	writeFrame(rt.wrapFrame(frame))
}

// setFocused records a focus change and draws the last frame again in its
// new look.
func setFocused(focused bool) {
	if rt.focused.Swap(focused) == focused {
		return
	}
	if !focused {
		forgetPointer()
	}
	if !screenActive || !dimWanted() {
		return
	}
	lastFrame.Lock()
	frame := lastFrame.s
	lastFrame.Unlock()
	if frame == "" {
		return
	}
	if !focused {
		frame = dimFrame(frame)
	}
	writeFrame(rt.wrapFrame(frame))
}

// dimFrame is frame drawn faint. Faint is set after every cursor move and
// again after every colour sequence that would end it (a reset, or 22,
// "normal intensity"), so the whole frame stays faint whatever colours it
// uses. Colours themselves are kept.
func dimFrame(frame string) string {
	const faint = "\x1b[2m"
	var b strings.Builder
	b.Grow(len(frame) + len(frame)/8)
	b.WriteString(faint)
	for i := 0; i < len(frame); {
		if frame[i] != 0x1b || i+1 >= len(frame) || frame[i+1] != '[' {
			if frame[i] == 0x1b {
				n := escLen(frame[i:])
				b.WriteString(frame[i : i+n])
				i += n
				continue
			}
			b.WriteByte(frame[i])
			i++
			continue
		}
		n := escLen(frame[i:])
		seq := frame[i : i+n]
		b.WriteString(seq)
		i += n
		final := seq[len(seq)-1]
		switch {
		case final == 'H' || final == 'J':
			b.WriteString(faint)
		case final == 'm' && endsFaint(seq[2:len(seq)-1]):
			b.WriteString(faint)
		}
	}
	return b.String()
}

// endsFaint reports whether SGR parameters turn faint off: a reset (empty
// or 0) or normal intensity (22).
func endsFaint(params string) bool {
	if params == "" {
		return true
	}
	for _, p := range strings.Split(params, ";") {
		if p == "0" || p == "22" || p == "" {
			return true
		}
	}
	return false
}

// writeFrame sends a frame to the terminal; tests replace it.
var writeFrame = out
