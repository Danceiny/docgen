package engine

import (
	"github.com/getkin/kin-openapi/openapi3"
)

// MountAllErrors adds a component for every error of the catalog; with none it
// adds nothing.
func MountAllErrors(doc *openapi3.T) {
	for _, err := range settings.Errors.Entries() {
		is := openapi3.NewIntegerSchema()
		is.Example = err.Code

		ss := openapi3.NewStringSchema()
		ss.Example = err.Message
		fk := errorKey(err.Name)
		doc.Components.Schemas[fk] = &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Title:       fk,
				Type:        &openapi3.Types{"object"},
				Description: err.Name,
				Properties: map[string]*openapi3.SchemaRef{
					"code":    is.NewRef(),
					"message": ss.NewRef(),
				},
			},
		}
	}
}
