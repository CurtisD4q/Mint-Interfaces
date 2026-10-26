package appkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A tool that writes its output to its last argument (ffmpeg): the app
// passes its output path there and does not overwrite the file with what
// the tool printed.
func TestOutputAsLastArgument(t *testing.T) {
	bin, log := fakeTool(t, "conv", `for last; do :; done; echo "progress 50%"; echo "real output" > "$last"`)
	a := newTestApp(t, ToolSpec{Name: "conv", Bin: bin, OutputsFile: true, OutputPositional: true, StdoutKind: StdoutLog, DefaultExt: "mkv"})
	a.SaveToFile = true
	a.Target = "-i in.mp4"
	out := a.ResolvedOutputPath()
	if !strings.HasSuffix(out, ".mkv") {
		t.Fatalf("output %q", out)
	}
	args, _ := a.BuildArgs()
	if len(args) < 3 || args[len(args)-1] != out || args[0] != "-i" {
		t.Fatalf("args %q", args)
	}
	if p := a.CommandPreview(); strings.Contains(p, "> ") || !strings.HasSuffix(p, out) {
		t.Fatalf("preview %q", p)
	}
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	for i := 0; i < 200 && len(a.History()) == 0; i++ {
		job.Wait()
	}
	b, _ := os.ReadFile(out)
	if string(b) != "real output\n" {
		t.Fatalf("output file holds %q (the progress messages overwrote it?)", b)
	}
	if !strings.Contains(runs(log), out) {
		t.Fatalf("tool not given the path: %s", runs(log))
	}
}

// A path that starts with "-" is never passed where it would read as an option.
func TestPositionalOutputNeverLooksLikeAnOption(t *testing.T) {
	bin, _ := fakeTool(t, "conv", "")
	a := newTestApp(t, ToolSpec{Name: "conv", Bin: bin, OutputsFile: true, OutputPositional: true})
	a.SaveToFile = true
	a.ResultsDir = ""
	a.OutputFile = "-y.mkv"
	args, _ := a.BuildArgs()
	if last := args[len(args)-1]; strings.HasPrefix(last, "-") || last != "./"+filepath.Clean("-y.mkv") {
		t.Fatalf("last argument %q", last)
	}
}

// A tool that only prints messages: saving them is offered, named as
// such, and off by default.
func TestLogOnlyOutput(t *testing.T) {
	bin, _ := fakeTool(t, "zipper", "")
	a := newTestApp(t, ToolSpec{Name: "zipper", Bin: bin, OutputsFile: true, StdoutKind: StdoutLog})
	if a.SaveToFile {
		t.Fatal("saving messages is on by default")
	}
	if l := a.saveToggleLabel(); !strings.Contains(l, "messages") {
		t.Fatalf("label %q", l)
	}
	b := newTestApp(t, ToolSpec{Name: "lister", Bin: bin, OutputsFile: true, StdoutKind: StdoutResults})
	if !b.SaveToFile || b.saveToggleLabel() != "Save output to file" {
		t.Fatalf("results tool: %v %q", b.SaveToFile, b.saveToggleLabel())
	}
}
