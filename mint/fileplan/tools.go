package fileplan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// optSpec describes a tool's options: which flags matter, and which take
// a value.
type optSpec struct {
	short     map[byte]string   // short flag → canonical name
	long      map[string]string // long flag (without --) → canonical name
	withValue map[string]bool   // canonical names that take a value
	ignore    map[string]bool   // known options that don't change the plan
}

var toolOptions = map[string]optSpec{
	"rm": {
		short: map[byte]string{'r': "recursive", 'R': "recursive", 'f': "force", 'd': "dir", 'i': "i", 'I': "I", 'v': "verbose"},
		long: map[string]string{"recursive": "recursive", "force": "force", "dir": "dir", "verbose": "verbose",
			"interactive": "i", "one-file-system": "one-file-system", "preserve-root": "preserve-root",
			"no-preserve-root": "no-preserve-root", "help": "help", "version": "version"},
		ignore: map[string]bool{"verbose": true},
	},
	"cp": {
		short: map[byte]string{'r': "recursive", 'R': "recursive", 'a': "archive", 'n': "no-clobber", 'i': "i",
			'f': "force", 'u': "update", 't': "target-directory", 'T': "no-target-directory", 'v': "verbose",
			'p': "preserve", 'P': "no-dereference", 'L': "dereference", 'H': "H", 'l': "link", 's': "symbolic-link",
			'b': "backup", 'S': "suffix", 'x': "one-file-system", 'd': "d"},
		long: map[string]string{"recursive": "recursive", "archive": "archive", "no-clobber": "no-clobber",
			"interactive": "i", "force": "force", "update": "update", "target-directory": "target-directory",
			"no-target-directory": "no-target-directory", "verbose": "verbose", "preserve": "preserve",
			"no-preserve": "preserve", "no-dereference": "no-dereference", "dereference": "dereference",
			"link": "link", "symbolic-link": "symbolic-link", "backup": "backup", "suffix": "suffix",
			"one-file-system": "one-file-system", "sparse": "sparse", "reflink": "reflink", "attributes-only": "attributes-only",
			"remove-destination": "remove-destination", "strip-trailing-slashes": "strip-trailing-slashes",
			"parents": "parents", "debug": "verbose", "help": "help", "version": "version"},
		withValue: map[string]bool{"target-directory": true, "suffix": true},
		ignore: map[string]bool{"verbose": true, "preserve": true, "sparse": true, "reflink": true,
			"strip-trailing-slashes": true, "dereference": true, "no-dereference": true, "H": true, "d": true},
	},
	"mv": {
		short: map[byte]string{'n': "no-clobber", 'i': "i", 'f': "force", 'u': "update", 't': "target-directory",
			'T': "no-target-directory", 'v': "verbose", 'b': "backup", 'S': "suffix", 'Z': "context"},
		long: map[string]string{"no-clobber": "no-clobber", "interactive": "i", "force": "force", "update": "update",
			"target-directory": "target-directory", "no-target-directory": "no-target-directory",
			"verbose": "verbose", "backup": "backup", "suffix": "suffix", "context": "context",
			"strip-trailing-slashes": "strip-trailing-slashes", "debug": "verbose", "exchange": "exchange",
			"no-copy": "no-copy", "help": "help", "version": "version"},
		withValue: map[string]bool{"target-directory": true, "suffix": true},
		ignore:    map[string]bool{"verbose": true, "context": true, "strip-trailing-slashes": true},
	},
}

type opts struct {
	set      map[string]string // canonical name → value ("" for a plain flag)
	operands []string
	unknown  []string
}

func (o opts) has(name string) bool { _, ok := o.set[name]; return ok }

// parseArgs reads GNU-style arguments: bundled short flags (-rf),
// attached and separate values (-tDIR, -t DIR, --target-directory=DIR),
// "--" ending options, and options after operands (GNU permutes).
func parseArgs(spec optSpec, args []string) (opts, error) {
	o := opts{set: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			o.operands = append(o.operands, args[i+1:]...)
			return o, nil
		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			canon, ok := spec.long[name]
			if !ok {
				canon, ok = uniquePrefix(spec.long, name)
			}
			if !ok {
				o.unknown = append(o.unknown, a)
				continue
			}
			if spec.withValue[canon] && !hasVal {
				if i+1 >= len(args) {
					return o, fmt.Errorf("%s needs a value", a)
				}
				i++
				val = args[i]
			}
			o.set[canon] = val
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for j := 1; j < len(a); j++ {
				canon, ok := spec.short[a[j]]
				if !ok {
					o.unknown = append(o.unknown, "-"+string(a[j]))
					continue
				}
				if spec.withValue[canon] {
					val := a[j+1:]
					if val == "" {
						if i+1 >= len(args) {
							return o, fmt.Errorf("-%c needs a value", a[j])
						}
						i++
						val = args[i]
					}
					o.set[canon] = val
					break
				}
				o.set[canon] = ""
			}
		default:
			o.operands = append(o.operands, a)
		}
	}
	return o, nil
}

