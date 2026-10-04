// Package overlay reads files of schemas that docgen puts into a document by
// hand: contracts whose Go representation cannot be discovered from ordinary
// struct fields, such as a sealed JSON union or a request type that lives next
// to its service instead of in a model package.
//
// An overlay file is YAML:
//
//	version: 1
//	schemas:
//	  example.com.shop.GetOrderReq:
//	    type: object
//	    required: [orderId]
//	    properties:
//	      orderId: {$ref: '#/components/schemas/example.com.shop.types.ID'}
//	      expand:  {type: array, items: {type: string}}
//
// Each entry is a component, keyed by its full key, written in the subset of
// OpenAPI's schema object that is listed on Schema. It replaces the schema docgen
// generated for the key, if there is one, and is added otherwise. A component's
// title is its key unless the entry says otherwise, as the titles of generated
// components are. The decoder is strict, like the configuration's: a key outside
// the subset is an error, so a typo cannot silently drop part of a contract.
package overlay

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/getkin/kin-openapi/openapi3"
)

// refPrefix starts the reference to a component of the same document.
const refPrefix = "#/components/schemas/"

// File is the content of an overlay file.
type File struct {
	// Version is the schema version of the file; only 1 exists.
	Version int `yaml:"version"`
	// Schemas are the components, by full key.
	Schemas map[string]*Schema `yaml:"schemas"`
}

