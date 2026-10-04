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
