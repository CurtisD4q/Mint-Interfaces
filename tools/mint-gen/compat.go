package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
)

// ExclusionCategory names the reason a program cannot run inside a pure TUI.
type ExclusionCategory string

const (
	CatCompatible       ExclusionCategory = "compatible"
	CatShellBuiltin     ExclusionCategory = "shell-builtin"
	CatFullScreenTUI    ExclusionCategory = "fullscreen-tui"
	CatGraphicalGUI     ExclusionCategory = "graphical-gui"
	CatDirectTTY        ExclusionCategory = "direct-tty"
	CatTerminalGraphics ExclusionCategory = "terminal-graphics"
	CatTerminalState    ExclusionCategory = "terminal-state"
)

// CompatReport describes whether a program can run inside a pure Mint TUI.
type CompatReport struct {
	Program     string            `json:"program"`
	BinaryPath  string            `json:"binary_path,omitempty"`
	Compatible  bool              `json:"compatible"`
	Category    ExclusionCategory `json:"category"`
	Reason      string            `json:"reason"`
	Alternative string            `json:"alternative,omitempty"`
	BatchHint   string            `json:"batch_hint,omitempty"`
	LinkedLibs  []string          `json:"linked_libs,omitempty"`
}

type exclusionRule struct {
	Category    ExclusionCategory
	Reason      string
	Alternative string
	BatchHint   string
}

// 1. Shell built-ins & parent-environment mutators
var shellBuiltins = map[string]string{
	"cd":        "changes the parent shell's working directory, which cannot persist from a child process",
	"pushd":     "mutates the parent shell's directory stack",
	"popd":      "mutates the parent shell's directory stack",
	"dirs":      "reads the parent shell's internal directory stack",
	"export":    "modifies the parent shell's environment variables",
	"unset":     "removes variables or functions from the parent shell",
	"alias":     "defines aliases inside the current interactive shell session",
	"unalias":   "removes aliases from the current interactive shell session",
	"ulimit":    "modifies resource limits of the parent shell process",
	"umask":     "sets the file mode creation mask of the parent shell",
	"source":    "executes commands in the current shell environment",
	".":         "executes commands in the current shell environment",
	"eval":      "evaluates arguments inside the current shell environment",
	"exec":      "replaces the current shell process image",
	"fg":        "brings a background job into the foreground of an interactive shell",
	"bg":        "resumes a suspended job in the parent shell's job table",
	"jobs":      "lists jobs managed by the current interactive shell",
	"wait":      "waits for child processes of the current shell session",
	"disown":    "removes jobs from the parent shell's active job table",
	"read":      "reads a line into a parent shell variable",
	"shift":     "shifts positional parameters in a shell script",
	"trap":      "sets signal handlers in the current shell",
	"return":    "returns from a shell function or sourced script",
	"exit":      "exits the current shell session",
	"logout":    "exits a login shell session",
	"local":     "declares a local variable inside a shell function",
	"declare":   "declares shell variables and attributes in the parent shell",
	"typeset":   "declares shell variables and attributes in the parent shell",
	"readonly":  "marks shell variables as read-only in the parent shell",
	"shopt":     "toggles bash shell options in the current shell",
	"set":       "sets shell options and positional parameters in the parent shell",
	"hash":      "manipulates the parent shell's command path hash table",
	"bind":      "alters readline key bindings of the interactive shell",
	"builtin":   "runs a shell builtin within the current shell",
	"caller":    "returns the context of any active subroutine call in the shell",
	"compgen":   "generates shell completion matches for the interactive shell",
	"complete":  "specifies how arguments are completed by the interactive shell",
	"compopt":   "modifies completion options inside an interactive shell",
	"enable":    "enables and disables builtin shell commands",
	"fc":        "lists or edits commands from the interactive shell history",
	"getopts":   "parses positional parameters inside a shell script",
	"history":   "reads or manipulates the interactive shell's command history",
	"let":       "evaluates arithmetic expressions in shell variables",
	"mapfile":   "reads lines into a bash array variable",
	"readarray": "reads lines into a bash array variable",
	"suspend":   "suspends the execution of the current shell",
	"times":     "prints accumulated user and system times for the shell",
}

