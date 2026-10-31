package appkit

import (
	"strings"
	"testing"
)

// A flag left at its default is not passed, so a slider at 0 does not send
// "--months 0" and block the flag from ever being dropped.
func TestDefaultValueOmitsTheFlag(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "cal", Bin: "cal", Flags: []FlagSpec{
		{Flag: "-n", Kind: FlagInt, Label: "Months", Default: "0", Min: 0, Max: 12},
		{Flag: "-y", Kind: FlagBool, Label: "Year"},
	}})

	args, _ := a.BuildArgs()
	if strings.Contains(strings.Join(args, " "), "-n") {
		t.Errorf("default slider value still sent: %v", args)
	}

	// Changed away from the default, it is sent.
	a.Values["-n"] = "4"
	args, _ = a.BuildArgs()
	if !strings.Contains(strings.Join(args, " "), "-n 4") {
		t.Errorf("changed value not sent: %v", args)
	}

	// Back at the default, it is dropped again, so -y can be used alone.
	a.Values["-n"] = "0"
	a.Values["-y"] = "true"
	args, _ = a.BuildArgs()
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-n") {
		t.Errorf("slider back at 0 still sent: %v", args)
	}
	if !strings.Contains(joined, "-y") {
		t.Errorf("year flag missing: %v", args)
	}
}

// A flag with no default is unaffected: an empty value is already skipped,
// and a real value is still sent.
func TestFlagWithoutDefaultUnchanged(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "x", Bin: "x", Flags: []FlagSpec{
		{Flag: "--name", Kind: FlagText, Label: "Name"},
	}})
	if args, _ := a.BuildArgs(); strings.Contains(strings.Join(args, " "), "--name") {
		t.Errorf("empty flag sent: %v", args)
	}
	a.Values["--name"] = "bob"
	if args, _ := a.BuildArgs(); !strings.Contains(strings.Join(args, " "), "--name bob") {
		t.Errorf("value not sent: %v", args)
	}
}
