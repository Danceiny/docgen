// Package nojson is a fixture: types JSON has no value for, in lists and maps.
package nojson

// Pages has complex numbers in a list and in a map.
type Pages struct {
	Roots []complex64           `json:"roots"`
	ByKey map[string]complex128 `json:"byKey"`
	Size  int                   `json:"size"`
}
