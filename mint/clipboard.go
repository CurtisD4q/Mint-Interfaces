package mint

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// Copying to the clipboard (OSC 52).
//
// Copy sends text to the terminal's clipboard with an OSC 52 sequence.
// The terminal, not Mint, puts it on the clipboard, so it works over SSH
// and inside tmux: the clipboard is the one on the computer you are
// sitting at. Mint only ever writes the clipboard; it never asks the
// terminal what is on it.
//
// Terminals that take OSC 52 include kitty, WezTerm, iTerm2 (allow it in
// Settings), Alacritty, foot, Windows Terminal, Ghostty and xterm (with
// allowWindowOps). tmux passes it on with `set -g set-clipboard on`, or
// with `set -g allow-passthrough on`.

// MaxCopyBytes is the most text Copy sends. Many terminals drop a longer
// OSC 52 sequence without a word, so Copy refuses instead.
const MaxCopyBytes = 74 * 1024

// ErrCopyTooLong is returned for text over MaxCopyBytes.
var ErrCopyTooLong = errors.New("too long to copy")

// CopyGuard, when set, is asked before a program's output is copied (the
// output viewer's y, or y on an output line or table row). An app whose
// results are secret returns an error to keep them off the clipboard,
// which clipboard managers often keep. Copying what Mint itself shows (a
// command preview, a setting) is not output and is not asked about.
var CopyGuard func() error

// Copy puts text on the terminal's clipboard. Escape sequences and
// control characters (other than newlines and tabs) are removed first, so
// what is pasted is the text as it was shown, and cannot drive the
// terminal it is pasted into.
func Copy(text string) error {
	text = copyText(text)
	if text == "" {
		return errors.New("nothing to copy")
	}
	if len(text) > MaxCopyBytes {
		return fmt.Errorf("%w (%d KB; the most is %d KB)", ErrCopyTooLong, (len(text)+1023)/1024, MaxCopyBytes/1024)
	}
	writeClipboard(osc52(text))
	return nil
}

// writeClipboard sends the sequence to the terminal; tests replace it.
var writeClipboard = out

// copyText is text as it will be copied: each line cleaned of escape
// sequences and control characters.
func copyText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, l := range lines {
		l = CleanLine(l)
		lines[i] = strings.Map(func(r rune) rune {
			if r == '\t' {
				return r
			}
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, l)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// osc52 is the sequence that sets the clipboard to text, wrapped so that
// tmux or GNU screen passes it on to the terminal they run in.
func osc52(text string) string {
	seq := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
	switch {
	case os.Getenv("TMUX") != "":
		// tmux takes the plain sequence itself with set-clipboard on, and
		// passes the wrapped one on with allow-passthrough on. Either may
		// be set, so both are sent; the terminal sets the same text twice.
		return seq + "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	case os.Getenv("STY") != "":
		return "\x1bP" + seq + "\x1b\\"
	}
	return seq
}

// copyOutput is Copy for a program's output: CopyGuard is asked first.
func copyOutput(text string) error {
	if CopyGuard != nil {
		if err := CopyGuard(); err != nil {
			return err
		}
	}
	return Copy(text)
}

// copied tells the user what Copy did.
func copied(err error, what string) {
	switch {
	case err != nil:
		ToastWarn("Not copied: " + err.Error())
	case !rt.profile.OSC52:
		Toast("Sent " + what + " to the clipboard (if this terminal takes OSC 52)")
	default:
		Toast("Copied " + what)
	}
}

// Copy is Copy for a screen's actions, with a toast saying what happened.
func (c *Ctx) Copy(text, what string) bool {
	err := Copy(text)
	copied(err, what)
	return err == nil
}

// linesWord is "1 line" or "n lines".
func linesWord(n int) string {
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}
