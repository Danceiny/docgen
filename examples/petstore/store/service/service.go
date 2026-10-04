// Package service holds the order service.
package service

import (
	"context"

	"example.com/petstore/store/protocol"
)

// OrderService has a name with a slash, so its routes are /api/store/order/...
type OrderService struct{}

// Name is the name of the service.
func (OrderService) Name() string { return "store/order" }

// Place adopts a pet.
//
// @tags: public
// @response:409,ConflictErr,The pet is already adopted
func (OrderService) Place(ctx context.Context, req *protocol.PlaceOrderReq) (*protocol.Order, error) {
	return nil, nil
}
