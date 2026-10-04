package engine

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// fullComponentName makes the full component key of a type.
func fullComponentName(pkgPath, typeName string) string {
	if pkgPath == "" {
		return typeName
	}
	return formatComponentKey(fmt.Sprintf("%s.%s", pkgPath, typeName))
}

// formatComponentKey turns a package path into the prefix of a component key: the slashes become dots.
func formatComponentKey(pkgPath string) string {
	// For example, "example.com/shop/pet/model" gives "example.com.shop.pet.model".
	// A package of the module and a package of another module are keyed the same
	// way: the full path with the path separators replaced with dots.
	return replaceSlashes(pkgPath)
}

// GetSchemaRef returns the OpenAPI component reference of a type.
func GetSchemaRef(desc *TypeDescriptor) *openapi3.SchemaRef {
	// basic types (int, string and so on)
	if isBasicType(desc.FullKey) {
		return buildBasicSchema(desc)
	}
	// references to custom types
	return buildCustomSchema(desc)
}
