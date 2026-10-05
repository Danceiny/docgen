// Package chains is a fixture: types declared as other types, in every order.
package chains

import "fmt"

// A1 is an alias of an alias of an alias of a type declared below.
type A1 = A2

// A2 is an alias.
type A2 = A3

// A3 is an alias of Money.
type A3 = Money

// Money2 is defined as Money, and Money3 as Money2.
type Money2 Money

// Money3 is defined as Money2.
type Money3 Money2

// Level is an enum.
type Level int

// The levels.
const (
	LevelLow Level = iota
	LevelHigh
)

// Level2 is defined as an enum.
type Level2 Level

// Names is a list of names.
type Names []string

// Names2 is defined as Names.
type Names2 Names

// Dict is a map.
type Dict map[string]string

// Dict2 is defined as Dict.
type Dict2 Dict

// Money is declared after the types that are made of it.
type Money struct {
	// Amount of it.
	Amount int `json:"amount"`
}

// Speaker is an interface.
type Speaker interface{ Speak() string }

// Holder has fields of all of them.
type Holder struct {
	A A1                          `json:"a"`
	M Money3                      `json:"m"`
	L Level2                      `json:"l" example:"1"`
	N Names2                      `json:"n"`
	D Dict2                       `json:"d"`
	S fmt.Stringer                `json:"s"`
	T Speaker                     `json:"t"`
	U interface{ Speak() string } `json:"u"`
}
