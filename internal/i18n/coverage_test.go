package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Every line the window writes has to be in the catalogue. A missing key shows
// English inside an otherwise Korean window, which no test of this package
// would otherwise catch, so the call sites are read here.
func TestEveryCallSiteIsTranslated(t *testing.T) {
	files, err := filepath.Glob("../../cmd/baybar/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no window sources found: %v", err)
	}
	fset := token.NewFileSet()
	missing := map[string][]string{}
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != "text" {
				return true
			}
			// T takes the line; P takes the singular and then the plural, and
			// the catalogue is keyed by the plural.
			var arg ast.Expr
			switch sel.Sel.Name {
			case "T":
				arg = call.Args[0]
			case "P":
				arg = call.Args[1]
			default:
				return true
			}
			key, ok := literal(arg)
			if !ok {
				t.Errorf("%s: text.%s is called with a value that is not a literal",
					fset.Position(call.Pos()), sel.Sel.Name)
				return true
			}
			if _, ok := korean[key]; !ok {
				missing[key] = append(missing[key], fset.Position(call.Pos()).String())
			}
			return true
		})
	}
	for key, where := range missing {
		t.Errorf("no Korean for %q (%s)", key, strings.Join(where, ", "))
	}
}

// literal returns the text of a string literal, joining the parts of a
// concatenation written across lines.
func literal(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		l, ok := literal(v.X)
		if !ok {
			return "", false
		}
		r, ok := literal(v.Y)
		if !ok {
			return "", false
		}
		return l + r, true
	}
	return "", false
}
