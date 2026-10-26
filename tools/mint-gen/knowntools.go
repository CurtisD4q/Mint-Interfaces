package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// KnownTool records what a program reads and writes when its help text
// cannot say it clearly. Built-in entries are below; more can be added in
// JSON files (see loadKnownToolFiles).
type KnownTool struct {
	Name    string        `json:"name"`
	Options []KnownOption `json:"options,omitempty"`
	// Stdout is what the program prints: "results" (worth saving), "log"
	// (progress and messages; the real output goes elsewhere), "binary",
	// or "" (unknown).
	Stdout string `json:"stdout,omitempty"`
	// OutputPositional: the last argument is the file it writes (ffmpeg),
	// with OutputExt as the default extension (ffmpeg picks the format from it).
	OutputPositional bool   `json:"output_positional,omitempty"`
	OutputExt        string `json:"output_ext,omitempty"`
	// StdoutExt: the extension for saving what it prints (jq prints JSON).
	StdoutExt string `json:"stdout_ext,omitempty"`
}

// KnownOption is one option of a KnownTool.
type KnownOption struct {
	Flag string `json:"flag"`
	Role string `json:"role"`           // an appkit.ArgRole, e.g. "output-file"
	Ext  string `json:"ext,omitempty"`  // extension of the file it writes: "pcap", "xml"
	Main bool   `json:"main,omitempty"` // the tool's main output file (used for "Save output to file")
}

// Stdout values (the same as appkit's).
const (
	StdoutResults = appkit.StdoutResults
	StdoutLog     = appkit.StdoutLog
	StdoutBinary  = appkit.StdoutBinary
)

// builtinKnownTools are facts about common tools.
var builtinKnownTools = []KnownTool{
	{Name: "tcpdump", Stdout: StdoutResults, Options: []KnownOption{
		{Flag: "-w", Role: "output-file", Ext: "pcap", Main: true}, {Flag: "-r", Role: "input-file"}, {Flag: "-F", Role: "input-file"}}},
	{Name: "tshark", Stdout: StdoutResults, Options: []KnownOption{
		{Flag: "-w", Role: "output-file", Ext: "pcapng", Main: true}, {Flag: "-r", Role: "input-file"}}},
	{Name: "dumpcap", Stdout: StdoutLog, Options: []KnownOption{{Flag: "-w", Role: "output-file", Ext: "pcapng", Main: true}}},
	{Name: "nmap", Stdout: StdoutResults, Options: []KnownOption{
		{Flag: "-oN", Role: "output-file", Ext: "nmap", Main: true}, {Flag: "-oX", Role: "output-file", Ext: "xml"},
		{Flag: "-oG", Role: "output-file", Ext: "gnmap"}, {Flag: "-oS", Role: "output-file", Ext: "txt"},
		{Flag: "-oA", Role: "output-base"}, {Flag: "-iL", Role: "input-file"}, {Flag: "--resume", Role: "input-file"},
		{Flag: "--excludefile", Role: "input-file"}, {Flag: "--stylesheet", Role: "input-file"}}},
	{Name: "masscan", Stdout: StdoutResults, Options: []KnownOption{
		{Flag: "-oL", Role: "output-file", Ext: "txt", Main: true}, {Flag: "-oX", Role: "output-file", Ext: "xml"},
		{Flag: "-oJ", Role: "output-file", Ext: "json"}, {Flag: "-oG", Role: "output-file", Ext: "gnmap"},
		{Flag: "-iL", Role: "input-file"}, {Flag: "-c", Role: "config-file"}}},
	{Name: "ffmpeg", Stdout: StdoutLog, OutputPositional: true, OutputExt: "mkv", Options: []KnownOption{{Flag: "-i", Role: "input-file"}}},
	{Name: "curl", Stdout: StdoutResults, Options: []KnownOption{
		{Flag: "-o", Role: "output-file", Main: true}, {Flag: "-D", Role: "output-file"}, {Flag: "-c", Role: "output-file"},
		{Flag: "-b", Role: "input-file"}, {Flag: "-K", Role: "config-file"}, {Flag: "-T", Role: "input-file"}}},
	{Name: "wget", Stdout: StdoutLog, Options: []KnownOption{
		{Flag: "-O", Role: "output-file", Main: true}, {Flag: "-o", Role: "log-file"}, {Flag: "-a", Role: "log-file"},
		{Flag: "-P", Role: "output-dir"}, {Flag: "-i", Role: "input-file"}}},
	{Name: "sort", Stdout: StdoutResults, Options: []KnownOption{{Flag: "-o", Role: "output-file", Main: true}, {Flag: "-T", Role: "dir"}}},
	{Name: "gcc", Stdout: StdoutLog, Options: []KnownOption{{Flag: "-o", Role: "output-file", Ext: "out", Main: true}}},
	// pandoc picks the output format from the file's extension; HTML is its default.
	{Name: "pandoc", Stdout: StdoutResults, Options: []KnownOption{{Flag: "-o", Role: "output-file", Ext: "html", Main: true}, {Flag: "-t", Role: "format"}}}, // -t is the output format; -f is the input's
	{Name: "tar", Stdout: StdoutLog, Options: []KnownOption{{Flag: "-f", Role: "file"}, {Flag: "-C", Role: "dir"}, {Flag: "-T", Role: "input-file"}, {Flag: "-X", Role: "input-file"}}},
	{Name: "gzip", Stdout: StdoutLog}, {Name: "xz", Stdout: StdoutLog}, {Name: "bzip2", Stdout: StdoutLog}, {Name: "zstd", Stdout: StdoutLog},
	{Name: "zip", Stdout: StdoutLog}, {Name: "7z", Stdout: StdoutLog}, {Name: "rsync", Stdout: StdoutLog},
	{Name: "cp", Stdout: StdoutLog}, {Name: "mv", Stdout: StdoutLog}, {Name: "dd", Stdout: StdoutLog},
	{Name: "ls", Stdout: StdoutResults}, {Name: "grep", Stdout: StdoutResults}, {Name: "jq", Stdout: StdoutResults, StdoutExt: "json"},
	{Name: "find", Stdout: StdoutResults}, {Name: "dig", Stdout: StdoutResults}, {Name: "whois", Stdout: StdoutResults},
}

