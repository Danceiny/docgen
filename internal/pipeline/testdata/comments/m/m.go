// Package m is a fixture: fields that have comments, of types declared before and
// after the struct that uses them.
package m

// Early is declared before Pet.
type Early struct {
	// Year of it.
	Year int `json:"year"`
}

// Kind says what a pet is.
type Kind int

// The kinds.
const (
	// KindDog barks.
	KindDog Kind = iota
	// KindCat purrs.
	KindCat
)

// Pet is an animal, with fields of types declared before and after it.
type Pet struct {
	// Home is of a type declared after Pet.
	Home Birth `json:"home"`
	// Before is of a type declared before Pet.
	Before Early `json:"before"`
	// Plain says nothing about its type.
	Plain Birth `json:"plain"`
	Bare  Birth `json:"bare"`
	// Kind is what the pet is.
	Kind Kind `json:"kind" example:"1" default:"0"`
	// Others own it too.
	Others []Birth `json:"others"`
	// Matrix is a matrix of numbers.
	Matrix [][]int `json:"matrix"`
	// Maybe may be missing.
	Maybe *Birth `json:"maybe,nullable"`
	// Tags are words.
	Tags []string `json:"tags"`
}

// Birth is when and where.
type Birth struct {
	// Place of it.
	Place string `json:"place"`
}
