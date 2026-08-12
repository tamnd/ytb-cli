package ytb

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// exitcode_test.go holds the exit codes to being worth documenting.
//
// README and the troubleshooting page both publish a table: 3 means nothing
// matched, 6 means the thing does not exist, 7 means an optional tool is
// missing. A script can branch on that, which is the only reason to have the
// table at all.
//
// Most of the domain was returning fmt.Errorf for those cases, and an
// unclassified error is exit 1. So `ytb search <nonsense>` printed "no search
// results found" and exited 1, and `ytb download <deleted video>` printed
// "video unavailable" and exited 1. The message was right and the number, which
// is the part a script reads, was not.
//
// This finds the next one in the source, rather than waiting for somebody to
// run the command and check $?.

// classified are the phrases that mean a specific exit code. A message with one
// of these in it and no kind attached is the defect.
var classified = []string{"not found", "unavailable", "no search results", "no results"}

func TestErrorsThatNameAKindCarryOne(t *testing.T) {
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
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isFmtErrorf(call.Fun) || len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			msg, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, phrase := range classified {
				if !strings.Contains(strings.ToLower(msg), phrase) {
					continue
				}
				t.Errorf("%s: fmt.Errorf(%q) exits 1, but the message says %q.\n"+
					"Use errs.NotFound, errs.NoResults or errs.Unsupported so the exit code matches the words.",
					fset.Position(call.Pos()), msg, phrase)
				break
			}
			return true
		})
	}
}

// isFmtErrorf matches fmt.Errorf(...).
func isFmtErrorf(fun ast.Expr) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Errorf" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "fmt"
}
