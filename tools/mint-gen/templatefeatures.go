package main

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// The learned template scorer reads a program's help as features: the
// words and word pairs in each part of it (name, summary, the opening of
// the DESCRIPTION, usage lines, all of it), the words in its option names
// and value placeholders, and its subcommand names. The weights for them
// are learned from labelled programs (see templatemodel_gen.go); no help
// text is stored, only the weights.

// Feature prefixes, one per part of the help.
const (
	featName     = "n:" // words in a hyphenated program name
	featSummary  = "s:"
	featDesc     = "d:"
	featUsage    = "u:"
	featBody     = "b:"
	featOption   = "o:" // a word in an option's name: --listen-port → port, listen
	featValue    = "v:" // a value placeholder: <host>, FILE
	featCommand  = "c:" // a subcommand name
	featPairMark = "_"  // joins a word pair: s:packet_capture
)

var (
	reFeatWord = regexp.MustCompile(`[a-z][a-z0-9+]*`)
	// --long-option names in the text, and <placeholders>/CAPS values after options.
	reFeatLongOpt = regexp.MustCompile(`(?:^|[\s,\[(])--([a-z][a-z0-9-]{1,40})`)
	reFeatValue   = regexp.MustCompile(`(?:^|\s)--?[A-Za-z][-A-Za-z0-9]*[ =](?:<([a-z][a-z0-9_-]{1,24})>|([A-Z][A-Z0-9_]{1,24})\b)`)
)

// featStop are words too common to say anything.
var featStop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the of to in on for and or but not no is are was were be been being it its this that
		these those with by as at from into than then there here which who whom what when where how all any each
		can may will would should could must do does did done has have had if else so such also only more most
		other some same very just use used using uses default defaults option options see one two set sets value
		values given specified specify specifies e g i ie eg etc see note can't don't you your we our us they them their`) {
		featStop[w] = true
	}
}

// featStem folds common endings, so "captures", "captured" and
// "capturing" are one feature.
func featStem(w string) string {
	switch {
	case len(w) > 5 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 6 && strings.HasSuffix(w, "ing"):
		return strings.TrimSuffix(w[:len(w)-3], "e")
	case len(w) > 5 && strings.HasSuffix(w, "ed"):
		return strings.TrimSuffix(w[:len(w)-2], "e")
	case len(w) > 5 && strings.HasSuffix(w, "es") && !strings.HasSuffix(w, "ses"):
		return strings.TrimSuffix(w[:len(w)-2], "e")
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us"):
		return strings.TrimSuffix(w[:len(w)-1], "e")
	}
	return strings.TrimSuffix(w, "e")
}

// featWords is text as stemmed words, without stop words or self.
func featWords(text string, self map[string]bool) []string {
	var out []string
	for _, w := range reFeatWord.FindAllString(strings.ToLower(text), -1) {
		if len(w) < 2 || featStop[w] || self[w] {
			continue
		}
		out = append(out, featStem(w))
	}
	return out
}

// addZone adds a part's words and word pairs. A short part (summary,
// usage) counts each feature once; a long one (the whole help) by how
// often it appears, dampened, so a long man page cannot win on length.
func addZone(f map[string]float64, prefix string, words []string, long bool) {
	counts := map[string]int{}
	for i, w := range words {
		counts[prefix+w]++
		if i+1 < len(words) {
			counts[prefix+w+featPairMark+words[i+1]]++
		}
	}
	for k, n := range counts {
		if long {
			f[k] += 1 + math.Log(float64(n))
		} else {
			f[k] = 1
		}
	}
}

// templateFeatures turns a program's text into weighted features. The
// long parts are scaled to unit length, so the length of a man page does
// not decide how much it counts.
func templateFeatures(p programText) map[string]float64 {
	name := strings.ToLower(p.Name)
	self := map[string]bool{name: true}
	for _, part := range regexp.MustCompile(`[-_.]+`).Split(name, -1) {
		self[part] = true
	}
	f := map[string]float64{}
	if strings.ContainsAny(name, "-_") {
		addZone(f, featName, featWords(strings.NewReplacer("-", " ", "_", " ").Replace(name), map[string]bool{}), false)
	}
	// In the summary the name can be a word ("Print sensors information").
	addZone(f, featSummary, featWords(p.Summary, map[string]bool{name: true}), false)
	addZone(f, featDesc, featWords(p.Description, self), false)
	addZone(f, featUsage, featWords(strings.Join(p.Usage, "\n"), self), false)

	body := map[string]float64{}
	addZone(body, featBody, featWords(p.Body, self), true)
	for _, m := range reFeatLongOpt.FindAllStringSubmatch(strings.ToLower(p.Body), -1) {
		for _, w := range strings.Split(m[1], "-") {
			if len(w) >= 2 && !featStop[w] {
				body[featOption+featStem(w)]++
			}
		}
	}
	for _, m := range reFeatValue.FindAllStringSubmatch(p.Body, -1) {
		v := strings.ToLower(m[1] + m[2])
		for _, w := range regexp.MustCompile(`[-_]+`).Split(v, -1) {
			if len(w) >= 2 {
				body[featValue+featStem(w)]++
			}
		}
	}
	for k, v := range body {
		if strings.HasPrefix(k, featOption) || strings.HasPrefix(k, featValue) {
			body[k] = 1 + math.Log(v)
		}
	}
	normalize(body)
	for k, v := range body {
		f[k] = v
	}

	manLines, helpLines := splitBodyLines(p.Body)
	for _, c := range extractSubcommands(manLines, helpLines) {
		if w := strings.ToLower(c.Name); w != "" {
			f[featCommand+featStem(w)] = 1
		}
	}
	return f
}

// splitBodyLines gives the body's lines to the subcommand finder, which
// reads man and help lines alike.
func splitBodyLines(body string) ([]string, []string) {
	return nil, cleanLines(body)
}

// normalize scales a feature set to unit length.
func normalize(f map[string]float64) {
	var sum float64
	for _, v := range f {
		sum += v * v
	}
	if sum == 0 {
		return
	}
	n := math.Sqrt(sum)
	for k := range f {
		f[k] /= n
	}
}

// sortedFeatureNames lists a feature set's names in a fixed order, so
// everything that adds them up does it the same way every run.
func sortedFeatureNames(f map[string]float64) []string {
	names := make([]string, 0, len(f))
	for k := range f {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// capFeatures keeps a program's short-part features and its strongest
// maxBody whole-help features (ties by name, so the cut is the same every
// run), in name order.
func capFeatures(f map[string]float64, maxBody int) ([]string, []float64) {
	var body []string
	for k := range f {
		if strings.HasPrefix(k, featBody) || strings.HasPrefix(k, featOption) || strings.HasPrefix(k, featValue) {
			body = append(body, k)
		}
	}
	if len(body) > maxBody {
		sort.Slice(body, func(i, j int) bool {
			if f[body[i]] != f[body[j]] {
				return f[body[i]] > f[body[j]]
			}
			return body[i] < body[j]
		})
		for _, k := range body[maxBody:] {
			delete(f, k)
		}
	}
	names := sortedFeatureNames(f)
	vals := make([]float64, len(names))
	for i, n := range names {
		vals[i] = f[n]
	}
	return names, vals
}

// softmaxInPlace turns scores into probabilities.
func softmaxInPlace(p []float64) {
	max := p[0]
	for _, v := range p {
		if v > max {
			max = v
		}
	}
	var sum float64
	for k := range p {
		p[k] = math.Exp(p[k] - max)
		sum += p[k]
	}
	for k := range p {
		p[k] /= sum
	}
}
