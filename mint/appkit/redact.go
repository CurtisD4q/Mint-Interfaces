package appkit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"math/bits"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// ── Secrets typed into free-text boxes ──────────────────────────────────────

// FreeTextSecret is a value in the Extra arguments or Target box that looks
// like a password, token or key.
type FreeTextSecret struct {
	Value string // the secret itself
	Kind  string // what it is, for messages: "password in a URL", ...
}

var (
	// scheme://user:password@host
	urlPassword = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s/@:]*:([^\s/@]+)@`)
	// Authorization: Bearer xyz, X-Api-Key: xyz, ...
	authHeader = regexp.MustCompile(`(?i)\b(?:proxy-)?authorization:\s*(?:bearer|basic|token|digest)?\s*(\S+)`)
	keyHeader  = regexp.MustCompile(`(?i)\b(?:x-api-key|api-key|x-auth-token|private-token|x-access-token):\s*(\S+)`)
	// an option name that ends in a secret noun: --password, --api-key, --db-pass
	secretOptName = regexp.MustCompile(`(?i)^(?:.*[-_])?(?:password|passwd|pass|pwd|passphrase|token|secret|api[-_]?key|access[-_]?key|secret[-_]?key|private[-_]?key|auth)$`)
	// NAME=value where NAME names a secret: PGPASSWORD=..., GH_TOKEN=...
	secretAssign = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*(?:PASSWORD|PASSWD|PASS|PWD|TOKEN|SECRET|API_?KEY|ACCESS_?KEY|PASSPHRASE)[A-Za-z0-9_]*)=(.+)$`)
)

// outputPatterns are the patterns Redact looks for in every line. Each has
// literal text that must appear first, so most lines skip the regexp.
var outputPatterns = []struct {
	re      *regexp.Regexp
	needs   []string // at least one must appear (lower-cased when fold)
	fold    bool
	group   bool   // hide only the first group, not the whole match
	finding string // what it is, for "the output contained …"
}{
	{knownTokens[0], []string{"gh"}, false, false, "an access token"},
	{knownTokens[1], []string{"github_pat_"}, false, false, "an access token"},
	{knownTokens[2], []string{"glpat-"}, false, false, "an access token"},
	{knownTokens[3], []string{"AKIA", "ASIA"}, false, false, "a cloud access key"},
	{knownTokens[4], []string{"xox"}, false, false, "an access token"},
	{knownTokens[5], []string{"sk-"}, false, false, "an API key"},
	{knownTokens[6], []string{"AIza"}, false, false, "an API key"},
	{knownTokens[7], []string{"PRIVATE KEY"}, false, false, "a private key"},
	{jwtToken, []string{"eyJ"}, false, false, "a login token (JWT)"},
	{passwordHash, []string{"$"}, false, false, "a password hash"},
	{awsSecret, []string{"secret_access_key"}, true, true, "a cloud secret key"},
	{authHeader, []string{"authorization:"}, true, true, "an authorization header"},
	{keyHeader, []string{"key:", "token:"}, true, true, "an API key"},
	{urlPassword, []string{"://"}, false, true, "a password in a URL"},
}

var (
	jwtToken     = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	passwordHash = regexp.MustCompile(`\$(?:1|2[aby]|5|6|y|gy|7)\$[./A-Za-z0-9$]{8,}`)
	awsSecret    = regexp.MustCompile(`(?i)aws_secret_access_key\s*[=:]\s*"?([A-Za-z0-9/+=]{20,})`)
)

