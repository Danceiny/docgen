package overlay

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const valid = `
version: 1
schemas:
  example.shop.GetOrderReq:
    type: object
    required: [orderId]
    properties:
      orderId: {$ref: '#/components/schemas/example.types.ID'}
      expand: {type: array, items: {type: string}}
      since: {type: string, format: date-time}
      limit: {type: integer, format: int64}
      flags: {type: object, additionalProperties: {type: boolean}}
      tags: {type: array, items: {$ref: '#/components/schemas/example.shop.Tag'}}
  example.shop.Ingress:
    oneOf:
      - $ref: '#/components/schemas/example.shop.First'
      - $ref: '#/components/schemas/example.shop.Second'
  example.shop.ActionId:
    title: ActionId
    description: an action
    type: string
`

func TestApplyBuildsTheSchemasTheFileDescribes(t *testing.T) {
	f, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"example.shop.GetOrderReq": {Value: openapi3.NewObjectSchema()}, // generated, to be replaced
	}}}
	replaced := f.Apply(doc)
	if !reflect.DeepEqual(replaced, []string{"example.shop.GetOrderReq"}) {
		t.Fatalf("replaced = %v", replaced)
	}
	if len(doc.Components.Schemas) != 3 {
		t.Fatalf("components = %d, want 3", len(doc.Components.Schemas))
	}

	req := doc.Components.Schemas["example.shop.GetOrderReq"].Value
	if req.Title != "example.shop.GetOrderReq" {
		t.Errorf("a component's title is its key unless set, got %q", req.Title)
	}
	if !req.Type.Is("object") || !reflect.DeepEqual(req.Required, []string{"orderId"}) {
		t.Errorf("type %v required %v", req.Type, req.Required)
	}
	orderID := req.Properties["orderId"]
	if orderID.Ref != "#/components/schemas/example.types.ID" || orderID.Value == nil {
		t.Errorf("orderId = %+v; a reference keeps an empty, non-nil value like the generator's own", orderID)
	}
	if since := req.Properties["since"].Value; !since.Type.Is("string") || since.Format != "date-time" {
		t.Errorf("since = %+v", since)
	}
	if limit := req.Properties["limit"].Value; !limit.Type.Is("integer") || limit.Format != "int64" {
		t.Errorf("limit = %+v", limit)
	}
	if expand := req.Properties["expand"].Value; !expand.Type.Is("array") || !expand.Items.Value.Type.Is("string") {
		t.Errorf("expand = %+v", expand)
	}
	if tags := req.Properties["tags"].Value; tags.Items.Ref != "#/components/schemas/example.shop.Tag" {
		t.Errorf("tags items = %+v", tags.Items)
	}
	if flags := req.Properties["flags"].Value; flags.AdditionalProperties.Schema == nil || !flags.AdditionalProperties.Schema.Value.Type.Is("boolean") {
		t.Errorf("flags = %+v", flags)
	}

	union := doc.Components.Schemas["example.shop.Ingress"].Value
	if union.Type != nil || len(union.OneOf) != 2 || union.OneOf[1].Ref != "#/components/schemas/example.shop.Second" {
		t.Errorf("union = %+v", union)
	}

	action := doc.Components.Schemas["example.shop.ActionId"].Value
	if action.Title != "ActionId" || action.Description != "an action" || !action.Type.Is("string") {
		t.Errorf("a title that is set is kept: %+v", action)
	}
}

func TestApplyCreatesTheComponentsOfADocumentThatHasNone(t *testing.T) {
	f, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	doc := &openapi3.T{}
	if replaced := f.Apply(doc); len(replaced) != 0 {
		t.Fatalf("replaced = %v", replaced)
	}
	if len(doc.Components.Schemas) != 3 {
		t.Fatalf("components = %d", len(doc.Components.Schemas))
	}
}

// What reaches the document is its wire form, so the shapes are compared as JSON.
func TestAppliedSchemasMarshalAsTheOpenAPIWritten(t *testing.T) {
	f, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	doc := &openapi3.T{}
	f.Apply(doc)
	got, err := json.Marshal(doc.Components.Schemas["example.shop.GetOrderReq"])
	if err != nil {
		t.Fatal(err)
	}
	var have, want map[string]any
	if err := json.Unmarshal(got, &have); err != nil {
		t.Fatal(err)
	}
	wantJSON := `{
	  "title": "example.shop.GetOrderReq", "type": "object", "required": ["orderId"],
	  "properties": {
	    "orderId": {"$ref": "#/components/schemas/example.types.ID"},
	    "expand": {"type": "array", "items": {"type": "string"}},
	    "since": {"type": "string", "format": "date-time"},
	    "limit": {"type": "integer", "format": "int64"},
	    "flags": {"type": "object", "additionalProperties": {"type": "boolean"}},
	    "tags": {"type": "array", "items": {"$ref": "#/components/schemas/example.shop.Tag"}}
	  }
	}`
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(have, want) {
		t.Fatalf("wire form = %s", got)
	}
}

