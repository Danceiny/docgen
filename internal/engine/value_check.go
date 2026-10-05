package engine

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// schemaRefPrefix is what a reference to a component starts with.
const schemaRefPrefix = "#/components/schemas/"

// valueChecker says whether a value is one that a schema allows, which is what
// the example and the default of a field have to be.
//
// It judges against the document as it is once every type is described. A field
// that refers to a type that is declared further down was read when that type was
// a placeholder, and its reference goes on pointing at the placeholder: what the
// reference says is looked up in the document, not taken from the reference.
//
// The schemas of the document hold the values of an enum as the numbers of Go,
// which the validation of kin-openapi does not take for the numbers of a JSON
// document. The checker validates against a twin of each schema it reaches, made
// once and kept, so that the work is that of the schemas the values reach and not
// that of the whole graph for every field with an example.
type valueChecker struct {
	doc   *openapi3.T
	twins map[*openapi3.Schema]*openapi3.Schema
}

func newValueChecker(doc *openapi3.T) *valueChecker {
	return &valueChecker{doc: doc, twins: map[*openapi3.Schema]*openapi3.Schema{}}
}

// schemaOf is the schema a reference stands for: the component it names, as the
// document has it now, or the schema the reference carries.
func (c *valueChecker) schemaOf(ref *openapi3.SchemaRef) *openapi3.Schema {
	for range 32 { // a component can be a reference to another one
		if ref == nil {
			return nil
		}
		if ref.Ref == "" || c.doc == nil || c.doc.Components == nil {
			return ref.Value
		}
		next := c.doc.Components.Schemas[strings.TrimPrefix(ref.Ref, schemaRefPrefix)]
		if next == nil || next == ref {
			return ref.Value
		}
		ref = next
	}
	return nil
}

// effective is the schema a value of a field has to be a value of: a reference
// that has a description of its own is the only member of an allOf, and the value
// is a value of what it refers to.
func (c *valueChecker) effective(schema *openapi3.Schema) *openapi3.Schema {
	for range 32 {
		if schema == nil || schema.Type != nil || len(schema.AllOf) != 1 || schema.AllOf[0] == nil {
			break
		}
		next := c.schemaOf(schema.AllOf[0])
		if next == nil {
			break
		}
		schema = next
	}
	return schema
}

// check reports why the schema does not allow the value, or nil.
func (c *valueChecker) check(schema *openapi3.Schema, value any) error {
	return c.twin(schema).VisitJSON(value)
}

// twin is the schema, and what it is made of, with the enums written as the
// numbers of a JSON document, which is what the validation of a value compares
// with. What no description reached stands for nothing known about the type, and
// allows any value.
func (c *valueChecker) twin(schema *openapi3.Schema) *openapi3.Schema {
	if schema == nil || isDefaultSchema(schema) {
		return &openapi3.Schema{}
	}
	if done, ok := c.twins[schema]; ok {
		return done
	}
	twin := *schema
	c.twins[schema] = &twin
	if len(schema.Enum) > 0 {
		twin.Enum = make([]any, len(schema.Enum))
		for i, v := range schema.Enum {
			switch n := v.(type) {
			case int:
				twin.Enum[i] = float64(n)
			case int64:
				twin.Enum[i] = float64(n)
			case uint64:
				twin.Enum[i] = float64(n)
			default:
				twin.Enum[i] = v
			}
		}
	}
	ref := func(r *openapi3.SchemaRef) *openapi3.SchemaRef {
		if r == nil {
			return nil
		}
		return &openapi3.SchemaRef{Value: c.twin(c.schemaOf(r))}
	}
	twin.Items = ref(schema.Items)
	twin.Not = ref(schema.Not)
	if schema.AdditionalProperties.Schema != nil {
		twin.AdditionalProperties.Schema = ref(schema.AdditionalProperties.Schema)
	}
	if len(schema.Properties) > 0 {
		twin.Properties = make(openapi3.Schemas, len(schema.Properties))
		for name, property := range schema.Properties {
			twin.Properties[name] = ref(property)
		}
	}
	for _, of := range []struct{ from, to *openapi3.SchemaRefs }{{&schema.AllOf, &twin.AllOf}, {&schema.OneOf, &twin.OneOf}, {&schema.AnyOf, &twin.AnyOf}} {
		if len(*of.from) > 0 {
			*of.to = make(openapi3.SchemaRefs, len(*of.from))
			for i, r := range *of.from {
				(*of.to)[i] = ref(r)
			}
		}
	}
	return &twin
}
