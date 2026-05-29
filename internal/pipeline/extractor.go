package pipeline

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"

	"github.com/Nestorservice/veloce/internal/state"
)

// ExtractGoTypes parses Go source and returns each top-level struct type.
func ExtractGoTypes(src string) []state.GoType {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	pkg := f.Name.Name
	var out []state.GoType
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		var fields []string
		for _, fld := range st.Fields.List {
			typeStr := exprToString(fld.Type)
			for _, name := range fld.Names {
				fields = append(fields, name.Name+" "+typeStr)
			}
		}
		out = append(out, state.GoType{Name: ts.Name.Name, Package: pkg, Fields: fields})
		return true
	})
	return out
}

func exprToString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(v.X)
	case *ast.ArrayType:
		return "[]" + exprToString(v.Elt)
	default:
		return "?"
	}
}

var dartClassRE = regexp.MustCompile(`(?m)class\s+(\w+)\s*\{([\s\S]*?)\}`)
var dartFieldRE = regexp.MustCompile(`(?m)^\s*(?:final\s+|static\s+|const\s+)?([\w<>,\s]+?)\s+(\w+)\s*;`)

// ExtractDartTypes parses Dart source and returns each top-level class.
func ExtractDartTypes(src string) []state.DartType {
	var out []state.DartType
	for _, m := range dartClassRE.FindAllStringSubmatch(src, -1) {
		name := m[1]
		body := m[2]
		var fields []string
		for _, fm := range dartFieldRE.FindAllStringSubmatch(body, -1) {
			typeStr := strings.TrimSpace(fm[1])
			fieldName := fm[2]
			fields = append(fields, typeStr+" "+fieldName)
		}
		out = append(out, state.DartType{Name: name, Fields: fields})
	}
	return out
}
