package pipeline

import (
	"strings"

	"github.com/Danceiny/docgen/internal/engine"

	"github.com/getkin/kin-openapi/openapi3"
)

func simplifyTitles(doc *openapi3.T) {
	// schemas in Components.Schemas
	for _, schema := range doc.Components.Schemas {
		if schema.Value == nil {
			continue
		}
		simplifySchemaTitles(schema.Value)
	}

	// schemas written inline in the paths
	for _, pathItem := range doc.Paths.Map() {
		operations := []*openapi3.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
			pathItem.Patch, pathItem.Head, pathItem.Options, pathItem.Trace,
		}

		for _, op := range operations {
			if op == nil {
				continue
			}

			// schemas of the request parameters
			for _, param := range op.Parameters {
				if param.Value != nil && param.Value.Schema != nil {
					simplifySchemaTitles(param.Value.Schema.Value)
				}
			}

			// schemas of the request bodies
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for _, content := range op.RequestBody.Value.Content {
					if content.Schema != nil {
						simplifySchemaTitles(content.Schema.Value)
					}
				}
			}

			// schemas of the responses
			for _, response := range op.Responses.Map() {
				if response.Value != nil {
					for _, content := range response.Value.Content {
						if content.Schema != nil {
							simplifySchemaTitles(content.Schema.Value)
						}
					}
				}
			}
		}
	}
}

// simplifySchemaTitles simplifies the title of a schema and of everything nested in it.
func simplifySchemaTitles(schema *openapi3.Schema) {
	if schema == nil {
		return
	}

	// simplify the title of this schema
	schema.Title = simplifyTitle(schema.Title)

	// go through all properties
	for _, prop := range schema.Properties {
		if prop.Value != nil {
			simplifySchemaTitles(prop.Value)
		}
	}

	// array items
	if schema.Items != nil && schema.Items.Value != nil {
		simplifySchemaTitles(schema.Items.Value)
	}

	// allOf, oneOf, anyOf
	for _, subSchema := range schema.AllOf {
		if subSchema.Value != nil {
			simplifySchemaTitles(subSchema.Value)
		}
	}
	for _, subSchema := range schema.OneOf {
		if subSchema.Value != nil {
			simplifySchemaTitles(subSchema.Value)
		}
	}
	for _, subSchema := range schema.AnyOf {
		if subSchema.Value != nil {
			simplifySchemaTitles(subSchema.Value)
		}
	}

	// additionalProperties
	if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
		simplifySchemaTitles(schema.AdditionalProperties.Schema.Value)
	}
}

func simplifyTitle(v string) string {
	if v == "" {
		return v
	}

	vs := strings.Split(v, ".")
	return vs[len(vs)-1]
}

// optimizeAnyOfAllOfOrder orders anyOf/allOf so that the error responses come last.
func optimizeAnyOfAllOfOrder(doc *openapi3.T) {
	// schemas in Components.Schemas
	for _, schema := range doc.Components.Schemas {
		if schema.Value == nil {
			continue
		}
		optimizeSchemaAnyOfAllOfOrder(schema.Value)
	}

	// schemas written inline in the paths
	for _, pathItem := range doc.Paths.Map() {
		operations := []*openapi3.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
			pathItem.Patch, pathItem.Head, pathItem.Options, pathItem.Trace,
		}

		for _, op := range operations {
			if op == nil {
				continue
			}

			// schemas of the request parameters
			for _, param := range op.Parameters {
				if param.Value != nil && param.Value.Schema != nil {
					optimizeSchemaAnyOfAllOfOrder(param.Value.Schema.Value)
				}
			}

			// schemas of the request bodies
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for _, content := range op.RequestBody.Value.Content {
					if content.Schema != nil {
						optimizeSchemaAnyOfAllOfOrder(content.Schema.Value)
					}
				}
			}

			// schemas of the responses
			for _, response := range op.Responses.Map() {
				if response.Value != nil {
					for _, content := range response.Value.Content {
						if content.Schema != nil {
							optimizeSchemaAnyOfAllOfOrder(content.Schema.Value)
						}
					}
				}
			}
		}
	}
}