func TestParseRejects(t *testing.T) {
	wrap := func(schema string) string { return "version: 1\nschemas:\n  a.B:\n" + schema }
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"no version", "schemas:\n  a.B: {type: string}\n", "version: want 1, got 0"},
		{"other version", "version: 2\nschemas:\n  a.B: {type: string}\n", "version: want 1, got 2"},
		{"no schemas", "version: 1\n", "at least one component is required"},
		{"unknown top-level key", "version: 1\nschema: {}\nschemas:\n  a.B: {type: string}\n", "field schema not found"},
		{"unknown schema key", wrap("    type: object\n    requried: [a]\n"), "field requried not found"},
		{"component that is a $ref", wrap("    $ref: '#/components/schemas/x.Y'\n"), "schemas.a.B: a component cannot be a $ref"},
		{"empty schema", wrap("    type: object\n    properties:\n      orderId:\n"), "schemas.a.B.properties.orderId: empty schema"},
		{"$ref with a sibling", wrap("    type: object\n    properties:\n      p: {$ref: '#/components/schemas/x.Y', description: d}\n"), "a $ref has no other field"},
		{"$ref outside the components", wrap("    type: object\n    properties:\n      p: {$ref: 'other.yaml#/X'}\n"), "properties.p.$ref"},
		{"$ref with no key", wrap("    type: object\n    properties:\n      p: {$ref: '#/components/schemas/'}\n"), "properties.p.$ref"},
		{"no type and no oneOf", wrap("    title: T\n"), "a type or a oneOf is required"},
		{"unknown type", wrap("    type: text\n"), `schemas.a.B.type: want one of string, integer, number, boolean, object, array, got "text"`},
		{"oneOf with a type", wrap("    type: object\n    oneOf: [{type: string}]\n"), "oneOf cannot be combined with a type"},
		{"format on an object", wrap("    type: object\n    format: date-time\n"), "only a string, integer or number has one"},
		{"properties on a string", wrap("    type: string\n    properties:\n      a: {type: string}\n"), "belong to an object"},
		{"required on an array", wrap("    type: array\n    items: {type: string}\n    required: [a]\n"), "belong to an object"},
		{"array without items", wrap("    type: array\n"), "an array needs its element type"},
		{"items on an object", wrap("    type: object\n    items: {type: string}\n"), "only an array has items"},
		{"required that is not a property", wrap("    type: object\n    required: [ghost]\n    properties:\n      a: {type: string}\n"), `"ghost" is not one of the properties`},
		{"required twice", wrap("    type: object\n    required: [a, a]\n    properties:\n      a: {type: string}\n"), `"a" is listed twice`},
		{"a problem in a oneOf alternative", wrap("    oneOf:\n      - {type: string}\n      - {type: text}\n"), "schemas.a.B.oneOf[1].type"},
		{"a problem in additionalProperties", wrap("    type: object\n    additionalProperties: {type: text}\n"), "additionalProperties.type"},
		{"a problem in items", wrap("    type: array\n    items: {type: text}\n"), "a.B.items.type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseReportsEveryProblemAtOnce(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
schemas:
  a.B: {type: text}
  c.D: {type: array}
  e.F: {title: T}
`))
	if err == nil {
		t.Fatal("accepted three bad components")
	}
	for _, want := range []string{"a.B.type", "c.D.items", "e.F: a type or a oneOf is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not report %q", err, want)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "overlay.yaml")
	if err := os.WriteFile(good, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(good)
	if err != nil || len(f.Schemas) != 3 {
		t.Fatalf("Load = %v, %v", f, err)
	}

	if _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil || !strings.Contains(err.Error(), "read overlay") {
		t.Fatalf("a missing file: %v", err)
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("version: 1\nschemas:\n  a.B: {type: text}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatalf("an invalid file must be reported with its path: %v", err)
	}
}