// uniquePrefix resolves an abbreviated long option, as getopt_long does.
func uniquePrefix(long map[string]string, p string) (string, bool) {
	var found string
	n := 0
	for name, canon := range long {
		if strings.HasPrefix(name, p) {
			if found != canon {
				n++
			}
			found = canon
		}
	}
	return found, n == 1
}

func (pl *planner) noteUnplanned(o opts, names ...string) {
	for _, n := range names {
		if o.has(n) {
			pl.warn(false, "--%s isn't modelled: the real result may differ from this preview", n)
		}
	}
}

func (pl *planner) rm(o opts) {
	if o.has("help") || o.has("version") {
		return
	}
	recursive, force := o.has("recursive"), o.has("force")
	pl.noteUnplanned(o, "one-file-system")
	if o.has("i") || o.has("I") {
		pl.warn(false, "rm's -i and -I ask questions on a terminal; Mint runs it without one, so rm will not ask")
	}
	if len(o.operands) == 0 && !force {
		pl.warn(true, "no files given: rm will fail")
	}
	for _, op := range o.operands {
		p := pl.abs(op)
		trimmed := strings.TrimRight(op, "/")
		if base := filepath.Base(trimmed); trimmed != "" && (base == "." || base == "..") {
			pl.add(Effect{Kind: Fail, Path: p, IsDir: true, Note: "rm refuses to remove . or .."})
			continue
		}
		fi, err := os.Lstat(p)
		if err != nil {
			if force {
				pl.add(Effect{Kind: Keep, Path: p, Note: "doesn't exist (ignored because of -f)"})
			} else {
				pl.add(Effect{Kind: Fail, Path: p, Note: "doesn't exist"})
			}
			continue
		}
		isLink := fi.Mode()&os.ModeSymlink != 0
		if why := pl.dangerous(p); why != "" && !isLink { // a link: only the link goes (link/ is checked below)
			if recursive && fi.IsDir() && !o.has("no-preserve-root") && p == "/" {
				pl.add(Effect{Kind: Fail, Path: p, IsDir: true, Note: "rm refuses to remove / (--preserve-root)"})
				continue
			}
			pl.warn(true, "%s is %s", op, why)
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0 && strings.HasSuffix(op, "/") && recursive:
			// "rm -r link/" follows the link: it empties the folder the
			// link points to and leaves the link and the folder.
			target, err := filepath.EvalSymlinks(p)
			tfi, serr := os.Stat(p)
			if err != nil || serr != nil || !tfi.IsDir() {
				pl.add(Effect{Kind: Fail, Path: p, Note: "a link with a trailing /, to something that isn't a folder"})
				continue
			}
			pl.warn(true, "%s ends with /, so rm follows the link and deletes everything inside %s. Without the / only the link is removed.", op, target)
			if why := pl.dangerous(target); why != "" {
				pl.warn(true, "%s points to %s", op, why)
			}
			pl.walk(target, func(path string, fi fs.FileInfo) {
				if path != target {
					pl.add(Effect{Kind: Delete, Path: path, IsDir: fi.IsDir(), Size: size(fi), Note: "inside " + target + ", through " + op})
				}
			})
		case fi.Mode()&os.ModeSymlink != 0:
			pl.add(Effect{Kind: Delete, Path: p, Note: "a symbolic link: only the link is removed"})
		case !fi.IsDir():
			pl.add(Effect{Kind: Delete, Path: p, Size: size(fi)})
		case recursive:
			pl.walk(p, func(path string, fi fs.FileInfo) {
				pl.add(Effect{Kind: Delete, Path: path, IsDir: fi.IsDir(), Size: size(fi)})
			})
		case o.has("dir"):
			entries, err := os.ReadDir(p)
			if err == nil && len(entries) == 0 {
				pl.add(Effect{Kind: Delete, Path: p, IsDir: true})
			} else {
				pl.add(Effect{Kind: Fail, Path: p, IsDir: true, Note: "folder isn't empty (needs -r)"})
			}
		default:
			pl.add(Effect{Kind: Fail, Path: p, IsDir: true, Note: "is a folder (needs -r)"})
		}
	}
	sortEffects(pl.plan.Effects)
}

