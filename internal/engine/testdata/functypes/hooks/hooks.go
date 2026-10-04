// Package hooks is a fixture: it declares a function type of another package.
package hooks

// Callback is called when something happens.
type Callback func(event string) error
