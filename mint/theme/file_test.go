package theme

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func withFile(t *testing.T, body string) string {
	t.Helper()
	old, oldDef := ConfigPath, defaultConfigPath
	p := filepath.Join(t.TempDir(), "theme.conf")
	ConfigPath = p
	t.Cleanup(func() { ConfigPath, defaultConfigPath = old, oldDef })
	if body != "" {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestThemeFileIsReadForgivingly(t *testing.T) {
	for _, body := range []string{
		"theme = nord", "theme=nord", "theme = \"nord\"", "theme = 'nord'", "theme = Nord",
		"Theme = nord", "THEME: NORD", "theme: nord", "nord", "  Nord  ", "# a comment\ntheme = nord\n",
	} {
		withFile(t, body+"\n")
		if got := Current(); got != "nord" {
			t.Errorf("%q: theme %q, want nord", body, got)
		}
		if Load("") != Load("nord") {
			t.Errorf("%q: palette is not nord", body)
		}
		if p := Problem(); p != "" {
			t.Errorf("%q: problem %q", body, p)
		}
	}
	if Load("NORD") != Load("nord") {
		t.Error("Load is case-sensitive")
	}
}

func TestThemeFileProblems(t *testing.T) {
	withFile(t, "theme = solarized\n")
	if Current() != "default" || !strings.Contains(Problem(), `"solarized"`) {
		t.Fatalf("unknown theme: current %q, problem %q", Current(), Problem())
	}
	withFile(t, "")
	if Current() != "default" || Problem() != "" {
		t.Fatal("no file should be the default with no problem")
	}
	// A plain word that is not a theme is not a theme line.
	withFile(t, "hello\n")
	if Current() != "default" || Problem() != "" {
		t.Fatalf("a stray word: current %q, problem %q", Current(), Problem())
	}
}

func TestCustomRolesAnyCase(t *testing.T) {
	withFile(t, "theme = custom\naccent = 200 10 10\ntext = 10 200 10\n")
	want := Load("")
	withFile(t, "theme = custom\nAccent = 200 10 10\nTEXT: 10 200 10\n")
	if got := Load(""); got != want {
		t.Fatalf("roles in capitals or with a colon were not read: %+v", got)
	}
	withFile(t, "theme = custom\n")
	if Load("") == want {
		t.Fatal("the test colours are the fallback colours")
	}
}

// Run as root through sudo with no theme file of root's own, Mint reads
// the theme file of the user who ran sudo. Only a regular file owned by
// that user (or root) counts, never a link.
func TestSudoReadsTheUsersThemeFile(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs to run as root")
	}
	home := t.TempDir()
	userFile := filepath.Join(home, ".config", "mint-interfaces", "theme.conf")
	if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userFile, []byte("theme = dracula\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldLookup := lookupUser
	t.Cleanup(func() { lookupUser = oldLookup })
	lookupUser = func(name string) (*user.User, error) {
		return &user.User{Username: name, Uid: strconv.Itoa(os.Getuid()), HomeDir: home}, nil
	}
	t.Setenv("SUDO_USER", "alex")

	rootFile := withFile(t, "")
	defaultConfigPath = ConfigPath // as if root's own default path
	if ReadPath() != userFile || Current() != "dracula" {
		t.Fatalf("read %q (%q), want the user's dracula", ReadPath(), Current())
	}
	// Saving writes root's own file, never the user's.
	if err := Set("nord"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(userFile); !strings.Contains(string(b), "dracula") {
		t.Fatal("Set wrote the user's file")
	}
	if ReadPath() != rootFile || Current() != "nord" {
		t.Fatalf("after saving, read %q (%q)", ReadPath(), Current())
	}

	// A link is not followed.
	os.Remove(rootFile)
	real := filepath.Join(t.TempDir(), "elsewhere.conf")
	os.WriteFile(real, []byte("theme = gruvbox\n"), 0o644)
	os.Remove(userFile)
	if err := os.Symlink(real, userFile); err != nil {
		t.Fatal(err)
	}
	if ReadPath() != rootFile || Current() != "default" {
		t.Fatalf("followed a link: %q (%q)", ReadPath(), Current())
	}

	// Without sudo, nothing changes.
	t.Setenv("SUDO_USER", "")
	if ReadPath() != rootFile {
		t.Fatal("read another file without sudo")
	}
}
