// Package fileplan works out what rm, cp and mv would do with a given
// argument list, without doing it, so an interface can show the effect
// before the user confirms.
//
// It only reads the tree: Lstat, ReadDir and EvalSymlinks, never following a
// symlink into a folder. Planning rm also runs "rm --version" once, to tell
// GNU coreutils before 9.12 (which follows "rm -r link/" and empties the
// folder) from 9.12 and later (which removes the link). The result is Mint's
// estimate from those rules, not output from the deletion itself, and the
// tree can change between planning and running; callers say so, and plan
// again just before running.
package fileplan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Kind is what happens to one path.
type Kind int

const (
	Delete    Kind = iota // removed
	Overwrite             // replaced by another file
	Create                // a new file or folder appears
	Move                  // renamed (the source goes away, the destination appears)
	Keep                  // left as it is (cp -n, mv -n, cp -u with a newer destination)
	Fail                  // the tool will refuse this one
)

func (k Kind) String() string {
	return [...]string{"delete", "overwrite", "create", "move", "keep", "fail"}[k]
}

// Effect is one path's fate.
type Effect struct {
	Kind  Kind
	Path  string // the path affected (for Move, the source)
	Dest  string // for Move and Create/Overwrite by copy: where it goes
	IsDir bool
	Size  int64  // bytes of a regular file
	Note  string // why, for Fail and Keep; extra detail otherwise
}

// Warning is something the user should know before running. Blocking
// warnings should stop the run unless the user explicitly goes ahead.
type Warning struct {
	Blocking bool
	Text     string
}

// Plan is the estimated effect of one run.
type Plan struct {
	Tool     string
	Effects  []Effect
	Warnings []Warning
	// Incomplete is set when a folder was too large to list fully within
	// the budget; counts are then "at least".
	Incomplete bool
	// Unknown lists options the planner doesn't understand; the plan may
	// be wrong when there are any.
	Unknown []string
}

// Totals counts effects by kind, and bytes deleted or overwritten.
type Totals struct {
	Count        map[Kind]int
	BytesRemoved int64
}

// Totals sums the plan.
func (p Plan) Totals() Totals {
	t := Totals{Count: map[Kind]int{}}
	for _, e := range p.Effects {
		t.Count[e.Kind]++
		if e.Kind == Delete || e.Kind == Overwrite {
			t.BytesRemoved += e.Size
		}
	}
	return t
}

// Blocked reports whether any warning is blocking.
func (p Plan) Blocked() bool {
	for _, w := range p.Warnings {
		if w.Blocking {
			return true
		}
	}
	return false
}

// Fingerprint summarises the plan, to tell whether planning again just
// before running gives a different answer.
func (p Plan) Fingerprint() string {
	var b strings.Builder
	for _, e := range p.Effects {
		fmt.Fprintf(&b, "%d|%s|%s|%v|%d\n", e.Kind, e.Path, e.Dest, e.IsDir, e.Size)
	}
	fmt.Fprintf(&b, "incomplete=%v", p.Incomplete)
	return b.String()
}

// Budget bounds how much of the tree is listed.
type Budget struct {
	Entries int           // most paths to look at (default 200,000)
	Time    time.Duration // longest to spend (default 2s)
}

// Env is what the plan is worked out against.
type Env struct {
	Dir    string // the folder the tool will run in
	Home   string // the user's home, for the dangerous-target check
	Budget Budget
}

type planner struct {
	env      Env
	plan     *Plan
	seen     int
	deadline time.Time
}

func (pl *planner) spend() bool {
	pl.seen++
	if pl.seen > pl.env.Budget.Entries || (pl.seen&255 == 0 && time.Now().After(pl.deadline)) {
		pl.plan.Incomplete = true
		return false
	}
	return true
}

func (pl *planner) abs(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(pl.env.Dir, p)
}

func (pl *planner) warn(blocking bool, format string, a ...any) {
	pl.plan.Warnings = append(pl.plan.Warnings, Warning{Blocking: blocking, Text: fmt.Sprintf(format, a...)})
}

func (pl *planner) add(e Effect) { pl.plan.Effects = append(pl.plan.Effects, e) }

