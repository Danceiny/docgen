// Package m is a fixture: fields with examples, of types declared before and after
// them, and a type that an overlay can replace.
package m

// Owner has examples of types that are declared further down.
type Owner struct {
	// Home is where the owner lives.
	Home Address `json:"home" example:"{\"street\":\"Main\"}"`
	// Status is the status of the owner.
	Status Status `json:"status" example:"active"`
	Level  Level  `json:"level" example:"2" default:"1"`
	Later  Later  `json:"later" example:"{\"city\":\"Dubai\"}"`
}

// Address is a place; an overlay replaces it in one of the configurations.
type Address struct {
	Street string `json:"street"`
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

// Later is declared after the struct that has an example of it.
type Later struct {
	City string `json:"city" validate:"required"`
}