// 2–6. Curated exclusion rules across full-screen TUIs, GUIs, /dev/tty tools,
// terminal pixel renderers, and terminal state manipulators.
var curatedExclusions = map[string]exclusionRule{
	// Category 2: Full-screen TUIs / curses / pagers / editors / multiplexers
	"vim":        {Category: CatFullScreenTUI, Reason: "full-screen interactive text editor requiring a controlling TTY and alternate screen buffer"},
	"nvim":       {Category: CatFullScreenTUI, Reason: "full-screen interactive Neovim editor requiring a controlling TTY and 2D cursor addressing"},
	"vi":         {Category: CatFullScreenTUI, Reason: "full-screen visual editor requiring a controlling TTY"},
	"view":       {Category: CatFullScreenTUI, Reason: "full-screen read-only vi editor requiring a controlling TTY"},
	"vimdiff":    {Category: CatFullScreenTUI, Reason: "full-screen curses diff editor requiring a controlling TTY", Alternative: "Use diff -u"},
	"nano":       {Category: CatFullScreenTUI, Reason: "full-screen curses text editor requiring a controlling TTY"},
	"pico":       {Category: CatFullScreenTUI, Reason: "full-screen curses text editor requiring a controlling TTY"},
	"emacs":      {Category: CatFullScreenTUI, Reason: "interactive editor/environment; Mint interfaces run inside Emacs rather than wrapping Emacs"},
	"micro":      {Category: CatFullScreenTUI, Reason: "full-screen terminal text editor requiring raw TTY mode"},
	"helix":      {Category: CatFullScreenTUI, Reason: "full-screen modal terminal editor requiring raw TTY mode"},
	"hx":         {Category: CatFullScreenTUI, Reason: "full-screen modal terminal editor requiring raw TTY mode"},
	"kak":        {Category: CatFullScreenTUI, Reason: "full-screen Kakoune terminal editor requiring raw TTY mode"},
	"joe":        {Category: CatFullScreenTUI, Reason: "full-screen terminal editor requiring raw TTY mode"},
	"mcedit":     {Category: CatFullScreenTUI, Reason: "full-screen Midnight Commander editor requiring curses TTY"},
	"htop":       {Category: CatFullScreenTUI, Reason: "full-screen ncurses interactive process viewer", Alternative: "Use ps or top -b -n 1"},
	"btop":       {Category: CatFullScreenTUI, Reason: "full-screen 2D terminal system monitor requiring alternate screen buffer", Alternative: "Use ps or top -b -n 1"},
	"atop":       {Category: CatFullScreenTUI, Reason: "full-screen curses performance monitor"},
	"glances":    {Category: CatFullScreenTUI, Reason: "full-screen curses system monitoring TUI"},
	"nmon":       {Category: CatFullScreenTUI, Reason: "full-screen curses system monitor"},
	"iotop":      {Category: CatFullScreenTUI, Reason: "full-screen curses I/O monitor", BatchHint: "-b -n 1"},
	"iftop":      {Category: CatFullScreenTUI, Reason: "full-screen ncurses network bandwidth monitor"},
	"nethogs":    {Category: CatFullScreenTUI, Reason: "full-screen ncurses per-process network monitor"},
	"iptraf-ng":  {Category: CatFullScreenTUI, Reason: "full-screen ncurses IP network monitor"},
	"wavemon":    {Category: CatFullScreenTUI, Reason: "full-screen ncurses wireless monitor"},
	"nvtop":      {Category: CatFullScreenTUI, Reason: "full-screen ncurses GPU monitor", Alternative: "Use nvidia-smi"},
	"powertop":   {Category: CatFullScreenTUI, Reason: "full-screen ncurses power consumption monitor"},
	"top":        {Category: CatFullScreenTUI, Reason: "interactive full-screen process viewer unless run in batch mode (-b -n 1)", BatchHint: "-b -n 1", Alternative: "Use ps or pass -b -n 1"},
	"less":       {Category: CatFullScreenTUI, Reason: "interactive terminal pager; Mint already provides mint.ViewLog for paging, search, and scrolling"},
	"more":       {Category: CatFullScreenTUI, Reason: "interactive terminal pager; Mint already provides mint.ViewLog"},
	"most":       {Category: CatFullScreenTUI, Reason: "interactive terminal pager; Mint already provides mint.ViewLog"},
	"pg":         {Category: CatFullScreenTUI, Reason: "interactive terminal pager; Mint already provides mint.ViewLog"},
	"mc":         {Category: CatFullScreenTUI, Reason: "full-screen Midnight Commander file manager TUI"},
	"ranger":     {Category: CatFullScreenTUI, Reason: "full-screen curses file manager TUI"},
	"lf":         {Category: CatFullScreenTUI, Reason: "full-screen terminal file manager TUI"},
	"nnn":        {Category: CatFullScreenTUI, Reason: "full-screen curses file manager TUI"},
	"vifm":       {Category: CatFullScreenTUI, Reason: "full-screen curses file manager TUI"},
	"yazi":       {Category: CatFullScreenTUI, Reason: "full-screen terminal file manager TUI"},
	"broot":      {Category: CatFullScreenTUI, Reason: "interactive tree navigation TUI"},
	"tig":        {Category: CatFullScreenTUI, Reason: "full-screen ncurses git repository browser", Alternative: "Use git"},
	"lazygit":    {Category: CatFullScreenTUI, Reason: "full-screen terminal UI for git", Alternative: "Use git"},
	"lazydocker": {Category: CatFullScreenTUI, Reason: "full-screen terminal UI for docker", Alternative: "Use docker"},
	"gitui":      {Category: CatFullScreenTUI, Reason: "full-screen terminal UI for git", Alternative: "Use git"},
	"ncdu":       {Category: CatFullScreenTUI, Reason: "full-screen ncurses disk usage analyzer", Alternative: "Use du"},
	"gdu":        {Category: CatFullScreenTUI, Reason: "full-screen disk usage analyzer TUI", Alternative: "Use du"},
	"alsamixer":  {Category: CatFullScreenTUI, Reason: "full-screen ncurses ALSA mixer TUI", Alternative: "Use amixer"},
	"pulsemixer": {Category: CatFullScreenTUI, Reason: "full-screen curses PulseAudio mixer TUI", Alternative: "Use pactl"},
	"nmtui":      {Category: CatFullScreenTUI, Reason: "full-screen newt/curses NetworkManager TUI", Alternative: "Use nmcli"},
	"cfdisk":     {Category: CatFullScreenTUI, Reason: "full-screen curses partition table editor", Alternative: "Use sfdisk or lsblk"},
	"cgdisk":     {Category: CatFullScreenTUI, Reason: "full-screen curses GPT partition editor", Alternative: "Use sgdisk"},
	"ccmake":     {Category: CatFullScreenTUI, Reason: "full-screen curses CMake configuration TUI", Alternative: "Use cmake"},
	"tmux":       {Category: CatFullScreenTUI, Reason: "terminal multiplexer requiring a controlling pty and raw terminal control"},
	"screen":     {Category: CatFullScreenTUI, Reason: "terminal multiplexer requiring a controlling pty and raw terminal control"},
	"byobu":      {Category: CatFullScreenTUI, Reason: "terminal multiplexer wrapper around tmux/screen"},
	"zellij":     {Category: CatFullScreenTUI, Reason: "terminal workspace and multiplexer TUI"},
	"whiptail":   {Category: CatFullScreenTUI, Reason: "newt full-screen dialog box renderer"},
	"dialog":     {Category: CatFullScreenTUI, Reason: "ncurses dialog box renderer"},
	"lynx":       {Category: CatFullScreenTUI, Reason: "full-screen curses web browser", BatchHint: "-dump"},
	"links":      {Category: CatFullScreenTUI, Reason: "full-screen terminal web browser", BatchHint: "-dump"},
	"elinks":     {Category: CatFullScreenTUI, Reason: "full-screen terminal web browser", BatchHint: "-dump"},
	"w3m":        {Category: CatFullScreenTUI, Reason: "full-screen terminal web browser/pager", BatchHint: "-dump"},
	"mutt":       {Category: CatFullScreenTUI, Reason: "full-screen curses mail client"},
	"neomutt":    {Category: CatFullScreenTUI, Reason: "full-screen curses mail client"},
	"irssi":      {Category: CatFullScreenTUI, Reason: "full-screen curses IRC client"},
	"weechat":    {Category: CatFullScreenTUI, Reason: "full-screen curses chat client"},
	"cmus":       {Category: CatFullScreenTUI, Reason: "full-screen curses music player"},
	"ncmpcpp":    {Category: CatFullScreenTUI, Reason: "full-screen curses MPD client"},

	// Category 3: Graphical (X11 / Wayland) applications with no pure CLI mode
	"wireshark":      {Category: CatGraphicalGUI, Reason: "graphical Qt network analyzer requiring an X11/Wayland display", Alternative: "Use tshark"},
	"gimp":           {Category: CatGraphicalGUI, Reason: "graphical GTK image editor", Alternative: "Use magick / convert"},
	"inkscape":       {Category: CatGraphicalGUI, Reason: "graphical vector graphics editor"},
	"blender":        {Category: CatGraphicalGUI, Reason: "graphical 3D suite"},
	"mpv":            {Category: CatGraphicalGUI, Reason: "multimedia player requiring a video/audio display window", Alternative: "Use ffprobe or ffmpeg"},
	"vlc":            {Category: CatGraphicalGUI, Reason: "graphical media player", Alternative: "Use ffmpeg"},
	"feh":            {Category: CatGraphicalGUI, Reason: "X11 image viewer requiring an X display"},
	"imv":            {Category: CatGraphicalGUI, Reason: "Wayland/X11 image viewer"},
	"sxiv":           {Category: CatGraphicalGUI, Reason: "X11 image viewer"},
	"eog":            {Category: CatGraphicalGUI, Reason: "GNOME graphical image viewer"},
	"firefox":        {Category: CatGraphicalGUI, Reason: "graphical web browser", Alternative: "Use curl"},
	"chromium":       {Category: CatGraphicalGUI, Reason: "graphical web browser", Alternative: "Use curl"},
	"google-chrome":  {Category: CatGraphicalGUI, Reason: "graphical web browser", Alternative: "Use curl"},
	"xev":            {Category: CatGraphicalGUI, Reason: "X11 window event tester requiring an X server"},
	"xeyes":          {Category: CatGraphicalGUI, Reason: "X11 graphical demo program"},
	"xclock":         {Category: CatGraphicalGUI, Reason: "X11 graphical clock"},
	"zenity":         {Category: CatGraphicalGUI, Reason: "GTK graphical dialog popup tool"},
	"kdialog":        {Category: CatGraphicalGUI, Reason: "KDE graphical dialog popup tool"},
	"yad":            {Category: CatGraphicalGUI, Reason: "GTK graphical dialog popup tool"},
	"xmessage":       {Category: CatGraphicalGUI, Reason: "X11 graphical message window"},
	"gparted":        {Category: CatGraphicalGUI, Reason: "GTK graphical partition editor", Alternative: "Use lsblk or sfdisk"},
	"pavucontrol":    {Category: CatGraphicalGUI, Reason: "GTK PulseAudio volume control GUI", Alternative: "Use pactl"},
	"arandr":         {Category: CatGraphicalGUI, Reason: "GTK X11 screen layout editor", Alternative: "Use xrandr"},
	"kitty":          {Category: CatGraphicalGUI, Reason: "GPU graphical terminal emulator"},
	"alacritty":      {Category: CatGraphicalGUI, Reason: "GPU graphical terminal emulator"},
	"wezterm":        {Category: CatGraphicalGUI, Reason: "GPU graphical terminal emulator"},
	"foot":           {Category: CatGraphicalGUI, Reason: "Wayland terminal emulator"},
	"xterm":          {Category: CatGraphicalGUI, Reason: "X11 terminal emulator"},
	"gnome-terminal": {Category: CatGraphicalGUI, Reason: "GNOME graphical terminal emulator"},
	"konsole":        {Category: CatGraphicalGUI, Reason: "KDE graphical terminal emulator"},
	"rofi":           {Category: CatGraphicalGUI, Reason: "X11/Wayland graphical window switcher and launcher"},
	"wofi":           {Category: CatGraphicalGUI, Reason: "Wayland graphical launcher"},
	"dmenu":          {Category: CatGraphicalGUI, Reason: "X11 graphical dynamic menu"},

	// Category 4: Direct /dev/tty hardware or security prompts
	"passwd":                         {Category: CatDirectTTY, Reason: "opens /dev/tty directly for interactive password entry; fails under Setsid (no controlling TTY)"},
	"su":                             {Category: CatDirectTTY, Reason: "opens /dev/tty directly for password prompts and spawns an interactive shell"},
	"ssh-add":                        {Category: CatDirectTTY, Reason: "prompts for key passphrases directly on /dev/tty or SSH_ASKPASS"},
	"visudo":                         {Category: CatDirectTTY, Reason: "locks sudoers and launches an interactive full-screen TTY editor"},
	"vipw":                           {Category: CatDirectTTY, Reason: "locks /etc/passwd and launches an interactive full-screen TTY editor"},
	"vigr":                           {Category: CatDirectTTY, Reason: "locks /etc/group and launches an interactive full-screen TTY editor"},
	"chsh":                           {Category: CatDirectTTY, Reason: "prompts on /dev/tty to change login shell"},
	"chfn":                           {Category: CatDirectTTY, Reason: "prompts on /dev/tty to change finger information"},
	"newgrp":                         {Category: CatDirectTTY, Reason: "spawns a new interactive shell session with a different group ID"},
	"pinentry":                       {Category: CatDirectTTY, Reason: "interactive PIN/passphrase dialog requiring a TTY or GUI display"},
	"pinentry-curses":                {Category: CatDirectTTY, Reason: "curses PIN entry dialog requiring a controlling /dev/tty"},
	"pinentry-tty":                   {Category: CatDirectTTY, Reason: "direct /dev/tty PIN entry prompt"},
	"systemd-tty-ask-password-agent": {Category: CatDirectTTY, Reason: "interacts directly with /dev/tty password prompts"},

	// Category 5: Terminal pixel & graphics renderers (Sixel, Kitty, iTerm2, ANSI truecolor blocks)
	"chafa":         {Category: CatTerminalGraphics, Reason: "renders images using Sixel/Kitty/ANSI escape sequences stripped by Mint's CleanLine"},
	"viu":           {Category: CatTerminalGraphics, Reason: "renders images using Kitty/iTerm2/ANSI half-block colour escapes stripped by CleanLine"},
	"timg":          {Category: CatTerminalGraphics, Reason: "terminal image and video viewer using Sixel/Kitty/ANSI pixel escapes"},
	"catimg":        {Category: CatTerminalGraphics, Reason: "prints images using ANSI 24-bit colour escape sequences stripped by CleanLine"},
	"lsix":          {Category: CatTerminalGraphics, Reason: "renders image thumbnails using Sixel graphics escape sequences"},
	"w3mimgdisplay": {Category: CatTerminalGraphics, Reason: "draws bitmap images directly onto the X11/terminal window"},
	"ueberzug":      {Category: CatTerminalGraphics, Reason: "draws images over terminal windows via X11 child windows"},
	"ueberzugpp":    {Category: CatTerminalGraphics, Reason: "draws images using Sixel/Kitty/Wayland/X11 overlays"},
	"pixterm":       {Category: CatTerminalGraphics, Reason: "draws images in terminals using ANSI truecolor escapes"},
	"cacaview":      {Category: CatTerminalGraphics, Reason: "full-screen libcaca colour image viewer"},

	// Category 6: Raw terminal / session recorders & state manipulators
	"script":       {Category: CatTerminalState, Reason: "allocates a pty to record an interactive terminal session"},
	"scriptreplay": {Category: CatTerminalState, Reason: "replays raw terminal timing and escape sequences directly to a TTY"},
	"asciinema":    {Category: CatTerminalState, Reason: "records or replays raw interactive terminal sessions via a pty"},
	"ttyrec":       {Category: CatTerminalState, Reason: "records raw interactive tty output"},
	"ttyplay":      {Category: CatTerminalState, Reason: "replays raw interactive tty recordings"},
	"reset":        {Category: CatTerminalState, Reason: "reinitializes the controlling terminal's termios state and clears the screen"},
	"clear":        {Category: CatTerminalState, Reason: "emits terminal-clearing escape codes that have no meaning in a structured Mint screen"},
	"stty":         {Category: CatTerminalState, Reason: "reads or mutates termios line settings on the controlling terminal (/dev/tty)"},
	"tput":         {Category: CatTerminalState, Reason: "queries terminfo to emit raw cursor/screen control escape sequences"},
	"tset":         {Category: CatTerminalState, Reason: "initializes controlling terminal termios modes"},
	"setterm":      {Category: CatTerminalState, Reason: "emits raw terminal control escape sequences to change terminal attributes"},
	"tabs":         {Category: CatTerminalState, Reason: "sets hardware tab stops on a physical/emulated terminal"},
	"resize":       {Category: CatTerminalState, Reason: "queries terminal dimensions via raw escape sequence round-trip on /dev/tty"},
	"openvt":       {Category: CatTerminalState, Reason: "allocates and switches Linux virtual consoles (/dev/ttyN)"},
	"chvt":         {Category: CatTerminalState, Reason: "switches foreground Linux virtual terminal"},
	"deallocvt":    {Category: CatTerminalState, Reason: "deallocates kernel virtual consoles"},
	"vlock":        {Category: CatTerminalState, Reason: "locks virtual console terminal sessions"},
}

