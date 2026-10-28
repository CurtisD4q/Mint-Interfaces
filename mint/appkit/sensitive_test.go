package appkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyData(t *testing.T) {
	cases := []struct {
		spec ToolSpec
		want string
	}{
		{ToolSpec{Name: "tcpdump"}, DataSensitive},
		{ToolSpec{Name: "x", Bin: "/usr/bin/gpg"}, DataSecret},
		{ToolSpec{Name: "pass"}, DataSecret},
		{ToolSpec{Name: "pg_dump"}, DataSensitive},
		{ToolSpec{Name: "mytool", ArchetypeID: "follow"}, DataSensitive},
		{ToolSpec{Name: "sha256sum", ArchetypeID: "inspect"}, DataNormal},
		{ToolSpec{Name: "ss", ArchetypeID: "list"}, DataNormal},
		{ToolSpec{Name: "ls"}, DataNormal},
	}
	for _, c := range cases {
		if got, _ := ClassifyData(c.spec); got != c.want {
			t.Errorf("%s/%s: %q, want %q", c.spec.Name, c.spec.ArchetypeID, got, c.want)
		}
	}
	// An explicit level is kept.
	s := ToolSpec{Name: "tcpdump", Data: DataSecret}
	ApplySafetyDefaults(&s)
	if s.Data != DataSecret {
		t.Fatal("explicit level overwritten")
	}
}

func TestLooksRevealing(t *testing.T) {
	yes := [][3]string{
		{"--export-secret-keys", "Export secret keys", ""},
		{"-d", "Decrypt data (default)", ""},
		{"--decrypt", "", ""},
		{"--show-password", "", ""},
		{"genkey", "Generate a private key", ""},
	}
	no := [][3]string{
		{"-u", "Use USER-ID to sign or decrypt", ""},
		{"-v", "Verbose", ""},
		{"--password", "Password", ""},
	}
	for _, c := range yes {
		if !LooksRevealing(c[0], c[1], c[2]) {
			t.Errorf("%v should reveal", c)
		}
	}
	for _, c := range no {
		if LooksRevealing(c[0], c[1], c[2]) {
			t.Errorf("%v should not reveal", c)
		}
	}
}

// A revealing option makes the run secret; a secret flag is never marked.
func TestRevealingOptionRaisesLevel(t *testing.T) {
	bin, _ := fakeTool(t, "keytool", "")
	a := newTestApp(t, ToolSpec{Name: "keytool", Bin: bin, Flags: []FlagSpec{
		{Flag: "--export-secret", Kind: FlagBool, Label: "Export secret keys"},
		{Flag: "--passphrase", Kind: FlagSecret, Label: "Passphrase to show private key"},
	}})
	if !a.Spec.Flags[0].Reveals || a.Spec.Flags[1].Reveals {
		t.Fatalf("reveals marks: %+v", a.Spec.Flags)
	}
	if l, _ := a.DataLevel(); l != DataNormal {
		t.Fatalf("level %q before the option is set", l)
	}
	a.Values["--export-secret"] = "true"
	if l, why := a.DataLevel(); l != DataSecret || !strings.Contains(why, "--export-secret") {
		t.Fatalf("level %q (%s)", l, why)
	}
	if b := a.statusBadges(); len(b) == 0 || b[0].Text != "secret data" {
		t.Fatalf("badges %+v", b)
	}
}

func TestPrivateKeyBlockHidden(t *testing.T) {
	m := newOutputMasker()
	in := []string{
		"generated:",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ",
		"AAAAMwAAAAtzc2gtZWQyNTUxOQAAACB",
		"-----END OPENSSH PRIVATE KEY-----",
		"done",
		"token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N",
		"root:$6$saltsalt$abcdefghijklmnopqrstuv:19000:0:99999:7:::",
	}
	var out []string
	for _, l := range in {
		out = append(out, m.Redact(l))
	}
	joined := strings.Join(out, "\n")
	for _, leak := range []string{"b3BlbnNz", "AAAAMwAA", "eyJzdWIi", "$6$saltsalt"} {
		if strings.Contains(joined, leak) {
			t.Fatalf("leaked %q:\n%s", leak, joined)
		}
	}
	if out[0] != "generated:" || out[5] != "done" {
		t.Fatalf("lines outside the key changed:\n%s", joined)
	}
	f := strings.Join(m.Findings(), ", ")
	for _, want := range []string{"a private key", "a login token (JWT)", "a password hash"} {
		if !strings.Contains(f, want) {
			t.Fatalf("findings %q lack %q", f, want)
		}
	}
}

// Sensitive results: new folders are private and the output file is
// readable only by you. A run whose output held a secret becomes secret.
func TestSensitiveResultsArePrivate(t *testing.T) {
	bin, _ := fakeTool(t, "cap", `echo "-----BEGIN RSA PRIVATE KEY-----"; echo "MIIEow"; echo "-----END RSA PRIVATE KEY-----"`)
	a := newTestApp(t, ToolSpec{Name: "cap", Bin: bin, Data: DataSensitive, DataReason: "test", OutputsFile: true})
	a.SaveToFile = true
	a.ResultsDir = filepath.Join(t.TempDir(), "new", "results")
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	out := a.ResolvedOutputPath()
	for i := 0; i < 200; i++ {
		if l, _ := a.DataLevel(); l == DataSecret {
			break
		}
		job.Wait()
	}
	if fi, err := os.Stat(a.ResultsDir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("results folder: %v %v", fi.Mode(), err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("output file: %v", err)
	}
	b, _ := os.ReadFile(out)
	if strings.Contains(string(b), "MIIEow") {
		t.Fatalf("saved output holds the key:\n%s", b)
	}
	if l, why := a.DataLevel(); l != DataSecret || !strings.Contains(why, "private key") {
		t.Fatalf("level after the run: %q (%s)", l, why)
	}
}

// A secret tool keeps results in memory by default; a choice is remembered.
func TestSecretToolKeepsResultsInMemory(t *testing.T) {
	bin, _ := fakeTool(t, "pass", "")
	a := newTestApp(t, ToolSpec{Name: "pass-test", Bin: bin, Data: DataSecret, DataReason: "test"})
	if !a.InMemory {
		t.Skip("no private in-memory folder here")
	}
	defer os.RemoveAll(a.ResultsDir)
	if fi, err := os.Stat(a.ResultsDir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("%s: %v", a.ResultsDir, err)
	}
	disk := a.diskDir
	if err := a.SetInMemory(false); err != nil || a.ResultsDir != disk {
		t.Fatalf("off: %s %v", a.ResultsDir, err)
	}
	if err := a.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	b := New(ToolSpec{Name: "pass-test", Bin: bin, Data: DataSecret})
	b.ConfigDir = a.ConfigDir
	b.memSetting = nil
	_ = b.LoadSettings()
	if b.memSetting == nil || *b.memSetting {
		t.Fatal("the choice to keep results on disk was not saved")
	}
}
