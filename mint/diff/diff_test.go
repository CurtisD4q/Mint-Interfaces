package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// clean splits text into lines and cleans each, as the runner would.
func clean(text string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		out = append(out, mint.CleanLine(l))
	}
	return out
}

// rows renders a hunk compactly: kind:leftNo:rightNo:text.
func rows(h Hunk) []string {
	var out []string
	for _, r := range h.Rows {
		s := r.Kind.String()
		ln, rn, lt, rt := "", "", "", ""
		if r.Left != nil {
			ln, lt = itoa(r.Left.No), r.Left.Text
			if r.Left.NoNewline {
				lt += "⊘"
			}
		}
		if r.Right != nil {
			rn, rt = itoa(r.Right.No), r.Right.Text
			if r.Right.NoNewline {
				rt += "⊘"
			}
		}
		if r.Hidden {
			s += "!"
		}
		out = append(out, s+" "+ln+":"+lt+" | "+rn+":"+rt)
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestParseUnifiedBasic(t *testing.T) {
	// Real `diff -u l r` output, including a CRLF-only change and a
	// missing final newline.
	out := "--- l\t2026-10-27 12:31:18.103964552 +0000\n" +
		"+++ r\t2026-10-27 12:31:18.103964552 +0000\n" +
		"@@ -1,4 +1,5 @@\n" +
		" a\n" +
		"-b\tx\n" +
		"-c\n" +
		"+B\tx\n" +
		"+c\r\n" +
		" d\n" +
		"+e\n" +
		"\\ No newline at end of file\n"
	files := ParseUnified(clean(out))
	if len(files) != 1 {
		t.Fatalf("files %+v", files)
	}
	f := files[0]
	if f.OldName != "l" || f.NewName != "r" || f.Status != Modified || len(f.Hunks) != 1 {
		t.Fatalf("file %+v", f)
	}
	h := f.Hunks[0]
	if h.OldStart != 1 || h.OldLines != 4 || h.NewStart != 1 || h.NewLines != 5 {
		t.Fatalf("hunk header %+v", h)
	}
	want := []string{
		"same 1:a | 1:a",
		// The runner expanded the tab with diff's marker column included,
		// so stops fall one column early; every diff line has the marker,
		// so they still line up with each other.
		"change 2:b      x | 2:B      x",
		"change! 3:c | 3:c", // differs only in the CR, which cleaning removed
		"same 4:d | 4:d",
		"insert : | 5:e⊘",
	}
	if got := rows(h); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows\n got %q\nwant %q", got, want)
	}
	if h.Added() != 3 || h.Removed() != 2 || f.Added() != 3 || f.Removed() != 2 {
		t.Fatalf("counts +%d -%d", h.Added(), h.Removed())
	}
}

func TestParseUnifiedRecursive(t *testing.T) {
	out := "Binary files A/bin and B/bin differ\n" +
		"diff -ru A/f B/f\n" +
		"--- A/f\t2026-10-27 12:31:18.105407499 +0000\n" +
		"+++ B/f\t2026-10-27 12:31:18.105407499 +0000\n" +
		"@@ -1 +1 @@\n" +
		"-1\n" +
		"+2\n" +
		"Only in A: only\n" +
		"Files A/g and B/g differ\n"
	files := ParseUnified(clean(out))
	if len(files) != 4 {
		t.Fatalf("got %d files: %+v", len(files), files)
	}
	if files[0].Status != Binary || files[0].OldName != "A/bin" || files[0].NewName != "B/bin" {
		t.Errorf("binary %+v", files[0])
	}
	if files[1].Status != Modified || files[1].OldName != "A/f" || files[1].NewName != "B/f" {
		t.Errorf("modified %+v", files[1])
	}
	if got := rows(files[1].Hunks[0]); !reflect.DeepEqual(got, []string{"change 1:1 | 1:2"}) {
		t.Errorf("rows %q", got)
	}
	if files[2].Status != OnlyIn || files[2].OldName != "A" || files[2].NewName != "only" {
		t.Errorf("only %+v", files[2])
	}
	if files[3].Status != Differ || files[3].OldName != "A/g" {
		t.Errorf("differ %+v", files[3])
	}
}

func TestParseUnifiedEdges(t *testing.T) {
	// A new, empty-before file: "-0,0".
	files := ParseUnified([]string{"--- /dev/null", "+++ b", "@@ -0,0 +1,2 @@", "+x", "+y"})
	if got := rows(files[0].Hunks[0]); !reflect.DeepEqual(got, []string{"insert : | 1:x", "insert : | 2:y"}) {
		t.Errorf("new file rows %q", got)
	}
	// A deleted file.
	files = ParseUnified([]string{"--- a", "+++ /dev/null", "@@ -1 +0,0 @@", "-x"})
	if got := rows(files[0].Hunks[0]); !reflect.DeepEqual(got, []string{"delete 1:x | :"}) {
		t.Errorf("deleted file rows %q", got)
	}
	// Both files end without a newline on the same context line.
	files = ParseUnified([]string{"--- a", "+++ b", "@@ -1,2 +1,2 @@", "-x", "+y", " z", `\ No newline at end of file`})
	if got := rows(files[0].Hunks[0]); !reflect.DeepEqual(got, []string{"change 1:x | 1:y", "same 2:z⊘ | 2:z⊘"}) {
		t.Errorf("shared no-newline rows %q", got)
	}
	// Only the old file lacks a final newline.
	files = ParseUnified([]string{"--- a", "+++ b", "@@ -1 +1 @@", "-x", `\ No newline at end of file`, "+x"})
	if got := rows(files[0].Hunks[0]); !reflect.DeepEqual(got, []string{"change! 1:x⊘ | 1:x"}) {
		t.Errorf("old no-newline rows %q", got)
	}
	// A context line whose leading space was trimmed.
	files = ParseUnified([]string{"--- a", "+++ b", "@@ -1,3 +1,3 @@", "", "-x", "+y", " z"})
	if got := rows(files[0].Hunks[0]); !reflect.DeepEqual(got, []string{"same 1: | 1:", "change 2:x | 2:y", "same 3:z | 3:z"}) {
		t.Errorf("trimmed context rows %q", got)
	}
	// Two change blocks: "-" after "+" starts a new one, and unequal runs
	// leave unpaired lines.
	files = ParseUnified([]string{"--- a", "+++ b", "@@ -1,3 +1,2 @@", "-a", "-b", "+A", "-c", "+C"})
	h := files[0].Hunks[0]
	if got := rows(h); !reflect.DeepEqual(got, []string{"change 1:a | 1:A", "delete 2:b | :", "change 3:c | 2:C"}) {
		t.Errorf("blocks rows %q", got)
	}
	// A hunk cut short keeps what was read; nothing panics on junk.
	files = ParseUnified([]string{"--- a", "+++ b", "@@ -1,5 +1,5 @@", " x", "-y"})
	if got := rows(files[0].Hunks[0]); !reflect.DeepEqual(got, []string{"same 1:x | 1:x", "delete 2:y | :"}) {
		t.Errorf("truncated rows %q", got)
	}
	for _, junk := range [][]string{nil, {"@@"}, {"@@ -x +y @@"}, {"+++ b"}, {"\\ No newline at end of file"}, {"@@ -1 +1 @@", "\\ No newline at end of file"}} {
		_ = ParseUnified(junk)
	}
}

// Content can't break out of a hunk: a removed line that reads like a
// file header stays a removed line, because the hunk still expects it.
func TestParseUnifiedHeaderLookalikes(t *testing.T) {
	files := ParseUnified([]string{"--- a", "+++ b", "@@ -1,2 +1,2 @@", "--- not a file", "-@@ -9 +9 @@", "+++ also not", "+Only in X: y"})
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("files %+v", files)
	}
	want := []string{"change 1:-- not a file | 1:++ also not", "change 2:@@ -9 +9 @@ | 2:Only in X: y"}
	if got := rows(files[0].Hunks[0]); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows\n got %q\nwant %q", got, want)
	}
}

