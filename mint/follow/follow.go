// Package follow keeps the recent lines of a log as they arrive, with a
// level for each and a filter, for a view that shows the newest lines
// like tail -f. It is fed lines; it never reads files or runs programs.
package follow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// Level is how important a line is. Higher is worse.
type Level int

const (
	Debug Level = iota
	Info
	Notice
	Warning
	Error
)

func (l Level) String() string {
	return [...]string{"debug", "info", "notice", "warning", "error"}[l]
}

// Levels lists every level, least important first.
func Levels() []Level { return []Level{Debug, Info, Notice, Warning, Error} }

// ParseLevel reads a level name, with common synonyms.
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug", "trace", "verbose":
		return Debug, true
	case "info", "information":
		return Info, true
	case "notice":
		return Notice, true
	case "warning", "warn":
		return Warning, true
	case "error", "err", "fatal", "critical", "crit", "alert", "emerg", "emergency":
		return Error, true
	}
	return Info, false
}

// Format is how lines are read.
type Format int

const (
	Text        Format = iota // plain lines; levels from patterns
	JournalJSON               // journalctl -o json: one JSON object per line
)

// Entry is one line as kept.
type Entry struct {
	No    int // from 1, counting every line ever added
	Text  string
	Err   bool // came from standard error
	Level Level
	Unit  string    // journal: the unit or program
	Time  time.Time // journal: when it was logged; zero if unknown
	Mark  bool      // a marker Mint added (a restart), not program output
}

// LevelPattern gives lines matching Re a level (Format Text).
type LevelPattern struct {
	Level Level
	Re    *regexp.Regexp
	// Hints, if set, are lower-case words of which a matching line must
	// contain at least one; lines with none skip Re, which is much faster.
	Hints []string
}

// DefaultPatterns recognise common level words in text logs.
var DefaultPatterns = []LevelPattern{
	{Error, regexp.MustCompile(`(?i)\b(error|err|fatal|crit(ical)?|panic|emerg|alert|failed|failure)\b`),
		[]string{"err", "fatal", "crit", "panic", "emerg", "alert", "fail"}},
	{Warning, regexp.MustCompile(`(?i)\b(warn(ing)?)\b`), []string{"warn"}},
	{Debug, regexp.MustCompile(`(?i)\b(debug|trace)\b`), []string{"debug", "trace"}},
}

// Buffer holds the newest Max lines and which of them pass the filter.
// It is not safe for concurrent use.
type Buffer struct {
	Max      int // lines kept (default 50,000)
	Format   Format
	Patterns []LevelPattern // for Text; nil means DefaultPatterns

	entries []Entry // oldest first; entries[0].No == first
	added   int     // lines ever added
	dropped int     // lines dropped from the front

	filter   string
	filterRe *regexp.Regexp
	min      Level
	shown    []int // indexes into entries of lines that pass, oldest first
}

// Add takes in a line from the program.
func (b *Buffer) Add(l mint.LogLine) {
	e := Entry{Text: l.Text, Err: l.Err}
	if b.Format == JournalJSON {
		b.decodeJournal(&e)
	} else {
		e.Level = b.levelOf(l.Text)
	}
	if l.Err && e.Level < Warning {
		e.Level = Warning
	}
	b.add(e)
}

// AddMark adds a line of Mint's own, such as "restarted".
func (b *Buffer) AddMark(text string) {
	b.add(Entry{Text: text, Mark: true, Level: Notice})
}

func (b *Buffer) add(e Entry) {
	if b.Max <= 0 {
		b.Max = 50000
	}
	b.added++
	e.No = b.added
	b.entries = append(b.entries, e)
	if b.passes(e) {
		b.shown = append(b.shown, len(b.entries)-1)
	}
	if len(b.entries) > b.Max {
		// Drop a batch, so this runs rarely.
		drop := len(b.entries) - b.Max + b.Max/10
		drop = min(drop, len(b.entries))
		b.entries = append([]Entry(nil), b.entries[drop:]...)
		b.dropped += drop
		k := 0
		for _, i := range b.shown {
			if i >= drop {
				b.shown[k] = i - drop
				k++
			}
		}
		b.shown = b.shown[:k]
	}
}

