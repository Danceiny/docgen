package engine

import (
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fieldShapesKey = "github.com.Danceiny.docgen.internal.engine.testdata.fieldshapes."

func fieldShapes(t *testing.T, s Settings) *openapi3.T {
	t.Helper()
	withSettings(t, s, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/fieldshapes")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "fieldshapes"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	return doc
}

func propertyNames(doc *openapi3.T, name string) []string {
	schema := doc.Components.Schemas[fieldShapesKey+name]
	if schema == nil || schema.Value == nil {
		return nil
	}
	var names []string
	for p := range schema.Value.Properties {
		names = append(names, p)
	}
	sort.Strings(names)
	return names
}

func requiredOf(doc *openapi3.T, name string) []string {
	required := append([]string(nil), doc.Components.Schemas[fieldShapesKey+name].Value.Required...)
	sort.Strings(required)
	return required
}

// A struct is documented as encoding/json reads it.
func TestFieldShapesAreReadAsEncodingJSONReadsThem(t *testing.T) {
	doc := fieldShapes(t, Settings{})

	assert.Equal(t, []string{"Lat", "Lng"}, propertyNames(doc, "Point"), "every name of a declaration is a field, and a blank one is none")
	assert.Equal(t, []string{"base", "name"}, propertyNames(doc, "Nested"), "an embedded field that has a json name is a field of that name")
	base := doc.Components.Schemas[fieldShapesKey+"Nested"].Value.Properties["base"]
	require.NotNil(t, base)
	assert.Equal(t, fieldShapesKey+"Base", base.Value.Title, "the schema of the embedded type is the field")
	assert.Equal(t, []string{"id", "name"}, propertyNames(doc, "Flat"), "an embedded field with no name is flattened")
	assert.Equal(t, []string{"quiet"}, propertyNames(doc, "Quiet"), "the type of the embedded field need not be exported")

	assert.Equal(t, []string{"binding", "plain", "several", "spaced", "tagged"}, requiredOf(doc, "Rules"),
		"a rule among others is a rule: required,email")

	anything := doc.Components.Schemas[fieldShapesKey+"Anything"].Value.Properties
	for _, name := range []string{"any", "empty"} {
		require.NotNil(t, anything[name], name)
		assert.Nil(t, anything[name].Value.Type, "%s takes any JSON value, so its schema has no type", name)
		assert.Empty(t, anything[name].Value.AnyOf, name)
	}
	assert.Nil(t, anything["list"].Value.Items.Value.Type, "the items of a list of any are any value")
}

// legacy_field_shapes keeps what documents generated before the fix have.
func TestFieldShapesOfLegacyDocuments(t *testing.T) {
	doc := fieldShapes(t, Settings{CompatLegacyFieldShapes: true})

	assert.Equal(t, []string{"Lat", "_"}, propertyNames(doc, "Point"), "the first name of a declaration, and the blank field as a property")
	assert.Equal(t, []string{"id", "name"}, propertyNames(doc, "Nested"), "an embedded field is flattened whatever its json name")
	assert.Equal(t, []string{"plain", "tagged"}, requiredOf(doc, "Rules"), "required only as the whole tag")

	anything := doc.Components.Schemas[fieldShapesKey+"Anything"].Value.Properties
	assert.True(t, anything["any"].Value.Type.Is("object"), "any is an object")
	assert.Len(t, anything["empty"].Value.AnyOf, 3, "interface{} is one of three types")
}
