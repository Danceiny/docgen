package pipeline

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func examplesDoc(owner *openapi3.Schema) *openapi3.T {
	status := openapi3.NewStringSchema()
	status.Enum = []any{"new", "active"}
	return &openapi3.T{
		OpenAPI: "3.0.0",
		Info:    &openapi3.Info{Title: "T", Version: "1"},
		Paths:   openapi3.NewPaths(),
		Components: &openapi3.Components{Schemas: openapi3.Schemas{
			"Owner":  owner.NewRef(),
			"Status": status.NewRef(),
		}},
	}
}

// The example and the default of a schema have to be values of what the schema
// refers to as well as of what it says itself: the references are followed, though
// they are cut afterwards. What fields say through their tags has been judged when
// the fields were read; this is for what nobody judged.
func TestTheExampleOfASchemaIsJudgedByWhatItRefersTo(t *testing.T) {
	field := func(example, defaultValue any) *openapi3.Schema {
		owner := openapi3.NewObjectSchema()
		owner.Properties["status"] = &openapi3.SchemaRef{Value: &openapi3.Schema{
			AllOf:   openapi3.SchemaRefs{{Ref: "#/components/schemas/Status"}},
			Example: example, Default: defaultValue,
		}}
		return owner
	}

	for name, owner := range map[string]*openapi3.Schema{
		"an example of the enum": field("active", nil),
		"a default of the enum":  field(nil, "new"),
		"no example at all":      field(nil, nil),
		"the example of a plain property": func() *openapi3.Schema {
			o := openapi3.NewObjectSchema()
			o.Properties["n"] = openapi3.NewIntegerSchema().NewRef()
			o.Properties["n"].Value.Example = 3
			return o
		}(),
	} {
		_, err := GenerateYAML(examplesDoc(owner), nil)
		assert.NoError(t, err, name)
	}

	for name, c := range map[string]struct {
		owner *openapi3.Schema
		kind  string
	}{
		"an example that is not in the enum": {field("bogus", nil), "example"},
		"a default that is not in the enum":  {field(nil, "bogus"), "default"},
		"an example that is not a string":    {field(12, nil), "example"},
	} {
		_, err := GenerateYAML(examplesDoc(c.owner), nil)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "component Owner", name)
		assert.Contains(t, err.Error(), "properties.status", name+": where")
		assert.Contains(t, err.Error(), "the "+c.kind, name)
		assert.False(t, strings.Contains(err.Error(), "\n"), name+": one line")
	}
}

// A document that keeps the old way of writing validates what the references name
// itself, and the check of the examples does not run twice.
func TestLegacyOutputLeavesTheExamplesToTheValidationOfTheDocument(t *testing.T) {
	old := legacyOutput
	legacyOutput = true
	t.Cleanup(func() { legacyOutput = old })

	owner := openapi3.NewObjectSchema()
	owner.Properties["status"] = &openapi3.SchemaRef{Value: &openapi3.Schema{
		AllOf:   openapi3.SchemaRefs{{Ref: "#/components/schemas/Status"}},
		Example: "bogus",
	}}
	_, err := GenerateYAML(examplesDoc(owner), nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "check the examples")
}

// A type that docgen could not describe is the placeholder of an object that says
// nothing about it, and the example of a field of that type is not judged by it:
// the run has said that the type is not described.
func TestTheExampleOfAFieldWhoseTypeWasNotDescribedIsNotJudgedByThePlaceholder(t *testing.T) {
	owner := openapi3.NewObjectSchema()
	owner.Properties["unknown"] = &openapi3.SchemaRef{Value: &openapi3.Schema{
		AllOf:   openapi3.SchemaRefs{{Ref: "#/components/schemas/Unknown"}},
		Example: "red",
	}}
	doc := examplesDoc(owner)
	doc.Components.Schemas["Unknown"] = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}, Pattern: "default"}}
	_, err := GenerateYAML(doc, nil)
	assert.NoError(t, err)
}
