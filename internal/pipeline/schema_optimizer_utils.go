package pipeline

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
)

// publicOptions are the settings of a public document, with the defaults filled in.
type publicOptions struct {
	tag                 string
	stripTagsContaining []string
	errorsLast          bool
}

// publicSettings fills in the defaults of a document's public section, which a
// configuration may leave out.
func publicSettings(p *config.Public) publicOptions {
	out := publicOptions{tag: config.DefaultPublicTag}
	if p == nil {
		return out
	}
	if p.Tag != "" {
		out.tag = p.Tag
	}
	out.stripTagsContaining = p.StripTagsContaining
	out.errorsLast = p.ErrorsLast
	return out
}

// containsAnyFold reports whether s contains one of the substrings, ignoring case.
func containsAnyFold(s string, substrings []string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrings {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// reorderAnyOfAllOf puts the alternatives that look like errors after the others,
// keeping the order within each group.
func reorderAnyOfAllOf(schemas []*openapi3.SchemaRef) []*openapi3.SchemaRef {
	if len(schemas) <= 1 {
		return schemas
	}

	var normalSchemas []*openapi3.SchemaRef
	var errorSchemas []*openapi3.SchemaRef

	for _, schemaRef := range schemas {
		if schemaRef.Value == nil {
			normalSchemas = append(normalSchemas, schemaRef)
			continue
		}
		if looksLikeErrorSchema(schemaRef.Value) {
			errorSchemas = append(errorSchemas, schemaRef)
		} else {
			normalSchemas = append(normalSchemas, schemaRef)
		}
	}

	result := make([]*openapi3.SchemaRef, 0, len(schemas))
	result = append(result, normalSchemas...)
	result = append(result, errorSchemas...)

	return result
}

// looksLikeErrorSchema says whether a schema describes an error: its title
// contains "err" in any case, or it has a "code" property that lists a positive
// number among its enum values.
func looksLikeErrorSchema(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}

	if strings.Contains(strings.ToLower(schema.Title), "err") {
		return true
	}

	if codeProp, exists := schema.Properties["code"]; exists && codeProp.Value != nil {
		for _, enumVal := range codeProp.Value.Enum {
			if codeVal, ok := enumVal.(float64); ok && codeVal > 0 {
				return true
			}
		}
	}

	return false
}
