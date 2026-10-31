package mint

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestManFlagOpensManualAndHelpStaysUsage(t *testing.T) {
	app := &App{
		Title: "◈ Tool",
		Items: []HomeItem{
			{Label: "Run", Desc: "run it"},
			{Label: "Manual", Desc: "Read the full manual (--man)"},
		},
	}
	old := os.Args
	t.Cleanup(func() { os.Args = old })

	os.Args = []string{"tool-mint", "--man"}
	done, item := app.handleFlags("◈ Tool", "tool", "")
	if done || item != 2 {
		t.Fatalf("--man done=%v item=%d", done, item)
	}

	os.Args = []string{"tool-mint", "-h"}
	out := captureStdout(t, func() {
		done, item = app.handleFlags("◈ Tool", "tool", "a note")
	})
	if !done || item != 0 {
		t.Fatalf("--help done=%v item=%d", done, item)
	}
	if !strings.Contains(out, "show this help") || !strings.Contains(out, "--help") {
		t.Fatalf("--help no longer prints usage:\n%s", out)
	}
	if !strings.Contains(out, "--man") || !strings.Contains(out, "open the manual") {
		t.Fatalf("usage does not offer --man:\n%s", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	return buf.String()
}
