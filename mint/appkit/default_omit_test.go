package appkit

import (
	"strings"
	"testing"
)

// cal's -n has no Default from mint-gen, so a slider left at 0 was sending
// "-n 0": the tool reads that as a real limit, and months and a year could
// not be asked for at once.
func TestZeroSliderOmitsTheFlag(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "cal", Bin: "cal", Flags: []FlagSpec{
		{Flag: "-n", Kind: FlagInt, Label: "Months", Min: 0, Max: 12},
		{Flag: "-y", Kind: FlagBool, Label: "Year"},
	}})

	a.Values["-n"] = "0"
	if args, _ := a.BuildArgs(); strings.Contains(strings.Join(args, " "), "-n") {
		t.Errorf("slider at 0 still sent: %v", args)
	}

	// Set to 4, it is sent.
	a.Values["-n"] = "4"
	if args, _ := a.BuildArgs(); !strings.Contains(strings.Join(args, " "), "-n 4") {
		t.Errorf("value 4 not sent: %v", args)
	}

	// Back to 0 with the year on: only -y goes.
	a.Values["-n"] = "0"
	a.Values["-y"] = "true"
	args, _ := a.BuildArgs()
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-n") {
		t.Errorf("slider back at 0 still sent: %v", args)
	}
	if !strings.Contains(joined, "-y") {
		t.Errorf("year flag missing: %v", args)
	}
}

// A declared Default is skipped too, for every kind of flag.
func TestDeclaredDefaultOmitsTheFlag(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "x", Bin: "x", Flags: []FlagSpec{
		{Flag: "--mode", Kind: FlagText, Label: "Mode", Default: "auto"},
		{Flag: "--depth", Kind: FlagInt, Label: "Depth", Default: "3", Min: 0, Max: 9},
	}})
	if args, _ := a.BuildArgs(); len(args) != 0 {
		t.Errorf("defaults were sent: %v", args)
	}
	a.Values["--mode"] = "manual"
	a.Values["--depth"] = "5"
	joined := strings.Join(func() []string { args, _ := a.BuildArgs(); return args }(), " ")
	if !strings.Contains(joined, "--mode manual") || !strings.Contains(joined, "--depth 5") {
		t.Errorf("changed values not sent: %v", joined)
	}
}
