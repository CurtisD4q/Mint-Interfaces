package mint

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func linksOnForTest(t *testing.T) {
	t.Helper()
	old := rt.profile.OSC8
	rt.profile.OSC8 = true
	t.Setenv("MINT_LINKS", "")
	t.Setenv("INSIDE_EMACS", "")
	t.Cleanup(func() { rt.profile.OSC8 = old })
}

func TestLinkSpans(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "scan.xml")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	host := linkHost()
	fileLink := "file://" + host + file

	for _, tc := range []struct {
		text string
		want []string // link texts
		uris []string
	}{
		{"see https://example.org/a?b=1.", []string{"https://example.org/a?b=1"}, []string{"https://example.org/a?b=1"}},
		{"(https://en.wikipedia.org/wiki/Foo_(bar))", []string{"https://en.wikipedia.org/wiki/Foo_(bar)"}, nil},
		{"Saved " + file + ":12:3 ok", []string{file}, []string{fileLink}},
		{"wrote '" + file + "'.", []string{file}, []string{fileLink}},
		{"net 10.0.0.0/24 via /nonexistent/x", nil, nil},
		{"and/or a/b", nil, nil},
		{"javascript://x/y data:text/html,hi", nil, nil},
		{"ftp://files.example.org/pub/x.tgz", []string{"ftp://files.example.org/pub/x.tgz"}, nil},
	} {
		spans := linkSpans(tc.text)
		r := []rune(tc.text)
		var got, uris []string
		for _, s := range spans {
			got = append(got, string(r[s.start:s.end]))
			uris = append(uris, s.uri)
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%q: links %q, want %q", tc.text, got, tc.want)
		}
		if tc.uris != nil && strings.Join(uris, "|") != strings.Join(tc.uris, "|") {
			t.Errorf("%q: uris %q, want %q", tc.text, uris, tc.uris)
		}
	}
}

// Links wrap only the link's own characters, skip colour sequences and the
// markers a cut line gets, and are always closed.
func TestWithLinks(t *testing.T) {
	linksOnForTest(t)
	text := "go https://example.org/x now"
	spans := linkSpans(text)
	styled := C.Text + text + Reset
	got := withLinks(styled, spans, func(v int) int { return v })
	want := C.Text + "go " + osc8Open("https://example.org/x") + "https://example.org/x" + osc8Close + " now" + Reset
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}

	// Cut short: "go https://exa…" — the visible part still opens the
	// whole link, and the "…" is not part of it.
	shown := "go https://exa…"
	got = withLinks(shown, spans, truncSrc(text, shown))
	if !strings.HasSuffix(got, "https://exa"+osc8Close+"…") {
		t.Fatalf("truncated: %q", got)
	}

	// Panned: "‹" first is a marker.
	got = withLinks("‹ps://example.org/x n…", spans, sliceSrc(6, 26, true))
	if !strings.HasPrefix(got, "‹"+osc8Open("https://example.org/x")+"ps://") {
		t.Fatalf("panned: %q", got)
	}

	// Off: nothing added.
	t.Setenv("MINT_LINKS", "0")
	if withLinks(styled, spans, func(v int) int { return v }) != styled {
		t.Fatal("links drawn with MINT_LINKS=0")
	}
	t.Setenv("MINT_LINKS", "")
	t.Setenv("INSIDE_EMACS", "29.1,term:0.96")
	if linksOn() {
		t.Fatal("links drawn in Emacs term")
	}
	t.Setenv("INSIDE_EMACS", "vterm")
	if !linksOn() {
		t.Fatal("links off in vterm")
	}
}

// A program cannot hide a link target: its own OSC 8 is cleaned away, and
// a value cut with "…" is not linked.
func TestLinksShowTheirTarget(t *testing.T) {
	linksOnForTest(t)
	if got := CleanLine("\x1b]8;;https://evil.example\x1b\\https://bank.example\x1b]8;;\x1b\\"); got != "https://bank.example" {
		t.Fatalf("program link kept: %q", got)
	}
	if got := linkWidget(C.Text + "https://example.org/very/lo…" + Reset); strings.Contains(got, "\x1b]8") {
		t.Fatalf("cut link linked: %q", got)
	}
	if got := linkWidget(C.Text + "https://example.org/x" + Reset); !strings.Contains(got, osc8Open("https://example.org/x")) {
		t.Fatalf("value not linked: %q", got)
	}
}

