// Package enums is a fixture: the ways a Go enum is declared.
package enums

// Priority repeats its type implicitly after the first constant.
type Priority int

// The priorities.
const (
	// Low is the first and has the iota.
	Low Priority = iota
	// Medium is the second.
	Medium
	High // High is the third.
)

// Level has a negative literal and an expression.
type Level int

const (
	Unknown Level = -1
	Base    Level = 10
	Next    Level = Base + 5
)

// Mask has constants made of shifts.
type Mask uint8

const (
	Read Mask = 1 << iota
	Write
	Exec
)

// Name has strings made of a constant, and a blank one.
type Name string

const prefix = "x-"

const (
	NameA Name = prefix + "a"
	NameB Name = "b"
	_     Name = "skipped"
)

// Alias is another name of string, so every string constant is of its type.
type Alias = string

const (
	AliasA Alias = "a"
	Other        = "not an Alias by its declaration"
)

// Plain has no constants.
type Plain string
