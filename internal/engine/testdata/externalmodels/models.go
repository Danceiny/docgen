// Package externalmodels is a fixture: structs with fields whose types are
// declared in other modules, some of which contain themselves.
package externalmodels

import (
	"go/ast"
	"net/http"
	"unsafe"
)

// Holder has fields of types that this module does not declare.
type Holder struct {
	// Name is a plain field.
	Name string `json:"name"`
	// Scope contains itself: a scope has an outer scope.
	Scope *ast.Scope `json:"scope"`
	// Request and Response contain each other.
	Request *http.Request `json:"request"`
	// Raw is a pointer that no JSON document can hold.
	Raw unsafe.Pointer `json:"raw"`
}
