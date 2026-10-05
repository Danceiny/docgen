package generics

// EarlyAlias is an instance of a generic type that is declared further down.
type EarlyAlias = Page[int]

// EarlyDefined is a defined type of an instance of one.
type EarlyDefined Page[string]