func TestLogLineLinks(t *testing.T) {
	linksOnForTest(t)
	cv := begin(10, 60)
	it := Row{logLine: LogLine{Text: "fetched https://example.org/file.tar.gz"}}
	cv.logLine(2, 60, &it)
	if !strings.Contains(cv.b.String(), osc8Open("https://example.org/file.tar.gz")+"https://example.org/file.tar.gz"+Reset+osc8Close) &&
		!strings.Contains(cv.b.String(), osc8Open("https://example.org/file.tar.gz")+"https://example.org/file.tar.gz"+osc8Close) {
		t.Fatalf("no link: %q", cv.b.String())
	}
}

func TestCopy(t *testing.T) {
	var sent []string
	old := writeClipboard
	writeClipboard = func(s string) { sent = append(sent, s) }
	defer func() { writeClipboard = old }()
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")

	if err := Copy("nmap -sV \x1b[31mhost\x1b[0m\x07\nline 2\n"); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("nmap -sV host\nline 2")) + "\x07"
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("sent %q, want %q", sent, want)
	}

	// Inside tmux: plain, and wrapped for passthrough.
	t.Setenv("TMUX", "/tmp/tmux-0/default,1,0")
	sent = nil
	Copy("x")
	if !strings.HasPrefix(sent[0], "\x1b]52;c;eA==\x07\x1bPtmux;\x1b\x1b]52;c;eA==\x07\x1b\\") {
		t.Fatalf("tmux: %q", sent[0])
	}

	// Too long, or nothing: refused, nothing sent.
	sent = nil
	if err := Copy(strings.Repeat("a", MaxCopyBytes+1)); !errors.Is(err, ErrCopyTooLong) || len(sent) != 0 {
		t.Fatalf("long: %v %d", err, len(sent))
	}
	if err := Copy("\x1b[0m\n"); err == nil {
		t.Fatal("empty copied")
	}

	// The guard keeps output off the clipboard; Mint's own text is not output.
	CopyGuard = func() error { return errors.New("secret") }
	defer func() { CopyGuard = nil }()
	if err := copyOutput("result"); err == nil || len(sent) != 0 {
		t.Fatal("guard ignored")
	}
	if err := Copy("command preview"); err != nil {
		t.Fatal(err)
	}
}

// y copies the selected row: a value as shown, a table row's cells.
func TestScreenCopiesSelectedRow(t *testing.T) {
	var sent []string
	old := writeClipboard
	writeClipboard = func(s string) { sent = append(sent, s) }
	defer func() { writeClipboard = old }()
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	decode := func(s string) string {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "\x1b]52;c;"), "\x07")
		b, _ := base64.StdEncoding.DecodeString(s)
		return string(b)
	}
	s := &Screen{Sections: []Section{{Name: "Run", Rows: []Row{
		Value(ValueOpts{Label: "Command preview", Get: func() string { return "nmap -sV scanme.nmap.org" }}),
		Action(ActionOpts{Label: "Run nmap"}),
	}}}}
	if !strings.Contains(stripFrame(s.renderFrame(20, 80)), "y copy") {
		t.Fatal("no y copy hint on a value row")
	}
	s.pressKey(char("y"), 20, 80)
	if len(sent) != 1 || decode(sent[0]) != "nmap -sV scanme.nmap.org" {
		t.Fatalf("copied %q", sent)
	}
	s.pressKey(named("down"), 20, 80)
	s.pressKey(char("y"), 20, 80)
	if len(sent) != 2 || decode(sent[1]) != "Run nmap" {
		t.Fatalf("copied %q", sent)
	}
	toast.Lock()
	toast.msg = ""
	toast.Unlock()
}
