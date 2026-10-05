// Package s is the service of the orders fixture.
package s

import (
	"context"

	"example.com/orders/m"
)

// Service is a service.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Get returns an order.
//
// @tags: public
func (Service) Get(ctx context.Context, req *m.Order) (*m.Order, error) { return nil, nil }