func (pl *planner) copyOrMove(move bool, o opts) {
	if o.has("help") || o.has("version") {
		return
	}
	pl.noteUnplanned(o, "backup", "suffix", "link", "symbolic-link", "attributes-only", "parents", "exchange", "remove-destination")
	if o.has("i") {
		pl.warn(false, "-i asks questions on a terminal; Mint runs the tool without one, so it will not ask")
	}
	recursive := move || o.has("recursive") || o.has("archive")
	sources := o.operands
	var dest string
	intoDir := false
	switch {
	case o.has("target-directory"):
		dest, intoDir = pl.abs(o.set["target-directory"]), true
		if fi, err := os.Stat(dest); err != nil || !fi.IsDir() {
			pl.warn(true, "%s is not an existing folder", o.set["target-directory"])
			return
		}
	case len(sources) < 2:
		pl.warn(true, "needs a source and a destination")
		return
	default:
		dest = pl.abs(sources[len(sources)-1])
		sources = sources[:len(sources)-1]
		fi, err := os.Stat(dest)
		isDir := err == nil && fi.IsDir()
		switch {
		case o.has("no-target-directory"):
		case isDir:
			intoDir = true
		case len(sources) > 1:
			pl.warn(true, "%d sources but %s is not an existing folder", len(sources), sources[len(sources)-1])
			return
		}
		if intoDir && len(sources) == 1 {
			pl.warn(false, "%s is an existing folder, so %s goes inside it, as %s",
				filepath.Base(dest), filepath.Base(sources[0]), filepath.Join(filepath.Base(dest), filepath.Base(strings.TrimRight(sources[0], "/"))))
		}
	}
	destDev, _ := devOf(dest)
	if !intoDir {
		destDev, _ = devOf(filepath.Dir(dest))
	}
	for _, s := range sources {
		src := pl.abs(s)
		if lfi, err := os.Lstat(src); err == nil && lfi.Mode()&os.ModeSymlink != 0 && strings.HasSuffix(s, "/") {
			pl.warn(false, "%s is a link with a trailing /: the tool follows it, which this preview doesn't model", s)
		}
		fi, err := os.Lstat(src)
		if err != nil {
			pl.add(Effect{Kind: Fail, Path: src, Note: "doesn't exist"})
			continue
		}
		target := dest
		if intoDir {
			target = filepath.Join(dest, filepath.Base(src))
		}
		if target == src {
			pl.add(Effect{Kind: Fail, Path: src, Note: "source and destination are the same file"})
			continue
		}
		if fi.IsDir() && strings.HasPrefix(target+string(filepath.Separator), src+string(filepath.Separator)) {
			if !move && recursive {
				// GNU cp copies what it can, then reports the error.
				if _, err := os.Lstat(target); err != nil {
					pl.walk(src, func(path string, sfi fs.FileInfo) {
						rel, _ := filepath.Rel(src, path)
						if path != target && !strings.HasPrefix(path, target+string(filepath.Separator)) {
							pl.add(Effect{Kind: Create, Path: filepath.Join(target, rel), IsDir: sfi.IsDir(), Size: size(sfi)})
						}
					})
				}
				pl.add(Effect{Kind: Fail, Path: src, IsDir: true, Note: "can't copy a folder into itself: cp copies part of it, then stops with an error"})
				continue
			}
			pl.add(Effect{Kind: Fail, Path: src, IsDir: true, Note: "can't move a folder into itself"})
			continue
		}
		if fi.IsDir() && !recursive {
			pl.add(Effect{Kind: Fail, Path: src, IsDir: true, Note: "is a folder: omitted (needs -r)"})
			continue
		}
		if why := pl.dangerous(target); why != "" {
			if tfi, err := os.Lstat(target); err == nil && !(fi.IsDir() && tfi.IsDir()) {
				pl.warn(true, "%s would replace %s", s, why)
			}
		}
		if move {
			if srcDev, ok := devOf(src); ok && destDev != 0 && srcDev != destDev {
				pl.warn(false, "%s is on a different filesystem: mv copies it and then deletes the original", s)
			}
			if why := pl.dangerous(src); why != "" {
				pl.warn(true, "moving %s moves %s", s, why)
			}
		}
		pl.place(move, o, src, target, fi)
	}
	sortEffects(pl.plan.Effects)
}

