package appkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/secure"
)

// How sensitive a tool's results are (ToolSpec.Data).
const (
	DataNormal    = ""          // nothing special
	DataSensitive = "sensitive" // may hold private information: kept readable only by you
	DataSecret    = "secret"    // holds keys, passwords or decrypted data: also kept in memory by default
)

func dataRank(level string) int {
	switch level {
	case DataSecret:
		return 2
	case DataSensitive:
		return 1
	}
	return 0
}

// higherData returns the more sensitive of two levels.
func higherData(a, b string) string {
	if dataRank(b) > dataRank(a) {
		return b
	}
	return a
}

type dataClass struct{ level, reason string }

// Templates whose tools usually handle private data.
var templateData = map[string]dataClass{
	"T13": {DataSensitive, "captures network traffic, which can include passwords and private messages"},
	"T14": {DataSensitive, "gathers information about people and organisations"},
	"T15": {DataSensitive, "its findings describe weaknesses of this computer or a target"},
	"T16": {DataSecret, "works with keys, passwords and decrypted data"},
	"T20": {DataSensitive, "logs can include user names, addresses and sometimes passwords"},
}

// Tools classified on their own, whatever their template.
var toolData = map[string]dataClass{
	// Password managers and secret stores print secrets.
	"pass": {DataSecret, "prints stored passwords"}, "gopass": {DataSecret, "prints stored passwords"},
	"bw": {DataSecret, "prints stored passwords"}, "op": {DataSecret, "prints stored passwords"},
	"keepassxc-cli": {DataSecret, "prints stored passwords"}, "vault": {DataSecret, "reads stored secrets"},
	"secret-tool": {DataSecret, "prints stored passwords"},
	// Key and certificate tools.
	"gpg": {DataSecret, "works with private keys and decrypted data"}, "gpg2": {DataSecret, "works with private keys and decrypted data"},
	"age": {DataSecret, "works with private keys and decrypted data"}, "age-keygen": {DataSecret, "creates private keys"},
	"ssh-keygen": {DataSecret, "creates private keys"}, "wg": {DataSecret, "creates and shows private keys"},
	"openssl": {DataSecret, "works with private keys and decrypted data"},
	"john":    {DataSecret, "shows cracked passwords"}, "hashcat": {DataSecret, "shows cracked passwords"},
	// Captures, dumps and tracers.
	"tcpdump": {DataSensitive, templateData["T13"].reason}, "tshark": {DataSensitive, templateData["T13"].reason},
	"dumpcap": {DataSensitive, templateData["T13"].reason}, "ngrep": {DataSensitive, templateData["T13"].reason},
	"termshark": {DataSensitive, templateData["T13"].reason},
	"strace":    {DataSensitive, "shows what a program reads and writes, including passwords"},
	"ltrace":    {DataSensitive, "shows what a program reads and writes, including passwords"},
	"pg_dump":   {DataSensitive, "copies a whole database"}, "pg_dumpall": {DataSensitive, "copies whole databases"},
	"mysqldump": {DataSensitive, "copies a whole database"}, "mariadb-dump": {DataSensitive, "copies a whole database"},
	"mongodump": {DataSensitive, "copies a whole database"}, "sqlite3": {DataSensitive, "reads database contents"},
	"printenv": {DataSensitive, "prints environment variables, which can hold tokens"},
	"env":      {DataSensitive, "prints environment variables, which can hold tokens"},
	"sqlmap":   {DataSensitive, "can extract data from the databases it tests"},
}

// Tools in a sensitive template whose results are not sensitive.
var normalDataTools = map[string]bool{
	"ss": true, "netstat": true, "md5sum": true, "sha1sum": true, "sha224sum": true, "sha256sum": true,
	"sha384sum": true, "sha512sum": true, "b2sum": true, "cksum": true,
}

// ClassifyData says how sensitive a tool's results are, and why: from the
// tool itself, else its template.
func ClassifyData(spec ToolSpec) (level, reason string) {
	base := toolBase(spec.BinaryName())
	if c, ok := toolData[base]; ok {
		return c.level, c.reason
	}
	if normalDataTools[base] {
		return DataNormal, ""
	}
	// The program's old template (named in the spec, or from the old lists)
	// keeps the level it always had, so moving to archetypes never makes a
	// program's results less protected.
	id := spec.Template
	if id == "" {
		if tpl, ok := TemplateForTool(base); ok {
			id = string(tpl.ID)
		}
	}
	if tpl, ok := LookupTemplate(id); ok {
		id = string(tpl.ID)
	}
	if c, ok := templateData[id]; ok {
		return c.level, c.reason
	}
	if a, ok := LookupArchetype(spec.ArchetypeID); ok && a.Data != "" {
		return a.Data, a.DataReason
	}
	return DataNormal, ""
}

// revealWords mark an option that makes a tool print secrets.
var revealWords = []string{"export-secret", "export secret", "private key", "private-key", "privkey",
	"show-secret", "show secret", "reveal", "plaintext", "show-password", "show password",
	"genkey", "keylog", "unmask"}

