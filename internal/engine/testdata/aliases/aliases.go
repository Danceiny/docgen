// Package aliases is a fixture: names that stand for other types.
package aliases

// Early is an alias of a struct declared after it.
type Early = Later

// Later is a struct.
type Later struct {
	// Name of it.
	Name string `json:"name"`
}

// Struct is a struct.
type Struct struct {
	// Size of it.
	Size int `json:"size"`
}

// Late is an alias of a struct declared before it.
type Late = Struct

// Name is an alias of a basic type.
type Name = string

// User has fields of the aliases and pointers to them.
type User struct {
	A Early     `json:"a"`
	B *Early    `json:"b"`
	C Late      `json:"c"`
	D **Late    `json:"d"`
	E *[]Late   `json:"e"`
	F Name      `json:"f"`
	G *Name     `json:"g"`
	H *Struct   `json:"h"`
	I *Late     `json:"i"`
	J *[]Struct `json:"j"`
	K **Struct  `json:"k"`
	L []*Late   `json:"l"`
}
