package appkit

import (
	"strings"
	"testing"
)

// A number is sent as set, 0 included: 0 often means something (hey -t 0
// is no timeout). "Off", below the lowest value, is empty and sends
// nothing, so cal's -n can still be dropped to use -y alone.
func TestZeroIsSentAndOffIsNot(t *testing.T) {
	a := newTestApp(t, ToolSpec{Name: "cal", Bin: "cal", Flags: []FlagSpec{
		{Flag: "-n", Kind: FlagInt, Label: "Months", Min: 0, Max: 12},
		{Flag: "-y", Kind: FlagBool, Label: "Year"},
	}})
	if args, _ := a.BuildArgs(); strings.Contains(strings.Join(args, " "), "-n") {
		t.Errorf("untouched number sent: %v", args)
	}
	a.Values["-n"] = "0"
	if args, _ := a.BuildArgs(); !strings.Contains(strings.Join(args, " "), "-n 0") {
		t.Errorf("0 not sent: %v", args)
	}
	a.Values["-n"] = "4"
	if args, _ := a.BuildArgs(); !strings.Contains(strings.Join(args, " "), "-n 4") {
		t.Errorf("4 not sent: %v", args)
	}
	// Stepped to off, with the year on: only -y goes.
	a.Values["-n"] = ""
	a.Values["-y"] = "true"
	if args, _ := a.BuildArgs(); strings.Join(args, " ") != "-y" {
		t.Errorf("off and year: %v", args)
	}
}

// The stepper's "off" sits one step below the lowest value, or below 0
// for an unbounded number.
func TestIntStepperRangeHasOff(t *testing.T) {
	for _, c := range []struct{ min, max, step, lo, hi, off int }{
		{0, 12, 1, -1, 12, -1},
		{1, 10, 1, 0, 10, 0},
		{5, 50, 5, 0, 50, 0},
		{0, 0, 1, -1, 1 << 30, -1},
	} {
		lo, hi, off := intStepperRange(c.min, c.max, c.step)
		if lo != c.lo || hi != c.hi || off != c.off {
			t.Errorf("range(%d,%d,%d) = %d,%d,%d, want %d,%d,%d", c.min, c.max, c.step, lo, hi, off, c.lo, c.hi, c.off)
		}
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
