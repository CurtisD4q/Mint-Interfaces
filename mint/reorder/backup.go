package reorder

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Backup writes l's rules in a form that puts them back by hand, with a
// header saying how: for when Mint can't restore them itself.
func Backup(l List) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Rules of %s %s, saved by Mint at %s before changing their order.\n", l.Tool, l.Scope, time.Now().Format(time.RFC3339))
	switch {
	case l.block != "":
		fmt.Fprintf(&b, "# Put the whole %s table back with:  %s-restore -c < THIS-FILE\n", strings.Fields(l.Scope)[0], l.Tool)
		b.WriteString(l.block)
	case l.Tool == "nft":
		f := strings.Fields(l.Scope)
		cmds := []any{map[string]any{"flush": map[string]any{"chain": map[string]string{"family": f[0], "table": f[1], "name": f[2]}}}}
		for _, r := range l.Rules {
			cmds = append(cmds, map[string]any{"add": map[string]any{"rule": json.RawMessage(r.put)}})
		}
		js, _ := json.Marshal(map[string]any{"nftables": cmds})
		b.WriteString("# Put the chain back with:  grep -v '^#' THIS-FILE | nft -j -f -\n")
		b.Write(js)
		b.WriteString("\n")
	case l.Tool == "ufw":
		b.WriteString("# The rules in order. To put them back: from the first rule that differs,\n" +
			"# delete the rules (ufw delete RULE) and add these again in this order.\n")
		for _, r := range l.Rules {
			b.WriteString("ufw " + r.put + "\n")
		}
	case l.Tool == "ip-rule":
		b.WriteString("# The rules and their priorities, as the commands that add them.\n")
		for _, r := range l.Rules {
			if !r.Fixed {
				b.WriteString("ip " + l.Scope + " rule add pref " + strconv.Itoa(r.pref) + " " + r.put + "\n")
			}
		}
	}
	return b.String()
}
