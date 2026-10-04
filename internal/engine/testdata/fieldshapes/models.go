// Package fieldshapes has the fields that encoding/json reads in a way that is
// easy to get wrong.
package fieldshapes

// Base is embedded by the structs below.
type Base struct {
	ID string `json:"id"`
}

// Point declares two names in one field, and has a blank field.
type Point struct {
	Lat, Lng float64
	_        struct{}
}

// Nested embeds Base under a json name, which encoding/json makes a field of
// that name.
type Nested struct {
	Base `json:"base"`
	Name string `json:"name"`
}

// Flat embeds Base with no name, so its fields are the fields of Flat.
type Flat struct {
	Base
	Name string `json:"name"`
}

// hidden is embedded under a json name although its type is not exported.
type hidden struct {
	Secret string `json:"secret"`
}

// Quiet embeds a type that is not exported, under a json name.
type Quiet struct {
	hidden `json:"quiet"`
}

// Rules says which fields are required with the rules of a validator.
type Rules struct {
	Plain    string `json:"plain" validate:"required"`
	Several  string `json:"several" validate:"required,email"`
	Binding  int    `json:"binding" binding:"required,gt=0"`
	Spaced   string `json:"spaced" validate:"min=1, required"`
	Optional string `json:"optional" validate:"omitempty,email"`
	Other    string `json:"other" validate:"required_if=Plain x"`
	Tagged   string `json:"tagged" required:"true"`
}

// Anything has fields that take any JSON value.
type Anything struct {
	Any   any         `json:"any"`
	Empty interface{} `json:"empty"`
	List  []any       `json:"list"`
}

// Collections has maps and slices of bytes.
type Collections struct {
	Tags   map[string]string `json:"tags"`
	Owners map[string]Base   `json:"owners"`
	Any    map[string]any    `json:"any"`
	Data   []byte            `json:"data"`
	Raw    []uint8           `json:"raw"`
	Hash   [4]byte           `json:"hash"`
	One    byte              `json:"one"`
}
