// Package reorder changes the order of firewall and routing-policy rules
// (iptables, nft, ufw, ip rule) without a shell, and puts the old order
// back when asked.
//
// Order means something different to each tool, so each has an Adapter:
// iptables and nft rebuild one chain in a single atomic transaction; ufw
// deletes the rules from the first one that moves and adds them again in
// the new order; ip rule gives rules new priorities. Routes have no order
// (the kernel picks the most specific route), so "ip-route" refuses.
//
// Apply re-reads the rules first and stops if they changed since the user
// saw them, since positions only mean something against that exact list.
// After applying it reads them again; if the order isn't what was planned
// it restores the old one and says so. Restore uses the same planning, from
// whatever state the rules are in, so it also repairs a half-applied change.
package reorder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Rule is one rule in a List.
type Rule struct {
	// Key is what the rule is, for comparing two readings: its text
	// without counters, handles or priority numbers.
	Key string
	// Text is the rule as shown to the user. It comes from the tool and
	// is untrusted: clean it before display.
	Text string
	// Fixed rules can't move (ip rule's local, main and default).
	Fixed bool

	put  string // how the tool writes the rule back (a save line, rule text, a ufw spec…)
	pref int    // ip rule: its priority
}

// List is the rules of one chain (or equivalent), in order.
type List struct {
	Tool  string // the adapter's name
	Scope string // which chain: "filter INPUT", "inet mint input", "-4"…
	Rules []Rule
	Time  time.Time

	block string // iptables: the table as iptables-save printed it
}

// Keys returns the rules' keys in order.
func (l List) Keys() []string {
	k := make([]string, len(l.Rules))
	for i, r := range l.Rules {
		k[i] = r.Key
	}
	return k
}

// Step is one command a plan runs. Commands are run directly, never by a
// shell; Stdin, when set, is written to the command's standard input.
type Step struct {
	Name  string
	Args  []string
	Stdin string
	// Check marks a step that only validates (iptables-restore --test,
	// nft -c): it changes nothing.
	Check bool
}

// String shows the step as a command line, for the review screen.
func (s Step) String() string {
	var b strings.Builder
	b.WriteString(s.Name)
	for _, a := range s.Args {
		b.WriteByte(' ')
		if a == "" || strings.ContainsAny(a, " \t\n'\"\\$`;&|<>(){}*?#~") {
			b.WriteString("'" + strings.ReplaceAll(a, "'", `'\''`) + "'")
		} else {
			b.WriteString(a)
		}
	}
	if s.Stdin != "" {
		n := strings.Count(strings.TrimRight(s.Stdin, "\n"), "\n") + 1
		fmt.Fprintf(&b, " < (%d lines)", n)
	}
	return b.String()
}

// Plan is what turns one order into another.
type Plan struct {
	Steps []Step
	// Atomic is true when the change happens in one transaction: there is
	// no moment with some rules missing.
	Atomic bool
	want   List
}

// Runner runs one command and returns its standard output. A failing
// command's error includes what it printed on standard error.
type Runner func(ctx context.Context, s Step) (string, error)

// Exec runs commands on this computer, with LC_ALL=C so their output
// is in the form the adapters read.
func Exec(ctx context.Context, s Step) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, StepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Name, s.Args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	if s.Stdin != "" {
		cmd.Stdin = strings.NewReader(s.Stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg == "" {
			return out.String(), fmt.Errorf("%s: %w", s.Name, err)
		}
		return out.String(), fmt.Errorf("%s: %s", s.Name, firstLines(msg, 3))
	}
	return out.String(), nil
}

// StepTimeout bounds each command.
var StepTimeout = 20 * time.Second

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, " / ")
}

// Adapter knows one tool.
type Adapter interface {
	// Name is the adapter's name ("iptables", "nft", "ufw", "ip-rule").
	Name() string
	// Scopes lists what can be reordered: chains, or one entry.
	Scopes(ctx context.Context, run Runner) ([]string, error)
	// List reads the rules of scope.
	List(ctx context.Context, run Runner, scope string) (List, error)
	// Plan returns the steps that turn cur into target's order. target
	// may hold rules cur doesn't (restoring after a half-applied change).
	Plan(cur List, target []Rule) (Plan, error)
}

// ErrChanged is returned by Apply when the rules changed since they were read.
var ErrChanged = errors.New("the rules changed since they were read; read them again and redo the change")

// Permute returns before's rules in order (a permutation of indexes),
// checking that fixed rules stay where they are.
func Permute(before List, order []int) ([]Rule, error) {
	if len(order) != len(before.Rules) {
		return nil, fmt.Errorf("the new order has %d rules, not %d", len(order), len(before.Rules))
	}
	seen := make([]bool, len(order))
	out := make([]Rule, len(order))
	for i, j := range order {
		if j < 0 || j >= len(order) || seen[j] {
			return nil, errors.New("the new order isn't a rearrangement of the rules")
		}
		seen[j] = true
		if (before.Rules[j].Fixed || before.Rules[i].Fixed) && i != j {
			return nil, fmt.Errorf("%q can't move", before.Rules[max(i, j)].Text)
		}
		out[i] = before.Rules[j]
	}
	return out, nil
}