var (
	tuiLibPrefixes = []string{
		"libncurses.so",
		"libncursesw.so",
		"libform.so",
		"libformw.so",
		"libmenu.so",
		"libmenuw.so",
		"libpanel.so",
		"libpanelw.so",
		"libslang.so",
		"libnotcurses.so",
		"libnewt.so",
		"libcdk.so",
	}
	guiLibPrefixes = []string{
		"libX11.so",
		"libwayland-client.so",
		"libgtk-3.so",
		"libgtk-4.so",
		"libQt5Gui.so",
		"libQt6Gui.so",
		"libQt5Widgets.so",
		"libQt6Widgets.so",
		"libSDL2-2.0.so",
		"libwx_gtk",
	}
)

// CheckCompatibility inspects bin and reports whether it can run inside a pure
// Mint TUI (detached session, no /dev/tty, ANSI escapes stripped by CleanLine).
func CheckCompatibility(bin string) CompatReport {
	name := filepath.Base(strings.TrimSpace(bin))
	if name == "" {
		return CompatReport{
			Program:    bin,
			Compatible: false,
			Category:   CatShellBuiltin,
			Reason:     "empty program name",
		}
	}

	// 1. Check shell built-ins & parent-environment mutators
	if reason, ok := shellBuiltins[name]; ok {
		return CompatReport{
			Program:    name,
			Compatible: false,
			Category:   CatShellBuiltin,
			Reason:     fmt.Sprintf("%q is a shell built-in / parent-environment mutator (%s)", name, reason),
		}
	}

	// 2–6. Check curated exclusion categories
	if rule, ok := curatedExclusions[name]; ok {
		return CompatReport{
			Program:     name,
			Compatible:  false,
			Category:    rule.Category,
			Reason:      rule.Reason,
			Alternative: rule.Alternative,
			BatchHint:   rule.BatchHint,
		}
	}

	// Locate binary on PATH if installed
	binPath, err := exec.LookPath(bin)
	if err != nil {
		// Not installed locally, or only a shell builtin: ask the shell.
		if out, terr := shellType(name); terr == nil {
			s := strings.ToLower(string(out))
			if strings.Contains(s, "shell builtin") || strings.Contains(s, "is a special shell builtin") {
				return CompatReport{
					Program:    name,
					Compatible: false,
					Category:   CatShellBuiltin,
					Reason:     fmt.Sprintf("%q is a shell built-in with no standalone executable", name),
				}
			}
		}
		return CompatReport{
			Program:    name,
			Compatible: true,
			Category:   CatCompatible,
			Reason:     "passes static TUI exclusion checks (binary not currently on PATH for ldd inspection)",
		}
	}

	libs := inspectLinkedLibs(binPath)
	manSummary := strings.ToLower(readOneLineSummary(name))

	// Dynamic ldd check for full-screen curses/newt/slang libraries
	for _, lib := range libs {
		for _, prefix := range tuiLibPrefixes {
			if strings.HasPrefix(lib, prefix) {
				// Some standard CLI tools link libncurses only for colour capability
				// checks while remaining line-oriented. Check if the summary or library
				// indicates a full-screen TUI.
				if strings.HasPrefix(lib, "libform") || strings.HasPrefix(lib, "libmenu") ||
					strings.HasPrefix(lib, "libpanel") || strings.HasPrefix(lib, "libnewt") ||
					strings.HasPrefix(lib, "libnotcurses") || strings.HasPrefix(lib, "libcdk") ||
					strings.Contains(manSummary, "interactive") ||
					strings.Contains(manSummary, "full-screen") ||
					strings.Contains(manSummary, "fullscreen") ||
					strings.Contains(manSummary, "text editor") ||
					strings.Contains(manSummary, "file manager") ||
					strings.Contains(manSummary, "curses") ||
					strings.Contains(manSummary, "terminal ui") ||
					strings.Contains(manSummary, "tui") ||
					strings.Contains(manSummary, "viewer") ||
					strings.Contains(manSummary, "pager") {
					return CompatReport{
						Program:    name,
						BinaryPath: binPath,
						Compatible: false,
						Category:   CatFullScreenTUI,
						Reason:     fmt.Sprintf("links against %s and operates as an interactive/full-screen terminal program", lib),
						LinkedLibs: libs,
					}
				}
			}
		}
	}

	// Dynamic ldd check for GUI libraries (GTK / Qt / Wayland / X11)
	for _, lib := range libs {
		for _, prefix := range guiLibPrefixes {
			if strings.HasPrefix(lib, prefix) {
				// Allow hybrid CLI tools (like ffmpeg / ffprobe / magick / graphviz)
				// that link X11/SDL as a codec dependency but are primarily CLI tools.
				if isKnownHybridCLI(name) {
					break
				}
				if strings.HasPrefix(lib, "libgtk") || strings.HasPrefix(lib, "libQt") || strings.HasPrefix(lib, "libwx_gtk") {
					return CompatReport{
						Program:    name,
						BinaryPath: binPath,
						Compatible: false,
						Category:   CatGraphicalGUI,
						Reason:     fmt.Sprintf("links against graphical toolkit %s and requires an X11/Wayland display", lib),
						LinkedLibs: libs,
					}
				}
			}
		}
	}

	return CompatReport{
		Program:    name,
		BinaryPath: binPath,
		Compatible: true,
		Category:   CatCompatible,
		Reason:     "compatible with Mint's detached line-oriented runner and CleanLine output pipeline",
		LinkedLibs: libs,
	}
}

