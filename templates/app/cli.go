package main

// cli.go: the helpers every catalogue example relies on. Copy this file
// into a new interface unchanged and set tool to the program being wrapped.

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// tool is the program this interface wraps.
const tool = "mytool"

// query runs the tool and returns its trimmed output, or "" if it failed.
// Use it to read a value. Never call it from a row's Get: getters run on
// every repaint. Read into variables in Enter or after a change instead.
func query(args ...string) string {
	out, _ := util.RunValue(tool, args...)
	return out
}

// queryJSON runs the tool and decodes its JSON output into v. Prefer this
// whenever the tool has a --json (or similar) output mode.
func queryJSON(v any, args ...string) error {
	out, err := util.Run(tool, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(out), v)
}

// apply runs the tool to change something. On failure the error carries
// the tool's own message, ready to show with mint.ToastError.
func apply(args ...string) error {
	out, err := exec.Command(tool, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		lines := strings.Split(msg, "\n")
		return errors.New(lines[len(lines)-1])
	}
	return err
}

// applyShown runs apply behind the footer spinner and reports the result
// as a toast. Returns whether it worked.
func applyShown(doing, done string, args ...string) bool {
	var err error
	mint.Busy(doing, func() { err = apply(args...) })
	if err != nil {
		mint.ToastError(err.Error())
		return false
	}
	if done != "" {
		mint.Toast(done)
	}
	return true
}
