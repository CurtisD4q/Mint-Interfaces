// Package theme holds the colour roles for the terminal UI.
//
// Derived from Terminal Settings Manager (Curtis, Apache-2.0):
// https://github.com/CurtisD4q/Terminal-Settings-Manager
//
// Foreground colours are painted on the terminal's own background.
//
//	default     built-in purple (256-colour)
//	terminal    the terminal's ANSI palette
//	nord / gruvbox / dracula / catppuccin
//	custom      RGB from ~/.config/mint-interfaces/theme.conf
package theme

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// Palette is a complete set of colour roles, each a ready-made SGR sequence.
type Palette struct {
	Accent, Bright, Text, Muted, On, Amber, SelBg, Bold, Dim string
	// Err colours error badges and toasts. It is optional: when empty,
	// Amber is used.
	Err string
}

// Complete reports whether every required role is set (Err is optional).
func (p Palette) Complete() bool {
	for _, s := range []string{p.Accent, p.Bright, p.Text, p.Muted, p.On, p.Amber, p.SelBg, p.Bold, p.Dim} {
		if s == "" {
			return false
		}
	}
	return true
}

func sgr(codes ...int) string {
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = strconv.Itoa(c)
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

var (
	bold = sgr(1)
	dim  = sgr(2)
)

func idxFg(n int) string { return sgr(38, 5, n) }
func idxBg(n int) string { return sgr(48, 5, n) }

// hasTrueColor reports whether the terminal environment explicitly advertises
// 24-bit colour via $COLORTERM ("truecolor" or "24bit").
func hasTrueColor() bool {
	ct := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	return ct == "truecolor" || ct == "24bit"
}

var cubeLevels = [6]int{0, 95, 135, 175, 215, 255}

// rgbTo256 returns the nearest xterm 256-colour index (16..255) for an RGB
// triplet using Euclidean distance across the 6x6x6 colour cube and 24-step
// greyscale ramp.
func rgbTo256(r, g, b int) int {
	bestIdx := 16
	bestDist := 1 << 30
	for ri, rv := range cubeLevels {
		dr := r - rv
		for gi, gv := range cubeLevels {
			dg := g - gv
			for bi, bv := range cubeLevels {
				db := b - bv
				d := dr*dr + dg*dg + db*db
				if d < bestDist {
					bestDist = d
					bestIdx = 16 + 36*ri + 6*gi + bi
				}
			}
		}
	}
	for i := 0; i < 24; i++ {
		gv := 8 + 10*i
		dr, dg, db := r-gv, g-gv, b-gv
		d := dr*dr + dg*dg + db*db
		if d < bestDist {
			bestDist = d
			bestIdx = 232 + i
		}
	}
	return bestIdx
}

func rgbFg(c [3]int) string {
	if !hasTrueColor() {
		return idxFg(rgbTo256(c[0], c[1], c[2]))
	}
	return sgr(38, 2, c[0], c[1], c[2])
}

func rgbBg(c [3]int) string {
	if !hasTrueColor() {
		return idxBg(rgbTo256(c[0], c[1], c[2]))
	}
	return sgr(48, 2, c[0], c[1], c[2])
}
func themeDefault() Palette {
	return Palette{
		Accent: idxFg(141),
		Bright: idxFg(183),
		Text:   idxFg(253),
		Muted:  idxFg(245),
		On:     idxFg(114),
		Amber:  idxFg(214),
		SelBg:  idxBg(237),
		Bold:   bold,
		Dim:    dim,
		Err:    idxFg(203),
	}
}

func themeTerminal() Palette {
	return Palette{
		Accent: sgr(95),
		Bright: sgr(97),
		Text:   sgr(37),
		Muted:  sgr(90),
		On:     sgr(92),
		Amber:  sgr(93),
		SelBg:  sgr(100),
		Bold:   bold,
		Dim:    dim,
		Err:    sgr(91),
	}
}

type rgbSpec map[string][3]int

func fromRGB(s rgbSpec) Palette {
	return Palette{
		Accent: rgbFg(s["accent"]),
		Bright: rgbFg(s["bright"]),
		Text:   rgbFg(s["text"]),
		Muted:  rgbFg(s["muted"]),
		On:     rgbFg(s["on"]),
		Amber:  rgbFg(s["amber"]),
		SelBg:  rgbBg(s["selbg"]),
		Bold:   bold,
		Dim:    dim,
		Err:    rgbFg(s["err"]),
	}
}

var presets = map[string]rgbSpec{
	"nord": {
		"accent": {136, 192, 208},
		"bright": {143, 188, 187},
		"text":   {216, 222, 233},
		"muted":  {118, 128, 144},
		"on":     {163, 190, 140},
		"amber":  {235, 203, 139},
		"selbg":  {59, 66, 82},
		"err":    {191, 97, 106},
	},
	"gruvbox": {
		"accent": {250, 189, 47},
		"bright": {254, 128, 25},
		"text":   {235, 219, 178},
		"muted":  {168, 153, 132},
		"on":     {184, 187, 38},
		"amber":  {250, 189, 47},
		"selbg":  {60, 56, 54},
		"err":    {251, 73, 52},
	},
	"dracula": {
		"accent": {189, 147, 249},
		"bright": {255, 121, 198},
		"text":   {248, 248, 242},
		"muted":  {98, 114, 164},
		"on":     {80, 250, 123},
		"amber":  {241, 250, 140},
		"selbg":  {68, 71, 90},
		"err":    {255, 85, 85},
	},
	"catppuccin": {
		"accent": {203, 166, 247},
		"bright": {245, 194, 231},
		"text":   {205, 214, 244},
		"muted":  {127, 132, 156},
		"on":     {166, 227, 161},
		"amber":  {249, 226, 175},
		"selbg":  {49, 50, 68},
		"err":    {243, 139, 168},
	},
}

// ConfigPath is where the selected theme (and custom RGB roles) are stored.
var ConfigPath = func() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = "."
		}
		base = home + "/.config"
	}
	return base + "/mint-interfaces/theme.conf"
}()

