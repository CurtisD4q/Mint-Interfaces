package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Fuzz tests for mint-gen's parsers of help text, which comes from the
// program being wrapped and is not trusted. Run one for longer with
//
//	go test ./tools/mint-gen -run '^$' -fuzz FuzzHelpToSpec -fuzztime 60s

const fuzzHelp = `Usage: tool [OPTIONS] COMMAND [ARG...]

Options:
  -o, --output FILE      Write to FILE
  -n, --count N          Repeat N times (default 3)
  -c, --color WHEN       Colour: always, never, auto
      --password PASS    The password
  -v                     Verbose

Commands:
  run       Run something
  list      List things

Run 'tool COMMAND --help' for more information.
`

// Whatever the help says, parsing it never panics, and the spec.go written
// from it is valid Go that only declares the spec: help text can never add
// code, imports or calls of its own to a generated app.
func FuzzHelpToSpec(f *testing.F) {
	f.Add("tool", "", fuzzHelp)
	f.Add("tool", fuzzHelp, "")
	f.Add("x", "", "Usage: x \"; os.RemoveAll(\"/\") //\n  --evil=\"`\\x00\"  bad\n")
	f.Add("x", "", "  -a, --a‮  reversed\n  --\x1b[31mred\x1b[0m  colour")
	f.Add("x", "", strings.Repeat("  --flag", 200))
	f.Fuzz(func(t *testing.T, name, man, help string) {
		if len(man)+len(help) > 1<<15 || !safeCommandWord(name) {
			return
		}
		spec := ParseManAndHelp(name, man, help)
		for _, fl := range spec.Flags {
			if fl.Flag == "" {
				t.Fatalf("a flag with no name: %+v", fl)
			}
		}
		src, err := renderSpecGo(spec)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), "spec.go", src, 0)
		if err != nil {
			t.Fatalf("spec.go does not parse: %v\n%s", err, src)
		}
		checkOnlyDeclaresTheSpec(t, file, src)
	})
}

// checkOnlyDeclaresTheSpec fails unless the file imports only appkit (and
// time), and every function call in it is one of the few the template uses.
func checkOnlyDeclaresTheSpec(t *testing.T, file *ast.File, src []byte) {
	t.Helper()
	for _, imp := range file.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p != "github.com/CurtisD4q/Mint-Interfaces/mint/appkit" && p != "time" {
			t.Fatalf("spec.go imports %q\n%s", p, src)
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fn.X.(*ast.Ident); ok && x.Name == "time" && fn.Sel.Name == "Duration" {
				return true
			}
		case *ast.Ident:
			if fn.Name == "spec" || fn.Name == "Spec" {
				return true
			}
		}
		// A conversion such as time.Duration(2000000000) or a type name
		// is fine; anything else is code that came from the help.
		t.Fatalf("spec.go calls %T in:\n%s", call.Fun, src)
		return true
	})
}

// The sentence patterns are built from the program's name; any name is
// quoted, so it cannot change the pattern or panic.
func FuzzCommandHelpForms(f *testing.F) {
	f.Add("docker", "Run 'docker COMMAND --help' for more.")
	f.Add("a.b*c(", "Run 'a.b*c( COMMAND --help'")
	f.Add("[", "Usage: [ [OPTIONS] COMMAND")
	f.Fuzz(func(t *testing.T, name, text string) {
		if len(text) > 1<<14 {
			return
		}
		if form, ok := commandHelpForm(name, text); ok {
			args := form("run")
			if len(args) != 2 {
				t.Fatalf("form %v", args)
			}
		}
		_ = optionsBeforeCommand(name, text)
	})
}

// A command word passed to a program for its help is never an option, a
// path or anything a shell or the program would read as more than a name.
func FuzzSafeCommandWord(f *testing.F) {
	for _, s := range []string{"run", "--rm", "../x", "a b", "$(x)", "x;y", "日本"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !safeCommandWord(s) {
			return
		}
		if strings.HasPrefix(s, "-") || strings.ContainsAny(s, " /\\$;|&`'\"<>()*?[]{}\n\t\x00~") || len(s) > 40 || s == "" {
			t.Fatalf("%q accepted", s)
		}
	})
}

// Commands found in any help are plain words too.
func FuzzExtractSubcommands(f *testing.F) {
	f.Add(fuzzHelp)
	f.Add("These are common Git commands used in various situations:\n   clone   Clone\n   -rf   bad\n")
	f.Fuzz(func(t *testing.T, help string) {
		if len(help) > 1<<14 {
			return
		}
		subs := extractSubcommands(nil, strings.Split(help, "\n"))
		if len(subs) > maxSuiteCommands {
			t.Fatalf("%d commands", len(subs))
		}
		for _, s := range subs {
			if !safeCommandWord(s.Name) {
				t.Fatalf("command %q from help", s.Name)
			}
		}
	})
}
