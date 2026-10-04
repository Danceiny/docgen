// Package m is a fixture: the types of the params fixture.
package m

// Item is returned.
type Item struct {
	// ID of it.
	ID string `json:"id"`
}

// Req is a request body.
type Req struct {
	// Q is the question.
	Q string `json:"q"`
}
