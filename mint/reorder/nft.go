package reorder

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// NFT reorders the rules of an nftables chain. Rules can't be moved, so
// the plan flushes the chain and adds its rules again in the new order,
// in one transaction, after checking it with "nft -c".
//
// Rules are read and written as nft's JSON, not as text: a comment
// holding "; flush ruleset" would be a second command in a text script,
// and the text form isn't promised to read back as it was written. JSON
// carries the rule as data. The text form is used only for display.
type NFT struct{}

func (NFT) Name() string { return "nft" }

var (
	nftIdent    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_./-]{0,63}$`)
	nftFamilies = map[string]bool{"ip": true, "ip6": true, "inet": true, "arp": true, "bridge": true, "netdev": true}
	nftHandleRe = regexp.MustCompile(`\s# handle (\d+)$`)
	nftCounter  = regexp.MustCompile(`\bcounter packets \d+ bytes \d+`)
	nftSpaces   = regexp.MustCompile(`\s+`)
)

type nftDoc struct {
	Nftables []map[string]json.RawMessage `json:"nftables"`
}

// Scopes lists "family table chain" for every chain.
func (NFT) Scopes(ctx context.Context, run Runner) ([]string, error) {
	out, err := run(ctx, Step{Name: "nft", Args: []string{"-j", "list", "chains"}})
	if err != nil {
		return nil, err
	}
	var doc nftDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return nil, fmt.Errorf("reading nft's list of chains: %v", err)
	}
	var scopes []string
	for _, o := range doc.Nftables {
		raw, ok := o["chain"]
		if !ok {
			continue
		}
		var c struct{ Family, Table, Name string }
		if json.Unmarshal(raw, &c) == nil && nftScopeOK(c.Family, c.Table, c.Name) {
			scopes = append(scopes, c.Family+" "+c.Table+" "+c.Name)
		}
	}
	return scopes, nil
}

func nftScopeOK(fam, table, chain string) bool {
	return nftFamilies[fam] && nftIdent.MatchString(table) && nftIdent.MatchString(chain)
}

func nftScope(scope string) ([]string, error) {
	f := strings.Fields(scope)
	if len(f) != 3 || !nftScopeOK(f[0], f[1], f[2]) {
		return nil, fmt.Errorf("not a family, table and chain: %q", scope)
	}
	return f, nil
}

func (n NFT) List(ctx context.Context, run Runner, scope string) (List, error) {
	f, err := nftScope(scope)
	if err != nil {
		return List{}, err
	}
	out, err := run(ctx, Step{Name: "nft", Args: append([]string{"-j", "list", "chain"}, f...)})
	if err != nil {
		return List{}, err
	}
	var doc nftDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return List{}, fmt.Errorf("reading nft's rules: %v", err)
	}
	// The text form, for showing the rules as people write them.
	texts := map[int64]string{}
	if txt, err := run(ctx, Step{Name: "nft", Args: append([]string{"-a", "list", "chain"}, f...)}); err == nil {
		texts = nftTexts(txt)
	}
	l := List{Tool: n.Name(), Scope: scope}
	for _, o := range doc.Nftables {
		raw, ok := o["rule"]
		if !ok {
			continue
		}
		var r map[string]any
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		if err := dec.Decode(&r); err != nil {
			return List{}, err
		}
		h, _ := r["handle"].(json.Number).Int64()
		delete(r, "handle")
		delete(r, "position")
		put, _ := json.Marshal(r)
		key, _ := json.Marshal(map[string]any{"expr": zeroCounters(r["expr"]), "comment": r["comment"]})
		text := texts[h]
		if text == "" {
			text = string(key)
		}
		l.Rules = append(l.Rules, Rule{Key: string(key), Text: text, put: string(put)})
	}
	return l, nil
}

// zeroCounters returns v with the numbers in counters removed, so two
// readings of a rule compare equal.
func zeroCounters(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = zeroCounters(e)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, e := range t {
			if k == "counter" {
				out[k] = nil
				continue
			}
			out[k] = zeroCounters(e)
		}
		return out
	}
	return v
}

// nftTexts maps rule handles to their text in "nft -a list chain" output,
// joining a rule printed over several lines (as some nft versions do).
func nftTexts(out string) map[int64]string {
	texts := map[int64]string{}
	var cur []string
	depth := 0
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if len(cur) == 0 {
			switch {
			case strings.HasPrefix(t, "table ") || strings.HasPrefix(t, "chain "):
				depth++
				continue
			case t == "}":
				depth--
				continue
			case depth != 2 || t == "" || strings.HasSuffix(t, ";"):
				continue
			}
		}
		cur = append(cur, t)
		if m := nftHandleRe.FindStringSubmatch(t); m != nil {
			var h int64
			fmt.Sscan(m[1], &h)
			joined := nftHandleRe.ReplaceAllString(strings.Join(cur, " "), "")
			texts[h] = nftCounter.ReplaceAllString(nftSpaces.ReplaceAllString(joined, " "), "counter")
			cur = nil
		}
	}
	return texts
}

func (n NFT) Plan(cur List, target []Rule) (Plan, error) {
	f, err := nftScope(cur.Scope)
	if err != nil {
		return Plan{}, err
	}
	chain := map[string]string{"family": f[0], "table": f[1], "name": f[2]}
	cmds := []any{map[string]any{"flush": map[string]any{"chain": chain}}}
	for _, r := range target {
		var rule map[string]any
		if err := json.Unmarshal([]byte(r.put), &rule); err != nil || rule == nil {
			return Plan{}, fmt.Errorf("can't add rule %q back", r.Text)
		}
		if rule["family"] != f[0] || rule["table"] != f[1] || rule["chain"] != f[2] {
			return Plan{}, fmt.Errorf("rule %q isn't in chain %s", r.Text, cur.Scope)
		}
		cmds = append(cmds, map[string]any{"add": map[string]any{"rule": json.RawMessage(r.put)}})
	}
	b, err := json.Marshal(map[string]any{"nftables": cmds})
	if err != nil {
		return Plan{}, err
	}
	in := string(b) + "\n"
	return Plan{
		Atomic: true,
		Steps: []Step{
			{Name: "nft", Args: []string{"-c", "-j", "-f", "-"}, Stdin: in, Check: true},
			{Name: "nft", Args: []string{"-j", "-f", "-"}, Stdin: in},
		},
		want: List{Tool: cur.Tool, Scope: cur.Scope, Rules: target},
	}, nil
}
