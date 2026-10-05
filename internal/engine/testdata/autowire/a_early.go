// Package autowire is a fixture: unions made of the types that implement an
// interface, declared before and after the structs that use them.
package autowire

// Early is a union that is declared before the struct that uses it.
//
// @autowire: true
type Early interface {
	isEarly()
}

// Alpha is an alternative of Early.
type Alpha struct {
	A string `json:"a"`
}

func (Alpha) isEarly() {}

// Beta is an alternative of Early.
type Beta struct {
	B int `json:"b"`
}

func (Beta) isEarly() {}
