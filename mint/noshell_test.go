package mint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// shells are programs that run their arguments as shell syntax.
var shells = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true,
	"mksh": true, "ash": true, "fish": true, "csh": true, "tcsh": true, "busybox": true,
}

// shellAllowed lists the functions that may start a shell, each reviewed:
// the key is "dir/file.go:Func".
var shellAllowed = map[string]string{
	// Telling a shell built-in from a program needs a shell. The name is
	// validated and passed as "$1", never as script text.
	"tools/mint-gen/compat.go:shellType": "positional parameter only",
}

// Mint runs programs directly. Library code, templates and mint-gen must
// never start a shell, where text from a user or a program could be run as
// commands. Tests and examples may.
func TestNoShellExecution(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"mint", "templates", "tools"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && strings.HasPrefix(d.Name(), "zz_") {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			for _, bad := range shellCalls(t, path) {
				if _, ok := shellAllowed[filepath.ToSlash(rel)+":"+bad.fn]; ok {
					continue
				}
				t.Errorf("%s:%d: %s starts a shell (%s); run the program directly", rel, bad.line, bad.fn, bad.what)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

type shellCall struct {
	fn   string
	line int
	what string
}

// shellCalls finds calls and runner.Cmd literals in path whose program is a
// shell given as a string literal.
func shellCalls(t *testing.T, path string) []shellCall {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var found []shellCall
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		fn := "(package)"
		if ok {
			fn = fd.Name.Name
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				name := callName(x.Fun)
				var prog ast.Expr
				switch name {
				case "exec.Command", "util.Run", "util.RunOK", "util.RunValue", "util.RunLines":
					if len(x.Args) > 0 {
						prog = x.Args[0]
					}
				case "exec.CommandContext":
					if len(x.Args) > 1 {
						prog = x.Args[1]
					}
				case "syscall.Exec", "os.StartProcess":
					if len(x.Args) > 0 {
						prog = x.Args[0]
					}
				}
				if s, ok := literal(prog); ok && isShell(s) {
					found = append(found, shellCall{fn, fset.Position(x.Pos()).Line, name + "(" + strconv.Quote(s) + ", …)"})
				}
			case *ast.CompositeLit:
				if n := callName(x.Type); n != "runner.Cmd" && n != "Cmd" {
					return true
				}
				for _, el := range x.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, _ := kv.Key.(*ast.Ident)
					if key == nil {
						continue
					}
					if key.Name == "Shell" {
						found = append(found, shellCall{fn, fset.Position(kv.Pos()).Line, "Cmd{Shell: …}"})
					}
					if s, ok := literal(kv.Value); ok && (key.Name == "Name" || key.Name == "Bin") && isShell(s) {
						found = append(found, shellCall{fn, fset.Position(kv.Pos()).Line, "Cmd{" + key.Name + ": " + strconv.Quote(s) + "}"})
					}
				}
			}
			return true
		})
	}
	return found
}

func callName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name + "." + x.Sel.Name
		}
	}
	return ""
}

func literal(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

func isShell(prog string) bool { return shells[filepath.Base(prog)] }

// The checker itself must catch each form.
func TestNoShellCheckerFindsShells(t *testing.T) {
	src := `package x
import ("os/exec"; "context")
func a() { exec.Command("sh", "-c", "x") }
func b() { exec.CommandContext(context.Background(), "/bin/bash", "-c", "x") }
func c() { _ = runner.Cmd{Name: "dash", Args: nil} }
func d() { _ = runner.Cmd{Shell: "x"} }
func e() { util.Run("zsh", "-c", "x") }
func ok() { exec.Command("ls", "-l"); _ = runner.Cmd{Name: "diff"} }
`
	path := filepath.Join(t.TempDir(), "x.go")
	if err := writeFile(path, src); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range shellCalls(t, path) {
		got[c.fn] = true
	}
	for _, fn := range []string{"a", "b", "c", "d", "e"} {
		if !got[fn] {
			t.Errorf("checker missed the shell in %s()", fn)
		}
	}
	if got["ok"] {
		t.Error("checker flagged a direct program run")
	}
}

func writeFile(path, s string) error { return os.WriteFile(path, []byte(s), 0o644) }