// LooksRevealing reports whether an option makes the tool print secrets
// (gpg --export-secret-keys, --decrypt, --show-password…).
func LooksRevealing(flag, label, hint string) bool {
	text := strings.ToLower(flag + " " + label + " " + hint)
	// "decrypt" only when the option is about decrypting (gpg -d "Decrypt
	// data"), not one that merely mentions it (-u "key to sign or decrypt").
	if strings.Contains(strings.ToLower(flag), "decrypt") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), "decrypt") {
		return true
	}
	for _, w := range revealWords {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func applyDataDefaults(spec *ToolSpec) {
	if spec.Data == "" {
		spec.Data, spec.DataReason = ClassifyData(*spec)
	}
	mark := func(flags []FlagSpec) {
		for i := range flags {
			f := &flags[i]
			if f.Kind != FlagSecret && !f.Reveals && LooksRevealing(f.Flag, f.Label, f.Hint) {
				f.Reveals = true
			}
		}
	}
	mark(spec.Flags)
	for i := range spec.Subcommands {
		mark(spec.Subcommands[i].Flags)
	}
}

// revealingOn lists the labels of revealing options set for the next run.
func (a *ToolApp) revealingOn() []string {
	var out []string
	for _, f := range a.Spec.Flags {
		if !f.Reveals {
			continue
		}
		v := strings.TrimSpace(a.Values[f.Flag])
		on := v != "" && v != "false"
		if f.Kind == FlagBool {
			on = v == "true" || v == "1" || v == "on" || v == "yes"
		}
		if on {
			out = append(out, f.Flag)
		}
	}
	return out
}

// DataLevel is how sensitive the next run's results are, and why: the
// tool's level, raised to secret by revealing options that are set.
func (a *ToolApp) DataLevel() (level, reason string) {
	level, reason = a.Spec.Data, a.Spec.DataReason
	if on := a.revealingOn(); len(on) > 0 && level != DataSecret {
		level, reason = DataSecret, strings.Join(on, ", ")+" makes it print secrets"
	}
	a.mu.Lock()
	last, found := a.lastData, a.lastFindings
	a.mu.Unlock()
	if dataRank(last) > dataRank(level) {
		level, reason = last, "the last run's output contained "+strings.Join(found, ", ")
	}
	return level, reason
}

func dataBadge(level string) (mint.Badge, bool) {
	switch level {
	case DataSecret:
		return mint.Badge{Text: "secret data", Kind: mint.BadgeWarn}, true
	case DataSensitive:
		return mint.Badge{Text: "sensitive data", Kind: mint.BadgeWarn}, true
	}
	return mint.Badge{}, false
}

// ── Where results are kept ─────────────────────────────────────────────────

// privateDir is this app's private, in-memory results folder.
func (a *ToolApp) privateDir() (string, bool, error) {
	uid, gid := os.Getuid(), os.Getgid()
	if a.owner != nil {
		uid, gid = a.owner.uid, a.owner.gid
	}
	name := strings.NewReplacer("/", "-", "\\", "-").Replace(a.Spec.Name)
	if name == "" || name == "." || name == ".." {
		name = "app"
	}
	return secure.PrivateDir(name, uid, gid)
}

// SetInMemory switches results between the private in-memory folder
// (cleared at logout) and the usual results folder.
func (a *ToolApp) SetInMemory(on bool) error {
	if !on {
		if a.InMemory && a.diskDir != "" {
			a.ResultsDir = a.diskDir
		}
		a.InMemory = false
		return nil
	}
	dir, inMem, err := a.privateDir()
	if err != nil {
		return err
	}
	if !a.InMemory {
		a.diskDir = a.ResultsDir
	}
	a.ResultsDir = dir
	a.InMemory = true
	a.memBacked = inMem
	return nil
}

// makeResultsDir creates dir. For sensitive results, folders it creates are
// private (0700); existing folders are left as they are.
func (a *ToolApp) makeResultsDir(dir string) error {
	level, _ := a.DataLevel()
	if level == DataNormal {
		return mkdirAll(dir, 0o755)
	}
	return mkdirAll(dir, 0o700)
}

// mkdirAll is os.MkdirAll that sets mode on every folder it creates
// (despite the umask), and leaves existing ones alone.
func mkdirAll(dir string, mode os.FileMode) error {
	dir = filepath.Clean(dir)
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s is not a folder", dir)
		}
		return nil
	}
	if parent := filepath.Dir(dir); parent != dir {
		if err := mkdirAll(parent, mode); err != nil {
			return err
		}
	}
	if err := os.Mkdir(dir, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.Chmod(dir, mode)
}

// protectFile makes a results file readable only by its owner when the
// results are sensitive.
func (a *ToolApp) protectFile(level, path string) {
	if path == "" || level == DataNormal {
		return
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		_ = os.Chmod(path, 0o600)
	}
}

// ── Encrypting results ─────────────────────────────────────────────────────

const minPassphrase = 8

