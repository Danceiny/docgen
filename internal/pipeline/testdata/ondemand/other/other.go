// Package other is a package of the module that no pattern of models matches.
package other

import "time"

// Thing is a thing in another package.
type Thing struct {
	// Name of the thing.
	Name string `json:"name"`
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
