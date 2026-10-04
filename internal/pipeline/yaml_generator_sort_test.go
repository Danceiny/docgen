package pipeline

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestSortSchemaRefsByKey_StabilizesOneOfOrder(t *testing.T) {
	refs := openapi3.SchemaRefs{
		{Ref: "#/components/schemas/example.com.shop.domain.UserRole"},
		{Ref: "#/components/schemas/example.com.shop.billing.OrderSummary"},
		{Ref: "#/components/schemas/example.com.shop.domain.User"},
	}
	sortSchemaRefsByKey(refs)
	want := []string{
		"#/components/schemas/example.com.shop.billing.OrderSummary",
		"#/components/schemas/example.com.shop.domain.User",
		"#/components/schemas/example.com.shop.domain.UserRole",
	}
	for i, w := range want {
		if refs[i] == nil || refs[i].Ref != w {
			t.Fatalf("index %d: got %v want %s", i, refs[i], w)
		}
	}

	// Re-sort must be idempotent (regen gate determinism).
	again := openapi3.SchemaRefs{
		{Ref: want[2]},
		{Ref: want[0]},
		{Ref: want[1]},
	}
	sortSchemaRefsByKey(again)
	for i, w := range want {
		if again[i].Ref != w {
			t.Fatalf("second sort index %d: got %s want %s", i, again[i].Ref, w)
		}
	}
}

func TestSchemaRefSortKey_PrefersRefThenTitle(t *testing.T) {
	if got := schemaRefSortKey(&openapi3.SchemaRef{Ref: "#/components/schemas/A"}); got != "ref:#/components/schemas/A" {
		t.Fatalf("ref key: %q", got)
	}
	if got := schemaRefSortKey(&openapi3.SchemaRef{Value: &openapi3.Schema{Title: "T"}}); got == "title:T" || len(got) <= len("title:T") {
		t.Fatalf("title key must include structure: %q", got)
	}
	if got := schemaRefSortKey(nil); got != "" {
		t.Fatalf("nil key: %q", got)
	}
}

func TestSortSchemaRefsByKey_UsesStructureForCompositionCollisions(t *testing.T) {
	makeRefs := func(reverse bool) openapi3.SchemaRefs {
		firstSchema := openapi3.NewStringSchema()
		firstSchema.Title = "same"
		secondSchema := openapi3.NewIntegerSchema()
		secondSchema.Title = "same"
		first := &openapi3.SchemaRef{Value: firstSchema}
		second := &openapi3.SchemaRef{Value: secondSchema}
		if reverse {
			return openapi3.SchemaRefs{second, first}
		}
		return openapi3.SchemaRefs{first, second}
	}
	a, b := makeRefs(false), makeRefs(true)
	sortSchemaRefsByKey(a)
	sortSchemaRefsByKey(b)
	if schemaRefSortKey(a[0]) != schemaRefSortKey(b[0]) || schemaRefSortKey(a[1]) != schemaRefSortKey(b[1]) {
		t.Fatalf("reverse input changed order: %q vs %q", schemaRefSortKey(a[0]), schemaRefSortKey(b[0]))
	}
}
