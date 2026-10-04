// Package embeddedcycles is a fixture: structs that embed themselves, directly
// and through each other. Go allows it with pointers, and encoding/json copes.
package embeddedcycles

// T embeds a pointer to itself.
type T struct {
	*T
	// Name of it.
	Name string `json:"name"`
}

// A and B embed each other.
type A struct {
	*B
	// X is a field of A.
	X int `json:"x"`
}

// B is embedded by A and embeds A.
type B struct {
	*A
	// Y is a field of B.
	Y int `json:"y"`
}

// Defaults has json tag options that look like a default and are not one.
type Defaults struct {
	// Plain has the option default with no value.
	Plain string `json:"plain,omitempty,default"`
	// Valued has a default.
	Valued string `json:"valued,default=x"`
}
