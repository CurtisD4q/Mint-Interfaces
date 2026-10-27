package appkit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/findings"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// Source context: the lines around a finding, read from the file it names.
//
// The file name comes from the tool's report and is untrusted. When the
// app runs as root, reading whatever a report names would show any file
// on the system, so Mint reads only files that are, after resolving every
// symlink, one of the files the user asked to scan or inside one of the
// folders they asked to scan.

// contextLines is how many lines are shown on each side of the finding.
const contextLines = 5

// maxContextFile bounds the size of a file read for context.
const maxContextFile = 8 << 20

// scanScope is what a run was asked to look at, resolved.
type scanScope struct {
	dir   string          // the working directory the tool ran in
	files map[string]bool // resolved paths of files named in the target
	dirs  []string        // resolved folders named in the target
}

// newScanScope resolves the target words of a run, relative to dir.
// Words that don't name an existing file or folder (options, globs that
// the tool expands itself, hosts) are left out, so they allow nothing.
func newScanScope(dir string, words []string) scanScope {
	s := scanScope{dir: dir, files: map[string]bool{}}
	for _, w := range words {
		if w == "" || w == "--" || strings.HasPrefix(w, "-") && !fileExists(filepath.Join(dir, w)) {
			continue
		}
		p := util.ExpandUser(w)
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		fi, err := os.Stat(real)
		switch {
		case err != nil:
		case fi.IsDir():
			s.dirs = append(s.dirs, real)
		case fi.Mode().IsRegular():
			s.files[real] = true
		}
	}
	return s
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// errOutsideScope is returned for a file the run wasn't asked to scan.
var errOutsideScope = errors.New("not one of the files or folders scanned, so Mint won't read it")

// resolve returns the real path of name if it is in scope.
func (s scanScope) resolve(name string) (string, error) {
	if name == "" {
		return "", errors.New("the finding names no file")
	}
	p := name
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.dir, p)
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("can't find %s", name)
	}
	if s.files[real] {
		return real, nil
	}
	for _, d := range s.dirs {
		if real == d || strings.HasPrefix(real, d+string(filepath.Separator)) || d == string(filepath.Separator) {
			return real, nil
		}
	}
	return "", errOutsideScope
}

// SourceLine is one line of context.
type SourceLine struct {
	No   int
	Text string
}

// readContext reads the lines around line (from 1) of the in-scope file
// name: up to contextLines on each side.
func (s scanScope) readContext(name string, line int) ([]SourceLine, error) {
	if line < 1 {
		return nil, errors.New("the finding gives no line")
	}
	real, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	// O_NOFOLLOW and the same-file check catch the file being swapped
	// for a symlink after it was resolved.
	before, err := os.Lstat(real)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if before.Size() > maxContextFile {
		return nil, fmt.Errorf("larger than %d MB", maxContextFile>>20)
	}
	fh, err := os.OpenFile(real, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	if after, err := fh.Stat(); err != nil || !os.SameFile(before, after) {
		return nil, errors.New("the file changed while opening it")
	}
	sc := bufio.NewScanner(io.LimitReader(fh, maxContextFile))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	var out []SourceLine
	for no := 1; sc.Scan(); no++ {
		if no < line-contextLines {
			continue
		}
		if no > line+contextLines {
			break
		}
		out = append(out, SourceLine{No: no, Text: sc.Text()})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the file has no line %d", line)
	}
	return out, nil
}

// contextStyled lays out source lines, marking the finding's line and
// column.
func contextStyled(src []SourceLine, line, col, width int) []mint.StyledLine {
	numW := len(fmt.Sprint(src[len(src)-1].No))
	var out []mint.StyledLine
	for _, l := range src {
		marker, role := "  ", mint.RoleMuted
		if l.No == line {
			marker, role = "▶ ", mint.RoleText
		}
		text := mint.CleanLineVisible(l.Text)
		if r := []rune(text); len(r) > width-numW-4 && width-numW-5 > 0 {
			text = string(r[:width-numW-5]) + "…"
		}
		out = append(out, mint.StyledLine{Segs: []mint.Seg{
			{Text: fmt.Sprintf("%s%*d ", marker, numW, l.No), Role: mint.RoleMuted},
			{Text: text, Role: role},
		}})
		if l.No == line && col > 0 && col <= len([]rune(text))+1 {
			out = append(out, mint.StyledLine{Segs: []mint.Seg{
				{Text: strings.Repeat(" ", numW+3+col-1) + "^", Role: mint.RoleWarn}}})
		}
	}
	return out
}

// New since last run.
//
// The fingerprints of each target's last findings are kept in the app's
// data folder (readable only by the user), so a run can say which
// findings are new and how many were fixed.

const maxRememberedTargets = 50

type findingsMemory struct {
	// Targets maps a hash of the target to its last findings' fingerprints.
	Targets map[string]rememberedRun `json:"targets"`
}

type rememberedRun struct {
	Seq          int      `json:"seq"` // for dropping the oldest
	Fingerprints []string `json:"fingerprints"`
}

func (a *ToolApp) findingsMemoryPath() string {
	return filepath.Join(a.DataDir, "findings-last.json")
}

// targetKey identifies a target without storing its text.
func targetKey(tool string, args []string) string {
	h := sha256.Sum256([]byte(tool + "\x00" + strings.Join(args, "\x00")))
	return hex.EncodeToString(h[:12])
}

// compareWithLastRun marks which findings are new for key and counts those
// fixed, then remembers this run's. first is true when there was nothing
// to compare with.
func (a *ToolApp) compareWithLastRun(key string, fs []findings.Finding) (isNew map[string]bool, fixed int, first bool) {
	if a.DataDir == "" {
		return nil, 0, true
	}
	var mem findingsMemory
	if b, err := os.ReadFile(a.findingsMemoryPath()); err == nil {
		_ = json.Unmarshal(b, &mem)
	}
	if mem.Targets == nil {
		mem.Targets = map[string]rememberedRun{}
	}
	now := map[string]bool{}
	var fps []string
	for _, f := range fs {
		fp := f.Fingerprint()
		if !now[fp] {
			now[fp] = true
			fps = append(fps, fp)
		}
	}
	prev, seen := mem.Targets[key]
	isNew = map[string]bool{}
	if seen {
		before := map[string]bool{}
		for _, fp := range prev.Fingerprints {
			before[fp] = true
			if !now[fp] {
				fixed++
			}
		}
		for fp := range now {
			if !before[fp] {
				isNew[fp] = true
			}
		}
	}
	seq := 0
	for _, r := range mem.Targets {
		seq = max(seq, r.Seq)
	}
	sort.Strings(fps)
	mem.Targets[key] = rememberedRun{Seq: seq + 1, Fingerprints: fps}
	for len(mem.Targets) > maxRememberedTargets {
		oldest, oldSeq := "", int(^uint(0)>>1)
		for k, r := range mem.Targets {
			if r.Seq < oldSeq {
				oldest, oldSeq = k, r.Seq
			}
		}
		delete(mem.Targets, oldest)
	}
	if b, err := json.Marshal(mem); err == nil {
		_ = util.Mkdir(a.DataDir)
		if os.WriteFile(a.findingsMemoryPath(), b, 0o600) == nil {
			a.giveBack(a.findingsMemoryPath())
		}
	}
	return isNew, fixed, !seen
}
