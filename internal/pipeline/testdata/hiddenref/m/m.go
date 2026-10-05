// Package m is a fixture: a type that the public document hides by its name, and
// a type that an overlay makes refer to it.
package m

// InternalThing is hidden from the public document by its name.
type InternalThing struct {
	// Secret is not for customers.
	Secret string `json:"secret"`
}

// Holder is shown, and an overlay replaces it.
type Holder struct {
	// Name of the holder.
	Name string `json:"name"`
}
