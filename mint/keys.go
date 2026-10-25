package mint

import (
	"os"
	"strings"
)

// Key profiles. Arrow keys, Enter, Space, Esc, b and q work in every
// profile; a profile adds its own keys on top.
//
//	default  arrows only
//	emacs    C-n/C-p move, C-f/C-b change, C-g back, C-v/M-v page,
//	         C-a/M-< first, C-e/M-> last
//	vim      j/k move, h/l change, g/G first/last, C-d/C-u (or C-f/C-b) page
//
// The person running an app picks one with the MINT_KEYS environment
// variable; App.Keys sets the app's default.
var KeyProfiles = []string{"default", "emacs", "vim"}

var keyProfile = "default"

var emacsKeys = map[string]string{
	"\x0e": "down", "\x10": "up", "\x06": "right", "\x02": "left",
	"\x07": "esc", "\x16": "pgdn", "M-v": "pgup",
	"\x01": "home", "\x05": "end", "M-<": "home", "M->": "end",
}

var vimControl = map[string]string{
	"\x04": "pgdn", "\x15": "pgup", "\x06": "pgdn", "\x02": "pgup",
}

var vimLetters = map[string]string{
	"j": "down", "k": "up", "h": "left", "l": "right", "g": "home", "G": "end",
}

// SetKeyProfile selects "default", "emacs" or "vim". It returns false,
// leaving the profile unchanged, for any other name.
func SetKeyProfile(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range KeyProfiles {
		if p == name {
			keyProfile = name
			applyProfileHints()
			return true
		}
	}
	return false
}

// KeyProfile returns the active profile's name.
func KeyProfile() string { return keyProfile }

// navKey translates a profile's keys into the standard names. While typing
// into a text field, plain letters are left alone so they can be typed.
func navKey(k *Key, typing bool) *Key {
	if k == nil || k.Mouse != nil {
		return k
	}
	switch keyProfile {
	case "emacs":
		if n, ok := emacsKeys[k.Name]; ok {
			return named(n)
		}
	case "vim":
		if n, ok := vimControl[k.Name]; ok && !k.Special {
			return named(n)
		}
		if !typing && !k.Special {
			if n, ok := vimLetters[k.Name]; ok {
				return named(n)
			}
		}
	}
	return k
}

// readNav reads one key and applies the profile.
func readNav(typing bool) *Key { return navKey(ReadKey(), typing) }

const (
	defaultHomeHints   = " ↑↓ move   ⏎ open   q quit "
	defaultScreenHints = " ↑↓ move   ←→/Space change   b home   q quit "
)

var profileHints = map[string][2]string{
	"default": {defaultHomeHints, defaultScreenHints},
	"emacs":   {" C-n/C-p move   ⏎ open   q quit ", " C-n/C-p move   C-f/C-b change   C-g back   q quit "},
	"vim":     {" j/k move   ⏎ open   q quit ", " j/k move   h/l change   b back   q quit "},
}

// applyProfileHints swaps the default footers for the profile's, leaving
// footers an app has customised alone.
func applyProfileHints() {
	for _, h := range profileHints {
		if HomeHints == h[0] {
			HomeHints = profileHints[keyProfile][0]
		}
		if ScreenHints == h[1] {
			ScreenHints = profileHints[keyProfile][1]
		}
	}
}

// profileFromEnv applies MINT_KEYS if it names a profile.
func profileFromEnv() {
	if v := os.Getenv("MINT_KEYS"); v != "" {
		SetKeyProfile(v)
	}
}
