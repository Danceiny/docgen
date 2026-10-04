package pipeline

import (
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func refTo(name string) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Ref: "#/components/schemas/" + name}
}

func objectWith(property string, ref *openapi3.SchemaRef) *openapi3.SchemaRef {
	schema := openapi3.NewObjectSchema()
	schema.Properties[property] = ref
	return schema.NewRef()
}

func schemaNames(doc *openapi3.T) []string {
	var names []string
	for name := range doc.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// A schema that the document keeps although no operation uses it, a webhook
// payload for one, is kept with everything it refers to: a reference to a
// schema that was left out would make the document invalid.
func TestForceKeptSchemasKeepTheirDependencies(t *testing.T) {
	doc := &openapi3.T{
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{Schemas: openapi3.Schemas{
			"Webhook": objectWith("pet", refTo("Pet")),
			"Pet":     objectWith("owner", refTo("Owner")),
			"Owner":   openapi3.NewObjectSchema().NewRef(),
			"Used":    openapi3.NewObjectSchema().NewRef(),
			"Unused":  objectWith("owner", refTo("Owner")),
		}},
	}
	doc.Paths.Set("/api/x", &openapi3.PathItem{Get: &openapi3.Operation{
		Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: &openapi3.Response{
			Content: openapi3.Content{"application/json": &openapi3.MediaType{Schema: refTo("Used")}},
		}})),
	}})

	removeUnusedSchemas(doc, []string{"Webhook"})

	got := schemaNames(doc)
	want := []string{"Owner", "Pet", "Used", "Webhook"}
	if len(got) != len(want) {
		t.Fatalf("schemas = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schemas = %v, want %v", got, want)
		}
	}
}
