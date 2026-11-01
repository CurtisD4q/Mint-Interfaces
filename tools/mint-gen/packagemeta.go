package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Package descriptions: what the package that installs a program says it
// is ("interactive processes viewer", "pager program similar to more").
// They are read from dpkg's own files only (Debian, Ubuntu, Mint); nothing
// is run. A package that installs many programs describes the bundle
// ("GNU core utilities"), not the program, so it gives nothing.

// packageInfo is a program's package description, if dpkg knows it.
type packageInfo struct {
	Package     string
	Summary     string // the first line of Description
	Description string // the rest of it, as one paragraph
	Programs    int    // how many programs the package installs
}

// maxPackagePrograms is how many programs a package may install and still
// describe each of them (chosen with PackageShare by cross-validation).
var maxPackagePrograms = 10

var (
	dpkgDir       = "/var/lib/dpkg"
	readPackages  = true // tests switch it off for programs named like real ones
	pkgOnce       sync.Once
	pkgOfPath     map[string]string // installed program path -> package
	pkgPrograms   map[string]int    // package -> programs it installs
	pkgDescs      map[string][2]string
	maxDpkgStatus int64 = 64 << 20
)

func loadPackages() {
	pkgOfPath, pkgPrograms, pkgDescs = map[string]string{}, map[string]int{}, map[string][2]string{}
	lists, _ := filepath.Glob(filepath.Join(dpkgDir, "info", "*.list"))
	for _, l := range lists {
		pkg := strings.TrimSuffix(filepath.Base(l), ".list")
		if i := strings.IndexByte(pkg, ':'); i >= 0 {
			pkg = pkg[:i] // "libfoo:amd64"
		}
		f, err := os.Open(l)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			p := sc.Text()
			// A program folder: /usr/bin, /sbin, /usr/games, or a tool's own
			// (/usr/lib/jvm/…/bin).
			if base := filepath.Base(filepath.Dir(p)); base == "bin" || base == "sbin" || base == "games" {
				pkgOfPath[p] = pkg
				pkgPrograms[pkg]++
			}
		}
		f.Close()
	}
	st, err := os.Stat(filepath.Join(dpkgDir, "status"))
	if err != nil || st.Size() > maxDpkgStatus {
		return
	}
	b, err := os.ReadFile(filepath.Join(dpkgDir, "status"))
	if err != nil {
		return
	}
	for _, stanza := range strings.Split(string(b), "\n\n") {
		pkg, short := "", ""
		var long []string
		inDesc := false
		for _, line := range strings.Split(stanza, "\n") {
			switch {
			case strings.HasPrefix(line, "Package: "):
				pkg = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
				inDesc = false
			case strings.HasPrefix(line, "Description: "):
				short = strings.TrimSpace(strings.TrimPrefix(line, "Description: "))
				inDesc = true
			case inDesc && strings.HasPrefix(line, " "):
				if t := strings.TrimSpace(line); t != "." {
					long = append(long, t)
				}
			default:
				inDesc = false
			}
		}
		if pkg != "" && short != "" {
			d := strings.Join(long, " ")
			if len(d) > 800 {
				d = d[:800]
			}
			pkgDescs[pkg] = [2]string{short, d}
		}
	}
}

// lookupPackage is the package description of the program called name,
// found through the program it would run.
func lookupPackage(name string) (packageInfo, bool) {
	if !readPackages || name == "" || strings.ContainsAny(name, "/\x00") {
		return packageInfo{}, false
	}
	pkgOnce.Do(loadPackages)
	path, err := exec.LookPath(name)
	if err != nil {
		return packageInfo{}, false
	}
	candidates := []string{path}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		candidates = append(candidates, real)
	}
	// dpkg records /usr/bin and /bin separately on merged-/usr systems.
	for _, c := range append([]string(nil), candidates...) {
		if strings.HasPrefix(c, "/usr/") {
			candidates = append(candidates, strings.TrimPrefix(c, "/usr"))
		} else {
			candidates = append(candidates, "/usr"+c)
		}
	}
	for _, c := range candidates {
		if pkg, ok := pkgOfPath[c]; ok {
			d, ok := pkgDescs[pkg]
			if !ok {
				return packageInfo{}, false
			}
			return packageInfo{Package: pkg, Summary: d[0], Description: d[1], Programs: pkgPrograms[pkg]}, true
		}
	}
	return packageInfo{}, false
}

// packageText is the package text the scorer reads for a program: its
// package's summary and description, unless the package installs many
// programs.
func packageText(name string) (summary, description string) {
	info, ok := lookupPackage(name)
	if !ok || info.Programs > maxPackagePrograms {
		return "", ""
	}
	return info.Summary, info.Description
}
