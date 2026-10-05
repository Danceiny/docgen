// Package mistakes is a fixture: the annotations and directives that are wrong in
// a way that makes them do nothing.
package mistakes

import "context"

// Req is a request.
type Req struct {
	// Q is the question.
	Q string `json:"q"`
	// Typo has a scope with a letter missing, which hides it from every document.
	Typo string `json:"typo" apidoc:"internl"`
	// Prose has a description where the scope goes.
	Prose string `json:"prose" apidoc:"the order id"`
	// Staff has a token that a document lists.
	Staff string `json:"staff" apidoc:"Staff"`
	// Internal has a scope.
	Internal string `json:"internal" apidoc:"internal"`
	// Gone has none, on purpose.
	Gone string `json:"gone" apidoc:"-"`
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

// Speed lists the values it shows by what they are on the wire, and the values
// of a directive are the names of the constants.
//
//apidoc:public:fast,SpeedSlow
type Speed string

// The speeds.
const (
	// SpeedFast is quick.
	SpeedFast Speed = "fast"
	// SpeedSlow is not.
	SpeedSlow Speed = "slow"
)

// Service is a service because its Name method returns a string.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Header names a header type that the configuration does not have.
//
// @headerType: Nobody
func (Service) Header(ctx context.Context, req *Req) (*Req, error) { return nil, nil }

// Two takes two requests, and the second has no place in the document.
// @path: /two
func (Service) Two(ctx context.Context, a *Req, b *Req) (*Req, error) { return nil, nil }

// Get has an annotation with a letter missing, one with a capital, and one that
// another reader of the comment may know and docgen does not.
//
// @respone:404,NotFoundErr,missing
// @Tags: x
// @auth: required
// @desc: this one is right
func (Service) Get(ctx context.Context, req *Req) (*Req, error) { return nil, nil }
