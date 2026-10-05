// Package mapped is a fixture: a type of the module that the configuration gives a
// schema, used by structs declared before it and after it.
package mapped

// Before uses Money before it is declared.
type Before struct {
	// Price is the price.
	Price Money `json:"price"`
	Plain Money `json:"plain"`
}