// Moved reports whether order changes anything.
func Moved(order []int) bool {
	for i, j := range order {
		if i != j {
			return true
		}
	}
	return false
}

// Prepare reads the rules again, checks they are the ones in before, and
// returns the plan that puts them in order, made from the fresh reading
// (so counters aren't wound back).
func Prepare(ctx context.Context, ad Adapter, run Runner, before List, order []int) (List, Plan, error) {
	if _, err := Permute(before, order); err != nil {
		return List{}, Plan{}, err
	}
	fresh, err := ad.List(ctx, run, before.Scope)
	if err != nil {
		return List{}, Plan{}, err
	}
	if !sameKeys(fresh.Keys(), before.Keys()) {
		return fresh, Plan{}, ErrChanged
	}
	target, _ := Permute(fresh, order)
	p, err := ad.Plan(fresh, target)
	return fresh, p, err
}

// Apply runs p (from Prepare), then reads the rules and checks they are
// in the planned order. If anything fails it restores the rules of
// original and returns what went wrong, and whether the restore worked.
func Apply(ctx context.Context, ad Adapter, run Runner, original List, p Plan) error {
	err := runSteps(ctx, run, p.Steps)
	if err == nil {
		err = check(ctx, ad, run, p.want)
	}
	if err == nil {
		return nil
	}
	if rerr := Restore(ctx, ad, run, original); rerr != nil {
		return fmt.Errorf("%v; putting the old order back also failed: %v", err, rerr)
	}
	return fmt.Errorf("%v; the old order is back", err)
}

// Restore puts the rules of original back, in its order, from whatever
// state they are in now.
func Restore(ctx context.Context, ad Adapter, run Runner, original List) error {
	cur, err := ad.List(ctx, run, original.Scope)
	if err != nil {
		return err
	}
	if sameKeys(cur.Keys(), original.Keys()) {
		return nil
	}
	p, err := ad.Plan(cur, original.Rules)
	if err != nil {
		return err
	}
	if err := runSteps(ctx, run, p.Steps); err != nil {
		return err
	}
	return check(ctx, ad, run, p.want)
}

func runSteps(ctx context.Context, run Runner, steps []Step) error {
	for _, s := range steps {
		if _, err := run(ctx, s); err != nil {
			if s.Check {
				return fmt.Errorf("%s rejected the new order: %v", s.Name, err)
			}
			return err
		}
	}
	return nil
}

func check(ctx context.Context, ad Adapter, run Runner, want List) error {
	got, err := ad.List(ctx, run, want.Scope)
	if err != nil {
		return fmt.Errorf("reading the rules back: %v", err)
	}
	if !sameKeys(got.Keys(), want.Keys()) {
		return errors.New("the rules didn't end up in the planned order")
	}
	return nil
}

func sameKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Adapters returns the adapter for a tool name: iptables, ip6tables (and
// their -nft and -legacy variants), nft, ufw, ip-rule, ip-route.
func For(name string) (Adapter, error) {
	switch {
	case name == "nft":
		return NFT{}, nil
	case name == "ufw":
		return UFW{}, nil
	case name == "ip-rule":
		return IPRule{}, nil
	case name == "ip-route":
		return nil, ErrNoOrder
	case strings.HasPrefix(name, "iptables") || strings.HasPrefix(name, "ip6tables"):
		switch strings.TrimPrefix(strings.TrimPrefix(name, "ip6tables"), "iptables") {
		case "", "-nft", "-legacy":
			return IPTables{Bin: name}, nil
		}
	}
	return nil, fmt.Errorf("no reorder adapter for %q", name)
}

// ErrNoOrder explains why routes can't be reordered.
var ErrNoOrder = errors.New("routes have no order: the kernel uses the most specific route that matches " +
	"(the longest prefix), and among equally specific ones the lowest metric. Change a route's metric instead")

// splitWords splits a ufw rule as "ufw show added" prints it, with
// 'quoted' or "quoted" words (comments), into arguments.
func splitWords(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false
	var quote rune
	has := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, has = r, true
		case r == ' ' || r == '\t':
			if in || has {
				out = append(out, cur.String())
				cur.Reset()
			}
			in, has = false, false
			continue
		default:
			cur.WriteRune(r)
		}
		in = true
	}
	if quote != 0 {
		return nil, fmt.Errorf("unbalanced quote in %q", s)
	}
	if in || has {
		out = append(out, cur.String())
	}
	return out, nil
}
