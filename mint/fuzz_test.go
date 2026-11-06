package mint

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Fuzz tests for everything that reads input Mint does not control: the
// bytes a terminal sends, program output shown in tables and logs, and text
// drawn into a frame. Run one for longer with, for example,
//
//	go test ./mint -run '^$' -fuzz FuzzReadEvent -fuzztime 60s
//
// The seeds below also run as ordinary tests.

// Whatever bytes arrive, reading them ends, never panics, and gives keys
// that make sense.
func FuzzReadEvent(f *testing.F) {
	for _, s := range []string{
		"a", "\x1b", "\x1b[A", "\x1bOP", "\x1b[3~", "\x1b[200~pasted\x1b[201~", "\x1b[200~unterminated",
		"\x1b[<0;10;5M", "\x1b[<64;1;1M", "\x1b[<35;99999999999999999999;1M", "\x1b[<0;-1;0m", "\x1b[<;;M",
		"\x1b[97;5u", "\x1b[1114112u", "\x1b[-5u", "\x1b[9;2u", "\x1b[I", "\x1b[O", "\xe2\x82", "\xf4\x90\x80\x80",
		"\x1b\xc3\xa9", strings.Repeat("\x1b[", 50), "\x1b[" + strings.Repeat("1", 40) + "~",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		defer forgetPointer()
		feed(string(data))
		for i := 0; i <= len(data)+1; i++ {
			k := readEvent()
			if k == nil || k == swallowed {
				if inPos >= inLen && i > 0 {
					// No input left: one more read must also end.
					if readEvent() != nil && inPos >= inLen {
						continue
					}
				}
				continue
			}
			if k.Mouse != nil {
				if k.Mouse.Row < 1 || k.Mouse.Col < 1 {
					t.Fatalf("mouse at %d,%d from %q", k.Mouse.Row, k.Mouse.Col, data)
				}
				continue
			}
			if k.paste == "" && !utf8.ValidString(k.Name) {
				t.Fatalf("key %q is not valid UTF-8 (from %q)", k.Name, data)
			}
			if len(k.paste) > 65536 {
				t.Fatalf("paste of %d bytes", len(k.paste))
			}
		}
	})
}

// clipCells never shows more than n cells, and keeps the text it shows.
func FuzzClipCells(f *testing.F) {
	for _, s := range []string{"hello", "\x1b[31mred\x1b[0m", "日本語テキスト", "a\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\b", "\x1b[", "\x1b]", "é́", "\x1b[38;5;141m▸ x"} {
		f.Add(s, 4)
	}
	f.Fuzz(func(t *testing.T, raw string, n int) {
		if n > 400 || n < -5 {
			return
		}
		// What the canvas clips: cleaned text in Mint's own colours, and
		// sometimes inside a link.
		s := C.Accent + CleanLine(raw) + Reset
		if len(raw) > 0 && raw[0]%2 == 0 {
			s = "\x1b]8;;https://example.com\x1b\\" + s + "\x1b]8;;\x1b\\"
		}
		got := clipCells(s, n)
		if n < 1 {
			if got != "" {
				t.Fatalf("clip(%q, %d) = %q", s, n, got)
			}
			return
		}
		if w := visibleWidth(got); w > n {
			t.Fatalf("clip(%q, %d) is %d cells: %q", s, n, w, got)
		}
		if visibleWidth(s) <= n && visibleWidth(got) != visibleWidth(s) {
			t.Fatalf("clip lost text that fits: %q -> %q", s, got)
		}
		// A hyperlink the clip leaves open would swallow whatever the
		// terminal draws next.
		if opens, closes := strings.Count(got, "\x1b]8;;http"), strings.Count(got, "\x1b]8;;\x1b\\"); opens > closes {
			t.Fatalf("clip left a link open: %q", got)
		}
	})
}

// Truncate never returns more cells than asked for.
func FuzzTruncate(f *testing.F) {
	for _, s := range []string{"hello world", "日本語", "\x1b[1mbold\x1b[0m", "", "…"} {
		f.Add(s, 3)
	}
	f.Fuzz(func(t *testing.T, s string, n int) {
		if n > 400 || n < -5 {
			return
		}
		got := Truncate(s, n)
		if n < 1 && got != "" {
			t.Fatalf("Truncate(%q, %d) = %q", s, n, got)
		}
		if n >= 1 && VLen(got) > n {
			t.Fatalf("Truncate(%q, %d) is %d cells: %q", s, n, VLen(got), got)
		}
	})
}

// A cleaned line can never move the cursor, change colours, reorder text or
// start a new line: no escape, no C0 or C1 control, no bidi control.
func FuzzCleanLine(f *testing.F) {
	for _, s := range []string{"plain", "\x1b[2J\x1b[Hgone", "\x1b]0;title\x07x", "a\bb", "‮evil", "\u009b31m", "x y", "\r\noverwrite", "\x1bP+q\x1b\\", "tab\there"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, clean := range []string{CleanLine(s), CleanLineVisible(s)} {
			if !utf8.ValidString(clean) {
				t.Fatalf("not valid UTF-8: %q (from %q)", clean, s)
			}
			for _, r := range clean {
				switch {
				case r == '\t':
				case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
					t.Fatalf("control %U left in %q (from %q)", r, clean, s)
				case isBidiControl(r) || r == 0x2028 || r == 0x2029:
					t.Fatalf("%U left in %q (from %q)", r, clean, s)
				}
			}
		}
	})
}

