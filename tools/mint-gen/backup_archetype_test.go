package main

import "testing"

const backupHelp = `NAME
       stash9 - deduplicating backup program

SYNOPSIS
       stash9 [options] COMMAND

DESCRIPTION
       stash9 backs up folders to an encrypted repository as snapshots,
       and restores files from any snapshot.

COMMANDS
       backup    Back up folders to the repository.
       restore   Restore a snapshot.
       snapshots List the snapshots.
       forget    Remove snapshots by a retention policy.
       prune     Remove data no snapshot needs.
       check     Check the repository.
`

const vcsHelp = `NAME
       rev9 - a version control system

SYNOPSIS
       rev9 [options] COMMAND

DESCRIPTION
       rev9 records changes to a working copy as commits on branches.

COMMANDS
       commit    Record changes.
       log       Show the commit history.
       checkout  Switch branches.
       branch    List or create branches.
       merge     Join two branches.
       diff      Show changes.
`

func TestBackupArchetypeAndVersionControlStayApart(t *testing.T) {
	p := programTextFrom("stash9", backupHelp, "")
	p.Related = nil
	if c := chooseArchetypeFor(p); c.ID != "backup" {
		t.Fatalf("stash9 chose %s", c.ID)
	}
	spec := ParseManAndHelp("stash9", backupHelp, "")
	if !spec.ConfirmRun || spec.Data == "" {
		t.Fatalf("a backup tool must ask first and keep its data sensitive: confirm=%v data=%q", spec.ConfirmRun, spec.Data)
	}
	q := programTextFrom("rev9", vcsHelp, "")
	q.Related = nil
	if c := chooseArchetypeFor(q); c.ID != "history" {
		t.Fatalf("rev9 chose %s, want history", c.ID)
	}
}