// place records copying or moving src (with info fi) to target.
func (pl *planner) place(move bool, o opts, src, target string, fi fs.FileInfo) {
	tfi, terr := os.Lstat(target)
	exists := terr == nil
	if move {
		switch {
		case !exists:
			pl.add(Effect{Kind: Move, Path: src, Dest: target, IsDir: fi.IsDir(), Size: size(fi)})
		case fi.IsDir() && !tfi.IsDir():
			pl.add(Effect{Kind: Fail, Path: src, Dest: target, IsDir: true, Note: "can't replace a file with a folder"})
		case !fi.IsDir() && tfi.IsDir():
			pl.add(Effect{Kind: Fail, Path: src, Dest: target, Note: "can't replace a folder with a file"})
		case fi.IsDir():
			if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
				pl.add(Effect{Kind: Fail, Path: src, Dest: target, IsDir: true, Note: "destination folder isn't empty"})
			} else {
				pl.add(Effect{Kind: Move, Path: src, Dest: target, IsDir: true, Note: "replaces an empty folder"})
			}
		case pl.kept(o, fi, tfi):
			pl.add(Effect{Kind: Keep, Path: target, Note: pl.keepReason(o)})
		default:
			pl.add(Effect{Kind: Overwrite, Path: target, Size: size(tfi), Note: "replaced by " + src})
			pl.add(Effect{Kind: Move, Path: src, Dest: target, Size: size(fi)})
		}
		return
	}
	if !fi.IsDir() {
		switch {
		case !exists:
			pl.add(Effect{Kind: Create, Path: target, Dest: target, Size: size(fi), Note: "copy of " + src})
		case tfi.IsDir():
			pl.add(Effect{Kind: Fail, Path: src, Dest: target, Note: "can't replace a folder with a file"})
		case pl.kept(o, fi, tfi):
			pl.add(Effect{Kind: Keep, Path: target, Note: pl.keepReason(o)})
		default:
			pl.add(Effect{Kind: Overwrite, Path: target, Size: size(tfi), Note: "replaced by a copy of " + src})
		}
		return
	}
	// A folder, copied recursively: map every entry under src to target.
	if exists && !tfi.IsDir() {
		pl.add(Effect{Kind: Fail, Path: src, Dest: target, IsDir: true, Note: "can't replace a file with a folder"})
		return
	}
	pl.walk(src, func(path string, sfi fs.FileInfo) {
		rel, _ := filepath.Rel(src, path)
		to := filepath.Join(target, rel)
		dfi, err := os.Lstat(to)
		switch {
		case err != nil:
			pl.add(Effect{Kind: Create, Path: to, Dest: to, IsDir: sfi.IsDir(), Size: size(sfi)})
		case sfi.IsDir() && dfi.IsDir():
			// merged into the existing folder: nothing to report
		case sfi.IsDir() != dfi.IsDir():
			pl.add(Effect{Kind: Fail, Path: path, Dest: to, IsDir: sfi.IsDir(), Note: "a file and a folder with the same name"})
		case pl.kept(o, sfi, dfi):
			pl.add(Effect{Kind: Keep, Path: to, Note: pl.keepReason(o)})
		default:
			pl.add(Effect{Kind: Overwrite, Path: to, Size: size(dfi), Note: "replaced by a copy of " + path})
		}
	})
}

// kept reports whether an existing destination is left alone.
func (pl *planner) kept(o opts, src, dst fs.FileInfo) bool {
	if o.has("no-clobber") {
		return true
	}
	if o.has("update") {
		switch o.set["update"] {
		case "none", "none-fail":
			return true
		case "all":
			return false
		}
		return !src.ModTime().After(dst.ModTime())
	}
	return false
}

func (pl *planner) keepReason(o opts) string {
	if o.has("no-clobber") {
		return "already exists (kept because of -n; newer coreutils then exit with an error)"
	}
	return "already exists and isn't older (kept because of -u)"
}

// ExpandPatterns replaces each word that is a pattern (with * ? or [)
// matching at least one file, relative to dir, by the files it matches, in
// order. Words that match nothing, or aren't patterns, stay as they are.
// It reports whether anything changed.
func ExpandPatterns(words []string, dir string) ([]string, bool) {
	var out []string
	changed := false
	for _, w := range words {
		if !strings.ContainsAny(w, "*?[") {
			out = append(out, w)
			continue
		}
		pat := w
		if !filepath.IsAbs(pat) {
			pat = filepath.Join(dir, pat)
		}
		matches, err := filepath.Glob(pat)
		if err != nil || len(matches) == 0 {
			out = append(out, w)
			continue
		}
		for _, m := range matches {
			if !filepath.IsAbs(w) {
				if rel, err := filepath.Rel(dir, m); err == nil {
					m = rel
				}
			}
			// Like a shell, a pattern doesn't match names starting with
			// "." unless it says so.
			if strings.HasPrefix(filepath.Base(m), ".") && !strings.HasPrefix(filepath.Base(w), ".") {
				continue
			}
			out = append(out, m)
		}
		changed = true
	}
	return out, changed
}