// askNewPassphrase asks for a passphrase twice.
func askNewPassphrase(c *mint.Ctx, what string) ([]byte, bool) {
	for {
		p1, ok := c.Password("Encrypt "+what, fmt.Sprintf("Passphrase (at least %d characters). Without it the file cannot be opened.", minPassphrase))
		if !ok {
			return nil, false
		}
		if len(p1) < minPassphrase {
			c.ToastWarn(fmt.Sprintf("Use at least %d characters", minPassphrase))
			continue
		}
		p2, ok := c.Password("Encrypt "+what, "Type the passphrase again")
		if !ok {
			return nil, false
		}
		if p1 != p2 {
			c.ToastWarn("The passphrases are different; try again")
			continue
		}
		return []byte(p1), true
	}
}

// encryptOutput encrypts the run's output file in place (path.mintseal,
// original removed), or, when there is none, saves the captured output
// encrypted.
func (a *ToolApp) encryptOutput(c *mint.Ctx) {
	a.mu.Lock()
	out, job := a.lastOut, a.activeJob
	a.mu.Unlock()
	if job != nil && job.Running() {
		c.ToastWarn("Wait for the run to finish")
		return
	}
	if out != "" && !strings.HasSuffix(out, secure.SealedExt) {
		if _, err := os.Stat(out); err == nil {
			pass, ok := askNewPassphrase(c, filepath.Base(out))
			if !ok {
				return
			}
			var dest string
			var err error
			c.Busy("Encrypting…", func() { dest, err = secure.SealFile(out, pass) })
			wipeBytes(pass)
			if err != nil {
				c.ToastError("Encryption failed: " + err.Error())
				return
			}
			a.giveBack(dest)
			a.mu.Lock()
			a.lastOut = dest
			a.mu.Unlock()
			c.Toast("Encrypted to " + dest + "; the original was removed")
			return
		}
	}
	if job == nil {
		c.ToastWarn("No output yet")
		return
	}
	dest := filepath.Join(a.ResultsDir, DefaultOutputFilename(a.Spec.Name, a.outputExt())+secure.SealedExt)
	pass, ok := askNewPassphrase(c, "the captured output")
	if !ok {
		return
	}
	var b strings.Builder
	for _, l := range job.Lines() {
		b.WriteString(mint.CleanLine(l.Text))
		b.WriteByte('\n')
	}
	var sealed []byte
	var err error
	c.Busy("Encrypting…", func() { sealed, err = secure.SealBytes([]byte(b.String()), pass) })
	wipeBytes(pass)
	if err == nil {
		if err = a.makeResultsDir(filepath.Dir(dest)); err == nil {
			err = os.WriteFile(dest, sealed, 0o600)
		}
	}
	if err != nil {
		c.ToastError("Encryption failed: " + err.Error())
		return
	}
	a.giveBack(dest)
	c.Toast("Saved encrypted to " + dest)
}

// openSealed asks for the passphrase and shows a sealed file, decrypted
// in memory only.
func openSealed(c *mint.Ctx, name string, sealed []byte) {
	pass, ok := c.Password("Open "+name, "Passphrase")
	if !ok {
		return
	}
	var plain []byte
	var err error
	c.Busy("Decrypting…", func() { plain, err = secure.OpenSealed(sealed, []byte(pass)) })
	if err != nil {
		c.ToastError(err.Error())
		return
	}
	lines := textToLines(string(plain))
	wipeBytes(plain)
	c.Push(mint.ViewLog(name+" (decrypted)", func() []mint.LogLine { return lines }))
}

func textToLines(s string) []mint.LogLine {
	raw := strings.Split(strings.TrimRight(s, "\n"), "\n")
	lines := make([]mint.LogLine, len(raw))
	for i, l := range raw {
		lines[i] = mint.LogLine{Text: mint.CleanLine(l)}
	}
	return lines
}

func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// dataRows explains the results' level and offers the in-memory folder.
func (a *ToolApp) dataRows() []mint.Row {
	level, reason := a.DataLevel()
	var rows []mint.Row
	if level != DataNormal {
		rows = append(rows, mint.Note(fmt.Sprintf("  %s data: %s. Result files are readable only by you.", strings.ToUpper(level[:1])+level[1:], reason)))
	}
	rows = append(rows, mint.Toggle(mint.ToggleOpts{
		ID:    "out:memory",
		Label: "Keep results in memory only",
		Get:   func() bool { return a.InMemory },
		Set: func(v bool) {
			if err := a.SetInMemory(v); err != nil {
				mint.ToastError("No private in-memory folder here: " + err.Error())
			}
		},
	}))
	switch {
	case a.InMemory && !a.memBacked:
		rows = append(rows, mint.Note("  A private folder, but it may not be in memory on this system."))
	case a.InMemory && strings.HasPrefix(a.ResultsDir, "/dev/shm/"):
		rows = append(rows, mint.Note("  A private folder in memory, cleared when the computer restarts."))
	case a.InMemory:
		rows = append(rows, mint.Note("  A private folder in memory, cleared when you log out."))
	}
	return rows
}
