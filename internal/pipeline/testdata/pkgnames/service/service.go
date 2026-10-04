// Package service is the service of the pkgnames fixture.
package service

import (
	"context"

	"example.com/pkgnames/model"
	"example.com/pkgnames/proto/v2"
)

// Service is a service because its Name method returns a string.
type Service struct{}

// Name is the name of the service.
func (Service) Name() string { return "shop" }

// Ask takes a type of package models and returns one of package proto.
func (Service) Ask(ctx context.Context, req *models.Req) (*proto.Reply, error) { return nil, nil }

// Answer takes a type of package proto and returns one of package models.
func (Service) Answer(ctx context.Context, req *proto.Reply) (*models.Resp, error) { return nil, nil }