// defaultConfigPath is ConfigPath as it starts; a caller (or a test) that
// sets ConfigPath itself is always read from there.
var defaultConfigPath = ConfigPath

// ReadPath is the file the theme is read from. It is ConfigPath, except
// when Mint runs as root through sudo and root has no theme file of its
// own: then it is the theme file of the user who ran sudo, so a sudo run
// keeps that user's colours. That file is only read, never written (Set
// writes ConfigPath), and only when it is a regular file, not a link,
// owned by that user or by root.
func ReadPath() string {
	if ConfigPath != defaultConfigPath {
		return ConfigPath
	}
	if _, err := os.Stat(ConfigPath); err == nil {
		return ConfigPath
	}
	if p := sudoUserConfigPath(); p != "" {
		return p
	}
	return ConfigPath
}

// lookupUser finds a user's home; tests replace it.
var lookupUser = user.Lookup

// sudoUserConfigPath is the theme file of the user who ran sudo, or "".
func sudoUserConfigPath() string {
	if os.Geteuid() != 0 {
		return ""
	}
	name := os.Getenv("SUDO_USER")
	if name == "" || name == "root" {
		return ""
	}
	u, err := lookupUser(name)
	if err != nil || u.HomeDir == "" || !filepath.IsAbs(u.HomeDir) {
		return ""
	}
	p := filepath.Join(u.HomeDir, ".config", "mint-interfaces", "theme.conf")
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		uid, err := strconv.ParseUint(u.Uid, 10, 32)
		if err != nil || (uint64(st.Uid) != uid && st.Uid != 0) {
			return ""
		}
	}
	return p
}

// Names lists the selectable themes.
var Names = []string{"default", "terminal", "nord", "gruvbox", "dracula", "catppuccin", "custom"}

const sp = `[ \t\n\v\f\r]`

var (
	// "accent = 1 2 3"; any case, = or :.
	roleLine = regexp.MustCompile(`^` + sp + `*([A-Za-z]+)` + sp + `*[=:]` + sp + `*(\d+)` + sp + `+(\d+)` + sp + `+(\d+)`)
	// "theme = nord", and the ways people also write it: Theme = Nord,
	// theme: nord, theme = "nord". The name is read up to the first
	// character that cannot be in one.
	themeLine = regexp.MustCompile(`(?i)^` + sp + `*theme` + sp + `*[=:]` + sp + `*["']?` + sp + `*([A-Za-z0-9_-]*)`)
	// A line that is only a theme's name ("nord").
	bareLine = regexp.MustCompile(`^` + sp + `*["']?([A-Za-z]+)["']?` + sp + `*$`)
)

func readLines(path string) ([]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	var out []string
	// A theme file is a few lines; read no more than 64 KiB of it.
	sc := bufio.NewScanner(io.LimitReader(f, 64<<10))
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, true
}

func clamp255(s string) int {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(v)
}

func themeCustom() (Palette, bool) {
	lines, ok := readLines(ReadPath())
	if !ok {
		return Palette{}, false
	}
	spec := rgbSpec{}
	for _, line := range lines {
		if m := roleLine.FindStringSubmatch(line); m != nil {
			spec[strings.ToLower(m[1])] = [3]int{clamp255(m[2]), clamp255(m[3]), clamp255(m[4])}
		}
	}
	fallback := rgbSpec{
		"accent": {141, 120, 210},
		"bright": {183, 160, 230},
		"text":   {220, 220, 220},
		"muted":  {150, 150, 150},
		"on":     {120, 200, 140},
		"amber":  {220, 180, 90},
		"selbg":  {60, 60, 70},
		"err":    {220, 90, 90},
	}
	for role, rgb := range fallback {
		if _, ok := spec[role]; !ok {
			spec[role] = rgb
		}
	}
	return fromRGB(spec), true
}

