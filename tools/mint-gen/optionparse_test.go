package main

import (
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func flagsOf(t *testing.T, name, man, help string) map[string]appkit.FlagSpec {
	t.Helper()
	out := map[string]appkit.FlagSpec{}
	for _, f := range ParseManAndHelp(name, man, help).Flags {
		out[f.Flag] = f
	}
	return out
}

// A placeholder that names a number makes a number, with no 0-100 cap;
// sizes, durations, limits and values with unit suffixes stay text.
func TestNumericPlaceholders(t *testing.T) {
	help := `Usage: tool9 [options]
Options:
  --max-delete=NUM      don't delete more than NUM files
  -c COUNT              stop after COUNT packets
  -p PORT               port to connect to
  --period-size=#       distance between interrupts is # frames
  -n requests           Number of requests to run
  -s N                  shred this many bytes (suffixes like K, M, G accepted)
  -M LIMIT              set a memory usage limit
  --block-size SIZE     number of bytes in a block
  -t duration           timeout in seconds, as a duration
  --mode {fast|slow}    number of passes, by name
`
	f := flagsOf(t, "tool9", "", help)
	for flag, want := range map[string]appkit.FlagKind{
		"--max-delete": appkit.FlagInt, "-c": appkit.FlagInt, "-p": appkit.FlagInt, "--period-size": appkit.FlagInt,
		"-n": appkit.FlagInt, "-s": appkit.FlagText, "-M": appkit.FlagText, "--block-size": appkit.FlagText,
		"-t": appkit.FlagText, "--mode": appkit.FlagChoice,
	} {
		got, ok := f[flag]
		if !ok {
			t.Errorf("%s not parsed", flag)
			continue
		}
		if got.Kind != want {
			t.Errorf("%s: kind %s, want %s", flag, got.Kind, want)
		}
		if got.Kind == appkit.FlagInt && (got.Min != 0 || got.Max != 0) {
			t.Errorf("%s: range %d-%d, want open", flag, got.Min, got.Max)
		}
	}
}

// find's numeric tests take "+n" and "-n", which a number stepper cannot
// give, so they stay text when the help explains them.
func TestSignedNStaysText(t *testing.T) {
	man := "NAME\n       find9 - search\n\nTESTS\n       +n     for greater than n,\n\n       -n     for less than n,\n\n       -atime n\n              File was last accessed n days ago.\n"
	if f := flagsOf(t, "find9", man, "")["-atime"]; f.Kind != appkit.FlagText {
		t.Errorf("-atime: kind %s, want text", f.Kind)
	}
}

// Option lines the other patterns missed: single-dash long names, names
// without a comma or with "|" or "or", a colon after the name, a typed
// value in its own column, and find's -exec. A sentence that starts with
// an option name gives nothing.
func TestGenericOptionLines(t *testing.T) {
	for _, c := range []struct {
		line, want string // want: names:placeholder, or "" for none
	}{
		{"       -delete", "-delete:"},
		{"       -exec command ;", "-exec:command ;"},
		{"   -d --decompress     force decompression", "-d,--decompress:"},
		{"   -v|--verbose      increase verbosity", "-v,--verbose:"},
		{"  --interface=<s> or -I <s>", "-I,--interface:=<s>"},
		{"  -sL: List Scan - simply list targets to scan", "-sL:"},
		{"  -iR <num hosts>: Choose random targets", "-iR:<num hosts>"},
		{"  -colors            <string>     ED..V....... set the colors", "-colors:<string>"},
		{"  -sdel : delete files after compression", "-sdel:"},
		{"       --fs to get an overview of filesystems, or findmnt(8) to search", ""},
		{"  -size n[cwbkMG]", "-size:n[cwbkMG]"},
	} {
		got := ""
		if os := scanOptionLine(c.line); len(os) > 0 {
			got = joinNames(os[0].Names) + ":" + os[0].Placeholder.Raw
		}
		if got != c.want {
			t.Errorf("%q: got %q, want %q", c.line, got, c.want)
		}
	}
}

func joinNames(n []string) string {
	s := ""
	for i, x := range n {
		if i > 0 {
			s += ","
		}
		s += x
	}
	return s
}

// A one-letter placeholder is not a path, whatever the description says;
// F is (--files0-from=F).
func TestOneLetterPlaceholders(t *testing.T) {
	man := "NAME\n       tool9 - x\n\nOPTIONS\n       -type c\n              File is of type c.\n\n       --files0-from=F\n              read input from the files specified by names in file F\n"
	f := flagsOf(t, "tool9", man, "")
	if f["-type"].Kind != appkit.FlagText {
		t.Errorf("-type: kind %s, want text", f["-type"].Kind)
	}
	if f["--files0-from"].Kind != appkit.FlagFile {
		t.Errorf("--files0-from: kind %s, want file", f["--files0-from"].Kind)
	}
}

// A password stays a secret when the description mentions files, and
// "pass phrase" counts; a password file stays a file.
func TestPasswordsStaySecret(t *testing.T) {
	help := `Usage: tool9 [options]
  -upw <string>        : user password (for encrypted files)
  --pass <phrase> Pass phrase for the private key
  --password-file FILE  read the password from FILE
`
	f := flagsOf(t, "tool9", "", help)
	for flag, want := range map[string]appkit.FlagKind{"-upw": appkit.FlagSecret, "--pass": appkit.FlagSecret, "--password-file": appkit.FlagFile} {
		if f[flag].Kind != want {
			t.Errorf("%s: kind %s, want %s", flag, f[flag].Kind, want)
		}
	}
}

// An environment password is not attached to an option that also holds a
// user name (lftp's "-u user[,pass]"): it stays an entry of its own.
func TestEnvSecretNotMergedIntoUserAndPassword(t *testing.T) {
	spec := appkit.ToolSpec{Name: "lftp9", Flags: []appkit.FlagSpec{
		{Flag: "-u", Kind: appkit.FlagSecret, Label: "Use the given username and password", Hint: "user[,pass"},
	}}
	attachEnvSecrets(&spec, []string{"LFTP_PASSWORD"})
	if spec.Flags[0].SecretEnv != "" {
		t.Errorf("-u took the variable: %+v", spec.Flags[0])
	}
	if len(spec.Flags) != 2 || spec.Flags[1].SecretEnv != "LFTP_PASSWORD" {
		t.Errorf("variable entry: %+v", spec.Flags)
	}
}
