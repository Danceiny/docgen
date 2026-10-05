package engine

import (
	"go/ast"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkerDoc(schemas openapi3.Schemas) *openapi3.T {
	return &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "t", Version: "1"}, Components: &openapi3.Components{Schemas: schemas}}
}

// A reference stands for the component the document has now, not for the schema it
// was given when it was made, which is the placeholder of a type that was not
// described yet.
func TestTheCheckerLooksAReferenceUpInTheDocument(t *testing.T) {
	status := openapi3.NewStringSchema()
	status.Enum = []any{"new", "active"}
	doc := checkerDoc(openapi3.Schemas{"Status": openapi3.NewSchemaRef("", status)})
	c := newValueChecker(doc)

	stale := &openapi3.SchemaRef{Ref: NewRefFromFullKey("Status"), Value: defaultSchema()}
	assert.Same(t, status, c.schemaOf(stale))
	assert.NoError(t, c.check(c.schemaOf(stale), "active"))
	assert.Error(t, c.check(c.schemaOf(stale), "bogus"))

	unknown := &openapi3.SchemaRef{Ref: NewRefFromFullKey("Nowhere"), Value: status}
	assert.Same(t, status, c.schemaOf(unknown), "what the document does not have, the reference carries")

	self := openapi3.NewSchemaRef(NewRefFromFullKey("Loop"), status)
	doc.Components.Schemas["Loop"] = self
	assert.Same(t, status, c.schemaOf(self), "a component that is a reference to itself is the schema it carries")

	assert.Nil(t, c.schemaOf(nil))
}

// A field of a reference that has a description of its own is a value of what the
// reference refers to, through any number of such wrappers.
func TestTheCheckerLooksThroughTheWrapperOfAReference(t *testing.T) {
	level := openapi3.NewIntegerSchema()
	doc := checkerDoc(openapi3.Schemas{"Level": openapi3.NewSchemaRef("", level)})
	c := newValueChecker(doc)

	inner := &openapi3.Schema{AllOf: openapi3.SchemaRefs{{Ref: NewRefFromFullKey("Level"), Value: defaultSchema()}}}
	outer := &openapi3.Schema{AllOf: openapi3.SchemaRefs{{Value: inner}}}
	assert.Same(t, level, c.effective(outer))

	typed := &openapi3.Schema{Type: &openapi3.Types{"string"}, AllOf: outer.AllOf}
	assert.Same(t, typed, c.effective(typed), "a schema that has a type of its own is the one that is judged")
}

// Where no description of a type was made there is nothing to judge a value by,
// and any value is allowed, so that a field is not blamed for what the type is.
func TestTheCheckerAllowsAnyValueWhereNothingIsKnownOfTheType(t *testing.T) {
	c := newValueChecker(checkerDoc(openapi3.Schemas{}))
	assert.NoError(t, c.check(defaultSchema(), "anything"))
	assert.NoError(t, c.check(nil, 3))
	object := openapi3.NewObjectSchema()
	object.Properties = openapi3.Schemas{"later": {Ref: NewRefFromFullKey("Later"), Value: defaultSchema()}}
	assert.NoError(t, c.check(object, map[string]any{"later": "text"}))
}

// The numbers of an enum are integers of Go in the document that is being made,
// and a number of a JSON document is a float: the enum is compared the way JSON is.
func TestTheCheckerComparesTheNumbersOfAnEnumAsJSONDoes(t *testing.T) {
	level := openapi3.NewIntegerSchema()
	level.Enum = []any{int(1), int64(2), uint64(3)}
	c := newValueChecker(checkerDoc(openapi3.Schemas{}))
	for _, ok := range []int64{1, 2, 3} {
		assert.NoError(t, c.check(level, ok), "%d", ok)
	}
	assert.Error(t, c.check(level, int64(4)))
	assert.Equal(t, []any{int(1), int64(2), uint64(3)}, level.Enum, "the schema of the document is not touched")
}

// What is made of a schema is made once, so that a field with an example costs
// what its value reaches, not the graph of types around its type, and a type that
// contains itself ends.
func TestTheCheckerMakesTheTwinOfASchemaOnce(t *testing.T) {
	node := openapi3.NewObjectSchema()
	node.Properties = openapi3.Schemas{
		"next":  {Ref: NewRefFromFullKey("Node")},
		"items": {Value: &openapi3.Schema{Type: &openapi3.Types{"array"}, Items: &openapi3.SchemaRef{Ref: NewRefFromFullKey("Node")}}},
		"one":   {Value: &openapi3.Schema{OneOf: openapi3.SchemaRefs{{Ref: NewRefFromFullKey("Node")}}}},
	}
	node.AdditionalProperties = openapi3.AdditionalProperties{Schema: &openapi3.SchemaRef{Ref: NewRefFromFullKey("Node")}}
	doc := checkerDoc(openapi3.Schemas{"Node": openapi3.NewSchemaRef("", node)})
	c := newValueChecker(doc)

	first := c.twin(node)
	require.NotNil(t, first)
	assert.Same(t, first, c.twin(node), "the second time it is the one that was made")
	assert.Same(t, first, first.Properties["next"].Value, "and what refers to it from inside is the twin itself")
	assert.Same(t, first, first.Properties["items"].Value.Items.Value)
	assert.Same(t, first, first.Properties["one"].Value.OneOf[0].Value)
	assert.Same(t, first, first.AdditionalProperties.Schema.Value)
	assert.NoError(t, c.check(node, map[string]any{"next": map[string]any{"next": map[string]any{}}}))
	assert.Len(t, c.twins, 3, "the type, the list and the oneOf, each once")

	err := c.check(node, map[string]any{"next": map[string]any{"items": "not a list"}})
	require.Error(t, err, "a value that is not allowed is refused")
	assert.Contains(t, err.Error(), "value must be an array", "and the message of that can be written, though the type contains itself")
}

// A nil pointer to a node is not a node to ask the position of: it is where
// nothing is.
func TestPositionOfANilNodeOfAConcreteTypeIsNowhere(t *testing.T) {
	pkg := loadFixture(t, "testdata/tagvalues")
	assert.Equal(t, "", positionOf(pkg, (*ast.Field)(nil)))
	assert.Equal(t, "", positionOf(pkg, (*ast.TypeSpec)(nil)))
	assert.Equal(t, "", positionOf(pkg, nil))
	assert.Equal(t, "", positionOf(nil, &ast.Field{}))
	assert.NotEmpty(t, positionOf(pkg, pkg.Syntax[0]))
}
