// Package m is a fixture: maps, and slices of bytes.
package m

// Owner owns a pet.
type Owner struct {
	// Name of the owner.
	Name string `json:"name"`
}

// Pet has maps of several kinds.
type Pet struct {
	// Tags are free text.
	Tags map[string]string `json:"tags"`
	// Owners are by name.
	Owners map[string]Owner `json:"owners"`
	// Extra is anything.
	Extra map[string]any `json:"extra"`
	// Photo is a picture.
	Photo []byte `json:"photo"`
}
