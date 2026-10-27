package appkit

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/reorder"
)

// fakeUFW is ufw in memory: "show added", adding and deleting rules.
type fakeUFW struct {
	mu      sync.Mutex
	rules   []string
	failOn  string // an added rule that fails, once (always, with failAll)
	failAll bool
}

func (f *fakeUFW) run(_ context.Context, s reorder.Step) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.Name != "ufw" {
		return "", errors.New("not ufw")
	}
	join := func(w []string) string {
		out := make([]string, len(w))
		for i, x := range w {
			out[i] = x
			if strings.Contains(x, " ") {
				out[i] = "'" + x + "'"
			}
		}
		return strings.Join(out, " ")
	}
	switch {
	case s.Args[0] == "show":
		return "Added user rules (see 'ufw status' for running firewall):\nufw " + strings.Join(f.rules, "\nufw ") + "\n", nil
	case s.Args[0] == "--force":
		spec := join(s.Args[2:])
		for i, r := range f.rules {
			if r == spec {
				f.rules = append(f.rules[:i], f.rules[i+1:]...)
				return "", nil
			}
		}
		return "", errors.New("Could not delete non-existent rule")
	default:
		spec := join(s.Args)
		if spec == f.failOn {
			if !f.failAll {
				f.failOn = ""
			}
			return "", errors.New("ERROR: injected")
		}
		f.rules = append(f.rules, spec)
		return "", nil
	}
}

func (f *fakeUFW) order() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.rules, " | ")
}

func reorderApp(t *testing.T) (*ToolApp, *fakeUFW) {
	t.Helper()
	f := &fakeUFW{rules: []string{"allow 22/tcp comment 'ssh in'", "deny from 10.0.0.5", "allow 80/tcp", "allow 443/tcp"}}
	a := newTestApp(t, ToolSpec{Name: "ufw", Reorder: &ReorderSpec{Adapter: "ufw"}})
	a.reorder().run = f.run
	if err := a.reorderLoad(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	return a, f
}

const ufwStart = "allow 22/tcp comment 'ssh in' | deny from 10.0.0.5 | allow 80/tcp | allow 443/tcp"

func TestReorderSpec(t *testing.T) {
	for bin, want := range map[string]string{"/usr/sbin/iptables": "iptables", "ip6tables-legacy": "ip6tables-legacy", "nft": "nft", "ufw": "ufw"} {
		if s, ok := KnownReorderSpec(bin); !ok || s.Adapter != want {
			t.Errorf("%s: %+v %v", bin, s, ok)
		}
	}
	if _, ok := KnownReorderSpec("ip"); ok {
		t.Error("ip has a reorder spec")
	}
	if err := (ToolSpec{Name: "ip", Reorder: &ReorderSpec{Adapter: "ip-route"}}).Validate(); err == nil || !strings.Contains(err.Error(), "metric") {
		t.Errorf("ip-route: %v", err)
	}
	if err := (ToolSpec{Name: "ufw", Reorder: &ReorderSpec{Adapter: "ufw", ConfirmWindow: time.Second}}).Validate(); err == nil {
		t.Error("1 s window accepted")
	}
}

func TestReorderKeep(t *testing.T) {
	a, f := reorderApp(t)
	if err := a.reorderMove(3, 0); err != nil {
		t.Fatal(err)
	}
	if err := a.reorderMove(9, 0); err == nil {
		t.Fatal("moved a rule that isn't there")
	}
	p, err := a.reorderPlan(context.Background())
	if err != nil || len(p.Steps) != 8 || p.Atomic {
		t.Fatalf("plan %d steps %v", len(p.Steps), err)
	}
	if f.order() != ufwStart {
		t.Fatal("planning changed the rules")
	}
	if err := a.reorderApply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "allow 443/tcp | allow 22/tcp comment 'ssh in' | deny from 10.0.0.5 | allow 80/tcp"
	if f.order() != want {
		t.Fatalf("applied: %s", f.order())
	}
	if left := a.reorderPending(); left < 25 || left > 30 {
		t.Fatalf("seconds to keep: %d", left)
	}
	if err := a.reorderApply(context.Background()); err == nil {
		t.Fatal("a second change before keeping the first")
	}
	if !a.reorderKeep() || a.reorderPending() != -1 {
		t.Fatal("not kept")
	}
	a.Close()
	if f.order() != want {
		t.Fatalf("closing undid a kept change: %s", f.order())
	}
	// The rules from before were saved, readable only by the user.
	p2 := a.reorder().pending
	st, err := os.Stat(p2.backup)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("backup %v %v", st, err)
	}
	b, _ := os.ReadFile(p2.backup)
	if !strings.Contains(string(b), "ufw allow 22/tcp comment 'ssh in'\nufw deny from 10.0.0.5\n") {
		t.Fatalf("backup:\n%s", b)
	}
}

