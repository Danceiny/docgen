package engine

import (
	"fmt"
	"go/ast"
)

// ExtractGenericTypeArg extracts the generic type argument from an AST field (such as domain.Summary).
func ExtractGenericTypeArg(field *ast.Field) string {
	if field == nil || field.Type == nil {
		return ""
	}
	switch t := field.Type.(type) {
	case *ast.IndexExpr:
		//is it an index expression (such as table.Page[T])?
		// handle the type parameter
		if selector, ok := t.Index.(*ast.SelectorExpr); ok {
			return fmt.Sprintf("%s.%s", selector.X.(*ast.Ident).Name, selector.Sel.Name)
		}
		if ident, ok := t.Index.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		if t.Name == "T" {
			return t.Name
		}
	case *ast.StarExpr:
		if vv, ok := t.X.(*ast.Ident); ok && vv.Name == "T" {
			return vv.Name
		}
	}
	return ""
}
