package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// Starting an app as root. mint-gen never sees a password: it only asks
// whether to start with sudo, and sudo's own prompt asks for the password.

// Seams for tests.
var (
	genEuid    = os.Geteuid
	stdinIsTTY = func() bool {
		st, err := os.Stdin.Stat()
		return err == nil && st.Mode()&os.ModeCharDevice != 0
	}
	promptIn  io.Reader = os.Stdin
	promptOut io.Writer = os.Stdout
)

// mayNeedRoot reports whether program's category usually needs root
// (packet capture, firewalls, scanners, services, packages, disks…).
func mayNeedRoot(program string, spec *appkit.ToolSpec) bool {
	if spec != nil && spec.NeedsRoot {
		return true
	}
	tpl, ok := appkit.TemplateForTool(program)
	return ok && tpl.Mixins.WithRootAuth
}

// startAsRoot decides whether --run starts the app with sudo: --root and
// --no-root decide outright; otherwise, for tools that usually need root,
// it asks. It never asks when already root or when input is not a terminal.
func startAsRoot(program string, spec *appkit.ToolSpec, rootFlag, noRootFlag bool) bool {
	switch {
	case genEuid() == 0 || noRootFlag:
		return false
	case rootFlag:
		return true
	case !stdinIsTTY() || !mayNeedRoot(program, spec):
		return false
	}
	fmt.Fprintf(promptOut, "%s usually needs root access. Start it with sudo? [y/N] ", program)
	line, _ := bufio.NewReader(promptIn).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// launchCommand is the command that starts the built app, through sudo
// when asRoot. sudo asks for the password on the terminal itself.
func launchCommand(binPath string, appArgs []string, asRoot bool) (*exec.Cmd, error) {
	if !asRoot {
		return exec.Command(binPath, appArgs...), nil
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return nil, fmt.Errorf("sudo is not installed; start %s as root another way", binPath)
	}
	return exec.Command(sudo, append([]string{"--", binPath}, appArgs...)...), nil
}
