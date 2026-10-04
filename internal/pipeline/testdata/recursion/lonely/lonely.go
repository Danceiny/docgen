// Package lonely imports nothing, and nothing imports it.
package lonely

// Lonely is a type of a package that is a model package and has no neighbours.
type Lonely struct {
	// Name of it.
	Name string `json:"name"`
}