// Against the real diff, when it is installed.
func TestParseUnifiedRealDiff(t *testing.T) {
	bin, err := exec.LookPath("diff")
	if err != nil {
		t.Skip("diff not installed")
	}
	dir := t.TempDir()
	left, right := filepath.Join(dir, "left"), filepath.Join(dir, "right")
	var a, b []string
	for i := 1; i <= 40; i++ {
		a = append(a, "line "+strings.Repeat("x", i%7))
		if i%9 == 0 {
			b = append(b, "changed "+strings.Repeat("y", i%5))
		} else if i%13 != 0 {
			b = append(b, a[len(a)-1])
		}
	}
	os.WriteFile(left, []byte(strings.Join(a, "\n")+"\n"), 0o644)
	os.WriteFile(right, []byte(strings.Join(b, "\n")+"\n"), 0o644)
	out, _ := exec.Command(bin, "-u", left, right).Output()
	files := ParseUnified(clean(string(out)))
	if len(files) != 1 {
		t.Fatalf("files %+v", files)
	}
	// Rebuild both files from the rows and the unchanged lines between hunks.
	gotA, gotB := map[int]string{}, map[int]string{}
	for _, h := range files[0].Hunks {
		for _, r := range h.Rows {
			if r.Left != nil {
				gotA[r.Left.No] = r.Left.Text
			}
			if r.Right != nil {
				gotB[r.Right.No] = r.Right.Text
			}
		}
	}
	for no, text := range gotA {
		if a[no-1] != text {
			t.Errorf("left %d = %q, want %q", no, text, a[no-1])
		}
	}
	for no, text := range gotB {
		if b[no-1] != text {
			t.Errorf("right %d = %q, want %q", no, text, b[no-1])
		}
	}
	if files[0].Added() == 0 || files[0].Removed() == 0 {
		t.Fatalf("counts +%d -%d", files[0].Added(), files[0].Removed())
	}
}

