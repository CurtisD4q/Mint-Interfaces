package theme

import (
	"strings"
	"testing"
)

func TestFromSpec(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	base := Load("default")
	p, err := FromSpec(base, "accent=#ff0000, text=250 ,selbg=#000102,err=#00ff00")
	if err != nil {
		t.Fatal(err)
	}
	if p.Accent != "\x1b[38;2;255;0;0m" || p.Text != "\x1b[38;5;250m" ||
		p.SelBg != "\x1b[48;2;0;1;2m" || p.Err != "\x1b[38;2;0;255;0m" {
		t.Fatalf("got %+v", p)
	}
	if p.Muted != base.Muted || p.Bold != base.Bold {
		t.Error("unset roles should keep the base palette")
	}
	for _, bad := range []string{"accent", "accent=#zzzzzz", "text=300", "nope=#000000"} {
		if q, err := FromSpec(base, bad); err == nil || q != base {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestTrueColorGatedOnColorTerm(t *testing.T) {
	t.Setenv("COLORTERM", "")
	nord := Load("nord")
	for _, seq := range []string{nord.Accent, nord.Bright, nord.Text, nord.Muted, nord.On, nord.Amber, nord.SelBg, nord.Err} {
		if strings.Contains(seq, "38;2;") || strings.Contains(seq, "48;2;") {
			t.Fatalf("expected 256-colour fallback when COLORTERM is unset, got %q", seq)
		}
		if !strings.Contains(seq, "38;5;") && !strings.Contains(seq, "48;5;") {
			t.Fatalf("expected 256-colour SGR when COLORTERM is unset, got %q", seq)
		}
	}

	specPal, err := FromSpec(Load("default"), "accent=#88c0d0,selbg=#3b4252")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(specPal.Accent, "38;2;") || strings.Contains(specPal.SelBg, "48;2;") {
		t.Fatalf("expected FromSpec #rrggbb to fall back to 256-colour when COLORTERM is unset: %+v", specPal)
	}

	t.Setenv("COLORTERM", "24bit")
	nord24 := Load("nord")
	if !strings.Contains(nord24.Accent, "38;2;") || !strings.Contains(nord24.SelBg, "48;2;") {
		t.Fatalf("expected 24-bit SGR when COLORTERM=24bit, got Accent=%q SelBg=%q", nord24.Accent, nord24.SelBg)
	}
}