// For returns the plan for running tool with args in env. tool is rm, cp
// or mv (by base name); any other tool is an error.
func For(tool string, args []string, env Env) (Plan, error) {
	if env.Budget.Entries <= 0 {
		env.Budget.Entries = 200000
	}
	if env.Budget.Time <= 0 {
		env.Budget.Time = 2 * time.Second
	}
	if env.Dir == "" {
		env.Dir, _ = os.Getwd()
	}
	base := filepath.Base(tool)
	pl := &planner{env: env, plan: &Plan{Tool: base}, deadline: time.Now().Add(env.Budget.Time)}
	spec, ok := toolOptions[base]
	if !ok {
		return Plan{}, fmt.Errorf("no planner for %s", base)
	}
	o, err := parseArgs(spec, args)
	if err != nil {
		return Plan{}, err
	}
	pl.plan.Unknown = o.unknown
	for _, op := range o.operands {
		pl.checkOperand(op)
	}
	switch base {
	case "rm":
		pl.rm(o)
	case "cp", "mv":
		pl.copyOrMove(base == "mv", o)
	}
	pl.plan.Effects = dedupe(pl.plan.Effects)
	return *pl.plan, nil
}

// dedupe keeps the first effect for each path: a file reached twice (as
// "build" and, when rm follows a trailing slash, through "link/") is
// still deleted once.
func dedupe(es []Effect) []Effect {
	seen := map[string]bool{}
	out := es[:0]
	for _, e := range es {
		key := fmt.Sprintf("%d\x00%s\x00%s", e.Kind, e.Path, e.Dest)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

// checkOperand warns about names that aren't what they look like.
func (pl *planner) checkOperand(op string) {
	if strings.ContainsAny(op, "*?[") {
		if _, err := os.Lstat(pl.abs(op)); err != nil {
			pl.warn(true, "%q is a pattern, but no shell expands it here: the tool will look for a file with exactly that name. Use \"Expand patterns\" to list the files it matches.", op)
		}
	}
	if strings.HasPrefix(op, "~") {
		if _, err := os.Lstat(pl.abs(op)); err != nil {
			pl.warn(true, "%q starts with ~, but no shell expands it here: the tool will look for a folder named \"~\".", op)
		}
	}
}

// dangerous reports why removing or overwriting p would be dangerous, or "".
func (pl *planner) dangerous(p string) string {
	real := p
	if r, err := filepath.EvalSymlinks(p); err == nil {
		real = r
	}
	switch real {
	case "/":
		return "the root of the filesystem"
	case filepath.Clean(pl.env.Home):
		if pl.env.Home != "" {
			return "your home folder"
		}
	}
	if filepath.Dir(real) == "/" {
		switch filepath.Base(real) {
		case "bin", "boot", "dev", "etc", "home", "lib", "lib32", "lib64", "libx32", "opt", "proc",
			"root", "run", "sbin", "srv", "sys", "usr", "var", "snap", "mnt", "media", "tmp":
			return "a top-level system folder"
		}
	}
	if isMountPoint(real) {
		return "a mount point (a whole filesystem)"
	}
	return ""
}

func isMountPoint(p string) bool {
	fi, err := os.Lstat(p)
	if err != nil || !fi.IsDir() {
		return false
	}
	parent, err := os.Lstat(filepath.Dir(p))
	if err != nil {
		return false
	}
	a, ok1 := fi.Sys().(*syscall.Stat_t)
	b, ok2 := parent.Sys().(*syscall.Stat_t)
	return ok1 && ok2 && a.Dev != b.Dev
}

func devOf(p string) (uint64, bool) {
	fi, err := os.Lstat(p)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}

// walk calls fn for root and, when it is a real folder (not a symlink to
// one), everything under it, parents before children. It stops when the
// budget runs out.
func (pl *planner) walk(root string, fn func(path string, fi fs.FileInfo)) {
	fi, err := os.Lstat(root)
	if err != nil {
		return
	}
	if !pl.spend() {
		return
	}
	fn(root, fi)
	if !fi.IsDir() {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		pl.warn(false, "can't list %s (%v): what's inside isn't counted", root, errors.Unwrap(err))
		return
	}
	for _, e := range entries {
		if pl.plan.Incomplete {
			return
		}
		pl.walk(filepath.Join(root, e.Name()), fn)
	}
}

func size(fi fs.FileInfo) int64 {
	if fi.Mode().IsRegular() {
		return fi.Size()
	}
	return 0
}

// sortEffects orders effects by path, keeping a folder before its contents.
func sortEffects(es []Effect) {
	sort.SliceStable(es, func(i, j int) bool { return es[i].Path < es[j].Path })
}
