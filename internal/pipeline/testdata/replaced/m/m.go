// Package m is a fixture: a type that an overlay replaces, which has a type that
// nothing else uses.
package m

// Address is only used by Owner.
type Address struct {
	// City of it.
	City string `json:"city"`
}

// Owner owns, and the overlay replaces it with a schema that has no address.
type Owner struct {
	// Address of the owner.
	Address Address `json:"address"`
}

// Pet has an owner, with a comment and without.
type Pet struct {
	// Owner is who owns the pet.
	Owner Owner `json:"owner"`
	Other Owner `json:"other"`
}
