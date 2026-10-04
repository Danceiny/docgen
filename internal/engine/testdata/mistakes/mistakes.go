// Package mistakes is a fixture: the annotations and directives that are wrong in
// a way that makes them do nothing.
package mistakes

import "context"

// Req is a request.
type Req struct {
	// Q is the question.
	Q string `json:"q"`
}

// Spaced has a directive with a space after the slashes: an ordinary comment.
//
// apidoc:hidden
type Spaced struct {
	// A is a field.
	A string `json:"a"`
}

// Bogus has a scope that does not exist.
//
//apidoc:bogus
type Bogus struct {
	// B is a field.
	B string `json:"b"`
}

// Fine has a directive that is right.
//
//apidoc:public
type Fine struct {
	// C is a field.
	C string `json:"c"`
}

// Service is a service because its Name method returns a string.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Get has an annotation with a letter missing, one with a capital, and one that
// another reader of the comment may know and docgen does not.
//
// @respone:404,NotFoundErr,missing
// @Tags: x
// @auth: required
// @desc: this one is right
func (Service) Get(ctx context.Context, req *Req) (*Req, error) { return nil, nil }