func isKnownHybridCLI(name string) bool {
	switch name {
	case "ffmpeg", "ffprobe", "magick", "convert", "identify", "mogrify",
		"dot", "neato", "twopi", "circo", "fdp", "sfdp", "pandoc", "gs", "gnuplot":
		return true
	}
	return false
}

func inspectLinkedLibs(binPath string) []string {
	out, err := exec.Command("ldd", binPath).CombinedOutput()
	if err != nil {
		return nil
	}
	var libs []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.Contains(fields[0], ".so") {
			libs = append(libs, filepath.Base(fields[0]))
		}
	}
	return libs
}

func readOneLineSummary(name string) string {
	out, err := exec.Command("whatis", name).CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return mint.CleanLine(lines[0])
}

// programName matches what a program name can be. Anything else (spaces,
// ";", "$(", quotes) is not a program and never reaches a shell.
var programName = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// shellType reports what the shell's "type" says name is. Telling a shell
// built-in from a program needs a shell, so this is the one place mint-gen
// starts one. name is passed as a positional parameter ("$1"), never as
// part of the script, so nothing in it is run as shell syntax.
func shellType(name string) ([]byte, error) {
	if !programName.MatchString(name) || strings.HasPrefix(name, "-") {
		return nil, fmt.Errorf("%q is not a program name", name)
	}
	return exec.Command("sh", "-c", `type "$1"`, "sh", name).CombinedOutput()
}