// ── Loading JSON files ─────────────────────────────────────────────────────

// Where JSON files are read from (tests replace these).
var knownToolDirs = func() []string {
	dirs := []string{"/etc/mint-interfaces/tools.d"}
	if d, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(d, "mint-interfaces", "tools.d"))
	}
	return dirs
}

const (
	maxKnownToolFiles   = 64
	maxKnownToolBytes   = 256 << 10
	maxKnownToolEntries = 500
	maxKnownOptions     = 200
)

var (
	reKnownName = regexp.MustCompile(`^[a-z0-9][a-z0-9._+-]{0,63}$`)
	reKnownFlag = regexp.MustCompile(`^--?[A-Za-z0-9][-A-Za-z0-9_.]{0,63}$`)
	reKnownExt  = regexp.MustCompile(`^[a-z0-9][a-z0-9+-]{0,15}(?:\.[a-z0-9][a-z0-9+-]{0,15})?$`)
)

// knownToolFile is the content of one JSON file.
type knownToolFile struct {
	Tools []KnownTool `json:"tools"`
}

// validKnownTool checks an entry from a file. Names, options and
// extensions must be plain (no paths, no "..", nothing that could reach
// generated code or a file name unchecked), and roles must be real roles.
func validKnownTool(t KnownTool) error {
	if !reKnownName.MatchString(t.Name) {
		return fmt.Errorf("tool name %q is not allowed", t.Name)
	}
	switch t.Stdout {
	case "", StdoutResults, StdoutLog, StdoutBinary:
	default:
		return fmt.Errorf("%s: stdout %q is not one of results, log, binary", t.Name, t.Stdout)
	}
	if t.OutputExt != "" && !reKnownExt.MatchString(t.OutputExt) {
		return fmt.Errorf("%s: output extension %q is not allowed", t.Name, t.OutputExt)
	}
	if t.StdoutExt != "" && !reKnownExt.MatchString(t.StdoutExt) {
		return fmt.Errorf("%s: stdout extension %q is not allowed", t.Name, t.StdoutExt)
	}
	if t.OutputPositional {
		for _, o := range t.Options {
			if o.Main {
				return fmt.Errorf("%s: a main output option and an output argument cannot both be set", t.Name)
			}
		}
	}
	if len(t.Options) > maxKnownOptions {
		return fmt.Errorf("%s: more than %d options", t.Name, maxKnownOptions)
	}
	mains := 0
	for _, o := range t.Options {
		if !reKnownFlag.MatchString(o.Flag) {
			return fmt.Errorf("%s: option %q is not allowed", t.Name, o.Flag)
		}
		if r := appkit.ArgRole(o.Role); !r.Valid() || r == appkit.RoleValue {
			return fmt.Errorf("%s %s: role %q is not a known role", t.Name, o.Flag, o.Role)
		}
		if o.Ext != "" && !reKnownExt.MatchString(o.Ext) {
			return fmt.Errorf("%s %s: extension %q is not allowed", t.Name, o.Flag, o.Ext)
		}
		if o.Main {
			mains++
			if appkit.ArgRole(o.Role) != appkit.RoleOutputFile {
				return fmt.Errorf("%s %s: only an output-file can be the main output", t.Name, o.Flag)
			}
		}
	}
	if mains > 1 {
		return fmt.Errorf("%s: more than one main output", t.Name)
	}
	return nil
}

// trustedOwner reports whether a file or folder may be trusted: owned by
// this user or root, and not writable by anyone else.
func trustedOwner(fi os.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot check its owner")
	}
	if int(st.Uid) != os.Geteuid() && st.Uid != 0 {
		return errors.New("it belongs to another user")
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return errors.New("other users can change it")
	}
	return nil
}

