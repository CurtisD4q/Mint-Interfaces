package appkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValuesTheBoxCouldNotHoldAreLeftOut(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "v", Bin: "echo", Flags: []FlagSpec{
		{Flag: "-n", Kind: FlagInt, Label: "Count", Min: 1, Max: 10},
		{Flag: "-k", Kind: FlagInt, Label: "Any number"},
		{Flag: "--mode", Kind: FlagChoice, Label: "Mode", Choices: []Choice{{Label: "Default", Value: ""}, {Label: "Fast", Value: "fast"}}},
		{Flag: "--dyn", Kind: FlagChoice, Label: "From the system", ChoicesCmd: []string{"true"}},
	}})
	for _, c := range []struct {
		flag, val string
		passed    bool
	}{
		{"-n", "5", true}, {"-n", "-5", false}, {"-n", "11", false}, {"-n", "abc", false}, {"-n", "99999999999999999999", false},
		{"-k", "-7", true}, {"-k", "3.5", false},
		{"--mode", "fast", true}, {"--mode", "--delete-everything", false},
		{"--dyn", "anything", true},
	} {
		a.Values = map[string]string{c.flag: c.val}
		line := strings.Join(must(a.BuildArgs()), " ")
		if got := strings.Contains(line, c.flag); got != c.passed {
			t.Errorf("%s %q: passed=%v, want %v (%q)", c.flag, c.val, got, c.passed, line)
		}
	}
}

func TestANulNeverReachesTheProgram(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "z", Bin: "echo", TargetLabel: "Target", Flags: []FlagSpec{{Flag: "--name", Kind: FlagText, Label: "Name"}}})
	a.Values["--name"] = "a\x00b"
	a.Target = "x\x00y"
	a.ExtraArgs = "--q=\x00"
	for _, w := range must(a.BuildArgs()) {
		if strings.ContainsRune(w, 0) {
			t.Fatalf("NUL in %q", w)
		}
	}
	if !strings.Contains(strings.Join(must(a.BuildArgs()), " "), "--name ab") {
		t.Fatalf("args %q", must(a.BuildArgs()))
	}
}

// A "not in memory" saved while an app's results were not secret does not
// keep them on disk once they are.
func TestAnOldDiskChoiceDoesNotLowerSecretResults(t *testing.T) {
	plain := newTestApp(t, ToolSpec{Name: "lvl", Bin: "echo"})
	if plain.InMemory {
		t.Skip("setup")
	}
	if err := plain.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(plain.ConfigDir, "settings.json"))
	if !strings.Contains(string(b), `"in_memory": false`) {
		t.Fatalf("settings %s", b)
	}
	secret := New(ToolSpec{Name: "lvl", Bin: "echo", Data: DataSecret, DataReason: "now secret"})
	secret.ConfigDir = plain.ConfigDir
	secret.memSetting = nil
	_ = secret.LoadSettings()
	if secret.memSetting != nil {
		t.Fatal("a choice saved for normal results was taken as a choice about secret ones")
	}
	// A choice made while the results were secret is kept.
	secret.Spec.Data = DataSecret
	secret.InMemory = false
	if err := secret.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	again := New(ToolSpec{Name: "lvl", Bin: "echo", Data: DataSecret})
	again.ConfigDir = plain.ConfigDir
	again.memSetting = nil
	_ = again.LoadSettings()
	if again.memSetting == nil || *again.memSetting {
		t.Fatal("a deliberate choice for secret results was lost")
	}
}
