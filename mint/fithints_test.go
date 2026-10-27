package mint

import "testing"

func TestFitHints(t *testing.T) {
	keys := " ↑↓ scroll   End follow   i command   c cancel   b close "
	if got := fitHints(keys, 80); got != keys {
		t.Fatalf("fits already: %q", got)
	}
	for _, c := range []struct {
		room int
		want string
	}{
		{45, " End follow   i command   c cancel   b close "},
		{32, " i command   c cancel   b close "},
		{9, " b close "},
		{0, ""},
	} {
		got := fitHints(keys, c.room)
		if got != c.want {
			t.Errorf("room %d: %q, want %q", c.room, got, c.want)
		}
		if VLen(got) > c.room {
			t.Errorf("room %d: %q is %d cells", c.room, got, VLen(got))
		}
	}
	// Nothing fits whole: the last hint, cut short.
	if got := fitHints(keys, 4); VLen(got) > 4 || got[:2] != " b" {
		t.Errorf("room 4: %q", got)
	}
}
