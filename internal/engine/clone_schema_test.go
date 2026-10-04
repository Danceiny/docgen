package engine

import (
	"reflect"
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestCloneSchema_PreservesContractFields(t *testing.T) {
	t.Parallel()

	min := 1.5
	max := 9.5
	multipleOf := 0.5
	maxLen := uint64(32)
	maxItems := uint64(10)
	maxProps := uint64(5)
	excl := true
	hasAdditional := false

	src := &openapi3.Schema{
		Type:            &openapi3.Types{"string"},
		Title:           "Status",
		Format:          "enum",
		Description:     "order status",
		Nullable:        true,
		Enum:            []any{"pending", "confirmed"},
		Default:         "pending",
		Example:         "confirmed",
		ReadOnly:        true,
		WriteOnly:       false,
		AllowEmptyValue: true,
		Deprecated:      true,
		UniqueItems:     true,
		ExclusiveMin:    openapi3.ExclusiveBound{Bool: &excl},
		ExclusiveMax:    openapi3.ExclusiveBound{Value: &max},
		Min:             &min,
		Max:             &max,
		MultipleOf:      &multipleOf,
		MinLength:       2,
		MaxLength:       &maxLen,
		Pattern:         "^[a-z]+$",
		MinItems:        1,
		MaxItems:        &maxItems,
		MinProps:        1,
		MaxProps:        &maxProps,
		Required:        []string{"id"},
		Properties: openapi3.Schemas{
			"id": {Value: &openapi3.Schema{Title: "id", Example: "x"}},
		},
		Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Title: "item"}},
		Not:   &openapi3.SchemaRef{Value: &openapi3.Schema{Title: "not"}},
		AllOf: openapi3.SchemaRefs{{Value: &openapi3.Schema{Title: "all"}}},
		OneOf: openapi3.SchemaRefs{{Value: &openapi3.Schema{Title: "one"}}},
		AnyOf: openapi3.SchemaRefs{{Value: &openapi3.Schema{Title: "any"}}},
		AdditionalProperties: openapi3.AdditionalProperties{
			Has:    &hasAdditional,
			Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Title: "extra"}},
		},
		Discriminator: &openapi3.Discriminator{
			PropertyName: "kind",
			Mapping: openapi3.StringMap[openapi3.MappingRef]{
				"a": {Ref: "#/components/schemas/A"},
			},
			Extensions: map[string]any{"x-disc": true},
		},
		Extensions:   map[string]any{"x-keep": 1},
		ExternalDocs: &openapi3.ExternalDocs{URL: "https://example.test"},
		XML:          &openapi3.XML{Name: "Status"},
	}

	got := cloneSchema(src)
	if got == src {
		t.Fatal("cloneSchema must return a new pointer")
	}

	if !reflect.DeepEqual(got.Enum, src.Enum) {
		t.Fatalf("Enum: got %#v want %#v", got.Enum, src.Enum)
	}
	if got.Default != src.Default {
		t.Fatalf("Default: got %#v want %#v", got.Default, src.Default)
	}
	if got.Example != src.Example {
		t.Fatalf("Example: got %#v want %#v", got.Example, src.Example)
	}
	if !got.ReadOnly || !got.Deprecated || !got.AllowEmptyValue || !got.UniqueItems {
		t.Fatalf("bool flags dropped: ReadOnly=%v Deprecated=%v AllowEmptyValue=%v UniqueItems=%v",
			got.ReadOnly, got.Deprecated, got.AllowEmptyValue, got.UniqueItems)
	}
	if got.Min == nil || *got.Min != min || got.Max == nil || *got.Max != max {
		t.Fatalf("Min/Max not preserved: min=%v max=%v", got.Min, got.Max)
	}
	if got.MultipleOf == nil || *got.MultipleOf != multipleOf {
		t.Fatalf("MultipleOf: got %v", got.MultipleOf)
	}
	if got.MinLength != 2 || got.MaxLength == nil || *got.MaxLength != maxLen || got.Pattern != "^[a-z]+$" {
		t.Fatalf("string constraints dropped: minLen=%d maxLen=%v pattern=%q", got.MinLength, got.MaxLength, got.Pattern)
	}
	if got.MinItems != 1 || got.MaxItems == nil || *got.MaxItems != maxItems {
		t.Fatalf("array constraints dropped: minItems=%d maxItems=%v", got.MinItems, got.MaxItems)
	}
	if got.MinProps != 1 || got.MaxProps == nil || *got.MaxProps != maxProps {
		t.Fatalf("object constraints dropped: minProps=%d maxProps=%v", got.MinProps, got.MaxProps)
	}
	if got.ExclusiveMin.Bool == nil || !*got.ExclusiveMin.Bool {
		t.Fatalf("ExclusiveMin.Bool not preserved: %#v", got.ExclusiveMin)
	}
	if got.ExclusiveMax.Value == nil || *got.ExclusiveMax.Value != max {
		t.Fatalf("ExclusiveMax.Value not preserved: %#v", got.ExclusiveMax)
	}
	if got.Items == nil || got.Items.Value == nil || got.Items.Value.Title != "item" {
		t.Fatalf("Items not preserved: %#v", got.Items)
	}
	if got.Not == nil || got.Not.Value == nil || got.Not.Value.Title != "not" {
		t.Fatalf("Not not preserved: %#v", got.Not)
	}
	if len(got.AllOf) != 1 || got.AllOf[0].Value.Title != "all" {
		t.Fatalf("AllOf not preserved: %#v", got.AllOf)
	}
	if len(got.OneOf) != 1 || got.OneOf[0].Value.Title != "one" {
		t.Fatalf("OneOf not preserved: %#v", got.OneOf)
	}
	if len(got.AnyOf) != 1 || got.AnyOf[0].Value.Title != "any" {
		t.Fatalf("AnyOf not preserved: %#v", got.AnyOf)
	}
	if got.AdditionalProperties.Has == nil || *got.AdditionalProperties.Has != false {
		t.Fatalf("AdditionalProperties.Has not preserved: %#v", got.AdditionalProperties.Has)
	}
	if got.AdditionalProperties.Schema == nil || got.AdditionalProperties.Schema.Value.Title != "extra" {
		t.Fatalf("AdditionalProperties.Schema not preserved: %#v", got.AdditionalProperties.Schema)
	}
	if got.Discriminator == nil || got.Discriminator.PropertyName != "kind" {
		t.Fatalf("Discriminator not preserved: %#v", got.Discriminator)
	}
	if ref, ok := got.Discriminator.Mapping["a"]; !ok || ref.Ref != "#/components/schemas/A" {
		t.Fatalf("Discriminator.Mapping not preserved: %#v", got.Discriminator.Mapping)
	}
	if got.Extensions["x-keep"] != 1 {
		t.Fatalf("Extensions not preserved: %#v", got.Extensions)
	}
	if got.ExternalDocs == nil || got.ExternalDocs.URL != "https://example.test" {
		t.Fatalf("ExternalDocs not preserved: %#v", got.ExternalDocs)
	}
	if got.XML == nil || got.XML.Name != "Status" {
		t.Fatalf("XML not preserved: %#v", got.XML)
	}

	// Mutating the clone must not mutate the source Enum / AdditionalProperties.Has.
	got.Enum[0] = "mutated"
	if src.Enum[0] != "pending" {
		t.Fatal("Enum slice aliasing: mutate clone mutated source")
	}
	*got.AdditionalProperties.Has = true
	if *src.AdditionalProperties.Has != false {
		t.Fatal("AdditionalProperties.Has aliasing: mutate clone mutated source")
	}
	*got.Min = 99
	if *src.Min != min {
		t.Fatal("Min pointer aliasing: mutate clone mutated source")
	}
}

