package main

import (
	"go/format"
	"strings"
	"testing"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestScaffoldWritesReorderSpec(t *testing.T) {
	r, ok := appkit.KnownReorderSpec("iptables")
	if !ok {
		t.Fatal("iptables has no reorder spec")
	}
	r.ConfirmWindow = 45 * time.Second
	src, err := renderSpecGo(appkit.ToolSpec{Name: "iptables", Reorder: &r})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := format.Source(src); err != nil {
		t.Fatalf("spec.go isn't Go: %v\n%s", err, src)
	}
	want := `&appkit.ReorderSpec{Adapter: "iptables", Scope: "filter INPUT", ConfirmWindow: 45000000000 /* 45s */},`
	if !strings.Contains(string(src), want) {
		t.Errorf("spec.go missing %s:\n%s", want, src)
	}
}

// mint-gen offers the rule order screen for the tools Mint can reorder.
func TestParserAttachesReorder(t *testing.T) {
	for bin, want := range map[string]string{"iptables": "iptables", "nft": "nft", "ufw": "ufw"} {
		spec, _ := ParseManAndHelpIO(bin, "", "Usage: "+bin+" [options]\n")
		if spec.Reorder == nil || spec.Reorder.Adapter != want {
			t.Errorf("%s: %+v", bin, spec.Reorder)
		}
	}
	if spec, _ := ParseManAndHelpIO("ls", "", "Usage: ls [options]\n"); spec.Reorder != nil {
		t.Error("ls got a reorder spec")
	}
}
