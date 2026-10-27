package reorder

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// IPRule reorders routing policy rules (ip rule). Their order is their
// priority number, so moving a rule means giving it another priority:
// the rules that move keep the same set of numbers, handed out in the new
// order. The plan adds each rule at its new priority before deleting it
// at the old one, so no rule is ever missing. Scope is "-4" or "-6".
type IPRule struct{}

func (IPRule) Name() string { return "ip-rule" }

func (IPRule) Scopes(context.Context, Runner) ([]string, error) { return []string{"-4", "-6"}, nil }

var ipRuleLine = regexp.MustCompile(`^(\d+):\s+(.+)$`)

func ipFamily(scope string) error {
	if scope != "-4" && scope != "-6" {
		return fmt.Errorf("scope must be -4 or -6, not %q", scope)
	}
	return nil
}

// fixedRule reports whether a rule is one the kernel creates and lookups
// depend on: local at 0, main at 32766, default at 32767.
func fixedRule(pref int, text string) bool {
	return pref == 0 ||
		pref == 32766 && text == "from all lookup main" ||
		pref == 32767 && text == "from all lookup default"
}

func (r IPRule) List(ctx context.Context, run Runner, scope string) (List, error) {
	if err := ipFamily(scope); err != nil {
		return List{}, err
	}
	out, err := run(ctx, Step{Name: "ip", Args: []string{scope, "rule", "show"}})
	if err != nil {
		return List{}, err
	}
	l := List{Tool: r.Name(), Scope: scope}
	for _, line := range strings.Split(out, "\n") {
		m := ipRuleLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		pref, _ := strconv.Atoi(m[1])
		text := strings.Join(strings.Fields(m[2]), " ")
		l.Rules = append(l.Rules, Rule{Key: text, Text: fmt.Sprintf("%d: %s", pref, text), Fixed: fixedRule(pref, text), put: text, pref: pref})
	}
	return l, nil
}

// ipRuleArgs turns a rule as ip prints it back into ip's arguments.
// Flags ip prints in brackets ("[detached]") are state, not settings.
func ipRuleArgs(text string) ([]string, error) {
	var out []string
	for _, w := range strings.Fields(text) {
		if strings.HasPrefix(w, "[") {
			if strings.HasSuffix(w, "]") {
				continue
			}
			return nil, fmt.Errorf("can't read ip rule %q", text)
		}
		out = append(out, w)
	}
	return out, nil
}

type prefRule struct {
	pref int
	text string
}

func (r IPRule) Plan(cur List, target []Rule) (Plan, error) {
	// The movable rules of target, with the priorities they will get.
	var movable []Rule
	var prefs []int
	for _, t := range target {
		if !t.Fixed {
			movable = append(movable, t)
			prefs = append(prefs, t.pref)
		}
	}
	sort.Ints(prefs)
	for i := 1; i < len(prefs); i++ {
		if prefs[i] == prefs[i-1] {
			return Plan{}, fmt.Errorf("two rules share priority %d, so their order can't be changed by priority: give one of them another priority first", prefs[i])
		}
	}
	want := map[prefRule]bool{}
	var add []prefRule
	for i, t := range movable {
		pr := prefRule{prefs[i], t.put}
		want[pr] = true
		add = append(add, pr)
	}
	have := map[prefRule]bool{}
	for _, c := range cur.Rules {
		if !c.Fixed {
			have[prefRule{c.pref, c.put}] = true
		}
	}
	var steps []Step
	for _, pr := range add {
		if have[pr] {
			continue
		}
		args, err := ipRuleArgs(pr.text)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, Step{Name: "ip", Args: append([]string{cur.Scope, "rule", "add", "pref", strconv.Itoa(pr.pref)}, args...)})
	}
	for _, c := range cur.Rules {
		pr := prefRule{c.pref, c.put}
		if c.Fixed || want[pr] {
			continue
		}
		args, err := ipRuleArgs(c.put)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, Step{Name: "ip", Args: append([]string{cur.Scope, "rule", "del", "pref", strconv.Itoa(c.pref)}, args...)})
	}
	// After the change the list, in priority order, is target's order.
	wantList := List{Tool: cur.Tool, Scope: cur.Scope}
	for _, t := range target {
		wantList.Rules = append(wantList.Rules, Rule{Key: t.Key})
	}
	return Plan{Steps: steps, want: wantList}, nil
}
