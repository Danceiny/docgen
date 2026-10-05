// Package m is a fixture: types that use types of a package that models does not match.
package m

import "example.com/ondemand/other"

// Holder uses them.
type Holder struct {
	// First is the first use of the thing, and it is nullable.
	First other.Thing `json:"first,nullable" example:"{}"`
	// Second is the second use.
	Second other.Thing `json:"second"`
	Plain  other.Thing `json:"plain"`
	Raw    other.Raw   `json:"raw"`
	Dur    other.Dur   `json:"dur"`
}
