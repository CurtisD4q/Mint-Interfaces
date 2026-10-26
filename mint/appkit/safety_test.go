package appkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeTool writes an executable script that records each run (its
// arguments, then its stdin) in a log file.
func fakeTool(t *testing.T, name, extra string) (bin, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "runs.log")
	bin = filepath.Join(dir, name)
	script := "#!/bin/sh\necho \"args: $*\" >> " + log + "\n" + extra + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func newTestApp(t *testing.T, spec ToolSpec) *ToolApp {
	t.Helper()
	tmp := t.TempDir()
	spec.DefaultResultsDir = tmp
	a := New(spec)
	a.ConfigDir = filepath.Join(tmp, "cfg")
	a.DataDir = filepath.Join(tmp, "data")
	return a
}

func runs(log string) string {
	b, _ := os.ReadFile(log)
	return string(b)
}

// Problem 1: a monitor must not repeat a command that changes things.
func TestMonitorDoesNotPollToolsThatChangeThings(t *testing.T) {
	bin, log := fakeTool(t, "zap", "")
	a := newTestApp(t, ToolSpec{Name: "zap", Bin: bin, Archetype: ArchetypeMonitor, PollInterval: 10 * time.Millisecond})
	a.Target = "sleep"
	s := a.MonitorScreen()
	s.Enter(s)
	for i := 0; i < 10; i++ {
		s.Poll(s)
		time.Sleep(15 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if got := runs(log); got != "" {
		t.Fatalf("monitor ran the tool by itself:\n%s", got)
	}
}

func TestMonitorPollsReadOnlyToolsInTheBackground(t *testing.T) {
	bin, log := fakeTool(t, "look", "sleep 0.3")
	a := newTestApp(t, ToolSpec{Name: "look", Bin: bin, Archetype: ArchetypeMonitor, ReadOnly: true, PollInterval: 10 * time.Millisecond})
	s := a.MonitorScreen()
	start := time.Now()
	s.Enter(s)
	s.Poll(s)
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("polling blocked the screen for %v", d)
	}
	deadline := time.Now().Add(3 * time.Second)
	for runs(log) == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if runs(log) == "" {
		t.Fatal("read-only tool was never polled")
	}
}

// Problem 2: refreshing a config tool's state must never run its apply command.
func TestConfigRefreshNeverRunsTheApplyCommand(t *testing.T) {
	bin, log := fakeTool(t, "setter", "")
	a := newTestApp(t, ToolSpec{Name: "setter", Bin: bin, Archetype: ArchetypeConfig,
		Flags: []FlagSpec{{Flag: "-w", Kind: FlagText}}})
	a.Values["-w"] = "key=value"
	if err := a.RefreshState(); !errors.Is(err, ErrNoStatusCommand) {
		t.Fatalf("err %v, want ErrNoStatusCommand", err)
	}
	if got := runs(log); got != "" {
		t.Fatalf("refresh ran the tool: %s", got)
	}
	a.Spec.StatusArgs = []string{"--show"}
	if err := a.RefreshState(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runs(log)); got != "args: --show" {
		t.Fatalf("status command was %q, want only the read-only arguments", got)
	}
}

func TestSafetyDefaultsForKnownTools(t *testing.T) {
	for _, c := range []struct {
		bin                    string
		readOnly, irreversible bool
		status                 bool
	}{
		{"ps", true, false, false}, {"df", true, false, false},
		{"pkill", false, true, false}, {"kill", false, true, false}, {"shred", false, true, false},
		{"mkfs.ext4", false, true, false}, {"sysctl", false, false, true}, {"systemctl", false, false, true},
		{"curl", false, false, false},
	} {
		s := ToolSpec{Name: c.bin}
		ApplySafetyDefaults(&s)
		if s.ReadOnly != c.readOnly || s.Irreversible != c.irreversible || (len(s.StatusArgs) > 0) != c.status {
			t.Errorf("%s: readOnly %v irreversible %v status %v", c.bin, s.ReadOnly, s.Irreversible, s.StatusArgs)
		}
	}
	if (ToolSpec{Name: "pkill"}).CanReadState() {
		t.Error("pkill must not count as able to read state")
	}
}

// Problem 4: secrets are never saved, shown or recorded.
func TestSecretsAreNotSavedShownOrRecorded(t *testing.T) {
	bin, log := fakeTool(t, "login", "cat >> "+"\"$(dirname \"$0\")/runs.log\"")
	a := newTestApp(t, ToolSpec{Name: "login", Bin: bin, KeepHistory: true, Flags: []FlagSpec{
		{Flag: "--password", Kind: FlagSecret, Label: "Password"},
		{Flag: "--token-stdin", Kind: FlagSecret, SecretStdin: true, Label: "Token"},
	}})
	a.SetSecret("--password", "hunter2")
	a.SetSecret("--token-stdin", "tok-123")

	args, _ := a.BuildArgs()
	if !strings.Contains(strings.Join(args, " "), "--password hunter2") || strings.Contains(strings.Join(args, " "), "tok-123") {
		t.Fatalf("args %q", args)
	}
	if p := a.CommandPreview(); strings.Contains(p, "hunter2") || strings.Contains(p, "tok-123") {
		t.Fatalf("preview shows a secret: %q", p)
	}
	if got := a.argvSecrets(); len(got) != 1 || got[0] != "Password" {
		t.Fatalf("argvSecrets %q", got)
	}

	if err := a.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(a.settingsPath())
	if strings.Contains(string(saved), "hunter2") || strings.Contains(string(saved), "tok-123") {
		t.Fatalf("settings contain a secret:\n%s", saved)
	}
	if st, _ := os.Stat(a.settingsPath()); st.Mode().Perm() != 0o600 {
		t.Fatalf("settings mode %v, want 0600", st.Mode().Perm())
	}

	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	time.Sleep(50 * time.Millisecond) // OnExit records history after the job ends
	got := runs(log)
	if !strings.Contains(got, "tok-123") || strings.Contains(strings.SplitN(got, "\n", 2)[0], "tok-123") {
		t.Fatalf("stdin secret should arrive on stdin, not in the arguments:\n%s", got)
	}
	hist, _ := os.ReadFile(a.historyPath())
	if strings.Contains(string(hist), "hunter2") || strings.Contains(string(hist), "tok-123") {
		t.Fatalf("history contains a secret:\n%s", hist)
	}
}

// Problem 6: a status command that never ends cannot hang the app.
func TestRefreshStateTimesOut(t *testing.T) {
	bin, _ := fakeTool(t, "follow", "exec sleep 30")
	old := statusTimeout
	statusTimeout = 200 * time.Millisecond
	defer func() { statusTimeout = old }()
	a := newTestApp(t, ToolSpec{Name: "follow", Bin: bin, ReadOnly: true})
	start := time.Now()
	err := a.RefreshState()
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
}

func TestLooksDestructive(t *testing.T) {
	yes := []struct{ flag, label string }{
		{"--force", ""}, {"--remove-files", "Remove files after adding them"}, {"--delete", ""},
		{"--no-preserve-root", "Do not treat '/' specially"}, {"--hard", ""},
		{"-f", "Ignore nonexistent files and arguments, never prompt"},
		{"-f", "Do not prompt before overwriting"}, {"-r", "Remove directories and their contents recursively"},
		{"--overwrite", "Overwrite existing files when extracting"},
	}
	no := []struct{ flag, label string }{
		{"-i", "Prompt before overwrite"}, {"-n", "Do not overwrite an existing file"},
		{"--no-overwrite-dir", "Preserve metadata of existing directories"},
		{"--owner", "Force NAME as owner for added files"}, {"-v", "Explain what is being done"},
		{"--hard-dereference", "Follow hard links"},
	}
	for _, c := range yes {
		if !LooksDestructive(c.flag, c.label) {
			t.Errorf("%s %q should look destructive", c.flag, c.label)
		}
	}
	for _, c := range no {
		if LooksDestructive(c.flag, c.label) {
			t.Errorf("%s %q should not look destructive", c.flag, c.label)
		}
	}
}

func TestSafetyReport(t *testing.T) {
	r := ToolSpec{Name: "shred", Flags: []FlagSpec{
		{Flag: "--password", Kind: FlagSecret, Label: "Password"},
		{Flag: "--token-stdin", Kind: FlagSecret, SecretStdin: true, Label: "Token"},
		{Flag: "--force", Kind: FlagBool, Label: "Force"},
		{Flag: "-v", Kind: FlagBool, Label: "Verbose"},
	}}.SafetyReport()
	if !strings.Contains(r.Running, "typed to confirm") {
		t.Errorf("running %q", r.Running)
	}
	if len(r.Secrets) != 2 || r.Secrets[0].Route != RouteArgument || r.Secrets[1].Route != RouteStdin {
		t.Errorf("secrets %+v", r.Secrets)
	}
	if len(r.Destructive) != 1 || r.Destructive[0].Flag != "--force" {
		t.Errorf("destructive %+v", r.Destructive)
	}
	if ps := (ToolSpec{Name: "ps"}).SafetyReport(); !strings.Contains(ps.Running, "may run automatically") {
		t.Errorf("ps running %q", ps.Running)
	}
}
