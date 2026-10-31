package mint

import (
	"strings"
	"testing"
)

func TestAlertRowIsRedAndBold(t *testing.T) {
	saved := C
	t.Cleanup(func() { C = saved })

	label := "Pattern - File path does not exist!"
	s := &Screen{Title: "Tree", Sections: []Section{{
		Name: "Input",
		Rows: []Row{Text(TextOpts{ID: "pattern", Label: label, Value: "$NOPE", Alert: true})},
	}}}

	C = loadPalette()
	frame := s.renderFrame(24, 100)
	plain := stripFrame(frame)
	if !strings.Contains(plain, label) {
		t.Fatalf("label missing from\n%s", plain)
	}
	style := styleBefore(frame, label)
	if !strings.Contains(style, C.Err) || !strings.Contains(style, C.Bold) {
		t.Fatalf("row is not red and bold: %q", style)
	}
	value := styleBefore(frame, "$NOPE")
	if !strings.Contains(value, C.Err) || !strings.Contains(value, C.Bold) {
		t.Fatalf("value is not red and bold: %q", value)
	}

	// NO_COLOR quantises to the mono palette: Err is bold, not a colour.
	// The label text is the warning on its own.
	C = monoPalette()
	frame = s.renderFrame(24, 100)
	plain = stripFrame(frame)
	if !strings.Contains(plain, label) {
		t.Fatalf("NO_COLOR label missing from\n%s", plain)
	}
	style = styleBefore(frame, label)
	if strings.Contains(style, "\x1b[38;") || strings.Contains(style, "\x1b[91m") || strings.Contains(style, "\x1b[31m") {
		t.Fatalf("NO_COLOR row still has a colour: %q", style)
	}
	if !strings.Contains(style, "\x1b[1m") {
		t.Fatalf("NO_COLOR row is not bold: %q", style)
	}
}

// styleBefore is the SGR immediately in front of needle. The frame is
// positioned with cursor sequences, so the whole screen is not one row.
func styleBefore(frame, needle string) string {
	i := strings.Index(frame, needle)
	if i < 0 {
		return ""
	}
	start := i - 80
	if start < 0 {
		start = 0
	}
	return frame[start:i]
}