func TestParseCmp(t *testing.T) {
	r := ParseCmp([]string{"cmp: EOF on l after byte 10", " 3 142 102", " 8  12  15", "10  12 144"})
	want := []ByteDiff{{3, 'b', 'B'}, {8, '\n', '\r'}, {10, '\n', 'd'}}
	if !reflect.DeepEqual(r.Diffs, want) || r.ShorterFile != "l" || r.ShorterAt != 10 {
		t.Fatalf("cmp -l %+v", r)
	}
	if s := r.Diffs[0].String(); s != "3: 62 b → 42 B" {
		t.Fatalf("String = %q", s)
	}
	if s := r.Diffs[1].String(); s != "8: 0a . → 0d ." {
		t.Fatalf("String = %q", s)
	}
	r = ParseCmp([]string{"l r differ: char 3, line 2"})
	if r.First != 3 || r.FirstLine != 2 {
		t.Fatalf("plain cmp %+v", r)
	}
	r = ParseCmp([]string{"left right differ: byte 7, line 1", "cmp: EOF on right after byte 9, line 4"})
	if r.First != 7 || r.ShorterFile != "right" || r.ShorterAt != 9 {
		t.Fatalf("newer cmp %+v", r)
	}
	r = ParseCmp([]string{"cmp: EOF on empty which is empty"})
	if r.ShorterFile != "empty" {
		t.Fatalf("empty %+v", r)
	}
}

func TestInlineChanges(t *testing.T) {
	cases := []struct {
		a, b        string
		left, right []Span
	}{
		{"same", "same", nil, nil},
		{"hello world", "hello there world", nil, []Span{{6, 12}}},
		{"colour", "color", []Span{{4, 5}}, nil},
		{"abc", "xyz", []Span{{0, 3}}, []Span{{0, 3}}},
		{"€uro", "€URO", []Span{{1, 4}}, []Span{{1, 4}}}, // runes, not bytes
		{"", "new", nil, []Span{{0, 3}}},
	}
	for _, c := range cases {
		l, r := InlineChanges(c.a, c.b)
		if !reflect.DeepEqual(l, c.left) || !reflect.DeepEqual(r, c.right) {
			t.Errorf("InlineChanges(%q, %q) = %v %v, want %v %v", c.a, c.b, l, r, c.left, c.right)
		}
	}
	long := strings.Repeat("a", MaxInlineRunes+1)
	if l, r := InlineChanges(long, long+"b"); !reflect.DeepEqual(l, []Span{{0, MaxInlineRunes + 1}}) || len(r) != 1 {
		t.Errorf("long lines: %v %v", l, r)
	}
}

// The unchanged characters InlineChanges leaves are the same on both sides.
func TestInlineChangesKeepsCommonText(t *testing.T) {
	pairs := [][2]string{
		{"func main() { fmt.Println(\"hi\") }", "func main() { log.Println(\"hello\") }"},
		{"the quick brown fox", "a quick red fox jumps"},
		{"aaaa", "abab"},
	}
	for _, p := range pairs {
		l, r := InlineChanges(p[0], p[1])
		if strip(p[0], l) != strip(p[1], r) {
			t.Errorf("%q / %q: kept %q vs %q", p[0], p[1], strip(p[0], l), strip(p[1], r))
		}
	}
}

func strip(s string, spans []Span) string {
	rs := []rune(s)
	drop := make([]bool, len(rs))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End; i++ {
			drop[i] = true
		}
	}
	var b strings.Builder
	for i, r := range rs {
		if !drop[i] {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Any input parses without panicking, and line numbers stay positive.
func FuzzParseUnified(f *testing.F) {
	f.Add("--- a\n+++ b\n@@ -1,2 +1,2 @@\n-x\n+y\n z\n\\ No newline at end of file\n")
	f.Add("@@ -0,0 +1 @@\n+x\nOnly in a: b\n")
	f.Add("Binary files a and b differ\n@@ -3 +3,0 @@\n-q\n")
	f.Fuzz(func(t *testing.T, s string) {
		for _, file := range ParseUnified(strings.Split(s, "\n")) {
			for _, h := range file.Hunks {
				for _, r := range h.Rows {
					if (r.Left != nil && r.Left.No < 1) || (r.Right != nil && r.Right.No < 1) {
						t.Fatalf("non-positive line number in %+v", r)
					}
					if r.Left == nil && r.Right == nil {
						t.Fatal("empty row")
					}
				}
			}
		}
		_ = ParseCmp(strings.Split(s, "\n"))
	})
}
