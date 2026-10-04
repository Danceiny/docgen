package pipeline

import (
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// filter returns the elements of s that satisfy keep, in order, as a new slice
// that is never nil.
func filter[T any](s []T, keep func(T) bool) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// dedupe removes repeated elements, keeping the first of each in order. An empty
// input is returned as it is.
func dedupe[T comparable](s []T) []T {
	if len(s) == 0 {
		return s
	}
	seen := make(map[T]struct{}, len(s))
	out := make([]T, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// sortedKeys returns the keys of the components in order, for loops that rewrite
// them in place: the result must not depend on how a hash table iterates.
func sortedKeys(schemas openapi3.Schemas) []string {
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