// optimizeSchemaAnyOfAllOfOrder orders anyOf/allOf in a schema and in everything nested in it.
func optimizeSchemaAnyOfAllOfOrder(schema *openapi3.Schema) {
	if schema == nil {
		return
	}

	// order anyOf/allOf of this schema
	optimizeAnyOfAllOfOrderInSchema(schema)

	// go through all properties
	for _, prop := range schema.Properties {
		if prop.Value != nil {
			optimizeSchemaAnyOfAllOfOrder(prop.Value)
		}
	}

	// array items
	if schema.Items != nil && schema.Items.Value != nil {
		optimizeSchemaAnyOfAllOfOrder(schema.Items.Value)
	}

	// allOf, oneOf, anyOf
	for _, subSchema := range schema.AllOf {
		if subSchema.Value != nil {
			optimizeSchemaAnyOfAllOfOrder(subSchema.Value)
		}
	}
	for _, subSchema := range schema.OneOf {
		if subSchema.Value != nil {
			optimizeSchemaAnyOfAllOfOrder(subSchema.Value)
		}
	}
	for _, subSchema := range schema.AnyOf {
		if subSchema.Value != nil {
			optimizeSchemaAnyOfAllOfOrder(subSchema.Value)
		}
	}

	// additionalProperties
	if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
		optimizeSchemaAnyOfAllOfOrder(schema.AdditionalProperties.Schema.Value)
	}
}

// optimizeAnyOfAllOfOrderInSchema orders anyOf/allOf of a single schema.
func optimizeAnyOfAllOfOrderInSchema(schema *openapi3.Schema) {
	if schema == nil {
		return
	}

	// order anyOf
	if len(schema.AnyOf) > 1 {
		schema.AnyOf = reorderAnyOfAllOf(schema.AnyOf)
	}

	// order allOf
	if len(schema.AllOf) > 1 {
		schema.AllOf = reorderAnyOfAllOf(schema.AllOf)
	}

	// order oneOf
	if len(schema.OneOf) > 1 {
		schema.OneOf = reorderAnyOfAllOf(schema.OneOf)
	}
}

// filterPublicPaths filters the paths and keeps the operations that have the public tag.
func filterPublicPaths(doc *openapi3.T, publicTag string) {
	pathsToRemove := make([]string, 0)

	for path, pathItem := range doc.Paths.Map() {
		operations := []*openapi3.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
			pathItem.Patch, pathItem.Head, pathItem.Options, pathItem.Trace,
		}

		hasOpenAPIOperation := false
		for _, op := range operations {
			if op == nil {
				continue
			}

			// does it have the public tag (case-insensitive)?
			for _, tag := range op.Tags {
				if strings.EqualFold(tag, publicTag) {
					hasOpenAPIOperation = true
					break
				}
			}
			if hasOpenAPIOperation {
				break
			}
		}

		// a path with no operation that has the public tag is marked for deletion
		if !hasOpenAPIOperation {
			pathsToRemove = append(pathsToRemove, path)
		}
	}

	// delete the marked paths
	for _, path := range pathsToRemove {
		doc.Paths.Delete(path)
	}
}

// optimizeTags removes from every operation the tags that contain one of the
// strings to strip, in any case, except the public tag.
func optimizeTags(doc *openapi3.T, public publicOptions) {
	for _, pathItem := range doc.Paths.Map() {
		operations := []*openapi3.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
			pathItem.Patch, pathItem.Head, pathItem.Options, pathItem.Trace,
		}

		for _, op := range operations {
			if op == nil {
				continue
			}

			filteredTags := make([]string, 0)
			for _, tag := range op.Tags {
				if strings.EqualFold(tag, public.tag) || !containsAnyFold(tag, public.stripTagsContaining) {
					filteredTags = append(filteredTags, tag)
				}
			}
			op.Tags = filteredTags
		}
	}
}

// printOptimizationStats logs statistics of the optimization.
func printOptimizationStats(doc *openapi3.T) {
	pathCount := len(doc.Paths.Map())
	schemaCount := len(doc.Components.Schemas)
	tagCount := len(doc.Tags)

	// count the operations
	operationCount := 0
	for _, pathItem := range doc.Paths.Map() {
		operations := []*openapi3.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
			pathItem.Patch, pathItem.Head, pathItem.Options, pathItem.Trace,
		}
		for _, op := range operations {
			if op != nil {
				operationCount++
			}
		}
	}

	if operationCount == 0 {
		engine.Logger().Warn("the public document has no operations: none has the public tag (the tag of the operations that are public is public.tag in the configuration, \"public\" by default)")
	}
	engine.Logger().Info("public document optimized",
		"paths", pathCount, "operations", operationCount, "schemas", schemaCount,
		"tags", tagCount, "servers", len(doc.Servers))
}