// Schema is the subset of an OpenAPI schema object an overlay can state. A
// schema is a reference to a component, or it has a type, or it is a oneOf.
type Schema struct {
	// Ref is a reference to a component: "#/components/schemas/<full key>". A
	// reference has no other field.
	Ref         string `yaml:"$ref"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	// Type is string, integer, number, boolean, object or array.
	Type string `yaml:"type"`
	// Format qualifies a string, integer or number, such as date-time or int64.
	Format string `yaml:"format"`
	// Required lists properties of an object that must be present.
	Required []string `yaml:"required"`
	// Properties are the properties of an object.
	Properties map[string]*Schema `yaml:"properties"`
	// Items is the element type of an array.
	Items *Schema `yaml:"items"`
	// AdditionalProperties is the type of the values of a map-like object.
	AdditionalProperties *Schema `yaml:"additionalProperties"`
	// OneOf lists the alternatives of a union, which has no type of its own.
	OneOf []*Schema `yaml:"oneOf"`
}

// Load reads and validates the overlay file at path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read overlay: %w", err)
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Parse decodes and validates the content of an overlay file. Every problem is
// reported at once.
func Parse(data []byte) (*File, error) {
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("decode overlay: %w", err)
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// Apply puts the file's schemas into the document's components and returns the
// keys of the components it replaced, sorted.
func (f *File) Apply(doc *openapi3.T) (replaced []string) {
	if doc.Components == nil {
		doc.Components = &openapi3.Components{}
	}
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = openapi3.Schemas{}
	}
	for key, s := range f.Schemas {
		if doc.Components.Schemas[key] != nil {
			replaced = append(replaced, key)
		}
		component := s.toSchema()
		if component.Title == "" {
			component.Title = key
		}
		doc.Components.Schemas[key] = &openapi3.SchemaRef{Value: component}
	}
	sort.Strings(replaced)
	return replaced
}

// toSchema builds the schema value; a $ref is handled by ref.
func (s *Schema) toSchema() *openapi3.Schema {
	out := &openapi3.Schema{Title: s.Title, Description: s.Description, Format: s.Format, Required: s.Required}
	if s.Type != "" {
		out.Type = &openapi3.Types{s.Type}
	}
	if len(s.Properties) > 0 {
		out.Properties = make(openapi3.Schemas, len(s.Properties))
		for name, p := range s.Properties {
			out.Properties[name] = p.ref()
		}
	}
	if s.Items != nil {
		out.Items = s.Items.ref()
	}
	if s.AdditionalProperties != nil {
		out.AdditionalProperties = openapi3.AdditionalProperties{Schema: s.AdditionalProperties.ref()}
	}
	for _, alt := range s.OneOf {
		out.OneOf = append(out.OneOf, alt.ref())
	}
	return out
}

// ref builds the reference a property, an item or an alternative stands for: a
// reference to a component keeps its target's value empty, like the references
// the generator itself writes, and any other schema is inline.
func (s *Schema) ref() *openapi3.SchemaRef {
	if s.Ref != "" {
		return openapi3.NewSchemaRef(s.Ref, &openapi3.Schema{})
	}
	return &openapi3.SchemaRef{Value: s.toSchema()}
}

var schemaTypes = []string{"string", "integer", "number", "boolean", "object", "array"}

func (f *File) validate() error {
	var problems []error
	if f.Version != 1 {
		problems = append(problems, fmt.Errorf("version: want 1, got %d", f.Version))
	}
	if len(f.Schemas) == 0 {
		problems = append(problems, errors.New("schemas: at least one component is required"))
	}
	keys := make([]string, 0, len(f.Schemas))
	for key := range f.Schemas {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		at := "schemas." + key
		if key == "" {
			problems = append(problems, errors.New("schemas: a key is empty"))
		}
		s := f.Schemas[key]
		if s != nil && s.Ref != "" {
			problems = append(problems, fmt.Errorf("%s: a component cannot be a $ref", at))
			continue
		}
		problems = append(problems, s.validate(at)...)
	}
	return errors.Join(problems...)
}

func (s *Schema) validate(at string) []error {
	if s == nil {
		return []error{fmt.Errorf("%s: empty schema", at)}
	}
	if s.Ref != "" {
		return s.validateRef(at)
	}
	var problems []error
	problems = append(problems, s.validateKind(at)...)
	problems = append(problems, s.validateRequired(at)...)
	return append(problems, s.validateChildren(at)...)
}

// validateRef checks a reference: a component's key and nothing else.
func (s *Schema) validateRef(at string) []error {
	if s.Title != "" || s.Description != "" || s.Type != "" || s.Format != "" || len(s.Required) > 0 ||
		len(s.Properties) > 0 || s.Items != nil || s.AdditionalProperties != nil || len(s.OneOf) > 0 {
		return []error{fmt.Errorf("%s: a $ref has no other field", at)}
	}
	if !strings.HasPrefix(s.Ref, refPrefix) || len(s.Ref) == len(refPrefix) {
		return []error{fmt.Errorf("%s.$ref: want %s<full key>, got %q", at, refPrefix, s.Ref)}
	}
	return nil
}

// validateKind checks that a schema is a typed value or a union, and that the
// fields it has belong to its type.
func (s *Schema) validateKind(at string) []error {
	var problems []error
	add := func(msg string) { problems = append(problems, errors.New(at+msg)) }
	switch {
	case s.Type != "" && len(s.OneOf) > 0:
		add(": oneOf cannot be combined with a type")
	case s.Type == "" && len(s.OneOf) == 0:
		add(": a type or a oneOf is required")
	case s.Type != "" && !slices.Contains(schemaTypes, s.Type):
		add(fmt.Sprintf(".type: want one of %s, got %q", strings.Join(schemaTypes, ", "), s.Type))
	}
	if s.Format != "" && s.Type != "string" && s.Type != "integer" && s.Type != "number" {
		add(".format: only a string, integer or number has one")
	}
	if s.Type != "object" && (len(s.Properties) > 0 || len(s.Required) > 0 || s.AdditionalProperties != nil) {
		add(": properties, required and additionalProperties belong to an object")
	}
	switch {
	case s.Type == "array" && s.Items == nil:
		add(".items: an array needs its element type")
	case s.Type != "array" && s.Items != nil:
		add(".items: only an array has items")
	}
	return problems
}

// validateRequired checks that each required name is a property, once.
func (s *Schema) validateRequired(at string) []error {
	var problems []error
	seen := map[string]bool{}
	for _, name := range s.Required {
		if _, ok := s.Properties[name]; !ok {
			problems = append(problems, fmt.Errorf("%s.required: %q is not one of the properties", at, name))
		}
		if seen[name] {
			problems = append(problems, fmt.Errorf("%s.required: %q is listed twice", at, name))
		}
		seen[name] = true
	}
	return problems
}

// validateChildren checks the schemas inside this one: the alternatives of a
// union, the properties of an object, the items of an array and the values of a map.
func (s *Schema) validateChildren(at string) []error {
	var problems []error
	for i, alt := range s.OneOf {
		problems = append(problems, alt.validate(fmt.Sprintf("%s.oneOf[%d]", at, i))...)
	}
	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" {
			problems = append(problems, fmt.Errorf("%s.properties: a name is empty", at))
		}
		problems = append(problems, s.Properties[name].validate(at+".properties."+name)...)
	}
	if s.Items != nil {
		problems = append(problems, s.Items.validate(at+".items")...)
	}
	if s.AdditionalProperties != nil {
		problems = append(problems, s.AdditionalProperties.validate(at+".additionalProperties")...)
	}
	return problems
}
