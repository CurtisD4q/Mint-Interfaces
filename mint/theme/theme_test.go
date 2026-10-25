package theme

import "testing"

func TestFromSpec(t *testing.T) {
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
