package appkit

import (
	"os"
	"path/filepath"
	"strings"
)

// Where an app keeps its own data.
//
// Run history, presets, the command transcript, the last findings and rule
// backups live in ~/.local/share/mint-interfaces-apps/<tool>/. Putting
// every app under one folder keeps ~/.local/share tidy, and the folder
// name cannot be mistaken for the Mint-Interfaces repository in
// ~/.local/share/mint-interfaces (the user install).
//
// Before this, each app used ~/.local/share/mint-<tool>/. The first run of
// an app moves the files it knows about from there; see migrateOldData.

// dataParent is the folder, in ~/.local/share, that holds every app's data.
const dataParent = "mint-interfaces-apps"

// dataFiles are the names an app keeps in its data folder. Only these are
// ever moved out of an old folder.
var dataFiles = []string{"runs.json", "presets.json", "commands.json", "findings-last.json", "rule-backups"}

// dataFolderName is tool as a single folder name: a name with a path
// separator in it cannot reach outside dataParent.
func dataFolderName(tool string) string {
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, tool)
	if name == "" || name == "." || name == ".." {
		return "_"
	}
	return name
}

// dataDirFor is where tool keeps its data for the user whose home is home.
func dataDirFor(home, tool string) string {
	return filepath.Join(home, ".local", "share", dataParent, dataFolderName(tool))
}

// oldDataDirFor is where earlier versions kept it.
func oldDataDirFor(home, tool string) string {
	return filepath.Join(home, ".local", "share", "mint-"+tool)
}

// Where an app keeps its settings: ~/.config/mint-interfaces/<tool>/, in
// the folder that also holds the theme and terminal cache every app
// shares. Earlier versions used ~/.config/mint-<tool>/.

// configFiles are the names an app keeps in its settings folder.
var configFiles = []string{"settings.json"}

// sharedConfigNames are what the shared folder ~/.config/mint-interfaces
// holds itself (the theme, the terminal cache, the security settings, the
// tool facts mint-gen reads). A tool is never given a folder of one of
// these names: its settings could be mistaken for one of them, or be read
// as tool facts.
var sharedConfigNames = map[string]bool{
	"theme.conf": true, "termcaps.json": true, "security.json": true, "tools.d": true,
}

// configFolderName is tool as a single folder name inside the shared
// folder.
func configFolderName(tool string) string {
	name := dataFolderName(tool)
	if sharedConfigNames[name] {
		return "app-" + name
	}
	return name
}

// configDirFor is where tool keeps its settings for the user whose home
// is home.
func configDirFor(home, tool string) string {
	return filepath.Join(home, ".config", "mint-interfaces", configFolderName(tool))
}

// oldConfigDirFor is where earlier versions kept them.
func oldConfigDirFor(home, tool string) string {
	return filepath.Join(home, ".config", "mint-"+tool)
}

// migrateOldData moves the data files of earlier versions from old into
// a.DataDir, once; see migrateFiles.
func (a *ToolApp) migrateOldData(old string) { a.migrateFiles(old, a.DataDir, dataFiles) }

// migrateOldConfig moves the settings of earlier versions from old into
// a.ConfigDir, once; see migrateFiles.
func (a *ToolApp) migrateOldConfig(old string) { a.migrateFiles(old, a.ConfigDir, configFiles) }

// migrateFiles moves names from old into dir. Nothing is moved or deleted
// unless it is one of names, is a real file or folder (never a link), and
// the same name is not already in dir; so the repository folder
// ~/.local/share/mint-interfaces, and the shared settings folder
// ~/.config/mint-interfaces, are never touched, even for a tool named
// "interfaces". The old folder is removed only if that leaves it empty.
// Files keep their permissions. Under sudo the new folders are handed back
// to the user.
func (a *ToolApp) migrateFiles(old, dir string, names []string) {
	if dir == "" || old == "" || filepath.Clean(old) == filepath.Clean(dir) {
		return
	}
	fi, err := os.Lstat(old)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return
	}
	moved := false
	for _, name := range names {
		src := filepath.Join(old, name)
		sfi, err := os.Lstat(src)
		if err != nil || sfi.Mode()&os.ModeSymlink != 0 || !(sfi.Mode().IsRegular() || sfi.IsDir()) {
			continue
		}
		dst := filepath.Join(dir, name)
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		if !moved {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return
			}
		}
		if os.Rename(src, dst) == nil {
			moved = true
		}
	}
	if moved {
		a.giveBack(dir)
	}
	_ = os.Remove(old) // only if it is empty now
}
