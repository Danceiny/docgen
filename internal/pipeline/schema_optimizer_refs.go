package pipeline

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// removeUnusedSchemas deletes the schema components that nothing refers to.
func removeUnusedSchemas(doc *openapi3.T, forceKeep []string) {
	// collect all schemas that are referenced
	usedSchemas := make(map[string]bool)
	var queue []string

	// first add the schemas that are always kept, and what they refer to is kept
	// with them
	for _, schemaName := range forceKeep {
		usedSchemas[schemaName] = true
		queue = append(queue, schemaName)
	}

	// the schemas the API paths refer to first
	for _, pathItem := range doc.Paths.Map() {
		operations := []*openapi3.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
			pathItem.Patch, pathItem.Head, pathItem.Options, pathItem.Trace,
		}

		for _, op := range operations {
			if op == nil {
				continue
			}

			// the schema references of the request bodies
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for _, content := range op.RequestBody.Value.Content {
					if content.Schema != nil {
						newRefs := collectSchemaRefs(content.Schema, usedSchemas)
						queue = append(queue, newRefs...)
					}
				}
			}

			// the schema references of the responses
			for _, response := range op.Responses.Map() {
				if response.Value != nil {
					for _, content := range response.Value.Content {
						if content.Schema != nil {
							newRefs := collectSchemaRefs(content.Schema, usedSchemas)
							queue = append(queue, newRefs...)
						}
					}
				}
			}

			// the schema references of the parameters
			for _, param := range op.Parameters {
				if param.Value != nil && param.Value.Schema != nil {
					newRefs := collectSchemaRefs(param.Value.Schema, usedSchemas)
					queue = append(queue, newRefs...)
				}
			}
		}
	}

	// go through all dependencies breadth-first with a queue, so that deep dependencies are found
	for len(queue) > 0 {
		schemaName := queue[0]
		queue = queue[1:]

		if schema, exists := doc.Components.Schemas[schemaName]; exists {
			newRefs := collectSchemaRefs(schema, usedSchemas)
			queue = append(queue, newRefs...)
		}
	}

	// delete the schemas that are not used
	for schemaName := range doc.Components.Schemas {
		if !usedSchemas[schemaName] {
			delete(doc.Components.Schemas, schemaName)
		}
	}
}

// collectSchemaRefs collects schema references recursively, and returns the dependencies it newly found.
func collectSchemaRefs(schemaRef *openapi3.SchemaRef, usedSchemas map[string]bool) []string {
	var newFound []string
	if schemaRef == nil {
		return newFound
	}

	// for a reference, take the name of the schema
	if schemaRef.Ref != "" {
		// take SchemaName from #/components/schemas/SchemaName
		if strings.HasPrefix(schemaRef.Ref, "#/components/schemas/") {
			schemaName := strings.TrimPrefix(schemaRef.Ref, "#/components/schemas/")
			if !usedSchemas[schemaName] {
				usedSchemas[schemaName] = true
				newFound = append(newFound, schemaName)
			}
		}
		return newFound
	}

	// for an inline schema, go through its properties recursively
	if schemaRef.Value != nil {
		schema := schemaRef.Value

		// object properties
		for _, propSchema := range schema.Properties {
			newFound = append(newFound, collectSchemaRefs(propSchema, usedSchemas)...)
		}

		// array items
		if schema.Items != nil {
			newFound = append(newFound, collectSchemaRefs(schema.Items, usedSchemas)...)
		}

		// allOf, oneOf, anyOf
		for _, subSchema := range schema.AllOf {
			newFound = append(newFound, collectSchemaRefs(subSchema, usedSchemas)...)
		}
		for _, subSchema := range schema.OneOf {
			newFound = append(newFound, collectSchemaRefs(subSchema, usedSchemas)...)
		}
		for _, subSchema := range schema.AnyOf {
			newFound = append(newFound, collectSchemaRefs(subSchema, usedSchemas)...)
		}

		// additionalProperties
		if schema.AdditionalProperties.Schema != nil {
			newFound = append(newFound, collectSchemaRefs(schema.AdditionalProperties.Schema, usedSchemas)...)
		}
	}
	return newFound
}
