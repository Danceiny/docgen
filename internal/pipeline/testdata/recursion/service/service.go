// Package service is the service of the recursion fixture.
package service

import (
	"context"

	"example.com/recursion/tree"
)

// TreeService is a service because its Name method returns a string.
type TreeService struct{}

// Name is the name of the service.
func (TreeService) Name() string { return "tree" }

// Get returns a tree.
//
// @tags: public
func (TreeService) Get(ctx context.Context, req *tree.GetReq) (*tree.Node, error) { return nil, nil }

// Browse returns a folder.
//
// @tags: public
func (TreeService) Browse(ctx context.Context, req *tree.GetReq) (*tree.Folder, error) {
	return nil, nil
}
