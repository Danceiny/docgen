package engine

import (
	"go/ast"
	"testing"

	"golang.org/x/tools/go/packages"
)

// A union of types, such as int | string in a constraint, is the type set of a
// constraint and never the type of a value: it has a key and is not an error.
func TestAConstraintUnionIsNotAnError(t *testing.T) {
	rec := captureLogs(t)
	p := &TypeParser{pkg: &packages.Package{}}
	union := &ast.BinaryExpr{X: ast.NewIdent("int"), Op: 0, Y: ast.NewIdent("string")}
	if got := p.generateTypeKeyUncached(union); got != "constraint" {
		t.Fatalf("key = %q", got)
	}
	if len(rec.records) != 0 {
		t.Fatalf("diagnostics = %v", rec.records)
	}
}
