// Package m is a fixture: an embedded struct that has a field of a hidden type.
package m

// InternalNote is hidden from the public document by its name.
type InternalNote struct {
	Text string `json:"text"`
}

// Embedded has a field of a hidden type.
type Embedded struct {
	Hidden InternalNote `json:"hidden"`
	Shown  string       `json:"shown"`
}

// Order embeds it.
type Order struct {
	Embedded
	ID string `json:"id"`
}
