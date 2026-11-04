package mint

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Clickable links (OSC 8).
//
// URLs and file paths in a program's output are sent to the terminal as
// OSC 8 hyperlinks, so a terminal that supports them (kitty, WezTerm,
// iTerm2, GNOME Terminal and other VTE terminals, Windows Terminal, foot,
// Alacritty, tmux 3.4+) opens them with a click, usually Ctrl- or
// Cmd-click. Elsewhere the sequences are ignored and the text looks the
// same.
//
// Only text that is itself the link is linked: an http, https or ftp URL,
// or an absolute (or ~/) path to a file or folder that exists. The target
// is always what is shown, never something hidden. A program's own OSC 8
// sequences are removed when its output is cleaned, so a program cannot
// make text that says one thing open another.
//
// Set MINT_LINKS=0 to turn them off.

// linkSpan is a link in a line: its runes [start, end) and where it goes.
type linkSpan struct {
	start, end int
	uri        string
}

const (
	osc8Close  = "\x1b]8;;\x1b\\"
	maxLinkURI = 2048
)

func osc8Open(uri string) string { return "\x1b]8;;" + uri + "\x1b\\" }

// linksOn reports whether output gets OSC 8 links.
func linksOn() bool {
	if !rt.profile.OSC8 || os.Getenv("MINT_LINKS") == "0" {
		return false
	}
	// Emacs's term shows unknown OSC sequences as text. vterm does not.
	if ie := os.Getenv("INSIDE_EMACS"); ie != "" && !strings.Contains(ie, "vterm") {
		return false
	}
	return true
}

var (
	urlRe = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[!#-'*-;=?-Z\x5d_a-z~]+(?:\([!#-'*-;=?-Z\x5d_a-z~]*\)[!#-'*-;=?-Z\x5d_a-z~]*)*`)
	// A path starts a word (or follows a quote, =, a bracket or a colon)
	// and is absolute or under ~/.
	pathRe = regexp.MustCompile(`(?:^|[\s"'=(\[<,:])((?:~)?/[A-Za-z0-9._+\-@%/=~#:,]+)`)
	// Line and column numbers after a path (main.go:12:4) are not part of it.
	lineSuffix = regexp.MustCompile(`(?::\d+){1,2}:?$`)
)

// linkSpans finds the links in plain text.
func linkSpans(text string) []linkSpan {
	if text == "" || (!strings.Contains(text, "://") && !strings.Contains(text, "/")) {
		return nil
	}
	var spans []linkSpan
	taken := func(a, b int) bool {
		for _, s := range spans {
			if a < s.end && b > s.start {
				return true
			}
		}
		return false
	}
	add := func(byteStart, byteEnd int, uri string) {
		if uri == "" || len(uri) > maxLinkURI {
			return
		}
		a := utf8.RuneCountInString(text[:byteStart])
		b := a + utf8.RuneCountInString(text[byteStart:byteEnd])
		if b > a && !taken(a, b) {
			spans = append(spans, linkSpan{start: a, end: b, uri: uri})
		}
	}
	for _, m := range urlRe.FindAllStringIndex(text, -1) {
		s := trimURL(text[m[0]:m[1]])
		if strings.Count(s, "/") >= 2 && len(s) > len("http://x") {
			add(m[0], m[0]+len(s), s)
		}
	}
	for _, m := range pathRe.FindAllStringSubmatchIndex(text, -1) {
		a, b := m[2], m[3]
		p := strings.TrimRight(text[a:b], ".,:;")
		p = lineSuffix.ReplaceAllString(p, "")
		p = strings.TrimRight(p, ".,:;")
		if len(p) < 2 || strings.Contains(text[max(0, a-3):a], "://") {
			continue
		}
		if uri := fileURI(p); uri != "" {
			add(a, a+len(p), uri)
		}
	}
	// Keep them in order for withLinks.
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].start < spans[j-1].start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}
	return spans
}

// trimURL drops punctuation that ends a sentence rather than the URL, and
// a closing bracket with no opening one inside the URL.
func trimURL(s string) string {
	for s != "" {
		last := s[len(s)-1]
		switch {
		case strings.IndexByte(".,;:!?'\"", last) >= 0:
			s = s[:len(s)-1]
		case last == ')' && strings.Count(s, "(") < strings.Count(s, ")"):
			s = s[:len(s)-1]
		case last == ']' && strings.Count(s, "[") < strings.Count(s, "]"):
			s = s[:len(s)-1]
		default:
			return s
		}
	}
	return s
}

