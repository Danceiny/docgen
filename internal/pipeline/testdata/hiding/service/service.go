// Package service is the service of the hiding fixture.
package service

import (
	"context"

	"example.com/hiding/domain"
)

// ShopService is a service because its Name method returns a string.
type ShopService struct{}

// Name is the name of the service.
func (ShopService) Name() string { return "shop" }

// Get returns an order.
//
// @tags: public
func (ShopService) Get(ctx context.Context, req *domain.Order) (*domain.Order, error) {
	return nil, nil
}

// Risk returns a type that the public document hides.
//
// @tags: public
func (ShopService) Risk(ctx context.Context, req *domain.Order) (*domain.InternalBefore, error) {
	return nil, nil
}

// Level returns an enum that the public document hides by its name.
//
// @tags: public
func (ShopService) Level(ctx context.Context, req *domain.Order) (*domain.InternalLevel, error) {
	return nil, nil
}

// Note takes a type that the public document hides.
//
// @tags: public
func (ShopService) Note(ctx context.Context, req *domain.StaffNote) (*domain.Order, error) {
	return nil, nil
}

// Made takes a type that is declared as one that the public document hides.
//
// @tags: public
func (ShopService) Made(ctx context.Context, req *domain.Derived) (*domain.Order, error) {
	return nil, nil
}

// Made2 returns a list that is declared as a list of one.
//
// @tags: public
func (ShopService) Made2(ctx context.Context, req *domain.Order) (*domain.DerivedList, error) {
	return nil, nil
}
