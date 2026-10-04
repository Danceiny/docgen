package pipeline

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
	"github.com/Danceiny/docgen/internal/errcat"

	"github.com/getkin/kin-openapi/openapi3"
)

// engineSettings turns the configuration into the settings the engine reads.
func engineSettings(cfg *config.Config) engine.Settings {
	s := engine.Settings{
		HeaderTypes:   cfg.Headers.Types,
		DefaultHeader: cfg.Headers.Default,
		ErrorPrefix:   cfg.Errors.ComponentPrefix,

		APIPrefix:        cfg.API.Prefix,
		SkipMethods:      cfg.API.SkipMethods,
		DefaultStatuses:  cfg.Response.DefaultStatuses,
		VendorExtensions: cfg.VendorExtensions,
		KeepEmptyTags:    cfg.KeepEmptyTags,

		FieldTokens:                fieldTokensOf(cfg),
		CompatLegacyOperationTypes: cfg.Compat.LegacyOperationTypes,
		CompatLegacySchemaShapes:   cfg.Compat.LegacySchemaShapes,
	}
	if env := cfg.Response.Envelope; env != nil {
		s.Envelope = &engine.Envelope{Code: env.Code, Message: env.Message, Data: env.Data}
	}
	if len(cfg.TypeMap) > 0 {
		s.TypeMap = make(map[string]*openapi3.Schema, len(cfg.TypeMap))
		for key, spec := range cfg.TypeMap {
			s.TypeMap[key] = schemaFromSpec(spec)
		}
	}
	if len(cfg.Request.Multipart) > 0 {
		s.Multipart = make(map[string][]engine.MultipartField, len(cfg.Request.Multipart))
		for key, fields := range cfg.Request.Multipart {
			for _, f := range fields {
				s.Multipart[key] = append(s.Multipart[key], engine.MultipartField{Name: f.Name, Kind: f.Kind, ScalarTo: f.ScalarTo, Required: f.Required})
			}
		}
	}
	if len(cfg.Request.Query) > 0 {
		s.QueryString = make(map[string][]engine.QueryField, len(cfg.Request.Query))
		for key, fields := range cfg.Request.Query {
			for _, f := range fields {
				s.QueryString[key] = append(s.QueryString[key], engine.QueryField{Name: f.Name, Type: f.Type, Required: f.Required, Description: f.Description})
			}
		}
	}
	if len(cfg.Request.RuntimeOnly) > 0 {
		s.RuntimeOnly = make(map[engine.RuntimeInputKey]struct{}, len(cfg.Request.RuntimeOnly))
		for _, ro := range cfg.Request.RuntimeOnly {
			s.RuntimeOnly[engine.RuntimeInputKey{Path: ro.Path, TypeKey: ro.Type}] = struct{}{}
		}
	}
	if len(cfg.Response.Binary) > 0 {
		s.BinaryResponses = make(map[string]engine.BinaryResponse, len(cfg.Response.Binary))
		for key, b := range cfg.Response.Binary {
			s.BinaryResponses[key] = engine.BinaryResponse{Description: b.Description, ContentTypes: b.ContentTypes, Errors: b.Errors}
		}
	}
	return s
}

// loadErrorCatalog reads the error catalog the configuration names, which is
// relative to the module directory. It returns nil when the configuration names
// none.
func loadErrorCatalog(dir string, e config.Errors) (*errcat.Catalog, error) {
	if e.File == "" {
		return nil, nil
	}
	c, err := errcat.Load(filepath.Join(dir, e.File))
	if err != nil {
		return nil, fmt.Errorf("errors.file: %w", err)
	}
	return c, nil
}

// schemaFromSpec builds the OpenAPI schema a type_map entry describes. The
// config package has already checked that the type is one of the known ones.
func schemaFromSpec(spec config.TypeSpec) *openapi3.Schema {
	var s *openapi3.Schema
	switch spec.Type {
	case "string":
		s = openapi3.NewStringSchema()
	case "integer":
		s = openapi3.NewIntegerSchema()
	case "number":
		s = openapi3.NewFloat64Schema()
	case "boolean":
		s = openapi3.NewBoolSchema()
	case "object":
		s = openapi3.NewObjectSchema()
	case "array":
		s = openapi3.NewArraySchema().WithItems(schemaFromSpec(*spec.Items))
	}
	if spec.Format != "" {
		s = s.WithFormat(spec.Format)
	}
	if spec.Description != "" {
		s.Description = spec.Description
	}
	if spec.Example != nil {
		s.Example = spec.Example
	}
	return s
}

// fieldTokensOf lists the legacy field tokens that any document of the
// configuration names.
func fieldTokensOf(cfg *config.Config) []string {
	var tokens []string
	for _, d := range cfg.Docs {
		for _, token := range d.LegacyFieldTokens {
			if !slices.Contains(tokens, token) {
				tokens = append(tokens, token)
			}
		}
	}
	return tokens
}