// fileURI is the file:// link for a path that exists, or "".
func fileURI(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		p = filepath.Join(home, p[2:])
	}
	p = filepath.Clean(p)
	if !filepath.IsAbs(p) || p == "/" || !pathExists(p) {
		return ""
	}
	u := url.URL{Scheme: "file", Host: linkHost(), Path: p}
	return u.String()
}

var (
	hostOnce sync.Once
	hostName string
)

// linkHost names this computer in file:// links, so a terminal on another
// computer (over SSH) can tell the file is not its own.
func linkHost() string {
	hostOnce.Do(func() {
		if h, err := os.Hostname(); err == nil && !strings.ContainsAny(h, "/\\@:") {
			hostName = h
		}
	})
	return hostName
}

// pathExists is os.Stat with a short-lived cache: output is drawn many
// times a second, and its paths rarely change that fast.
var pathCache = struct {
	sync.Mutex
	m map[string]pathSeen
}{m: map[string]pathSeen{}}

type pathSeen struct {
	ok   bool
	when time.Time
}

func pathExists(p string) bool {
	now := time.Now()
	pathCache.Lock()
	defer pathCache.Unlock()
	if s, ok := pathCache.m[p]; ok && now.Sub(s.when) < 5*time.Second {
		return s.ok
	}
	if len(pathCache.m) > 4096 {
		pathCache.m = map[string]pathSeen{}
	}
	_, err := os.Stat(p)
	pathCache.m[p] = pathSeen{ok: err == nil, when: now}
	return err == nil
}

// withLinks wraps the links of a line in OSC 8 sequences. styled is what
// is drawn: the line's visible text, perhaps cut short or panned, with
// colour sequences in it. srcOf maps the n-th visible rune to its rune in
// the line the spans were found in, or -1 for a marker ("‹", "…") that is
// not part of the line.
func withLinks(styled string, spans []linkSpan, srcOf func(int) int) string {
	if len(spans) == 0 || !linksOn() {
		return styled
	}
	var b strings.Builder
	open := -1 // index of the span that is open
	v := 0
	for i := 0; i < len(styled); {
		if styled[i] == 0x1b {
			n := escLen(styled[i:])
			b.WriteString(styled[i : i+n])
			i += n
			continue
		}
		_, size := utf8.DecodeRuneInString(styled[i:])
		want := -1
		if src := srcOf(v); src >= 0 {
			for k, s := range spans {
				if src >= s.start && src < s.end {
					want = k
					break
				}
			}
		}
		if want != open {
			if open >= 0 {
				b.WriteString(osc8Close)
			}
			if want >= 0 {
				b.WriteString(osc8Open(spans[want].uri))
			}
			open = want
		}
		b.WriteString(styled[i : i+size])
		i += size
		v++
	}
	if open >= 0 {
		b.WriteString(osc8Close)
	}
	return b.String()
}

// escLen is the length of the escape sequence at the start of s.
func escLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[':
		for j := 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j + 1
			}
		}
		return len(s)
	case ']':
		for j := 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
		}
		return len(s)
	}
	return 2
}

// truncSrc is srcOf for a line cut with Truncate to shown: every rune is
// the line's own, except a final "…" that Truncate added.
func truncSrc(full, shown string) func(int) int {
	n := utf8.RuneCountInString(shown)
	cut := shown != full
	return func(v int) int {
		if cut && v == n-1 {
			return -1
		}
		return v
	}
}

// sliceSrc is srcOf for a window made by util.SliceColsSpan: a "‹" first
// when from > 0, then the line's runes [from, to), then perhaps "…".
func sliceSrc(from, to int, left bool) func(int) int {
	return func(v int) int {
		if left {
			if v == 0 {
				return -1
			}
			v--
		}
		if from+v >= to {
			return -1
		}
		return from + v
	}
}

// linkWidget links the URLs and paths in a row's value, as drawn. A link
// cut short by "…" is left alone: it would go somewhere else.
func linkWidget(wt string) string {
	if wt == "" || !linksOn() {
		return wt
	}
	plain := []rune(sgrSeq.ReplaceAllString(wt, ""))
	spans := linkSpans(string(plain))
	kept := spans[:0]
	for _, s := range spans {
		if s.end < len(plain) && plain[s.end] == '…' {
			continue
		}
		kept = append(kept, s)
	}
	return withLinks(wt, kept, func(v int) int { return v })
}
