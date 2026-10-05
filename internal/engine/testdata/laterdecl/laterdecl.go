// Package laterdecl is a fixture: fields with an example or a default whose types
// are declared further down the package than the fields.
package laterdecl

// Order has fields of types that are declared after it.
type Order struct {
	Status   Status            `json:"status" example:"active" default:"new"`
	Bad      Status            `json:"bad" example:"bogus"`
	Level    *Level            `json:"level,nullable" example:"2"`
	Addr     Addr              `json:"addr" example:"{\"city\":\"Dubai\"}"`
	BadAddr  Addr              `json:"badAddr" example:"{\"city\":1}"`
	NoCity   Addr              `json:"noCity" example:"{}"`
	Tags     []Status          `json:"tags" example:"[\"active\"]"`
	BadTags  []Status          `json:"badTags" example:"[\"bogus\"]"`
	ByKey    map[string]Status `json:"byKey" example:"{\"a\":\"new\"}"`
	Derived  Derived           `json:"derived" example:"active"`
	BadChild Child             `json:"badChild" example:"{\"name\":\"n\"}"`
	Child    Child             `json:"child" example:"{\"id\":\"x\"}"`
}

// Status is an enum of text.
type Status string

// The statuses.
const (
	StatusNew    Status = "new"
	StatusActive Status = "active"
)

// Level is an enum of numbers.
type Level int

// The levels.
const (
	LevelLow  Level = 1
	LevelHigh Level = 2
)

// Addr is where something is, and it is somewhere.
type Addr struct {
	City string `json:"city" validate:"required"`
	Zip  int    `json:"zip"`
}

// Derived is declared as a type that is declared after it.
type Derived Status

// Child gets its required field from the struct it embeds, which is declared
// after it.
type Child struct {
	Base
	Name string `json:"name"`
}

// Base is embedded.
type Base struct {
	ID string `json:"id" validate:"required"`
}
