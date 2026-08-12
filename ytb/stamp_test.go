package ytb

import (
	"io/fs"

	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// stamp_test.go walks the source and asks every read the same question: when a
// session is attached, does the record you hand back say so?
//
// Doc 01 section 12 promises that a dataset built with cookies is
// distinguishable from one built without, and the only thing that promise can
// live on is the record. The envelope is filled in by the parsers, which have no
// client and cannot know the tier, so each read stamps it on the way out. That
// is one line per read and it is exactly the kind of line a new read forgets,
// with no symptom: the records come back, they are correct, and the tier field
// says 0 for a read that used your account.
//
// So it is checked here rather than left to review. The test reads the package's
// own source, finds every method on *Client that produces a record, and fails on
// one whose body never stamps.

// unstamped excuses a method that produces records and does not stamp them, with
// the reason. An entry here is a decision somebody can check; a method missing
// from both this map and the stamp calls is the bug.
var unstamped = map[string]string{
	// The walk calls the reads below it, so every node it yields holds a record
	// that was stamped on the way out of the read that produced it.
	"Walk": "the nodes it emits hold records the reads below it already stamped",
	// Each of these is one line: return theOtherOne(...), and that is where the
	// stamp is. The test cannot see through a delegation and it is not worth
	// teaching it to, because a wrapper that grows a body will be flagged then.
	"StreamPlaylistItems":      "delegates to streamPlaylist, which stamps",
	"StreamPlaylistWithHeader": "delegates to streamPlaylist, which stamps",
	"StreamHashtag":            "delegates to StreamHashtagWithHeader, which stamps",
}

func TestEveryReadStampsItsTier(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse the package: %v", err)
	}
	pkg := pkgs["ytb"]
	if pkg == nil {
		t.Fatal("the ytb package did not parse")
	}

	carriers := recordTypes(pkg)
	if len(carriers) < 7 {
		t.Fatalf("found %d record types, which is fewer than the model has: %v", len(carriers), carriers)
	}

	seen := map[string]bool{}
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isClientMethod(fn) {
				continue
			}
			if !producesRecord(fn, carriers) {
				continue
			}
			seen[fn.Name.Name] = true
			if reason, excused := unstamped[fn.Name.Name]; excused {
				if stamps(fn) {
					t.Errorf("%s stamps and is excused as %q: delete the excuse", fn.Name.Name, reason)
				}
				continue
			}
			if !stamps(fn) {
				t.Errorf("%s returns records and never stamps them, so a read made with cookies reports tier 0.\n"+
					"Call c.stamp on what it returns, stampAll on a slice, or wrap the callback with stampEmit.",
					fn.Name.Name)
			}
		}
	}
	for name, reason := range unstamped {
		if !seen[name] {
			t.Errorf("unstamped excuses %q (%s) and there is no such read", name, reason)
		}
	}
}

// recordTypes is every type that carries an envelope, directly or by holding one
// that does. The second half is what catches VideoResult, which is not a record
// itself and is the thing `ytb video` actually hands back.
func recordTypes(pkg *ast.Package) map[string]bool {
	direct := map[string]bool{}
	fields := map[string][]string{}
	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, f := range st.Fields.List {
				name := baseTypeName(f.Type)
				if len(f.Names) == 0 && name == "Envelope" {
					direct[ts.Name.Name] = true
					continue
				}
				fields[ts.Name.Name] = append(fields[ts.Name.Name], name)
			}
			return true
		})
	}
	out := map[string]bool{}
	for name := range direct {
		out[name] = true
	}
	for name, held := range fields {
		for _, h := range held {
			if direct[h] {
				out[name] = true
			}
		}
	}
	return out
}

func isClientMethod(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	return baseTypeName(fn.Recv.List[0].Type) == "Client"
}

// producesRecord reports whether a method hands records to its caller, either as
// a result or through a callback. A method that takes a record to fill in, like
// attachAbout, is not one: the read that owns that record stamps it.
func producesRecord(fn *ast.FuncDecl, carriers map[string]bool) bool {
	if fn.Type.Results != nil {
		for _, r := range fn.Type.Results.List {
			if carriers[baseTypeName(r.Type)] {
				return true
			}
		}
	}
	for _, p := range fn.Type.Params.List {
		ft, ok := p.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		for _, arg := range ft.Params.List {
			name := baseTypeName(arg.Type)
			// `any` is the mixed stream that search and music search emit.
			if carriers[name] || name == "any" {
				return true
			}
		}
	}
	return false
}

// stamps reports whether the body reaches any of the stamping helpers.
func stamps(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := call.Fun.(type) {
		case *ast.Ident:
			found = found || strings.HasPrefix(f.Name, "stamp")
		case *ast.SelectorExpr:
			found = found || strings.HasPrefix(f.Sel.Name, "stamp")
		case *ast.IndexExpr:
			// stampEmit[Video](...) if it is ever written with its type argument.
			if id, ok := f.X.(*ast.Ident); ok {
				found = found || strings.HasPrefix(id.Name, "stamp")
			}
		}
		return true
	})
	return found
}

// baseTypeName strips the pointers, slices and ellipses off a type expression
// and returns the name underneath, or "" for anything else.
func baseTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return baseTypeName(t.X)
	case *ast.ArrayType:
		return baseTypeName(t.Elt)
	case *ast.Ellipsis:
		return baseTypeName(t.Elt)
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}
