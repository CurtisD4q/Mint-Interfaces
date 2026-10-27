package main

import (
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

func TestScaffoldWritesDiffSpec(t *testing.T) {
	d, _ := appkit.KnownDiffSpec("cmp")
	src, err := renderSpecGo(appkit.ToolSpec{Name: "cmp", Diff: &d})
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{`&appkit.DiffSpec{Format: "cmp-bytes"`, `ForceArgs: []string{"-l"}`, `ReadableFlags: []string{"-l", "--verbose"}`} {
		if !strings.Contains(s, want) {
			t.Errorf("spec.go missing %s:\n%s", want, s)
		}
	}
}
