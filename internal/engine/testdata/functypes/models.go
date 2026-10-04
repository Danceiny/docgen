// Package functypes is a fixture: structs with fields whose types are named
// function and channel types, which no schema can describe.
package functypes

import "github.com/Danceiny/docgen/internal/engine/testdata/functypes/hooks"

// Handler is a function type of this package.
type Handler func(string) error

// Events is a channel type.
type Events chan string

// Options has fields a schema cannot describe next to fields it can.
type Options struct {
	Name    string         `json:"name"`
	OnDone  Handler        `json:"onDone"`
	Events  Events         `json:"events"`
	OnEvent hooks.Callback `json:"onEvent"`
	Retries int            `json:"retries"`
	// Hook is a function type that was not given a name.
	Hook func(string) `json:"hook"`
	// Ticks is a channel type that was not given a name.
	Ticks <-chan int `json:"ticks"`
	// Handlers is a list of a named function type.
	Handlers []Handler `json:"handlers"`
	// Hooks is a list of a function type that was not given a name.
	Hooks []func() `json:"hooks"`
	// ByName is a map of a named function type.
	ByName map[string]Handler `json:"byName"`
}
