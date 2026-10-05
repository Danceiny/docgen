package pipeline

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// A reference to a component that does not exist is a generator defect: erasing
// it, or replacing it with an object, would change the contract and hide the drift.
func TestGenerateYAMLRejectsMissingLocalSchemaRef(t *testing.T) {
	for _, missing := range []*openapi3.SchemaRef{
		openapi3.NewSchemaRef("#/components/schemas/Missing", nil),
		openapi3.NewSchemaRef("#/components/schemas/Missing", openapi3.NewObjectSchema()),
	} {
		doc := &openapi3.T{
			OpenAPI: "3.0.0",
			Info:    &openapi3.Info{Title: "test", Version: "test"},
			Paths:   &openapi3.Paths{},
			Components: &openapi3.Components{Schemas: openapi3.Schemas{
				"Root": openapi3.NewObjectSchema().WithPropertyRef("missing", missing).NewRef(),
			}},
		}
		if _, err := GenerateYAML(doc, nil); err == nil || !strings.Contains(err.Error(), "missing local schema reference") {
			t.Fatalf("expected fail-closed missing ref error, got %v", err)
		}
	}
}

// A missing reference in an operation says which operation, and that a method
// that is not part of the API can be left out.
func TestMissingReferenceOfAnOperationSaysWhichAndWhatToDo(t *testing.T) {
	op := openapi3.NewOperation()
	op.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().WithJSONSchemaRef(
		openapi3.NewSchemaRef("#/components/schemas/net.http.Request", nil))}
	paths := openapi3.NewPaths()
	paths.Set("/api/form/bind", &openapi3.PathItem{Post: op})
	doc := &openapi3.T{
		OpenAPI:    "3.0.0",
		Info:       &openapi3.Info{Title: "test", Version: "test"},
		Paths:      paths,
		Components: &openapi3.Components{Schemas: openapi3.Schemas{}},
	}

	err := normalizeLocalSchemaRefs(doc)
	if err == nil {
		t.Fatal("a reference to a component that does not exist must fail")
	}
	for _, want := range []string{"/api/form/bind", "request body", "net.http.Request", `"models"`, "@apidoc: -"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}
}

// A schema that contains itself other than through a $ref cannot be written:
// the YAML encoder would follow the cycle until the process ran out of memory.
// The check says where the cycle is instead.
func TestCheckSchemasHaveNoCycles(t *testing.T) {
	node := openapi3.NewObjectSchema()
	node.Properties = openapi3.Schemas{"next": openapi3.NewSchemaRef("", node)} // inline: the same schema
	doc := &openapi3.T{
		Paths:      openapi3.NewPaths(),
		Components: &openapi3.Components{Schemas: openapi3.Schemas{"Node": openapi3.NewSchemaRef("", node)}},
	}
	err := checkSchemasHaveNoCycles(doc)
	if err == nil {
		t.Fatal("a schema inside itself must be reported")
	}
	for _, want := range []string{"component Node", "properties.next", "contains itself", "$ref"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not say %q", err, want)
		}
	}

	// Through a $ref the same shape is how a tree is written, and is fine.
	node.Properties["next"] = openapi3.NewSchemaRef("#/components/schemas/Node", node)
	if err := checkSchemasHaveNoCycles(doc); err != nil {
		t.Fatalf("a reference to itself is not a cycle: %v", err)
	}

	// A schema used in two places is shared, not a cycle.
	shared := openapi3.NewStringSchema()
	pair := openapi3.NewObjectSchema().WithPropertyRef("a", openapi3.NewSchemaRef("", shared)).WithPropertyRef("b", openapi3.NewSchemaRef("", shared))
	doc.Components.Schemas["Pair"] = pair.NewRef()
	if err := checkSchemasHaveNoCycles(doc); err != nil {
		t.Fatalf("a shared schema is not a cycle: %v", err)
	}
}

// A reference to the type "unknown" is what an operation that takes or returns a
// map, a list of lists or another unnamed type gets; naming a package to add to
// "models" would send the reader the wrong way.
func TestMissingReferenceToAnUnnamedTypeSaysSo(t *testing.T) {
	op := openapi3.NewOperation()
	op.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().WithJSONSchemaRef(
		openapi3.NewSchemaRef("#/components/schemas/unknown", nil))}
	paths := openapi3.NewPaths()
	paths.Set("/api/shape/q", &openapi3.PathItem{Post: op})
	doc := &openapi3.T{OpenAPI: "3.0.0", Paths: paths, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}

	err := normalizeLocalSchemaRefs(doc)
	if err == nil || !strings.Contains(err.Error(), "not a named type of the module") || strings.Contains(err.Error(), `pattern of "models"`) {
		t.Fatalf("error = %v", err)
	}
}

// Cutting what references name, so that a document is validated in a short time,
// leaves each component validated where it is declared: a schema that is not valid
// is refused, one that refers to another and one that does not.
func TestAnInvalidSchemaIsStillRefusedWhenReferencesAreCut(t *testing.T) {
	for name, build := range map[string]func(*openapi3.Schema){
		"declared": func(s *openapi3.Schema) {},
		"in a property": func(s *openapi3.Schema) {
			s.Properties["inner"] = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Default: "not a number"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			other := openapi3.NewObjectSchema()
			owner := openapi3.NewObjectSchema()
			owner.Properties["other"] = &openapi3.SchemaRef{Ref: "#/components/schemas/Other"}
			if name == "declared" {
				owner.Type = &openapi3.Types{"integer"}
				owner.Default = "not a number"
			}
			build(owner)
			doc := &openapi3.T{
				OpenAPI: "3.0.0",
				Info:    &openapi3.Info{Title: "T", Version: "1"},
				Paths:   openapi3.NewPaths(),
				Components: &openapi3.Components{Schemas: openapi3.Schemas{
					"Owner": owner.NewRef(),
					"Other": other.NewRef(),
				}},
			}
			if _, err := GenerateYAML(doc, nil); err == nil || !strings.Contains(err.Error(), "invalid default") {
				t.Fatalf("a schema with a default its type does not allow was accepted: %v", err)
			}
		})
	}
}
