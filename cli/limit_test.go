package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// limit_test.go holds -n to meaning the same thing on every command.
//
// The flag is described once, in the global help, as "stop after N records". A
// paging read is told the number up front through PageOptions so it can stop
// asking for continuations. A read that answers in one request has nothing to
// stop asking for and has to cut its own list on the way out, which is easy to
// write and easy to forget, and forgetting is silent: `ytb captions -n 2`
// printed all six tracks, `ytb predicates -n 3` printed all twenty one, and the
// commands looked fine because the flag works everywhere else.
//
// So the rule is mechanical. A loop that writes rows goes through App.Emit or
// EmitAll, and both weigh the count against Limit. A bare Out.Emit inside a loop
// is the defect, and this finds it in the source rather than waiting for
// somebody to run the command with a number and count the lines.
func TestNoUnlimitedEmitLoop(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("%s did not parse: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.RangeStmt, *ast.ForStmt:
			default:
				return true
			}
			ast.Inspect(n, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok || !isOutEmit(call.Fun) {
					return true
				}
				t.Errorf("%s: Out.Emit inside a loop, so -n does nothing here.\n"+
					"Use EmitAll(app, items, row) for a plain list, or app.Emit(row) and break when it says stop.",
					fset.Position(call.Pos()))
				return true
			})
			return true
		})
	}
}

// isOutEmit matches x.Out.Emit(...), the renderer call that counts nothing.
func isOutEmit(fun ast.Expr) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Emit" {
		return false
	}
	inner, ok := sel.X.(*ast.SelectorExpr)
	return ok && inner.Sel.Name == "Out"
}
