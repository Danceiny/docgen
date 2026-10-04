// struct_registry.go
package engine

import (
	"github.com/getkin/kin-openapi/openapi3"
)

type TypeDescriptor struct {
	FullKey     string // the name of the base type (such as "pet.domain.Category")
	IsPointer   bool   // whether it is a pointer
	Dimensions  int    // the array dimensions (for example [][]int → 2)
	Description string
}

// buildCustomSchema builds the schema of a custom type.
func buildCustomSchema(desc *TypeDescriptor) *openapi3.SchemaRef {
	refPath := "#/components/schemas/" + formatComponentKey(desc.FullKey)
	refSchema := openapi3.NewSchemaRef(refPath, defaultSchema())

	// handle the array dimensions
	for i := 0; i < desc.Dimensions; i++ {
		arraySchema := openapi3.NewArraySchema()
		arraySchema.Items = refSchema
		refSchema = openapi3.NewSchemaRef("", arraySchema)
	}

	return refSchema
}

// buildBasicSchema builds the schema of a basic type.
func buildBasicSchema(desc *TypeDescriptor) *openapi3.SchemaRef {
	schema := cloneSchema(getBasicTypeSchema(desc.FullKey))

	// handle the array dimensions
	for i := 0; i < desc.Dimensions; i++ {
		arraySchema := openapi3.NewArraySchema()
		arraySchema.Items = openapi3.NewSchemaRef("", schema)
		schema = arraySchema
	}

	schema.Description = desc.Description
	return openapi3.NewSchemaRef("", schema)
}
