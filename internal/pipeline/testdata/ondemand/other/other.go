// Package other is a package of the module that no pattern of models matches.
package other

import "time"

// Thing is a thing in another package.
type Thing struct {
	// Name of the thing.
	Name string `json:"name"`
	// Place is where it is, and its type is reached through this one only.
	Place Place `json:"place"`
	// Shape is one of the shapes of the package.
	Shape Shape `json:"shape"`
}

// Raw is a byte-based enum.
type Raw byte

// The raw values.
const (
	RawA Raw = 'a'
	RawB Raw = 'b'
)

// Dur is an enum of durations.
type Dur time.Duration

// The durations.
const (
	DurShort Dur = Dur(time.Second)
	DurLong  Dur = Dur(time.Minute)
)

// Place is only reached through Thing, and refers to types declared after it, a
// list and a map of them, and to itself and to what refers to it.
type Place struct {
	Country Country        `json:"country" example:"AE" default:"AE"`
	Geo     *Geo           `json:"geo"`
	Tags    []Tag          `json:"tags"`
	ByName  map[string]Tag `json:"byName"`
	Parent  *Place         `json:"parent"`
	Owner   *Thing         `json:"owner"`
}

// Country is an enum, and no package of models reaches it but through Place.
type Country string

// The countries.
const (
	CountryAE Country = "AE"
	CountryGB Country = "GB"
)

// Geo is a position.
type Geo struct {
	Lat float64 `json:"lat"`
}

// Tag is a label.
type Tag struct {
	Name string `json:"name"`
}

// Shape is a union of the shapes of the package that implement it, and no package
// of models reaches them but through Thing.
//
// @autowire: true
type Shape interface {
	shape()
}

// Circle is a shape.
type Circle struct {
	Radius float64 `json:"radius"`
}

func (Circle) shape() {}

// Square is a shape that has the fields of Base too.
type Square struct {
	Base
	Side float64 `json:"side"`
}

func (Square) shape() {}

// Base is embedded.
type Base struct {
	Name string `json:"name" validate:"required"`
}

// Secret is a shape that no document shows.
//
//apidoc:hidden
type Secret struct {
	Key string `json:"key"`
}

func (Secret) shape() {}
