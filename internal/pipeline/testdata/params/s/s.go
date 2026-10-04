// Package s is the service of the params fixture.
package s

import (
	"context"

	"example.com/params/m"
)

// Service is a service because its Name method returns a string.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Count counts the items in a state.
//
// @method: GET
// @param:query status string required "the state of the items"
func (Service) Count(ctx context.Context, status string) (*m.Item, error) { return nil, nil }

// ByID looks an item up.
//
// @method: GET
// @path: /byId/{id}
// @param:path id string optional "the id of the item"
func (Service) ByID(ctx context.Context, id string) (*m.Item, error) { return nil, nil }

// Many takes several parameters of basic types.
//
// @method: GET
// @param:query limit int optional "how many"
// @param:query tags []string optional
// @param:header token string required
func (Service) Many(ctx context.Context, limit int, tags []string, token string) (*m.Item, error) {
	return nil, nil
}

// Pos names its parameter by position.
//
// @method: GET
// @param:query param1 string required "the state"
func (Service) Pos(ctx context.Context, state string) (*m.Item, error) { return nil, nil }

// Unmatched has an annotation for a parameter it does not have.
//
// @param:query limit int required "how many"
func (Service) Unmatched(ctx context.Context, req *m.Req) (*m.Item, error) { return nil, nil }

// Plain takes a basic type that nothing describes: it is the body.
func (Service) Plain(ctx context.Context, name string) (*m.Item, error) { return nil, nil }