// loadKnownToolFiles reads *.json files from dir. A file is used only if
// it and its folder pass trustedOwner, it is a regular file (not a link),
// is small, and is valid JSON of exactly the expected shape (unknown
// fields are refused, so a file cannot set anything else). Each problem is
// reported and the file (or entry) skipped; nothing here can fail mint-gen.
func loadKnownToolFiles(dir string) ([]KnownTool, []string) {
	var tools []KnownTool
	var warn []string
	di, err := os.Lstat(dir)
	if err != nil {
		return nil, nil // no folder: nothing to read
	}
	if !di.IsDir() {
		return nil, []string{dir + ": not a folder"}
	}
	if err := trustedPath(dir); err != nil {
		return nil, []string{dir + ": ignored, " + err.Error()}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	sort.Strings(matches)
	if len(matches) > maxKnownToolFiles {
		warn = append(warn, fmt.Sprintf("%s: only the first %d files are read", dir, maxKnownToolFiles))
		matches = matches[:maxKnownToolFiles]
	}
	for _, path := range matches {
		b, err := readTrustedFile(path)
		if err != nil {
			warn = append(warn, path+": ignored, "+err.Error())
			continue
		}
		var f knownToolFile
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			warn = append(warn, path+": ignored, "+err.Error())
			continue
		}
		if dec.More() {
			warn = append(warn, path+": ignored, more than one JSON value")
			continue
		}
		for _, t := range f.Tools {
			if len(tools) >= maxKnownToolEntries {
				warn = append(warn, path+": too many entries; the rest are ignored")
				break
			}
			if err := validKnownTool(t); err != nil {
				warn = append(warn, path+": entry skipped, "+err.Error())
				continue
			}
			tools = append(tools, t)
		}
	}
	return tools, warn
}

// readTrustedFile opens path without following links and checks what was
// actually opened (so it cannot be swapped between the check and the read):
// a regular file, trusted by trustedOwner, no larger than maxKnownToolBytes.
func readTrustedFile(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, errors.New("it is a link")
		}
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if err := trustedOwner(fi); err != nil {
		return nil, err
	}
	if fi.Size() > maxKnownToolBytes {
		return nil, fmt.Errorf("larger than %d KB", maxKnownToolBytes>>10)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxKnownToolBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxKnownToolBytes {
		return nil, errors.New("file grew while being read")
	}
	return b, nil
}

// trustedPath checks dir and every folder above it: each must be trusted
// by trustedOwner, except that a sticky folder owned by root (/tmp) may be
// writable by others, since they cannot replace what is inside it.
func trustedPath(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for p := abs; ; p = filepath.Dir(p) {
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a link", p)
		}
		if err := trustedOwner(fi); err != nil {
			st, _ := fi.Sys().(*syscall.Stat_t)
			sticky := fi.Mode()&os.ModeSticky != 0 && st != nil && st.Uid == 0 && p != abs
			if !sticky {
				return fmt.Errorf("%s: %v", p, err)
			}
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}

var (
	knownOnce     sync.Once
	knownRegistry map[string]KnownTool
	knownWarnings []string
)

// loadKnownTools builds the registry: built-in entries, then the system
// folder, then the user's, each replacing earlier entries of the same name.
func loadKnownTools() {
	knownRegistry = map[string]KnownTool{}
	for _, t := range builtinKnownTools {
		knownRegistry[t.Name] = t
	}
	for _, dir := range knownToolDirs() {
		tools, warn := loadKnownToolFiles(dir)
		knownWarnings = append(knownWarnings, warn...)
		for _, t := range tools {
			knownRegistry[t.Name] = t
		}
	}
}

// KnownToolIO returns what is known about a tool's inputs and outputs.
func KnownToolIO(name string) (KnownTool, bool) {
	knownOnce.Do(loadKnownTools)
	t, ok := knownRegistry[toolBaseName(name)]
	return t, ok
}

// KnownToolWarnings lists problems with the JSON files, to show the user.
func KnownToolWarnings() []string {
	knownOnce.Do(loadKnownTools)
	return append([]string(nil), knownWarnings...)
}

// resetKnownTools forgets the registry (tests).
func resetKnownTools() {
	knownOnce = sync.Once{}
	knownRegistry, knownWarnings = nil, nil
}

// option returns the entry for flag.
func (t KnownTool) option(flag string) (KnownOption, bool) {
	for _, o := range t.Options {
		if o.Flag == flag {
			return o, true
		}
	}
	return KnownOption{}, false
}

// mainOutput is the tool's main output option, if listed.
func (t KnownTool) mainOutput() (KnownOption, bool) {
	for _, o := range t.Options {
		if o.Main {
			return o, true
		}
	}
	return KnownOption{}, false
}

// describe is a one-line summary of the entry, for --explain and reports.
func (t KnownTool) describe() string {
	var parts []string
	for _, o := range t.Options {
		s := o.Flag + "=" + o.Role
		if o.Ext != "" {
			s += "(." + o.Ext + ")"
		}
		if o.Main {
			s += "*"
		}
		parts = append(parts, s)
	}
	if t.Stdout != "" {
		parts = append(parts, "stdout="+t.Stdout)
	}
	if t.OutputPositional {
		parts = append(parts, "last argument is the output")
	}
	return strings.Join(parts, ", ")
}
