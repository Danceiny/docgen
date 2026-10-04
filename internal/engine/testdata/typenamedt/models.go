// Package typenamedt is a fixture: a type that happens to be named T.
package typenamedt

// T is an ordinary type, not a type parameter.
type T struct {
	Name string `json:"name"`
}

// Holder has a field of the type T next to ordinary fields.
type Holder struct {
	// Item is of the type T.
	Item *T `json:"item"`
	// Count is a plain field.
	Count int `json:"count"`
}
