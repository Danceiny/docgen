// Package m is a fixture: a type that docgen cannot describe.
package m

// Payload is a sealed interface that nothing implements, so there is nothing
// to describe it by.
//
// @autowire: true
type Payload interface{ sealed() }

// Holder has a field of it.
type Holder struct {
	P Payload `json:"p"`
}
