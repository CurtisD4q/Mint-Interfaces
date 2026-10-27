package fileplan

import (
	"strings"
)

// Tools with a dry run of their own print what they would do; these read
// that output into a Plan. Paths are as the tool prints them (relative to
// its destination for rsync) and untrusted.

// ParseRsyncItemize reads rsync --dry-run --itemize-changes output.
//
//	*deleting   path          → Delete
//	>f+++++++++ path          → Create (a new file)
//	>f.st...... path          → Overwrite (a changed file)
//	cd+++++++++ dir/          → Create (a new folder)
//	cL+++++++++ link -> to    → Create (a new link)
//	.d..t...... dir/          → attributes only: not listed
func ParseRsyncItemize(lines []string) Plan {
	p := Plan{Tool: "rsync"}
	for _, l := range lines {
		if strings.HasPrefix(l, "*deleting") {
			path := strings.TrimSpace(strings.TrimPrefix(l, "*deleting"))
			p.Effects = append(p.Effects, Effect{Kind: Delete, Path: path, IsDir: strings.HasSuffix(path, "/")})
			continue
		}
		code, path, ok := strings.Cut(l, " ")
		if !ok || len(code) < 11 || len(code) > 12 || !strings.ContainsRune("<>ch.", rune(code[0])) || !strings.ContainsRune("fdLDS", rune(code[1])) {
			continue // a message, the summary, or something else
		}
		path = strings.TrimLeft(path, " ")
		note := ""
		if code[1] == 'L' {
			if name, to, ok := strings.Cut(path, " -> "); ok {
				path, note = name, "a link to "+to
			}
		}
		e := Effect{Path: path, IsDir: code[1] == 'd', Note: note}
		attrs := code[2:]
		switch {
		case strings.Trim(attrs, "+") == "":
			e.Kind = Create
		case code[0] == '.' || code[0] == 'h':
			continue // attributes only, or a hard link to a file already there
		default:
			e.Kind = Overwrite
			e.Note = rsyncChanges(attrs)
		}
		p.Effects = append(p.Effects, e)
	}
	sortEffects(p.Effects)
	return p
}

// rsyncChanges says which of rsync's attribute letters changed.
func rsyncChanges(attrs string) string {
	names := map[byte]string{'c': "content", 's': "size", 't': "time", 'T': "time", 'p': "permissions",
		'o': "owner", 'g': "group", 'u': "use time", 'a': "ACL", 'x': "extended attributes"}
	var out []string
	for i := 0; i < len(attrs); i++ {
		if n, ok := names[attrs[i]]; ok {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return "changed: " + strings.Join(out, ", ")
}

// ParseGitClean reads git clean --dry-run output.
func ParseGitClean(lines []string) Plan {
	p := Plan{Tool: "git clean"}
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "Would remove "):
			path := strings.TrimPrefix(l, "Would remove ")
			p.Effects = append(p.Effects, Effect{Kind: Delete, Path: path, IsDir: strings.HasSuffix(path, "/")})
		case strings.HasPrefix(l, "Would skip repository "):
			path := strings.TrimPrefix(l, "Would skip repository ")
			p.Effects = append(p.Effects, Effect{Kind: Keep, Path: path, IsDir: true, Note: "a nested repository (needs -ff)"})
		}
	}
	sortEffects(p.Effects)
	return p
}
