// Package s is a fixture: a service.
package s

import (
	"context"

	"example.com/maps/m"
)

// Service is a service.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Get returns a pet.
//
// @tags: public
func (Service) Get(ctx context.Context, req *m.Pet) (*m.Pet, error) { return nil, nil }
