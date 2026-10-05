// Package s is the service of the examples fixture.
package s

import (
	"context"

	"example.com/examples/m"
)

// Service is a service.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "owner" }

// Get returns an owner.
//
// @tags: public
func (Service) Get(ctx context.Context, req *m.Owner) (*m.Owner, error) { return nil, nil }
