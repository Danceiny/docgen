// Package fieldshapes has the fields that encoding/json reads in a way that is
// easy to get wrong.
package fieldshapes

import "encoding/json"

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
	Plain    string   `json:"plain" validate:"required"`
	Several  string   `json:"several" validate:"required,email"`
	Binding  int      `json:"binding" binding:"required,gt=0"`
	Spaced   string   `json:"spaced" validate:"min=1, required"`
	Optional string   `json:"optional" validate:"omitempty,email"`
	Other    string   `json:"other" validate:"required_if=Plain x"`
	Tagged   string   `json:"tagged" required:"true"`
	Elements []string `json:"elements" validate:"omitempty,dive,required"`
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

// Payload is any value.
type Payload any

// Alias is another name of any.
type Alias = any

// Raw is the empty interface.
type Raw interface{}

// Holders have fields of the types above.
type Holders struct {
	// P is a payload.
	P Payload `json:"p"`
	A Alias   `json:"a"`
	R Raw     `json:"r"`
	// Ps are payloads.
	Ps []Payload `json:"ps"`
}

// Raw2 holds JSON as it is.
type Raw2 struct {
	Body json.RawMessage `json:"body"`
}

// BeforeMapped has a field of a struct that type_map says is a string, declared
// after it.
type BeforeMapped struct {
	M Mapped `json:"m"`
}

// Mapped is a struct that the configuration describes as a string.
type Mapped struct {
	X int `json:"x"`
}

// AfterMapped has a field of the struct, declared before it.
type AfterMapped struct {
	M Mapped `json:"m"`
}

// Inner has a field the outer struct has too.
type Inner struct {
	Name string `json:"name" validate:"required"`
	Kept string `json:"kept"`
}

// OuterFirst writes its own name before it embeds Inner.
type OuterFirst struct {
	Name int `json:"name"`
	Inner
}

// OuterLast embeds Inner before it writes its own name.
type OuterLast struct {
	Inner
	Name int `json:"name"`
}

// Box is generic.
type Box[T any] struct {
	// Data is the payload.
	// @generic: Base,Inner
	Data T `json:"data"`
	// Note says something.
	Note string `json:"note"`
}

// Box2 lists its candidates with a space after each comma, and a text after them.
type Box2[T any] struct {
	// @generic: Base, Inner, OuterFirst; the payload
	Data T `json:"data"`
}

// NamesList is a list type, embedded below.
type NamesList []string

// EmbedsList embeds a list type, which encoding/json writes as a field named after it.
type EmbedsList struct {
	NamesList
	Other string `json:"other"`
}

// Pointers has a pointer to a type that has no schema.
type Pointers struct {
	C *complex64 `json:"c"`
	D *int       `json:"d"`
}
