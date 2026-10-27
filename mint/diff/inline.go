package diff

// Span is a run of changed characters in a line: runes [Start, End).
type Span struct{ Start, End int }

// MaxInlineRunes bounds the per-line comparison: it costs time and memory
// proportional to the product of the two lengths, so longer lines are
// marked as changed as a whole.
const MaxInlineRunes = 500

// InlineChanges finds the characters that differ between two versions of
// a line, as runs of runes in each. Equal lines give no spans. Lines longer
// than MaxInlineRunes are reported as changed from end to end.
//
// It trims the common prefix and suffix first, then takes a longest
// common subsequence of what is left, so a one-word edit in a long line
// is cheap.
func InlineChanges(a, b string) (left, right []Span) {
	ra, rb := []rune(a), []rune(b)
	if string(ra) == string(rb) {
		return nil, nil
	}
	if len(ra) > MaxInlineRunes || len(rb) > MaxInlineRunes {
		return whole(len(ra)), whole(len(rb))
	}
	pre := 0
	for pre < len(ra) && pre < len(rb) && ra[pre] == rb[pre] {
		pre++
	}
	suf := 0
	for suf < len(ra)-pre && suf < len(rb)-pre && ra[len(ra)-1-suf] == rb[len(rb)-1-suf] {
		suf++
	}
	ma, mb := ra[pre:len(ra)-suf], rb[pre:len(rb)-suf]
	keepA, keepB := lcs(ma, mb)
	return spans(keepA, pre), spans(keepB, pre)
}

func whole(n int) []Span {
	if n == 0 {
		return nil
	}
	return []Span{{0, n}}
}

// lcs marks which runes of a and b belong to a longest common subsequence.
func lcs(a, b []rune) (keepA, keepB []bool) {
	keepA, keepB = make([]bool, len(a)), make([]bool, len(b))
	if len(a) == 0 || len(b) == 0 {
		return
	}
	// table[i][j] = LCS length of a[i:] and b[j:]
	w := len(b) + 1
	table := make([]int32, (len(a)+1)*w)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i*w+j] = table[(i+1)*w+j+1] + 1
			} else if x, y := table[(i+1)*w+j], table[i*w+j+1]; x >= y {
				table[i*w+j] = x
			} else {
				table[i*w+j] = y
			}
		}
	}
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			keepA[i], keepB[j] = true, true
			i++
			j++
		case table[(i+1)*w+j] >= table[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return
}

// spans turns "not kept" runs into Spans, shifted by off.
func spans(keep []bool, off int) []Span {
	var out []Span
	for i := 0; i < len(keep); {
		if keep[i] {
			i++
			continue
		}
		start := i
		for i < len(keep) && !keep[i] {
			i++
		}
		out = append(out, Span{off + start, off + i})
	}
	return out
}
