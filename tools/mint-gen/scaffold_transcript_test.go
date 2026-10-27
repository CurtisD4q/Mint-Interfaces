package main

import (
	"go/format"
	"strings"
	"testing"

	"github.com/CurtisD4q/Mint-Interfaces/mint/appkit"
)

// A spec with WithTranscript keeps it when mint-gen writes spec.go.
func TestScaffoldWritesTranscriptSpec(t *testing.T) {
	tr, _ := appkit.KnownTranscriptSpec("psql")
	src, err := renderSpecGo(appkit.ToolSpec{Name: "psql", Transcript: &tr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := format.Source(src); err != nil {
		t.Fatalf("spec.go isn't Go: %v\n%s", err, src)
	}
	if want := `&appkit.TranscriptSpec{Adapter: "psql"},`; !strings.Contains(string(src), want) {
		t.Errorf("spec.go missing %s:\n%s", want, src)
	}
}
