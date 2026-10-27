package reorder

import (
	"context"
	"fmt"
	"strings"
)

// UFW reorders ufw's rules, as "ufw show added" lists them. ufw can only
// insert at a position, and its positions count IPv4 and IPv6 rules
// separately, so the plan deletes every rule from the first one that
// moves and adds them again, in the new order, at the end. Rules before
// that stay put. Connections already open are unaffected: ufw accepts
// them before it looks at these rules.
type UFW struct{}

func (UFW) Name() string { return "ufw" }

func (UFW) Scopes(context.Context, Runner) ([]string, error) { return []string{"rules"}, nil }

func (u UFW) List(ctx context.Context, run Runner, scope string) (List, error) {
	out, err := run(ctx, Step{Name: "ufw", Args: []string{"show", "added"}})
	if err != nil {
		return List{}, err
	}
	l := List{Tool: u.Name(), Scope: scope}
	for _, line := range strings.Split(out, "\n") {
		spec, ok := strings.CutPrefix(strings.TrimSpace(line), "ufw ")
		if !ok {
			continue // the heading, or "(None)"
		}
		if _, err := splitWords(spec); err != nil {
			return List{}, err
		}
		l.Rules = append(l.Rules, Rule{Key: spec, Text: spec, put: spec})
	}
	return l, nil
}

// ufwArgs returns the arguments that add (or delete) spec.
func ufwArgs(spec string, del bool) ([]string, error) {
	w, err := splitWords(spec)
	if err != nil || len(w) == 0 {
		return nil, fmt.Errorf("can't read ufw rule %q", spec)
	}
	switch w[0] {
	case "allow", "deny", "reject", "limit", "route":
	default:
		return nil, fmt.Errorf("unexpected ufw rule %q", spec)
	}
	if !del {
		return w, nil
	}
	if w[0] == "route" {
		return append([]string{"--force", "route", "delete"}, w[1:]...), nil
	}
	return append([]string{"--force", "delete"}, w...), nil
}

func (u UFW) Plan(cur List, target []Rule) (Plan, error) {
	first := 0
	for first < len(cur.Rules) && first < len(target) && cur.Rules[first].Key == target[first].Key {
		first++
	}
	var steps []Step
	for i := len(cur.Rules) - 1; i >= first; i-- {
		args, err := ufwArgs(cur.Rules[i].put, true)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, Step{Name: "ufw", Args: args})
	}
	for _, r := range target[first:] {
		args, err := ufwArgs(r.put, false)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, Step{Name: "ufw", Args: args})
	}
	return Plan{Steps: steps, want: List{Tool: cur.Tool, Scope: cur.Scope, Rules: target}}, nil
}
