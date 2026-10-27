package reorder

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// TestReal runs the real tools. It changes firewall rules, so it runs
// itself again inside a new, empty network namespace (unshare -n), where
// nothing it does reaches this computer's own network. It needs root.
// ufw keeps its rules in files under /etc/ufw, which a namespace doesn't
// separate, so ufw is tested only when MINT_TEST_UFW=1.
func TestReal(t *testing.T) {
	if os.Getenv("MINT_REORDER_NETNS") != "1" {
		if os.Getuid() != 0 {
			t.Skip("needs root; runs in a private network namespace")
		}
		unshare, err := exec.LookPath("unshare")
		if err != nil {
			t.Skip("no unshare")
		}
		if out, err := exec.Command(unshare, "-n", "true").CombinedOutput(); err != nil {
			t.Skipf("can't make a network namespace: %s", out)
		}
		cmd := exec.Command(unshare, "-n", os.Args[0], "-test.run", "^TestReal$", "-test.v", "-test.count=1")
		cmd.Env = append(os.Environ(), "MINT_REORDER_NETNS=1")
		out, err := cmd.CombinedOutput()
		t.Logf("in a network namespace:\n%s", out)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	sh := func(t *testing.T, name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return string(out)
	}
	// reverse reorders scope, checks the order, and restores it.
	reverse := func(t *testing.T, ad Adapter, scope string, show func() string) {
		t.Helper()
		before, err := ad.List(ctx, Exec, scope)
		if err != nil {
			t.Fatal(err)
		}
		if len(before.Rules) < 3 {
			t.Fatalf("only %d rules: %+v", len(before.Rules), before.Rules)
		}
		was := show()
		order := swapFree(before)
		_, p, err := Prepare(ctx, ad, Exec, before, order)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range p.Steps {
			t.Log(s)
		}
		if err := Apply(ctx, ad, Exec, before, p); err != nil {
			t.Fatal(err)
		}
		got, _ := ad.List(ctx, Exec, scope)
		expect(t, before, order, got)
		t.Logf("reversed:\n%s", show())
		if err := Restore(ctx, ad, Exec, before); err != nil {
			t.Fatal(err)
		}
		if now := show(); now != was {
			t.Fatalf("not restored:\n%s\nwas:\n%s", now, was)
		}
	}

	t.Run("iptables", func(t *testing.T) {
		if _, err := exec.LookPath("iptables-restore"); err != nil {
			t.Skip("no iptables")
		}
		sh(t, "iptables", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT")
		sh(t, "iptables", "-A", "INPUT", "-p", "tcp", "--dport", "22", "-m", "comment", "--comment", "ssh; in \"quoted\"", "-j", "ACCEPT")
		sh(t, "iptables", "-A", "INPUT", "-s", "10.0.0.1", "-j", "DROP")
		sh(t, "iptables", "-A", "INPUT", "-j", "LOG", "--log-prefix", "in: ")
		sh(t, "iptables", "-A", "OUTPUT", "-j", "ACCEPT")
		sh(t, "iptables", "-t", "nat", "-A", "POSTROUTING", "-o", "eth9", "-j", "MASQUERADE")
		nat := sh(t, "iptables", "-t", "nat", "-S")
		reverse(t, IPTables{Bin: "iptables"}, "filter INPUT", func() string { return sh(t, "iptables", "-S") })
		if sh(t, "iptables", "-t", "nat", "-S") != nat {
			t.Fatal("the nat table changed")
		}
	})

	t.Run("nft", func(t *testing.T) {
		if _, err := exec.LookPath("nft"); err != nil {
			t.Skip("no nft")
		}
		// A set long enough that nft wraps it over several lines.
		var set []string
		for i := 1; i <= 40; i++ {
			set = append(set, "10.1.0."+strconv.Itoa(i))
		}
		ruleset := `table inet mint {
  chain input {
    type filter hook input priority 0; policy accept;
    iif "lo" accept comment "loop; flush ruleset # not a command"
    ip saddr { ` + strings.Join(set, ", ") + ` } drop
    tcp dport { 22, 80, 443 } counter accept
    ct state established,related accept
  }
}
table ip other {
  chain c {
    counter
  }
}
`
		cmd := exec.Command("nft", "-f", "-")
		cmd.Stdin = strings.NewReader(ruleset)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		other := sh(t, "nft", "list", "table", "ip", "other")
		show := func() string { return sh(t, "nft", "list", "chain", "inet", "mint", "input") }
		reverse(t, NFT{}, "inet mint input", show)
		if sh(t, "nft", "list", "table", "ip", "other") != other {
			t.Fatal("another table changed")
		}
		l, _ := NFT{}.List(ctx, Exec, "inet mint input")
		if !strings.Contains(l.Rules[0].Text, `comment "loop; flush ruleset # not a command"`) {
			t.Fatalf("text: %q", l.Rules[0].Text)
		}
		if !strings.Contains(sh(t, "nft", "list", "chain", "inet", "mint", "input"), "10.1.0.40 }") || !strings.Contains(l.Rules[1].Text, "10.1.0.1, ") || !strings.Contains(l.Rules[1].Text, "10.1.0.40 } drop") {
			t.Fatalf("wrapped set: %q", l.Rules[1].Text)
		}
	})

	t.Run("ip-rule", func(t *testing.T) {
		if _, err := exec.LookPath("ip"); err != nil {
			t.Skip("no ip")
		}
		sh(t, "ip", "rule", "add", "pref", "100", "from", "10.0.0.0/8", "lookup", "100")
		sh(t, "ip", "rule", "add", "pref", "150", "to", "192.168.0.0/16", "iif", "lo", "unreachable")
		sh(t, "ip", "rule", "add", "pref", "200", "fwmark", "0x1/0xff", "lookup", "200")
		reverse(t, IPRule{}, "-4", func() string { return sh(t, "ip", "-4", "rule", "show") })
	})

	t.Run("ufw", func(t *testing.T) {
		if os.Getenv("MINT_TEST_UFW") != "1" {
			t.Skip("changes /etc/ufw; set MINT_TEST_UFW=1 on a throwaway machine")
		}
		sh(t, "ufw", "--force", "reset")
		sh(t, "ufw", "--force", "enable")
		sh(t, "ufw", "allow", "22/tcp", "comment", "ssh in")
		sh(t, "ufw", "deny", "from", "10.0.0.5")
		sh(t, "ufw", "route", "allow", "in", "on", "eth0", "out", "on", "eth1")
		sh(t, "ufw", "allow", "from", "192.168.1.0/24", "to", "any", "port", "443", "proto", "tcp")
		reverse(t, UFW{}, "rules", func() string { return sh(t, "ufw", "show", "added") })
	})
}