// knownTokens are formats that are secrets wherever they appear.
var knownTokens = []*regexp.Regexp{
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`),         // GitHub
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`),       // GitHub fine-grained
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),           // GitLab
	regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),          // AWS access key id
	regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}\b`),      // Slack
	regexp.MustCompile(`\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}`), // OpenAI / Anthropic style
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),              // Google API key
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),     // a pasted private key
}

// FindFreeTextSecrets looks for passwords, tokens and keys typed into a
// free-text box. secretFlags are the tool's own secret options (such as
// "-p"), whose value is a secret when typed here by hand.
func FindFreeTextSecrets(text string, secretFlags []string) []FreeTextSecret {
	var out []FreeTextSecret
	seen := map[string]bool{}
	add := func(v, kind string) {
		v = strings.TrimSpace(v)
		if v == "" || v == secretMask || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, FreeTextSecret{Value: v, Kind: kind})
	}
	isSecretFlag := func(w string) bool {
		for _, f := range secretFlags {
			if f != "" && w == strings.TrimSuffix(f, "=") {
				return true
			}
		}
		return false
	}

	words := splitShellWords(text)
	for i, w := range words {
		next := ""
		if i+1 < len(words) {
			next = words[i+1]
		}
		for _, m := range urlPassword.FindAllStringSubmatch(w, -1) {
			add(m[1], "password in a URL")
		}
		for _, m := range authHeader.FindAllStringSubmatch(w, -1) {
			add(m[1], "authorization header")
		}
		for _, m := range keyHeader.FindAllStringSubmatch(w, -1) {
			add(m[1], "API key header")
		}
		for _, re := range knownTokens {
			for _, m := range re.FindAllString(w, -1) {
				add(m, "access token or key")
			}
		}
		if m := secretAssign.FindStringSubmatch(w); m != nil && m[1] != "PWD" && m[1] != "OLDPWD" {
			add(m[2], m[1]+" value")
		}

		if !strings.HasPrefix(w, "-") {
			continue
		}
		name, val, hasEq := strings.Cut(w, "=")
		bare := strings.TrimLeft(name, "-")
		boolLike := strings.HasPrefix(bare, "no-") || strings.HasPrefix(bare, "ask-") || strings.HasPrefix(bare, "prompt")
		switch {
		case hasEq && (isSecretFlag(name) || (strings.HasPrefix(name, "--") && secretOptName.MatchString(bare))):
			add(val, "value of "+name)
		case !hasEq && isSecretFlag(w) && next != "":
			add(next, "value of "+w)
		case !hasEq && strings.HasPrefix(w, "--") && !boolLike && secretOptName.MatchString(bare) &&
			next != "" && !strings.HasPrefix(next, "-"):
			add(next, "value of "+w)
		case !hasEq && !strings.HasPrefix(w, "--") && len(w) > 2 && isSecretFlag(w[:2]):
			add(w[2:], "value of "+w[:2]) // -pSECRET, as mysql takes it
		}
		// --user name:password (curl, wget, ...)
		if name == "--user" || name == "-u" {
			cred := val
			if !hasEq {
				cred = next
			}
			if _, pass, ok := strings.Cut(cred, ":"); ok && pass != "" && strings.Trim(pass, "0123456789") != "" {
				add(pass, "password in "+name)
			}
		}
	}
	return out
}

// maskFreeText hides the secrets FindFreeTextSecrets finds in text.
func maskFreeText(text string, secretFlags []string) string {
	found := FindFreeTextSecrets(text, secretFlags)
	// Longest first, so a secret containing another is hidden whole.
	sort.Slice(found, func(i, j int) bool { return len(found[i].Value) > len(found[j].Value) })
	for _, s := range found {
		text = strings.ReplaceAll(text, s.Value, secretMask)
	}
	return text
}

// ── Hiding secrets in a program's output ───────────────────────────────────

// minMaskLen is the shortest secret hidden in output: shorter ones would
// blank out ordinary words.
const minMaskLen = 4

const mersenne61 = 1<<61 - 1

// outputMasker hides known secrets in output lines without keeping them:
// it holds only a salted SHA-256 of each and a keyed rolling hash, and
// scans each line with the rolling hash, confirming candidates with SHA-256.
//
// It also notes what kinds of secret it hid (Findings), and hides every
// line of a private key block, not just its first line.
type outputMasker struct {
	salt  [16]byte
	base  uint64
	byLen map[int]map[uint64][][32]byte
	lens  []int // longest first

	mu       sync.Mutex // Redact runs for stdout and stderr at once
	inKey    bool       // inside -----BEGIN … PRIVATE KEY----- … -----END
	findings map[string]bool
}

func (m *outputMasker) found(what string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.findings == nil {
		m.findings = map[string]bool{}
	}
	m.findings[what] = true
	m.mu.Unlock()
}

// Findings lists the kinds of secret the output contained.
func (m *outputMasker) Findings() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.findings))
	for f := range m.findings {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// keyBlock hides the lines of a private key block.
func (m *outputMasker) keyBlock(line string) (string, bool) {
	if m == nil {
		return "", false
	}
	begin := strings.Contains(line, "-----BEGIN") && strings.Contains(line, "PRIVATE KEY")
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case begin:
		m.inKey = !strings.Contains(line, "-----END") // a one-line key ends here
		if m.findings == nil {
			m.findings = map[string]bool{}
		}
		m.findings["a private key"] = true
		return secretMask + " (private key hidden)", true
	case m.inKey:
		if strings.Contains(line, "-----END") {
			m.inKey = false
		}
		return secretMask, true
	}
	return "", false
}

func newOutputMasker() *outputMasker {
	m := &outputMasker{byLen: map[int]map[uint64][][32]byte{}}
	var b [8]byte
	_, _ = rand.Read(m.salt[:])
	_, _ = rand.Read(b[:])
	m.base = binary.LittleEndian.Uint64(b[:])%(mersenne61-256) + 256
	return m
}

func mulmod(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	// (hi·2^64 + lo) mod 2^61-1
	r := (lo & mersenne61) + (lo >> 61) + (hi << 3)
	r = (r & mersenne61) + (r >> 61)
	if r >= mersenne61 {
		r -= mersenne61
	}
	return r
}

func addmod(a, b uint64) uint64 {
	r := a + b
	if r >= mersenne61 {
		r -= mersenne61
	}
	return r
}

func (m *outputMasker) roll(s []byte) uint64 {
	var h uint64
	for _, c := range s {
		h = addmod(mulmod(h, m.base), uint64(c))
	}
	return h
}

func (m *outputMasker) sum(s []byte) [32]byte {
	h := sha256.New()
	h.Write(m.salt[:])
	h.Write(s)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// add records a secret (and its URL-encoded forms) to hide. The caller may
// wipe secret afterwards: nothing here keeps it.
func (m *outputMasker) add(secret []byte) {
	if m == nil || len(secret) < minMaskLen {
		return
	}
	forms := [][]byte{secret}
	s := string(secret)
	for _, enc := range []string{url.QueryEscape(s), url.PathEscape(s)} {
		if enc != s {
			forms = append(forms, []byte(enc))
		}
	}
	for _, f := range forms {
		n := len(f)
		if m.byLen[n] == nil {
			m.byLen[n] = map[uint64][][32]byte{}
			m.lens = append(m.lens, n)
			sort.Sort(sort.Reverse(sort.IntSlice(m.lens)))
		}
		r := m.roll(f)
		m.byLen[n][r] = append(m.byLen[n][r], m.sum(f))
	}
}

func (m *outputMasker) empty() bool { return m == nil || len(m.lens) == 0 }

// Redact returns line with every known secret, every well-known token
// format and every authorization header value replaced by secretMask.
func (m *outputMasker) Redact(line string) string {
	if hidden, ok := m.keyBlock(line); ok {
		return hidden
	}
	type span struct{ from, to int }
	var spans []span
	b := []byte(line)

	if m != nil {
		for _, n := range m.lens {
			if n > len(b) {
				continue
			}
			table := m.byLen[n]
			pow := uint64(1) // base^(n-1)
			for i := 1; i < n; i++ {
				pow = mulmod(pow, m.base)
			}
			h := m.roll(b[:n])
			for i := 0; ; i++ {
				if sums, ok := table[h]; ok {
					got := m.sum(b[i : i+n])
					for _, s := range sums {
						if s == got {
							spans = append(spans, span{i, i + n})
							m.found("a password or key you entered")
							break
						}
					}
				}
				if i+n >= len(b) {
					break
				}
				// drop b[i], take b[i+n]
				h = addmod(h, mersenne61-mulmod(uint64(b[i]), pow))
				h = addmod(mulmod(h, m.base), uint64(b[i+n]))
			}
		}
	}
	lower := ""
	for _, p := range outputPatterns {
		hay := line
		if p.fold {
			if lower == "" {
				lower = strings.ToLower(line)
			}
			hay = lower
		}
		hit := false
		for _, n := range p.needs {
			if strings.Contains(hay, n) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		for _, loc := range p.re.FindAllStringSubmatchIndex(line, -1) {
			switch {
			case !p.group:
				spans = append(spans, span{loc[0], loc[1]})
				m.found(p.finding)
			case loc[2] >= 0 && loc[3]-loc[2] >= minMaskLen:
				spans = append(spans, span{loc[2], loc[3]})
				m.found(p.finding)
			}
		}
	}
	if len(spans) == 0 {
		return line
	}

	sort.Slice(spans, func(i, j int) bool { return spans[i].from < spans[j].from })
	var out strings.Builder
	pos := 0
	for i := 0; i < len(spans); {
		from, to := spans[i].from, spans[i].to
		for i++; i < len(spans) && spans[i].from <= to; i++ {
			if spans[i].to > to {
				to = spans[i].to
			}
		}
		out.WriteString(line[pos:from])
		out.WriteString(secretMask)
		pos = to
	}
	out.WriteString(line[pos:])
	return out.String()
}
