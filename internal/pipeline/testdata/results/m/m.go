// Package m is a fixture: the types the results of the operations are made of.
package m

// Product is sold.
type Product struct {
	// ID of it.
	ID string `json:"id"`
}

// Page is a generic page of items.
type Page[T any] struct {
	// Total number of items.
	Total int `json:"total"`
}

// Req asks.
type Req struct {
	// Q is the question.
	Q string `json:"q"`
}
