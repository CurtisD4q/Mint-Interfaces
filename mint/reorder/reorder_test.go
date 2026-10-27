package reorder

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

var ctx = context.Background()

// setup returns a fake holding n rules for the adapter, and the scope.
func setup(name string, n int) (*fake, Adapter, string) {
	f := &fake{tables: map[string][]string{}}
	switch name {
	case "iptables":
		lines := []string{":INPUT ACCEPT [0:0]", ":FORWARD ACCEPT [0:0]", ":OUTPUT ACCEPT [0:0]", ":MINE - [0:0]"}
		for i := 0; i < n; i++ {
			lines = append(lines, fmt.Sprintf(`[%d:%d] -A INPUT -s 10.0.0.%d/32 -m comment --comment "rule %d" -j DROP`, i, i*60, i, i))
			if i == 1 {
				lines = append(lines, "[5:5] -A OUTPUT -j ACCEPT", "[0:0] -A MINE -j RETURN")
			}
		}
		f.tables["filter"] = lines
		f.tables["nat"] = []string{":POSTROUTING ACCEPT [0:0]", "[0:0] -A POSTROUTING -o eth0 -j MASQUERADE"}
		return f, IPTables{Bin: "iptables"}, "filter INPUT"
	case "nft":
		for i := 0; i < n; i++ {
			f.nft = append(f.nft, map[string]any{"family": "inet", "table": "t", "chain": "c",
				"comment": fmt.Sprintf("rule %d; flush ruleset", i),
				"expr":    []any{map[string]any{"counter": map[string]any{"packets": i, "bytes": i * 100}}, map[string]any{"accept": nil}}})
		}
		return f, NFT{}, "inet t c"
	case "ufw":
		for i := 0; i < n; i++ {
			if i%3 == 2 {
				f.ufw = append(f.ufw, fmt.Sprintf("route allow in on eth0 out on eth%d", i))
			} else {
				f.ufw = append(f.ufw, fmt.Sprintf("allow %d/tcp comment 'rule %d'", 1000+i, i))
			}
		}
		return f, UFW{}, "rules"
	case "ip-rule":
		f.rules = []prefRule{{0, "from all lookup local"}, {32766, "from all lookup main"}, {32767, "from all lookup default"}}
		for i := 0; i < n; i++ {
			f.rules = append(f.rules, prefRule{100 + 10*i, fmt.Sprintf("from 10.%d.0.0/16 lookup %d", i, 100+i)})
		}
		return f, IPRule{}, "-4"
	}
	panic(name)
}

var adapters = []string{"iptables", "nft", "ufw", "ip-rule"}

// randomOrder returns a permutation that leaves fixed rules in place.
func randomOrder(rng *rand.Rand, l List) []int {
	var free []int
	for i, r := range l.Rules {
		if !r.Fixed {
			free = append(free, i)
		}
	}
	order := make([]int, len(l.Rules))
	for i := range order {
		order[i] = i
	}
	perm := rng.Perm(len(free))
	for k, i := range free {
		order[i] = free[perm[k]]
	}
	return order
}

func expect(t *testing.T, before List, order []int, got List) {
	t.Helper()
	for i, j := range order {
		if got.Rules[i].Key != before.Rules[j].Key {
			t.Fatalf("position %d: %q, want %q\nall: %q", i, got.Rules[i].Text, before.Rules[j].Text, got.Keys())
		}
	}
}

// Any order of up to 12 rules ends up as planned, for every tool.
func TestRandomOrders(t *testing.T) {
	for _, name := range adapters {
		rng := rand.New(rand.NewSource(1))
		for trial := 0; trial < 150; trial++ {
			f, ad, scope := setup(name, 1+rng.Intn(12))
			before, err := ad.List(ctx, f.run, scope)
			if err != nil {
				t.Fatal(name, err)
			}
			order := randomOrder(rng, before)
			_, p, err := Prepare(ctx, ad, f.run, before, order)
			if err != nil {
				t.Fatal(name, err)
			}
			if err := Apply(ctx, ad, f.run, before, p); err != nil {
				t.Fatalf("%s trial %d: %v", name, trial, err)
			}
			got, _ := ad.List(ctx, f.run, scope)
			expect(t, before, order, got)
			// And back.
			if err := Restore(ctx, ad, f.run, before); err != nil {
				t.Fatal(name, err)
			}
			got, _ = ad.List(ctx, f.run, scope)
			expect(t, before, identity(len(order)), got)
		}
	}
}

// swapFree reverses the movable rules, so that the change touches every
// one of them.
func swapFree(l List) []int {
	var free []int
	for i, r := range l.Rules {
		if !r.Fixed {
			free = append(free, i)
		}
	}
	order := identity(len(l.Rules))
	for k, i := range free {
		order[i] = free[len(free)-1-k]
	}
	return order
}

