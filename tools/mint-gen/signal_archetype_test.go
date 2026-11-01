package main

import "testing"

const signalHelp = `NAME
       zapper - send a signal to processes by name

SYNOPSIS
       zapper [options] name...

DESCRIPTION
       zapper sends a signal to each running process whose name matches.

OPTIONS
       -s, --signal SIGNAL
              Send this signal instead of SIGTERM.
       -p, --pid PID
              Only processes with this process ID.
`

func TestSignalArchetypeFromItsHelp(t *testing.T) {
	p := programTextFrom("zapper", signalHelp, "")
	p.Related = nil
	if c := chooseArchetypeFor(p); c.ID != "signal" {
		t.Fatalf("zapper chose %s", c.ID)
	}
	spec := ParseManAndHelp("zapper", signalHelp, "")
	if !spec.ConfirmRun || spec.ReadOnly {
		t.Fatalf("a signal sender must ask first and never be read-only: confirm=%v ro=%v", spec.ConfirmRun, spec.ReadOnly)
	}
}

func TestActsOnRunningProcesses(t *testing.T) {
	yes := []string{
		"  -p, --pid PID\n  -s, --signal SIG\n",
		"  -n, --priority N\n  -p, --pid PID\n",
		"  -c, --class CLASS\n  -p, --pid PID...\n",
	}
	no := []string{
		"  -F, --pidfile FILE\n  -s, --signal SIG\n", // pgrep reads a pid file
		"  -p, --pid PID\n  -v, --verbose\n",
		"  -s, --signal SIG\n",
	}
	for _, s := range yes {
		if !actsOnRunningProcesses(s) {
			t.Errorf("missed %q", s)
		}
	}
	for _, s := range no {
		if actsOnRunningProcesses(s) {
			t.Errorf("wrongly matched %q", s)
		}
	}
}
