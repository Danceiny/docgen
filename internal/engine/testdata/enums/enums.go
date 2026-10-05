// Package enums is a fixture: the ways a Go enum is declared.
package enums

import "time"

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
	AliasA    Alias  = "a"
	Other            = "not an Alias by its declaration"
	Unrelated string = "a string, which is what Alias is"
)

// Plain has no constants.
type Plain string

// Raw is made of a byte, and Letter of a rune: the values are numbers.
type Raw byte

// The raw values.
const (
	RawA Raw = 'a'
	RawB Raw = 2
)

// Letter is made of a rune.
type Letter rune

// LetterA is a letter.
const LetterA Letter = 'z'

// Up is made of a pointer-sized unsigned integer.
type Up uintptr

// UpOne is the one.
const UpOne Up = 1

// Dur is made of a time.Duration, which is an int64.
type Dur time.Duration

// The durations.
const (
	DurShort Dur = Dur(time.Second)
	DurLong  Dur = Dur(time.Hour)
)

// Wrapped is declared as another enum of the package, so it is made of a string.
type Wrapped Name

// W1 is a wrapped name.
const W1 Wrapped = "w1"

// Dup has constants that are other names for a value, and a sentinel.
type Dup int

// The dups.
const (
	DupA Dup = iota
	DupB
	DupAlias     = DupB
	DupMax   Dup = DupB
	dupCount Dup = 99
)