func TestCloneSchema_KnownGaps(t *testing.T) {
	t.Parallel()

	requiredPreserved := []string{"Enum", "Default", "Example"}
	for _, name := range requiredPreserved {
		if !slices.Contains(CloneSchemaPreservedFields, name) {
			t.Fatalf("%s must be listed in CloneSchemaPreservedFields", name)
		}
		if slices.Contains(CloneSchemaKnownGaps, name) {
			t.Fatalf("%s must not be listed as a known gap", name)
		}
	}

	overlap := make([]string, 0)
	for _, name := range CloneSchemaPreservedFields {
		if slices.Contains(CloneSchemaKnownGaps, name) {
			overlap = append(overlap, name)
		}
	}
	if len(overlap) > 0 {
		t.Fatalf("fields cannot be both preserved and known-gap: %v", overlap)
	}

	schemaType := reflect.TypeOf(openapi3.Schema{})
	for _, name := range CloneSchemaPreservedFields {
		if _, ok := schemaType.FieldByName(name); !ok {
			t.Fatalf("CloneSchemaPreservedFields contains unknown Schema field %q", name)
		}
	}
	for _, name := range CloneSchemaKnownGaps {
		if _, ok := schemaType.FieldByName(name); !ok {
			t.Fatalf("CloneSchemaKnownGaps contains unknown Schema field %q", name)
		}
	}

	// Exhaustiveness: every exported Schema field is either preserved or a known gap.
	// Unexported / untracked fields are ignored.
	covered := make(map[string]struct{}, len(CloneSchemaPreservedFields)+len(CloneSchemaKnownGaps))
	for _, name := range CloneSchemaPreservedFields {
		covered[name] = struct{}{}
	}
	for _, name := range CloneSchemaKnownGaps {
		covered[name] = struct{}{}
	}
	missing := make([]string, 0)
	for i := 0; i < schemaType.NumField(); i++ {
		f := schemaType.Field(i)
		if !f.IsExported() {
			continue
		}
		if _, ok := covered[f.Name]; !ok {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("Schema fields neither preserved nor known-gap (silent drift risk): %v", missing)
	}
}

func TestCloneSchema_NilAndEmpty(t *testing.T) {
	t.Parallel()

	if got := cloneSchema(nil); got == nil {
		t.Fatal("nil input must yield empty schema, not nil")
	}
	got := cloneSchema(&openapi3.Schema{})
	if got == nil {
		t.Fatal("empty schema clone is nil")
	}
	if len(got.Enum) != 0 || got.Default != nil || got.Example != nil {
		t.Fatalf("empty schema leaked values: %#v", got)
	}
}
