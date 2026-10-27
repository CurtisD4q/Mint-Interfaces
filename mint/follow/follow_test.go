package follow

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

func texts(b *Buffer) []string {
	var out []string
	for i := 0; i < b.Len(); i++ {
		out = append(out, b.At(i).Text)
	}
	return out
}

func TestLevelsFromText(t *testing.T) {
	var b Buffer
	for _, s := range []string{"ok started", "WARN disk 90%", "ERROR: boom", "debug: x=1", "Failed to start foo.service"} {
		b.Add(mint.LogLine{Text: s})
	}
	b.Add(mint.LogLine{Text: "plain on stderr", Err: true})
	want := []Level{Info, Warning, Error, Debug, Error, Warning}
	for i, w := range want {
		if got := b.At(i).Level; got != w {
			t.Errorf("line %q: level %v, want %v", b.At(i).Text, got, w)
		}
	}
	b.SetMinLevel(Warning)
	if got := strings.Join(texts(&b), "|"); got != "WARN disk 90%|ERROR: boom|Failed to start foo.service|plain on stderr" {
		t.Fatalf("min warning: %q", got)
	}
}

// Real journalctl -o json lines (fields trimmed).
func TestJournalJSON(t *testing.T) {
	b := Buffer{Format: JournalJSON}
	for _, l := range []string{
		`{"__REALTIME_TIMESTAMP":"1761566400000000","PRIORITY":"6","_SYSTEMD_UNIT":"ssh.service","MESSAGE":"Accepted publickey for curtis"}`,
		`{"__REALTIME_TIMESTAMP":"1761566401000000","PRIORITY":"3","SYSLOG_IDENTIFIER":"kernel","MESSAGE":"I/O error, dev sda"}`,
		`{"PRIORITY":"4","MESSAGE":[104,105,255]}`,
		`{"PRIORITY":"7","MESSAGE":"dbg"}`,
		`-- No entries --`,
		`{"MESSAGE":"cut off at 16 Ki` + strings.Repeat("x", 10) + ` … [line too long`,
	} {
		b.Add(mint.LogLine{Text: l})
	}
	if e := b.At(0); e.Text != "Accepted publickey for curtis" || e.Unit != "ssh.service" || e.Level != Info || e.Time.Unix() != 1761566400 {
		t.Errorf("entry 0 %+v", e)
	}
	if e := b.At(1); e.Unit != "kernel" || e.Level != Error {
		t.Errorf("entry 1 %+v", e)
	}
	if e := b.At(2); e.Text != "hi\xff" || e.Level != Warning {
		t.Errorf("byte-array message %+v", e)
	}
	if e := b.At(3); e.Level != Debug {
		t.Errorf("debug %+v", e)
	}
	if e := b.At(4); e.Text != "-- No entries --" {
		t.Errorf("not json %+v", e)
	}
	if e := b.At(5); !strings.HasPrefix(e.Text, `{"MESSAGE":"cut off`) {
		t.Errorf("cut-off line %+v", e)
	}
	// The filter looks at the unit too.
	b.SetFilter("SSH.SERVICE", false)
	if got := texts(&b); len(got) != 1 || got[0] != "Accepted publickey for curtis" {
		t.Fatalf("unit filter %q", got)
	}
}

func TestFilter(t *testing.T) {
	var b Buffer
	for i := 1; i <= 20; i++ {
		b.Add(mint.LogLine{Text: fmt.Sprintf("GET /item/%d 200", i)})
	}
	b.AddMark("── restarted ──")
	b.Add(mint.LogLine{Text: "GET /item/7 500"})
	if err := b.SetFilter("/item/7 ", false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(texts(&b), "|"); got != "GET /item/7 200|── restarted ──|GET /item/7 500" {
		t.Fatalf("substring: %q", got)
	}
	if err := b.SetFilter(` 5\d\d$`, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(texts(&b), "|"); got != "── restarted ──|GET /item/7 500" {
		t.Fatalf("regex: %q", got)
	}
	// New lines are filtered as they arrive.
	b.Add(mint.LogLine{Text: "GET /x 503"})
	b.Add(mint.LogLine{Text: "GET /y 200"})
	if b.Len() != 3 || b.At(2).Text != "GET /x 503" {
		t.Fatalf("incremental: %q", texts(&b))
	}
	if err := b.SetFilter("(", true); err == nil {
		t.Fatal("bad regex accepted")
	}
	b.SetFilter("", false)
	if b.Len() != 24 {
		t.Fatalf("cleared: %d", b.Len())
	}
}

func TestDropsOldest(t *testing.T) {
	b := Buffer{Max: 100}
	for i := 1; i <= 1000; i++ {
		b.Add(mint.LogLine{Text: fmt.Sprintf("line %d", i)})
		if i == 500 {
			b.SetFilter("7", false)
		}
	}
	if b.Kept() > 100 || b.Dropped()+b.Kept() != 1000 {
		t.Fatalf("kept %d dropped %d", b.Kept(), b.Dropped())
	}
	// Every shown line contains 7 and is still held; numbering is absolute.
	for i := 0; i < b.Len(); i++ {
		e := b.At(i)
		if !strings.Contains(e.Text, "7") || e.Text != fmt.Sprintf("line %d", e.No) {
			t.Fatalf("shown %d: %+v", i, e)
		}
	}
	if last := b.At(b.Len() - 1); last.No != 997 {
		t.Fatalf("last shown %+v", last)
	}
}

// Filtering stays fast: a pattern that is catastrophic for a backtracking
// engine costs about the same as a simple one here. Compared with a
// baseline, not a clock, so slow machines and -race don't matter.
func TestRegexIsLinear(t *testing.T) {
	var b Buffer
	for i := 0; i < 50; i++ {
		b.Add(mint.LogLine{Text: strings.Repeat("a", 16000) + "!"})
	}
	timeOf := func(pattern string) time.Duration {
		start := time.Now()
		if err := b.SetFilter(pattern, true); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	base := timeOf(`a+$`)
	evil := timeOf(`(a+)+$`)
	if b.Len() != 0 {
		t.Fatal("matched a line ending in !")
	}
	if evil > 20*base+50*time.Millisecond {
		t.Fatalf("(a+)+$ took %v, a+$ took %v", evil, base)
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]Level{"WARN": Warning, "crit": Error, "trace": Debug, "notice": Notice} {
		if got, ok := ParseLevel(in); !ok || got != want {
			t.Errorf("%q → %v %v", in, got, ok)
		}
	}
}

func BenchmarkAdd(b *testing.B) {
	buf := Buffer{}
	buf.SetFilter("error", false)
	line := mint.LogLine{Text: "2026-10-27T12:00:00 app[123]: request handled in 3ms"}
	for i := 0; i < b.N; i++ {
		buf.Add(line)
	}
}
