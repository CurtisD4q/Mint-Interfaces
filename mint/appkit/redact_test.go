package appkit

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func values(found []FreeTextSecret) []string {
	var out []string
	for _, s := range found {
		out = append(out, s.Value)
	}
	return out
}

func TestFindFreeTextSecrets(t *testing.T) {
	cases := []struct {
		text  string
		flags []string
		want  []string
	}{
		{"https://alice:s3cr3t@example.com/repo.git", nil, []string{"s3cr3t"}},
		{"postgres://app:pa%40ss@db:5432/prod", nil, []string{"pa%40ss"}},
		{"--password=hunter22", nil, []string{"hunter22"}},
		{"--db-pass hunter22 target", nil, []string{"hunter22"}},
		{"--api-key ABCD1234", nil, []string{"ABCD1234"}},
		{"-H 'Authorization: Bearer abc.def.ghi'", nil, []string{"abc.def.ghi"}},
		{`-H "X-Api-Key: k3y-value"`, nil, []string{"k3y-value"}},
		{"--user bob:pw-1234 https://x", nil, []string{"pw-1234"}},
		{"PGPASSWORD=letmein", nil, []string{"letmein"}},
		{"-p s3cr3t host", []string{"-p"}, []string{"s3cr3t"}},
		{"-ps3cr3t host", []string{"-p"}, []string{"s3cr3t"}},
		{"token ghp_" + strings.Repeat("a", 36), nil, []string{"ghp_" + strings.Repeat("a", 36)}},
		{"AKIAABCDEFGHIJKLMNOP", nil, []string{"AKIAABCDEFGHIJKLMNOP"}},

		// Not secrets.
		{"--token-file /etc/tok", nil, nil},
		{"--no-password mydb", nil, nil},
		{"--ask-pass host", nil, nil},
		{"-u 1000:1000 alpine", nil, nil},
		{"https://alice@example.com http://host:8080/path", nil, nil},
		{"-H 'Accept: application/json' -p 80", nil, nil},
		{"PWD=/tmp", nil, nil},
		{"-sV -p 1-1000 10.0.0.0/24", nil, nil},
	}
	for _, c := range cases {
		got := values(FindFreeTextSecrets(c.text, c.flags))
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%q: got %q, want %q", c.text, got, c.want)
		}
	}
}

func TestMaskFreeText(t *testing.T) {
	got := maskFreeText("clone https://alice:s3cr3t@host/r.git --password=s3cr3t", nil)
	if strings.Contains(got, "s3cr3t") || !strings.Contains(got, "alice:"+secretMask+"@host") {
		t.Fatalf("not masked: %s", got)
	}
}

func TestOutputMasker(t *testing.T) {
	m := newOutputMasker()
	m.add([]byte("hunter22"))
	m.add([]byte("p@ss word"))
	m.add([]byte("abc")) // too short to hide safely

	cases := map[string]string{
		"login with hunter22 ok":              "login with " + secretMask + " ok",
		"hunter22":                            secretMask,
		"xhunter22hunter22":                   "x" + secretMask,
		"url: p%40ss+word and p@ss%20word":    "url: " + secretMask + " and " + secretMask,
		"abc stays":                           "abc stays",
		"nothing here":                        "nothing here",
		"> Authorization: Basic dXNlcjpwYXNz": "> Authorization: Basic " + secretMask,
		"key AKIAABCDEFGHIJKLMNOP used":       "key " + secretMask + " used",
	}
	for in, want := range cases {
		if got := m.Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
	// The masker keeps fingerprints, not the secrets.
	if dump := fmt.Sprintf("%+v %+v %+v %+v", m.salt, m.base, m.byLen, m.lens); strings.Contains(dump, "hunter22") {
		t.Fatal("masker holds the secret in plain text")
	}
	// A nil masker still hides well-known token formats.
	var none *outputMasker
	if got := none.Redact("glpat-" + strings.Repeat("x", 20)); got != secretMask {
		t.Fatalf("nil masker: %q", got)
	}
}

// A secret the program prints, from the store or typed into Extra
// arguments, never reaches the screen, the saved output or history.
func TestSecretsHiddenInOutputAndRecords(t *testing.T) {
	bin, log := fakeTool(t, "dbtool", `echo "connecting with $DB_PASSWORD"; echo "extra: $*"`)
	a := newTestApp(t, ToolSpec{Name: "dbtool", Bin: bin, KeepHistory: true, Flags: []FlagSpec{
		{Flag: "--password", Kind: FlagSecret, SecretEnv: "DB_PASSWORD", Label: "Password"},
	}})
	defer a.Close()
	if err := a.SetSecret("--password", "store-secret-1"); err != nil {
		t.Fatal(err)
	}
	a.ExtraArgs = "--url https://bob:typed-secret-2@db.example"
	a.SaveToFile = true

	if found := a.FreeTextSecrets(); len(found) != 1 || found[0].Value != "typed-secret-2" {
		t.Fatalf("free-text secret not found: %+v", found)
	}
	if p := a.CommandPreview(); strings.Contains(p, "typed-secret-2") {
		t.Fatalf("preview shows the secret: %s", p)
	}
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	// The program itself received the real value.
	if !strings.Contains(runs(log), "typed-secret-2") {
		t.Fatalf("program did not get its argument: %s", runs(log))
	}

	var shown []string
	for _, l := range job.Lines() {
		shown = append(shown, l.Text)
	}
	out := strings.Join(shown, "\n")
	if strings.Contains(out, "store-secret-1") || strings.Contains(out, "typed-secret-2") || !strings.Contains(out, secretMask) {
		t.Fatalf("output not masked:\n%s", out)
	}

	// Wait for OnExit to write history and the saved output.
	for i := 0; i < 200 && len(a.History()) == 0; i++ {
		job.Wait()
	}
	files := []string{a.settingsPath(), a.historyPath()}
	if h := a.History(); len(h) > 0 && h[0].OutputFile != "" {
		files = append(files, h[0].OutputFile)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if strings.Contains(string(b), "store-secret-1") || strings.Contains(string(b), "typed-secret-2") {
			t.Fatalf("%s contains a secret:\n%s", f, b)
		}
	}

	// Next session: the hidden value has to be typed again, rather than
	// running with "••••••" as the password.
	b := newTestApp(t, ToolSpec{Name: "dbtool", Bin: bin})
	b.ConfigDir = a.ConfigDir
	if err := b.LoadSettings(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.StartJob(); err == nil || !strings.Contains(err.Error(), "type it again") {
		t.Fatalf("ran with a hidden placeholder: %v", err)
	}
}
