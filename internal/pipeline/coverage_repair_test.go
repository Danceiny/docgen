package pipeline

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

func TestDependencyGraphTopologicalOrder(t *testing.T) {
	graph := buildDependencyGraph([]*packages.Package{
		{ID: "a", Imports: map[string]*packages.Package{"b": {ID: "b"}, "c": {ID: "c"}}},
	})
	order, err := topologicalSort(graph)
	if err != nil || len(order) == 3 && (order[0] != "b" || order[1] != "c" || order[2] != "a") {
		t.Fatalf("topological sort: %v %v", order, err)
	}
	if len(order) != 3 || order[0] != "b" || order[1] != "c" || order[2] != "a" {
		t.Fatalf("topological order=%v", order)
	}
	if _, err := topologicalSort(map[string][]string{"a": {"b"}, "b": {"a"}}); err == nil {
		t.Fatal("cycle must fail")
	}
}

func TestGenerateYAMLSyntheticTraversal(t *testing.T) {
	stringSchema := openapi3.NewStringSchema()
	stringSchema.Title = "Nested"
	root := openapi3.NewObjectSchema()
	root.Title = "Root"
	root.Required = []string{"nested"}
	root.Properties = openapi3.Schemas{
		"nested": {Value: &openapi3.Schema{Title: "Nested", Type: &openapi3.Types{"object"}, Properties: openapi3.Schemas{"value": {Value: stringSchema}}}},
		"items":  {Value: &openapi3.Schema{Type: &openapi3.Types{"array"}, Items: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}}},
	}
	root.OneOf = openapi3.SchemaRefs{{Value: openapi3.NewStringSchema()}, {Value: openapi3.NewIntegerSchema()}}
	doc := &openapi3.T{OpenAPI: "3.0.0", Info: &openapi3.Info{Title: "synthetic", Version: "1"}, Paths: &openapi3.Paths{}, Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"Root": {Value: root}, "Nested": {Value: stringSchema}, "example.com.shop.types.ID": {Value: openapi3.NewStringSchema()},
	}}}
	if data, err := GenerateYAML(doc, nil); err != nil || len(data) == 0 {
		t.Fatalf("GenerateYAML: %v (%d bytes)", err, len(data))
	}
	if got := processSchemaRef(doc, nil); got != nil {
		t.Fatal("nil schema ref should remain nil")
	}
	alias := &openapi3.SchemaRef{Ref: "#/components/schemas/Nested", Value: &openapi3.Schema{Pattern: "default", Description: "alias"}}
	if got := processSchemaRef(doc, alias); got == nil || got.Ref != "" {
		t.Fatal("default alias ref was not expanded")
	}
	if got := doc.Components.Schemas["Root"].Value.Properties["nested"]; got == nil || got.Ref != "#/components/schemas/Nested" {
		t.Fatalf("nested title was not converted to ref: %#v", got)
	}
	missing := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"Broken": {Value: &openapi3.Schema{Properties: openapi3.Schemas{"x": {Ref: "#/components/schemas/Missing"}}}},
	}}}
	if err := normalizeLocalSchemaRefs(missing); err == nil {
		t.Fatal("missing local ref must fail closed")
	}
}

// A package comes after the packages it imports; one that imports nothing and
// that nobody imports is not in the graph and comes last.
func TestOrderPackagesKeepsPackagesWithNoNeighbours(t *testing.T) {
	pkgs := []*packages.Package{
		{ID: "a", Imports: map[string]*packages.Package{"b": {ID: "b"}}},
		{ID: "b"},
		{ID: "c"},
		{ID: "d", Imports: map[string]*packages.Package{"b": {ID: "b"}}},
		{ID: "0"},
	}
	order, err := orderPackages(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, " "); got != "b a d 0 c" {
		t.Fatalf("order = %q, want %q", got, "b a d 0 c")
	}
}
