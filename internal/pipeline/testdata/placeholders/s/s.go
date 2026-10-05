// Package s is the service of the placeholders fixture.
package s

import (
	"context"

	"example.com/placeholders/m"
)

// Service is a service.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Get returns a holder.
func (Service) Get(ctx context.Context, req *m.Holder) (*m.Holder, error) { return nil, nil }
