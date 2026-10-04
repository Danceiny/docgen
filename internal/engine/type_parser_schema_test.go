package engine

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func docWith(keys ...string) *openapi3.T {
	schemas := openapi3.Schemas{}
	for _, k := range keys {
		schemas[k] = &openapi3.SchemaRef{Value: &openapi3.Schema{Title: k}}
	}
	return &openapi3.T{Components: &openapi3.Components{Schemas: schemas}}
}

func TestSearchSchemaFromDocPrefersTheExactKey(t *testing.T) {
	doc := docWith("shop.Cart", "a.shop.Cart", "Cart")
	if got := searchSchemaFromDoc(doc, "Cart"); got == nil || got.Value.Title != "Cart" {
		t.Fatalf("exact key not preferred: %+v", got)
	}
}

// A short name can end several components, when two packages declare a type of
// that name. The smallest full key is the answer every time; a hash table's
// iteration order must not decide which package's type a document describes.
func TestSearchSchemaFromDocPicksTheSmallestSuffixMatch(t *testing.T) {
	doc := docWith("z.shop.Cart", "m.shop.Cart", "a.shop.Cart", "b.shop.CartLine", "x.Other")
	for i := 0; i < 300; i++ {
		got := searchSchemaFromDoc(doc, "Cart")
		if got == nil || got.Value.Title != "a.shop.Cart" {
			t.Fatalf("run %d: got %+v, want a.shop.Cart", i, got)
		}
	}
}

func TestSearchSchemaFromDocFindsNothingWhenNothingEndsWithTheKey(t *testing.T) {
	doc := docWith("a.shop.Cart", "shop.CartLine")
	if got := searchSchemaFromDoc(doc, "Basket"); got != nil {
		t.Fatalf("got %+v", got)
	}
	// A suffix must start at a dot: "art" is not the name "Cart".
	if got := searchSchemaFromDoc(doc, "art"); got != nil {
		t.Fatalf("a partial name matched: %+v", got)
	}
}
