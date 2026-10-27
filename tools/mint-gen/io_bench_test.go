package main

// The I/O benchmark: does mint-gen work out where a program's output goes?
// It runs only when asked:
//
//	MINT_IO_BENCH=1 go test -run TestIOBenchmark -v .
//
// For each program in testdata/io_benchmark.txt it reads the help (from
// the same cache as the template benchmark) and builds the I/O model twice:
// blind, with the known-tools list switched off, and as shipped. Verdicts:
//
//	correct  "Save output to file" works as labelled
//	missed   the program writes its own output file, but Mint saves what it
//	         prints instead (or the other way between capture and log): it
//	         still works, it is just not the best way
//	WRONG    the app would pass its output file to an option or argument
//	         that is not the program's output: the one that matters
//
// What the program prints (results, log, binary) is scored separately.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type ioLabel struct {
	Name         string
	Save, Stdout []string // alternatives
	Skip         bool
}

func loadIOLabels(t *testing.T) []ioLabel {
	f, err := os.Open("testdata/io_benchmark.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []ioLabel
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 {
			t.Fatalf("bad line %q", line)
		}
		out = append(out, ioLabel{Name: f[0], Save: strings.Split(f[1], "/"), Stdout: strings.Split(f[2], "/"), Skip: f[1] == "skip"})
	}
	return out
}

// ioAnswer is the model's save decision as a label word: a mode, or the
// option it writes through.
func ioAnswer(m IOModel) string {
	switch m.Save {
	case SaveByOption:
		if m.Main != nil {
			return m.Main.Flag
		}
		return "option?"
	case SaveByPositional:
		return "positional"
	case SaveByCapture:
		return "capture"
	case SaveLog:
		return "log"
	}
	return "none"
}

func saveMatches(answer, alt string) bool {
	if answer == alt {
		return true
	}
	if strings.HasPrefix(alt, "-") {
		for _, f := range strings.Split(alt, "|") {
			if f == answer {
				return true
			}
		}
	}
	return false
}

func ioVerdict(l ioLabel, answer string) string {
	for _, alt := range l.Save {
		if saveMatches(answer, alt) {
			return "correct"
		}
	}
	if answer == "capture" || answer == "log" || answer == "none" {
		return "missed"
	}
	return "wrong"
}

func stdoutVerdict(l ioLabel, m IOModel) bool {
	got := m.Stdout
	if got == "" {
		got = StdoutResults // unknown is treated as results
	}
	for _, s := range l.Stdout {
		if s == got {
			return true
		}
	}
	return false
}

func TestIOBenchmark(t *testing.T) {
	if os.Getenv("MINT_IO_BENCH") == "" {
		t.Skip("set MINT_IO_BENCH=1 to run the input/output benchmark")
	}
	cache := os.Getenv("MINT_TEMPLATE_BENCH_CACHE")
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "mint-template-bench")
	}
	defer func() { knownToolsOff = false }()
	type row struct {
		l                    ioLabel
		blind, shipped       string
		blindV, shippedV     string
		blindOut, shippedOut bool
		why                  string
	}
	var rows []row
	var missing []string
	for _, l := range loadIOLabels(t) {
		if l.Skip {
			continue
		}
		man, help, ok := benchHelp(t, cache, l.Name)
		if !ok {
			missing = append(missing, l.Name)
			continue
		}
		knownToolsOff = true
		_, mb := ParseManAndHelpIO(l.Name, man, help)
		knownToolsOff = false
		_, ms := ParseManAndHelpIO(l.Name, man, help)
		r := row{l: l, blind: ioAnswer(mb), shipped: ioAnswer(ms), blindOut: stdoutVerdict(l, mb), shippedOut: stdoutVerdict(l, ms)}
		r.blindV, r.shippedV = ioVerdict(l, r.blind), ioVerdict(l, r.shipped)
		r.why = mb.SaveWhy + " | prints: " + mb.Stdout + " (" + mb.StdoutWhy + ")"
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].l.Name < rows[j].l.Name })
	all := os.Getenv("MINT_IO_BENCH") == "all"
	for _, r := range rows {
		if all || r.blindV != "correct" || r.shippedV != "correct" || !r.blindOut {
			t.Logf("%-13s want %-28s blind %-14s %-8s shipped %-14s %-8s stdout ok %-5v [%s]",
				r.l.Name, strings.Join(r.l.Save, "/"), r.blind, r.blindV, r.shipped, r.shippedV, r.blindOut, r.why)
		}
	}
	report := func(title string, verdict func(row) string, out func(row) bool) {
		n := map[string]int{}
		ok := 0
		for _, r := range rows {
			n[verdict(r)]++
			if out(r) {
				ok++
			}
		}
		pct := func(k int) string { return fmt.Sprintf("%3d (%3.0f%%)", k, 100*float64(k)/float64(len(rows))) }
		t.Logf("%-8s %d programs | save: correct %s missed %s WRONG %s | prints: right %s",
			title, len(rows), pct(n["correct"]), pct(n["missed"]), pct(n["wrong"]), pct(ok))
	}
	report("blind", func(r row) string { return r.blindV }, func(r row) bool { return r.blindOut })
	report("shipped", func(r row) string { return r.shippedV }, func(r row) bool { return r.shippedOut })
	if len(missing) > 0 {
		t.Logf("not installed (skipped): %s", strings.Join(missing, " "))
	}
}

// The verdict rules themselves (always run).
func TestIOVerdicts(t *testing.T) {
	l := ioLabel{Save: []string{"capture", "-o|--output"}}
	for answer, want := range map[string]string{"capture": "correct", "--output": "correct", "log": "missed", "-f": "wrong", "positional": "wrong"} {
		if got := ioVerdict(l, answer); got != want {
			t.Errorf("%s: %s, want %s", answer, got, want)
		}
	}
}
