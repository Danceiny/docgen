// Package s is the service of the results fixture.
package s

import (
	"context"

	"example.com/results/m"
)

// Service is a service because its Name method returns a string.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// One returns a product.
func (Service) One(ctx context.Context, req *m.Req) (*m.Product, error) { return nil, nil }

// List returns a list of products.
func (Service) List(ctx context.Context, req *m.Req) ([]m.Product, error) { return nil, nil }

// Pointers returns a list of pointers.
func (Service) Pointers(ctx context.Context, req *m.Req) ([]*m.Product, error) { return nil, nil }

// Grid returns a list of lists.
func (Service) Grid(ctx context.Context, req *m.Req) ([][]m.Product, error) { return nil, nil }

// Names returns a list of strings.
func (Service) Names(ctx context.Context, req *m.Req) ([]string, error) { return nil, nil }

// Counts returns a map.
func (Service) Counts(ctx context.Context, req *m.Req) (map[string]int, error) { return nil, nil }

// Paged returns a generic page.
func (Service) Paged(ctx context.Context, req *m.Req) (*m.Page[m.Product], error) { return nil, nil }