func TestReorderRevert(t *testing.T) {
	a, f := reorderApp(t)
	a.reorderMove(0, 3)
	if err := a.reorderApply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.reorderRevert(context.Background(), "you asked"); err != nil {
		t.Fatal(err)
	}
	if f.order() != ufwStart || !strings.Contains(a.reorder().pending.result, "old order is back") {
		t.Fatalf("%s / %s", f.order(), a.reorder().pending.result)
	}
	if a.reorderKeep() {
		t.Fatal("kept after going back")
	}
}

// Not kept in time: the old order comes back by itself.
func TestReorderTimesOut(t *testing.T) {
	a, f := reorderApp(t)
	a.Spec.Reorder.ConfirmWindow = 50 * time.Millisecond
	a.reorderMove(1, 2)
	if err := a.reorderApply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.order() == ufwStart {
		t.Fatal("not applied")
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.order() != ufwStart && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.order() != ufwStart || a.reorderPending() != -1 {
		t.Fatalf("not put back: %s", f.order())
	}
}

// Quitting before keeping a change puts the old order back.
func TestReorderCloseReverts(t *testing.T) {
	a, f := reorderApp(t)
	a.reorderMove(2, 0)
	a.reorderApply(context.Background())
	a.Close()
	if f.order() != ufwStart {
		t.Fatalf("still changed after Close: %s", f.order())
	}
}

func TestReorderStaleAndFailure(t *testing.T) {
	a, f := reorderApp(t)
	a.reorderMove(3, 0)
	f.mu.Lock()
	f.rules = append(f.rules, "deny 23")
	f.mu.Unlock()
	if err := a.reorderApply(context.Background()); !errors.Is(err, reorder.ErrChanged) {
		t.Fatalf("stale: %v", err)
	}
	if f.order() != ufwStart+" | deny 23" || reorder.Moved(a.reorder().order) {
		t.Fatalf("after a stale apply: %s, staged %v", f.order(), a.reorder().order)
	}
	// A command failing halfway: the old order comes back.
	a.reorderMove(4, 0)
	f.failOn = "allow 22/tcp comment 'ssh in'"
	err := a.reorderApply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "old order is back") {
		t.Fatalf("failure: %v", err)
	}
	if f.order() != ufwStart+" | deny 23" {
		t.Fatalf("after a failure: %s", f.order())
	}
	if a.reorderPending() != -1 {
		t.Fatal("a failed change is waiting to be kept")
	}
	// Putting it back fails too: the error says where the copy is.
	a.reorderMove(4, 0)
	f.failOn, f.failAll = "allow 22/tcp comment 'ssh in'", true
	err = a.reorderApply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "putting the old order back also failed") || !strings.Contains(err.Error(), "rule-backups") {
		t.Fatalf("double failure: %v", err)
	}
}

func TestReorderScreenBuilds(t *testing.T) {
	a, _ := reorderApp(t)
	a.reorderMove(0, 1)
	scr := a.ReorderScreen()
	scr.Refresh(scr)
	var names []string
	for _, s := range scr.Sections {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "Chain,Rules, first to last (4),Apply" {
		t.Fatalf("sections %q", names)
	}
	a.reorderApply(context.Background())
	scr.Refresh(scr)
	if scr.Sections[0].Name != "Keep the new order?" {
		t.Fatalf("pending section: %q", scr.Sections[0].Name)
	}
	if b := scr.Status(); len(b) == 0 || !strings.HasPrefix(b[0].Text, "old order back in") {
		t.Fatalf("badge %+v", b)
	}
	a.reorderKeep()
}

func TestMovedRules(t *testing.T) {
	for _, c := range []struct {
		order []int
		want  string
	}{
		{[]int{3, 0, 1, 2}, "3"},
		{[]int{1, 2, 3, 0}, "0"},
		{[]int{0, 1, 2}, ""},
	} {
		m := movedRules(c.order)
		var got []string
		for i := range c.order {
			if m[i] {
				got = append(got, strconv.Itoa(i))
			}
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%v: moved %v, want %s", c.order, got, c.want)
		}
	}
	// A swap moves one of the two; reversed, all but one moved.
	if m := movedRules([]int{0, 2, 1, 3}); len(m) != 1 || !(m[1] || m[2]) {
		t.Errorf("swap: %v", m)
	}
	if m := movedRules([]int{3, 2, 1, 0}); len(m) != 3 {
		t.Errorf("reversed: %v", m)
	}
}