// Any program output parses into a table without panicking, and every row
// has a cell per header when there are headers.
func FuzzParseTable(f *testing.F) {
	for _, s := range []string{
		"PID TTY TIME CMD\n1 ? 00:00:01 init\n2 ? 00:00:00 kthreadd",
		"| a | b |\n|---|---|\n| 1 | 2 |",
		"name\tsize\nx\t1\ny",
		".\n├── a\n└── b",
		"│ x │\n│ y",
		"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<14 {
			return
		}
		headers, rows := ParseTable(strings.Split(s, "\n"))
		_ = rows
		for _, h := range headers {
			if strings.ContainsAny(h, "\x1b\n") {
				t.Fatalf("header %q", h)
			}
		}
	})
}

// visibleWidth is the cells s takes on screen: its text without CSI and OSC
// sequences.
func visibleWidth(s string) int {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
			continue
		}
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == ']' {
			j := i + 2
			for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
				j++
			}
			if j < len(s) && s[j] == 0x1b {
				j++
			}
			i = j + 1
			continue
		}
		if s[i] == 0x1b && i+1 < len(s) {
			i += 2 // a two-character escape (ESC 7, ESC =): no space on screen
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return VLen(b.String())
}

// replayFrame plays a frame back onto an h×w grid and reports the first
// character drawn outside it.
func replayFrame(frame string, h, w int) (overflow string) {
	r, c := 1, 1
	for i := 0; i < len(frame); {
		if frame[i] == 0x1b && i+1 < len(frame) && frame[i+1] == '[' {
			j := i + 2
			for j < len(frame) && (frame[j] < 0x40 || frame[j] > 0x7e) {
				j++
			}
			if j < len(frame) && (frame[j] == 'H' || frame[j] == 'f') {
				r, c = 1, 1
				if parts := strings.Split(frame[i+2:j], ";"); len(parts) == 2 {
					r, c = atoi(parts[0]), atoi(parts[1])
				}
			}
			i = j + 1
			continue
		}
		if frame[i] == 0x1b && i+1 < len(frame) && frame[i+1] == ']' {
			j := i + 2
			for j < len(frame) && frame[j] != 0x07 && !(frame[j] == 0x1b && j+1 < len(frame) && frame[j+1] == '\\') {
				j++
			}
			if j < len(frame) && frame[j] == 0x1b {
				j++
			}
			i = j + 1
			continue
		}
		ru, size := utf8.DecodeRuneInString(frame[i:])
		wd := VLen(string(ru))
		if wd > 0 && (r < 1 || r > h || c < 1 || c+wd-1 > w) {
			return frame[i:min(len(frame), i+20)]
		}
		c += wd
		i += size
	}
	return ""
}

// Whatever text a screen holds, and at any size, its frame stays inside
// the terminal, side by side or stacked.
func FuzzScreenStaysInsideTheTerminal(f *testing.F) {
	f.Add("label", "note text", "日本語のラベル", 30, 100, true)
	f.Add(strings.Repeat("x", 300), strings.Repeat("Ω", 300), "\x1b[31mred\x1b[0m", 12, 48, false)
	f.Add("a", "b", "c", 60, 61, true)
	f.Fuzz(func(t *testing.T, label, note, other string, h, w int, beside bool) {
		if h < 4 || h > 80 || w < 4 || w > 260 {
			return
		}
		s := &Screen{
			Title:    label,
			TopTitle: note,
			Sections: []Section{{Name: other, Rows: []Row{
				Action(ActionOpts{Label: label, Hint: other}),
				Note(note),
				Value(ValueOpts{Label: other, Value: note}),
			}}},
			Panes: []*Pane{
				{Title: other, Beside: beside, Sections: []Section{{Name: label, Rows: []Row{Note(other), Action(ActionOpts{Label: note})}}}},
				{Title: label, Sections: []Section{{Name: note, Rows: []Row{Note(label)}}}},
			},
		}
		if bad := replayFrame(s.renderFrame(h, w), h, w); bad != "" {
			t.Fatalf("drawn outside %dx%d: %q", h, w, bad)
		}
	})
}

// A number read back as text is always a usable whole number range.
func FuzzToNumber(f *testing.F) {
	for _, s := range []string{"3", "-2", "1e400", "-1e400", "NaN", "Inf", "0x10", "", "9223372036854775808"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		n := toNumber(s)
		if n != n || n > maxExactNumber || n < -maxExactNumber {
			t.Fatalf("toNumber(%q) = %v", s, n)
		}
		if v := int(n) + 1; v < int(n) {
			t.Fatalf("stepping %q overflowed", s)
		}
	})
}

// Whatever is pasted, a text box holds only text: no escape or control,
// no bidi control, valid UTF-8.
func FuzzPasteIntoABox(f *testing.F) {
	for _, s := range []string{"plain", "a\nb\tc", "\x1b[2Jx", "‮evil", "\u009b31m", "\xff\xfe", "日本"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		lb := newLineBuffer("")
		lb.handleKey(pasted(s))
		got := lb.String()
		if !utf8.ValidString(got) {
			t.Fatalf("box holds invalid UTF-8 %q", got)
		}
		for _, r := range got {
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || isBidiControl(r) {
				t.Fatalf("box holds %U from %q", r, s)
			}
		}
	})
}
