package appkit

import (
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Mint never asks for or handles a sudo password. A tool that needs root is
// run by starting the whole app with sudo, so sudo's own prompt asks before
// Mint starts. These helpers tell the user how, and keep the files the app
// writes owned by the user who ran sudo.

// Seams for tests.
var (
	euid          = os.Geteuid
	envLookup     = os.Getenv
	lookupHomeDir = func(name string) (string, error) {
		u, err := user.Lookup(name)
		if err != nil {
			return "", err
		}
		return u.HomeDir, nil
	}
	chown = os.Lchown
)

// runningAsRoot reports whether the app itself runs as root.
func runningAsRoot() bool { return euid() == 0 }

// invoker is the user who started the app with sudo.
type invoker struct {
	uid, gid int
	home     string
}

// sudoInvoker returns the user behind `sudo app`, or nil when the app was
// not started through sudo.
func sudoInvoker() *invoker {
	if !runningAsRoot() {
		return nil
	}
	uid, err1 := strconv.Atoi(envLookup("SUDO_UID"))
	gid, err2 := strconv.Atoi(envLookup("SUDO_GID"))
	name := envLookup("SUDO_USER")
	if err1 != nil || err2 != nil || uid == 0 || name == "" {
		return nil
	}
	home, err := lookupHomeDir(name)
	if err != nil || home == "" {
		return nil
	}
	return &invoker{uid: uid, gid: gid, home: home}
}

// giveBack makes paths, and any folders above them inside the user's home,
// belong to the user who started the app with sudo. It does nothing when
// the app was not started through sudo.
func (a *ToolApp) giveBack(paths ...string) {
	if a.owner == nil {
		return
	}
	home := filepath.Clean(a.owner.home)
	for _, p := range paths {
		if p == "" {
			continue
		}
		for dir := filepath.Clean(p); strings.HasPrefix(dir, home+string(filepath.Separator)); dir = filepath.Dir(dir) {
			_ = chown(dir, a.owner.uid, a.owner.gid)
		}
	}
}

// restartHint is the command that starts this app with root rights.
func restartHint() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "sudo <this app>"
	}
	return "sudo " + exe
}

// needsRootMessage explains, in one line, how to get root for tool.
func needsRootMessage(tool string) string {
	return tool + " needs root for this. Quit and start the app with: " + restartHint()
}

// permissionDenied recognises the usual ways programs say they need root.
var permissionDenied = regexp.MustCompile(`(?i)permission denied|operation not permitted|must be (run as )?root|requires root|need(s)? (to be )?root|are you root|superuser|insufficient privileges|access denied|not permitted`)

// looksLikeNeedsRoot reports whether a failed run's error output suggests
// it lacked root rights.
func looksLikeNeedsRoot(lastErr string) bool {
	return permissionDenied.MatchString(lastErr)
}
