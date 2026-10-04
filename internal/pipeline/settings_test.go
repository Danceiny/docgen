package pipeline

import (
	"testing"

	"github.com/Danceiny/docgen/internal/config"
)

func TestSchemaFromSpecCarriesTheExample(t *testing.T) {
	s := schemaFromSpec(config.TypeSpec{Type: "string", Format: "uuid", Description: "an id", Example: "6f1c"})
	if !s.Type.Is("string") || s.Format != "uuid" || s.Description != "an id" || s.Example != "6f1c" {
		t.Fatalf("schema = %+v", s)
	}
	n := schemaFromSpec(config.TypeSpec{Type: "integer", Example: 7})
	if n.Example != 7 {
		t.Fatalf("an example keeps its YAML type: %#v", n.Example)
	}
	if none := schemaFromSpec(config.TypeSpec{Type: "boolean"}); none.Example != nil {
		t.Fatalf("no example, no example: %#v", none.Example)
	}
	arr := schemaFromSpec(config.TypeSpec{Type: "array", Items: &config.TypeSpec{Type: "string", Example: "x"}})
	if arr.Items.Value.Example != "x" {
		t.Fatalf("the example of an element: %#v", arr.Items.Value.Example)
	}
}