func (b *Buffer) levelOf(text string) Level {
	ps := b.Patterns
	if ps == nil {
		ps = DefaultPatterns
	}
	lower := ""
	for _, p := range ps {
		if len(p.Hints) > 0 {
			if lower == "" {
				lower = strings.ToLower(text)
			}
			found := false
			for _, h := range p.Hints {
				if strings.Contains(lower, h) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if p.Re.MatchString(text) {
			return p.Level
		}
	}
	return Info
}

// decodeJournal reads one journalctl -o json object. A line that isn't
// one (a cut-off line, a message from journalctl itself) is kept as text.
func (b *Buffer) decodeJournal(e *Entry) {
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(e.Text), &obj) != nil {
		e.Level = b.levelOf(e.Text)
		return
	}
	str := func(key string) string {
		raw, ok := obj[key]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		// journald writes non-UTF-8 or binary fields as byte arrays.
		var bs []byte
		var nums []int
		if json.Unmarshal(raw, &nums) == nil {
			for _, n := range nums {
				bs = append(bs, byte(n))
			}
			return string(bs)
		}
		return ""
	}
	e.Text = str("MESSAGE")
	e.Unit = str("_SYSTEMD_UNIT")
	if e.Unit == "" {
		e.Unit = str("SYSLOG_IDENTIFIER")
	}
	if us, err := strconv.ParseInt(str("__REALTIME_TIMESTAMP"), 10, 64); err == nil {
		e.Time = time.UnixMicro(us)
	}
	e.Level = Info
	if p, err := strconv.Atoi(str("PRIORITY")); err == nil {
		switch {
		case p <= 3:
			e.Level = Error
		case p == 4:
			e.Level = Warning
		case p == 5:
			e.Level = Notice
		case p == 7:
			e.Level = Debug
		}
	}
}

// SetFilter shows only lines containing text (case-insensitive), or,
// when regex is true, matching it as a regular expression. Go's regular
// expressions take time linear in the line, so no pattern can hang the
// view. "" shows everything.
func (b *Buffer) SetFilter(text string, regex bool) error {
	var re *regexp.Regexp
	if text != "" && regex {
		var err error
		if re, err = regexp.Compile("(?i)" + text); err != nil {
			return fmt.Errorf("not a valid pattern: %w", err)
		}
	}
	b.filter, b.filterRe = strings.ToLower(text), re
	b.refilter()
	return nil
}

// SetMinLevel hides lines below min.
func (b *Buffer) SetMinLevel(min Level) {
	b.min = min
	b.refilter()
}

// Filter returns the current filter text and minimum level.
func (b *Buffer) Filter() (string, Level) { return b.filter, b.min }

func (b *Buffer) refilter() {
	b.shown = b.shown[:0]
	for i, e := range b.entries {
		if b.passes(e) {
			b.shown = append(b.shown, i)
		}
	}
}

func (b *Buffer) passes(e Entry) bool {
	if e.Mark {
		return true
	}
	if e.Level < b.min {
		return false
	}
	switch {
	case b.filterRe != nil:
		return b.filterRe.MatchString(e.Text) || b.filterRe.MatchString(e.Unit)
	case b.filter != "":
		return strings.Contains(strings.ToLower(e.Text), b.filter) || strings.Contains(strings.ToLower(e.Unit), b.filter)
	}
	return true
}

// Len is how many lines pass the filter.
func (b *Buffer) Len() int { return len(b.shown) }

// At returns the i-th line that passes the filter, oldest first.
func (b *Buffer) At(i int) Entry { return b.entries[b.shown[i]] }

// Kept is how many lines are held, and Dropped how many older ones were
// let go to stay within Max.
func (b *Buffer) Kept() int    { return len(b.entries) }
func (b *Buffer) Dropped() int { return b.dropped }
