package transcript

import (
	"errors"
	"fmt"
	"strings"
)

// ScriptCommand is one command from a script file.
type ScriptCommand struct {
	Line int    // where it starts, from 1
	Text string // as sent: one line, or several for a SQL statement
	// Escape says why the command reaches outside the program (runs a
	// shell, loads code), or is "". A script may come from someone else,
	// so these are confirmed before it runs.
	Escape string
	// Refuse says why the command must not be sent (Adapter.Refuse).
	Refuse string
}

// MaxScript is the largest script SplitScript reads.
const MaxScript = 1 << 20

// SplitScript cuts a script into the commands to send one at a time:
// comments and blank lines are left out, and SQL statements may run over
// several lines, ending at ";". A statement still open at the end is an
// error: sent, it would hold back every later end-of-output marker.
func SplitScript(a Adapter, text string) ([]ScriptCommand, error) {
	if len(text) > MaxScript {
		return nil, fmt.Errorf("the script is larger than %d KiB", MaxScript>>10)
	}
	if strings.ContainsRune(text, 0) {
		return nil, errors.New("the script contains a NUL byte; it isn't a text file")
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out []ScriptCommand
	var cur []string
	start := 0
	for i, line := range strings.Split(text, "\n") {
		if len(cur) == 0 {
			if a.Comment != nil && a.Comment(line) {
				continue
			}
			start = i + 1
		}
		cur = append(cur, line)
		joined := strings.Join(cur, "\n")
		if a.Complete != nil && !a.Complete(joined) {
			continue
		}
		cmd := ScriptCommand{Line: start, Text: strings.TrimRight(joined, " \t")}
		// Every line of a statement is checked: psql runs a backslash
		// command found on any of them.
		for _, l := range cur {
			if why := a.Escape(l); why != "" && cmd.Escape == "" {
				cmd.Escape = why
			}
			if a.Refuse != nil && cmd.Refuse == "" {
				cmd.Refuse = a.Refuse(l)
			}
		}
		out = append(out, cmd)
		cur = nil
	}
	if len(cur) > 0 && strings.TrimSpace(strings.Join(cur, "\n")) != "" {
		return out, fmt.Errorf("line %d: the statement there never ends (no closing ;)", start)
	}
	return out, nil
}
