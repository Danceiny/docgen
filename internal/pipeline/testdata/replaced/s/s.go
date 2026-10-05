// Package s is the service of the replaced fixture.
package s

import (
	"context"

	"example.com/replaced/m"
)

// Service is a service.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Get returns a pet.
//
// @tags: public
func (Service) Get(ctx context.Context, req *m.Pet) (*m.Pet, error) { return nil, nil }
