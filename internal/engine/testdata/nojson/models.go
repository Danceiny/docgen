// Package nojson is a fixture: types JSON has no value for, in lists and maps.
package nojson

import "unsafe"

// Pages has complex numbers and unsafe pointers in lists and in maps.
type Pages struct {
	Roots []complex64               `json:"roots"`
	ByKey map[string]complex128     `json:"byKey"`
	Raw   []unsafe.Pointer          `json:"raw"`
	ByRaw map[string]unsafe.Pointer `json:"byRaw"`
	Size  int                       `json:"size"`
}
