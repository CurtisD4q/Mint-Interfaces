package mint

import "testing"

func TestHistoryNav(t *testing.T) {
	h := newHistoryNav([]string{"one", "two"})
	cur := "typing"
	steps := []struct {
		d    int
		want string
		ok   bool
	}{
		{+1, "typing", false}, // nothing newer than the line being typed
		{-1, "two", true},
		{-1, "one", true},
		{-1, "one", false}, // nothing older
		{+1, "two", true},
		{+1, "typing", true}, // back to what was being typed
	}
	for i, s := range steps {
		got, ok := h.step(s.d, cur)
		if got != s.want || ok != s.ok {
			t.Fatalf("step %d: %q %v, want %q %v", i, got, ok, s.want, s.ok)
		}
		cur = got
	}
	if got, ok := newHistoryNav(nil).step(-1, "x"); got != "x" || ok {
		t.Fatalf("empty history: %q %v", got, ok)
	}
}