func identity(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}

// A command failing at any point leaves the original order.
func TestFailureRestores(t *testing.T) {
	for _, name := range adapters {
		for failAt := 1; failAt <= 14; failAt++ {
			f, ad, scope := setup(name, 6)
			before, _ := ad.List(ctx, f.run, scope)
			order := swapFree(before)
			_, p, err := Prepare(ctx, ad, f.run, before, order)
			if err != nil {
				t.Fatal(err)
			}
			f.failAt = failAt
			err = Apply(ctx, ad, f.run, before, p)
			if f.writes >= failAt && (err == nil || !strings.Contains(err.Error(), "old order is back")) {
				t.Fatalf("%s fail at %d: %v", name, failAt, err)
			}
			got, _ := ad.List(ctx, f.run, scope)
			if err != nil {
				expect(t, before, identity(len(order)), got)
			} else {
				expect(t, before, order, got)
			}
		}
	}
}

// Someone else changed the rules between reading and applying.
func TestStale(t *testing.T) {
	for _, name := range adapters {
		f, ad, scope := setup(name, 4)
		before, _ := ad.List(ctx, f.run, scope)
		switch name {
		case "iptables":
			f.tables["filter"] = append(f.tables["filter"], "[0:0] -A INPUT -j ACCEPT")
		case "nft":
			f.nft = f.nft[1:]
		case "ufw":
			f.ufw = append(f.ufw, "deny 23")
		case "ip-rule":
			f.rules = append(f.rules, prefRule{105, "from 1.2.3.4 lookup 9"})
		}
		writes := f.writes
		if _, _, err := Prepare(ctx, ad, f.run, before, swapFree(before)); !errors.Is(err, ErrChanged) {
			t.Fatalf("%s: %v", name, err)
		}
		if f.writes != writes {
			t.Fatalf("%s: wrote after noticing a change", name)
		}
	}
}

// Counters going up isn't a change, and isn't wound back.
func TestCountersAreNotChanges(t *testing.T) {
	f, ad, scope := setup("iptables", 3)
	before, _ := ad.List(ctx, f.run, scope)
	f.tables["filter"][4] = strings.Replace(f.tables["filter"][4], "[0:0]", "[999:888]", 1)
	_, p, err := Prepare(ctx, ad, f.run, before, []int{2, 1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, ad, f.run, before, p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.tables["filter"], "\n"), "[999:888]") {
		t.Fatal("counters wound back")
	}
	// Other chains and tables are untouched.
	if !strings.Contains(strings.Join(f.tables["filter"], "\n"), "-A OUTPUT -j ACCEPT") || len(f.tables["nat"]) != 2 {
		t.Fatalf("other rules changed: %q", f.tables)
	}

	f2, ad2, scope2 := setup("nft", 3)
	before, _ = ad2.List(ctx, f2.run, scope2)
	f2.nft[0]["expr"] = []any{map[string]any{"counter": map[string]any{"packets": 7, "bytes": 7}}, map[string]any{"accept": nil}}
	if _, _, err := Prepare(ctx, ad2, f2.run, before, []int{2, 1, 0}); err != nil {
		t.Fatalf("nft counter change counted: %v", err)
	}
}

func TestFixedAndPermute(t *testing.T) {
	f, ad, scope := setup("ip-rule", 2)
	before, _ := ad.List(ctx, f.run, scope)
	if !before.Rules[0].Fixed || before.Rules[1].Fixed || !before.Rules[3].Fixed {
		t.Fatalf("fixed: %+v", before.Rules)
	}
	if _, err := Permute(before, []int{1, 0, 2, 3, 4}); err == nil {
		t.Fatal("moved local")
	}
	if _, err := Permute(before, []int{0, 1, 1, 3, 4}); err == nil {
		t.Fatal("duplicate index accepted")
	}
	if _, err := Permute(before, []int{0, 1}); err == nil {
		t.Fatal("short order accepted")
	}
	// Two rules sharing a priority can't be told apart by priority.
	f.rules = append(f.rules, prefRule{100, "from 9.9.9.9 lookup 9"})
	before, _ = ad.List(ctx, f.run, scope)
	if _, _, err := Prepare(ctx, ad, f.run, before, []int{0, 2, 1, 3, 4, 5}); err == nil || !strings.Contains(err.Error(), "share priority 100") {
		t.Fatalf("shared priority: %v", err)
	}
}