// readSelected is the theme the file names (lower case, the last one if
// several), or "" when it names none. A line that is only a theme's name
// counts when no theme = line does.
func readSelected() string {
	lines, ok := readLines(ReadPath())
	if !ok {
		return ""
	}
	name, bare := "", ""
	for _, line := range lines {
		if m := themeLine.FindStringSubmatch(line); m != nil {
			name = strings.ToLower(m[1])
		} else if m := bareLine.FindStringSubmatch(line); m != nil && known(strings.ToLower(m[1])) {
			bare = strings.ToLower(m[1])
		}
	}
	if name == "" {
		name = bare
	}
	return name
}

func readSelectedName() string {
	if name := readSelected(); known(name) {
		return name
	}
	return "default"
}

func known(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}

// Problem says what is wrong with the theme ConfigPath names, or "" when
// it names a theme (or nothing, which is the default).
func Problem() string {
	name := readSelected()
	if name == "" || known(name) {
		return ""
	}
	return fmt.Sprintf("%s names the theme %q, which Mint does not have, so it uses default. Themes: %s.",
		ReadPath(), name, strings.Join(Names, ", "))
}

// Load returns the palette for a theme name. An empty name means the theme
// selected in ConfigPath. Unknown names fall back to the default purple.
func Load(name string) Palette {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = readSelectedName()
	}
	switch name {
	case "default":
		return themeDefault()
	case "terminal":
		return themeTerminal()
	case "custom":
		if p, ok := themeCustom(); ok {
			return p
		}
		return themeDefault()
	}
	if spec, ok := presets[name]; ok {
		return fromRGB(spec)
	}
	return themeDefault()
}

// HasFile reports whether a theme file is there to read (the one ReadPath
// names). A built app's own default theme only applies when there is not.
func HasFile() bool {
	_, ok := readLines(ReadPath())
	return ok
}

// Current returns the theme name selected in ConfigPath.
func Current() string { return readSelectedName() }

// Set writes the selected theme to ConfigPath, keeping any custom RGB lines.
func Set(name string) error {
	var roles []string
	if lines, ok := readLines(ReadPath()); ok {
		for _, l := range lines {
			if roleLine.MatchString(l) {
				roles = append(roles, l)
			}
		}
	}
	_ = os.MkdirAll(filepath.Dir(ConfigPath), 0o755)
	var b strings.Builder
	b.WriteString("# mint-interfaces theme\n")
	b.WriteString("theme = " + name + "\n")
	if len(roles) > 0 {
		b.WriteString("\n# custom RGB (used when theme = custom); role = R G B (0-255)\n")
		for _, l := range roles {
			b.WriteString(l + "\n")
		}
	} else if name == "custom" {
		b.WriteString("\n# custom RGB (theme = custom); role = R G B (0-255)\n")
		b.WriteString("accent = 124 58 237\n")
		b.WriteString("bright = 167 139 250\n")
		b.WriteString("text = 229 229 229\n")
		b.WriteString("muted = 150 150 150\n")
		b.WriteString("on = 52 211 153\n")
		b.WriteString("amber = 251 191 36\n")
		b.WriteString("selbg = 55 48 107\n")
	}
	return os.WriteFile(ConfigPath, []byte(b.String()), 0o644)
}

// FromSpec overrides roles of base from a spec such as
// "accent=#af87ff,text=253,selbg=#3a3a3a". Each value is a #rrggbb colour
// (24-bit) or a 256-colour index. Roles: accent, bright, text, muted, on,
// amber, selbg, err. Unknown roles and bad values are reported, and base
// is returned unchanged.
func FromSpec(base Palette, spec string) (Palette, error) {
	p := base
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		role, val, ok := strings.Cut(part, "=")
		if !ok {
			return base, fmt.Errorf("theme: %q is not role=colour", part)
		}
		role, val = strings.ToLower(strings.TrimSpace(role)), strings.TrimSpace(val)
		fg, bg, err := colourSGR(val)
		if err != nil {
			return base, fmt.Errorf("theme: %s: %v", role, err)
		}
		switch role {
		case "accent":
			p.Accent = fg
		case "bright":
			p.Bright = fg
		case "text":
			p.Text = fg
		case "muted":
			p.Muted = fg
		case "on":
			p.On = fg
		case "amber":
			p.Amber = fg
		case "err":
			p.Err = fg
		case "selbg":
			p.SelBg = bg
		default:
			return base, fmt.Errorf("theme: unknown role %q", role)
		}
	}
	return p, nil
}

// colourSGR turns "#rrggbb" or "0".."255" into foreground and background
// SGR sequences.
func colourSGR(v string) (fg, bg string, err error) {
	if strings.HasPrefix(v, "#") && len(v) == 7 {
		n, perr := strconv.ParseUint(v[1:], 16, 32)
		if perr != nil {
			return "", "", fmt.Errorf("bad colour %q", v)
		}
		c := [3]int{int(n >> 16), int(n >> 8 & 0xff), int(n & 0xff)}
		return rgbFg(c), rgbBg(c), nil
	}
	if i, perr := strconv.Atoi(v); perr == nil && i >= 0 && i <= 255 {
		return idxFg(i), idxBg(i), nil
	}
	return "", "", fmt.Errorf("bad colour %q (use #rrggbb or 0-255)", v)
}
