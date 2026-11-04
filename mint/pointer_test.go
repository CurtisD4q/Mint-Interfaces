package mint

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func resetPointer(t *testing.T) {
	t.Helper()
	old := rt.profile.SGRMouse
	rt.profile.SGRMouse = true
	t.Setenv("MINT_HOVER", "")
	ptr.mu.Lock()
	ptr.known, ptr.down, ptr.row, ptr.col, ptr.pressRow = false, false, 0, 0, 0
	ptr.mu.Unlock()
	t.Cleanup(func() { rt.profile.SGRMouse = old; forgetPointer() })
}

// Motion reports move the pointer. Only a new row, or the button going
// down or up, wakes a screen; motion within a row is swallowed.
func TestMotionReports(t *testing.T) {
	resetPointer(t)
	var w pointerWatch
	w.changed()
	if k := mouseKey("35;10;5", true); k != nil {
		t.Fatalf("first motion: %+v", k)
	}
	if !w.changed() {
		t.Fatal("watch missed the first motion")
	}
	if k := mouseKey("35;11;5", true); k != swallowed {
		t.Fatalf("same row: %+v", k)
	}
	if w.changed() {
		t.Fatal("same row woke the screen")
	}
	if r, ok := hoverRow(); !ok || r != 5 {
		t.Fatalf("hover row %d %v", r, ok)
	}
	if k := mouseKey("35;11;6", true); k != nil || !hoverAt(6) || hoverAt(5) {
		t.Fatal("row change not reported")
	}

	// A left press is still a click, and the row looks pressed until the
	// button goes up or the pointer leaves the row.
	k := mouseKey("0;11;6", true)
	if k == nil || k.Mouse == nil || !k.Mouse.Click || !pressedAt(6) {
		t.Fatalf("press: %+v", k)
	}
	if mouseKey("32;11;7", true) != nil || pressedAt(6) || pressedAt(7) {
		t.Fatal("dragging off the row should drop the pressed look")
	}
	if mouseKey("32;11;6", true) != nil || !pressedAt(6) {
		t.Fatal("dragging back should press it again")
	}
	if mouseKey("0;11;6", false) != nil || pressedAt(6) {
		t.Fatal("release should end the pressed look")
	}
	if mouseKey("0;11;6", false) != swallowed {
		t.Fatal("a second release changes nothing")
	}

	// The wheel still scrolls; losing focus drops the hover.
	if k := mouseKey("65;11;6", true); k == nil || k.Mouse == nil || !k.Mouse.WheelDown {
		t.Fatalf("wheel: %+v", k)
	}
	forgetPointer()
	if hoverAt(6) {
		t.Fatal("hover kept after focus was lost")
	}

	// Malformed reports are ignored.
	for _, p := range []string{"35;0;5", "x;1;1", "35;1", "-1;2;2"} {
		if k := mouseKey(p, true); k != nil {
			t.Fatalf("%q gave %+v", p, k)
		}
	}
}

func TestHoverOffWithEnv(t *testing.T) {
	resetPointer(t)
	t.Setenv("MINT_HOVER", "0")
	mouseKey("35;10;5", true)
	if hoverAt(5) {
		t.Fatal("hover drawn with MINT_HOVER=0")
	}
	if strings.Contains(rt.enterSeq(), "?1003h") {
		t.Fatal("motion reports asked for with MINT_HOVER=0")
	}
	t.Setenv("MINT_HOVER", "")
	if !strings.Contains(rt.enterSeq(), "?1003h") || !strings.Contains(rt.leaveSeq(), "?1003l") {
		t.Fatal("motion reports not turned on and off")
	}
}

var moveSeq = regexp.MustCompile(`\x1b\[(\d+);(\d+)H`)

// rowOf is the screen row a text is drawn on in a frame.
func rowOf(t *testing.T, frame, text string) int {
	t.Helper()
	i := strings.Index(frame, text)
	if i < 0 {
		t.Fatalf("%q not drawn", text)
	}
	m := moveSeq.FindAllStringSubmatch(frame[:i], -1)
	if len(m) == 0 {
		t.Fatalf("no cursor move before %q", text)
	}
	r, _ := strconv.Atoi(m[len(m)-1][1])
	return r
}

// The row under the pointer gets the hover mark and bright label; a
// pressed row is drawn in reverse video.
func TestHoverAndPressedLooks(t *testing.T) {
	resetPointer(t)
	s := paneScreen(1)
	plain := s.renderFrame(24, 80)
	r := rowOf(t, plain, "top-next")
	if strings.Contains(stripFrame(plain), hoverMark+"top-next") {
		t.Fatal("hover drawn with no pointer")
	}

	mouseKey("35;20;"+strconv.Itoa(r), true)
	if !s.ptrSeen.changed() {
		t.Fatal("screen not told the pointer moved")
	}
	hov := s.renderFrame(24, 80)
	if !strings.Contains(stripFrame(hov), hoverMark+"top-next") {
		t.Fatalf("no hover look:\n%s", stripFrame(hov))
	}
	if !strings.Contains(hov, C.Bright+"top-next") {
		t.Fatal("hovered label not bright")
	}

	pressPointer(r, 20)
	pr := s.renderFrame(24, 80)
	if !strings.Contains(pr, reverse) || !strings.Contains(stripFrame(pr), "▸ top-next") {
		t.Fatalf("no pressed look:\n%q", pr)
	}
	releasePointer(r, 20)
	if strings.Contains(s.renderFrame(24, 80), reverse) {
		t.Fatal("pressed look kept after release")
	}
}
