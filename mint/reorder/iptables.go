package reorder

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// IPTables reorders a chain of iptables or ip6tables (Bin: "iptables",
// "ip6tables", or an -nft/-legacy variant). It rebuilds the table that
// holds the chain from "-save -c" output and loads it with "-restore -c",
// which replaces the whole table in one transaction, after "--test".
type IPTables struct{ Bin string }

func (t IPTables) Name() string { return t.Bin }

var (
	counterPrefix = regexp.MustCompile(`^\[\d+:\d+\] `)
	chainName     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

func (t IPTables) save(ctx context.Context, run Runner, table string) (string, error) {
	args := []string{"-c"}
	if table != "" {
		args = append(args, "-t", table)
	}
	return run(ctx, Step{Name: t.Bin + "-save", Args: args})
}

// Scopes lists "table CHAIN" for every chain, built-in ones first, as
// iptables-save prints them.
func (t IPTables) Scopes(ctx context.Context, run Runner) ([]string, error) {
	out, err := t.save(ctx, run, "")
	if err != nil {
		return nil, err
	}
	var scopes []string
	table := ""
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "*"):
			table = strings.TrimPrefix(l, "*")
		case strings.HasPrefix(l, ":") && table != "":
			name, _, _ := strings.Cut(l[1:], " ")
			scopes = append(scopes, table+" "+name)
		}
	}
	return scopes, nil
}

func splitScope(scope string) (table, chain string, err error) {
	table, chain, ok := strings.Cut(scope, " ")
	if !ok || !chainName.MatchString(table) || !chainName.MatchString(chain) {
		return "", "", fmt.Errorf("not a table and chain: %q", scope)
	}
	return table, chain, nil
}

func (t IPTables) List(ctx context.Context, run Runner, scope string) (List, error) {
	table, chain, err := splitScope(scope)
	if err != nil {
		return List{}, err
	}
	out, err := t.save(ctx, run, table)
	if err != nil {
		return List{}, err
	}
	block, err := tableBlock(out, table)
	if err != nil {
		return List{}, err
	}
	l := List{Tool: t.Name(), Scope: scope, block: block}
	found := false
	prefix := "-A " + chain + " "
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, ":"+chain+" ") {
			found = true
		}
		rule := counterPrefix.ReplaceAllString(line, "")
		if !strings.HasPrefix(rule, prefix) {
			continue
		}
		l.Rules = append(l.Rules, Rule{Key: rule, Text: strings.TrimPrefix(rule, prefix), put: line})
	}
	if !found {
		return List{}, fmt.Errorf("there is no chain %s in table %s", chain, table)
	}
	return l, nil
}

// tableBlock returns the lines from "*table" to its "COMMIT".
func tableBlock(save, table string) (string, error) {
	var out []string
	in := false
	for _, l := range strings.Split(save, "\n") {
		if l == "*"+table {
			in = true
		}
		if !in || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
		if l == "COMMIT" {
			return strings.Join(out, "\n") + "\n", nil
		}
	}
	return "", fmt.Errorf("%s-save printed no %s table", "iptables", table)
}

// Plan writes cur's table with the chain's rules in target's order.
func (t IPTables) Plan(cur List, target []Rule) (Plan, error) {
	_, chain, err := splitScope(cur.Scope)
	if err != nil {
		return Plan{}, err
	}
	prefix := "-A " + chain + " "
	var b strings.Builder
	done := false
	emit := func() {
		for _, r := range target {
			b.WriteString(r.put + "\n")
		}
		done = true
	}
	for _, line := range strings.Split(strings.TrimSuffix(cur.block, "\n"), "\n") {
		if strings.HasPrefix(counterPrefix.ReplaceAllString(line, ""), prefix) {
			if !done {
				emit()
			}
			continue
		}
		if line == "COMMIT" && !done {
			emit() // the chain had no rules (restoring rules someone deleted)
		}
		b.WriteString(line + "\n")
	}
	for _, r := range target {
		if !strings.HasPrefix(counterPrefix.ReplaceAllString(r.put, ""), prefix) || strings.ContainsAny(r.put, "\n\r") {
			return Plan{}, fmt.Errorf("rule %q isn't in chain %s", r.Text, chain)
		}
	}
	in := b.String()
	return Plan{
		Atomic: true,
		Steps: []Step{
			{Name: t.Bin + "-restore", Args: []string{"--test", "-c"}, Stdin: in, Check: true},
			{Name: t.Bin + "-restore", Args: []string{"-c"}, Stdin: in},
		},
		want: List{Tool: cur.Tool, Scope: cur.Scope, Rules: target},
	}, nil
}