func TestPlans(t *testing.T) {
	// ufw: rules before the first move are left alone; route rules are
	// deleted with "route delete"; comments stay one argument.
	f, ad, scope := setup("ufw", 4)
	before, _ := ad.List(ctx, f.run, scope)
	_, p, _ := Prepare(ctx, ad, f.run, before, []int{0, 1, 3, 2})
	var got []string
	for _, s := range p.Steps {
		got = append(got, s.String())
	}
	want := []string{
		"ufw --force delete allow 1003/tcp comment 'rule 3'",
		"ufw --force route delete allow in on eth0 out on eth2",
		"ufw allow 1003/tcp comment 'rule 3'",
		"ufw route allow in on eth0 out on eth2",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || p.Atomic {
		t.Fatalf("ufw plan:\n%s", strings.Join(got, "\n"))
	}
	// ip rule: added at the new priority before deleted at the old.
	f, ad, scope = setup("ip-rule", 2)
	before, _ = ad.List(ctx, f.run, scope)
	_, p, _ = Prepare(ctx, ad, f.run, before, []int{0, 2, 1, 3, 4})
	got = nil
	for _, s := range p.Steps {
		got = append(got, s.String())
	}
	want = []string{
		"ip -4 rule add pref 100 from 10.1.0.0/16 lookup 101",
		"ip -4 rule add pref 110 from 10.0.0.0/16 lookup 100",
		"ip -4 rule del pref 100 from 10.0.0.0/16 lookup 100",
		"ip -4 rule del pref 110 from 10.1.0.0/16 lookup 101",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ip rule plan:\n%s", strings.Join(got, "\n"))
	}
	// iptables and nft: a check, then one transaction.
	for _, name := range []string{"iptables", "nft"} {
		f, ad, scope := setup(name, 3)
		before, _ := ad.List(ctx, f.run, scope)
		_, p, _ := Prepare(ctx, ad, f.run, before, []int{2, 1, 0})
		if !p.Atomic || len(p.Steps) != 2 || !p.Steps[0].Check || p.Steps[1].Check {
			t.Fatalf("%s plan: %+v", name, p.Steps)
		}
	}
}

func TestIPRouteRefused(t *testing.T) {
	if _, err := For("ip-route"); !errors.Is(err, ErrNoOrder) || !strings.Contains(err.Error(), "longest prefix") {
		t.Fatalf("ip-route: %v", err)
	}
	for _, n := range []string{"iptables", "ip6tables-nft", "iptables-legacy", "nft", "ufw", "ip-rule"} {
		if _, err := For(n); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	if _, err := For("iptables-evil"); err == nil {
		t.Fatal("iptables-evil accepted")
	}
}

func TestScopes(t *testing.T) {
	f, _, _ := setup("iptables", 2)
	s, _ := IPTables{Bin: "iptables"}.Scopes(ctx, f.run)
	if strings.Join(s, ",") != "filter INPUT,filter FORWARD,filter OUTPUT,filter MINE,nat POSTROUTING" {
		t.Fatalf("iptables scopes %q", s)
	}
	if _, err := (IPTables{Bin: "iptables"}).List(ctx, f.run, "filter INPUT; rm"); err == nil {
		t.Fatal("bad scope accepted")
	}
	if _, err := (NFT{}).List(ctx, f.run, "inet t c extra"); err == nil {
		t.Fatal("bad nft scope accepted")
	}
	s, _ = NFT{}.Scopes(ctx, f.run)
	if strings.Join(s, ",") != "inet t c" {
		t.Fatalf("nft scopes %q", s)
	}
}

func TestSplitWords(t *testing.T) {
	got, err := splitWords(`allow 22/tcp comment 'ssh in' "x y" ''`)
	if err != nil || strings.Join(got, "|") != "allow|22/tcp|comment|ssh in|x y|" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := splitWords(`allow comment 'open`); err == nil {
		t.Fatal("unbalanced quote accepted")
	}
}

func TestStepString(t *testing.T) {
	s := Step{Name: "ufw", Args: []string{"allow", "22", "comment", "it's; rm"}, Stdin: "a\nb\n"}
	if got := s.String(); got != `ufw allow 22 comment 'it'\''s; rm' < (2 lines)` {
		t.Fatalf("%s", got)
	}
}

func TestNFTTexts(t *testing.T) {
	out := "table inet t {\n\tchain c { # handle 1\n\t\ttype filter hook input priority filter; policy accept;\n" +
		"\t\tip saddr { 10.0.0.1,\n\t\t\t     10.0.0.2 } counter packets 5 bytes 300 drop # handle 7\n" +
		"\t\tcomment \"x; y\" accept # handle 8\n\t}\n}\n"
	got := nftTexts(out)
	if got[7] != "ip saddr { 10.0.0.1, 10.0.0.2 } counter drop" || got[8] != `comment "x; y" accept` || len(got) != 2 {
		t.Fatalf("%q", got)
	}
}
