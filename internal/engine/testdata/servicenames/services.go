// Package servicenames is a fixture: the ways a service can return its name.
package servicenames

import "github.com/Danceiny/docgen/internal/engine/testdata/servicenames/names"

// Literal returns a literal.
type Literal struct{}

// Name is the name.
func (Literal) Name() string { return "literal" }

// SameFile returns a constant of its own file.
type SameFile struct{}

const sameFile = "samefile"

// Name is the name.
func (SameFile) Name() string { return sameFile }

// OtherFile returns a constant of another file.
type OtherFile struct{}

// Name is the name.
func (OtherFile) Name() string { return fromAnotherFile }

// OtherPackage returns a constant of another package.
type OtherPackage struct{}

// Name is the name.
func (OtherPackage) Name() string { return names.Order }

// Joined returns constants joined.
type Joined struct{}

// Name is the name.
func (Joined) Name() string { return prefix + "/joined" }

// Variable returns a variable: it is not a constant, so it is not a service.
type Variable struct{}

// Name is the name.
func (Variable) Name() string { return notConstant }

// Computed returns what a function computes: not a service either.
type Computed struct{}

// Name is the name.
func (Computed) Name() string { return name() }

// Each of them has a method that makes it a service if it is one.
func (Literal) Get()      {}
func (SameFile) Get()     {}
func (OtherFile) Get()    {}
func (OtherPackage) Get() {}
func (Joined) Get()       {}
func (Variable) Get()     {}
func (Computed) Get()     {}
