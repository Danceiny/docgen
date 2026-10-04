package engine

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

// dedupe removes repeated elements, keeping the first of each in order, so that
// the result does not depend on how a hash table happens to iterate. An empty
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

// stringPtr returns nil for the empty string and a pointer to a copy otherwise.
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
