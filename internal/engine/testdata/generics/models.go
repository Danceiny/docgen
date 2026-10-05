// Package generics is a fixture: generic declarations that nothing instantiates.
package generics

// Page is a generic page of items.
type Page[T any] struct {
	// Items of the page.
	Items []T `json:"items"`
	// Total number of items.
	Total int `json:"total"`
	// First item.
	First T `json:"first"`
	// Next is the next page of the same type arguments.
	Next *Page[T] `json:"next"`
}

// Pair has two type parameters.
type Pair[K comparable, V any] struct {
	Key K `json:"key"`
	Val V `json:"val"`
	// Count is a field of a plain type.
	Count int `json:"count"`
}

// Num is a constraint of a type parameter.
type Num interface{ ~int | ~string }

// Boxed has a type parameter that has a constraint of its own.
type Boxed[T Num] struct {
	V T `json:"v"`
}

// Keyed is a map type with two type parameters, the first with a constraint.
type Keyed[K comparable, V any] map[K]V

// IntPage is declared as an instance of a generic type.
type IntPage Page[int]

// Apple is a candidate.
type Apple struct {
	Colour string `json:"colour"`
}

// Pear is another.
type Pear struct {
	Ripe bool `json:"ripe"`
}

// Wrap has only the generic field, which lists two candidates.
type Wrap[T any] struct {
	// @generic: Apple, Pear
	Data T `json:"data"`
}

// Basket holds an instance for each candidate.
type Basket struct {
	A Wrap[Apple] `json:"a"`
	B Wrap[Pear]  `json:"b"`
}

// Tagged has a type parameter with a constraint, and candidates for its field.
type Tagged[T Num] struct {
	// @generic: Apple, Pear
	V T `json:"v"`
}
