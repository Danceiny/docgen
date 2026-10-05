package pipeline

import (
	"container/list"
	"fmt"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
)

func structSchema(properties ...string) *openapi3.SchemaRef {
	schema := openapi3.NewObjectSchema()
	for _, p := range properties {
		schema.Properties[p] = openapi3.NewStringSchema().NewRef()
	}
	return schema.NewRef()
}

// A struct embedding a struct that embeds a struct has the fields of all of
// them, however deep, and whatever order the tasks come in: the last link first
// is the slowest way to pass them along.
func TestEmbeddedFieldsAreMergedAtAnyDepth(t *testing.T) {
	const depth = 40
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	for i := 0; i <= depth; i++ {
		doc.Components.Schemas[fmt.Sprint("S", i)] = structSchema(fmt.Sprint("field", i))
	}
	tasks := list.New()
	for i := 0; i < depth; i++ { // S<i> embeds S<i+1>; the tasks start from the top, which needs the most passes
		tasks.PushBack(engine.MergeTask{TargetKey: fmt.Sprint("S", i), SourceKey: fmt.Sprint("S", i+1)})
	}

	runMergeTasks(doc, tasks)

	if got := len(doc.Components.Schemas["S0"].Value.Properties); got != depth+1 {
		t.Errorf("S0 has %d fields, want %d: the fields of %d levels of embedding", got, depth+1, depth)
	}
}

// Structs that embed each other, which Go allows through pointers, make the
// loop meet its own work again; it must end with each having the fields of both.
func TestEmbeddedFieldsOfStructsThatEmbedEachOtherAreMerged(t *testing.T) {
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"A": structSchema("a"),
		"B": structSchema("b"),
	}}}
	tasks := list.New()
	tasks.PushBack(engine.MergeTask{TargetKey: "A", SourceKey: "B"})
	tasks.PushBack(engine.MergeTask{TargetKey: "B", SourceKey: "A"})

	runMergeTasks(doc, tasks)

	for _, key := range []string{"A", "B"} {
		if got := len(doc.Components.Schemas[key].Value.Properties); got != 2 {
			t.Errorf("%s has %d fields, want 2", key, got)
		}
	}
}

// A struct that has a field of its own with the name of one it embeds is not made
// to require it by the embedded one: the field that is required or not is its own.
// The documents of a configuration that keeps the old way of writing do require it.
func TestAFieldThatShadowsAnEmbeddedOneIsNotRequiredByIt(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		old := legacyOutput
		legacyOutput = legacy
		t.Cleanup(func() { legacyOutput = old })

		base := structSchema("id", "email")
		base.Value.Required = []string{"id", "email"}
		outer := structSchema("id") // its own id, which is not required
		outer.Value.Required = nil
		doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{"Base": base, "Outer": outer}}}
		tasks := list.New()
		tasks.PushBack(engine.MergeTask{TargetKey: "Outer", SourceKey: "Base"})

		runMergeTasks(doc, tasks)

		want := []string{"email"}
		if legacy {
			want = []string{"id", "email"}
		}
		if got := outer.Value.Required; !reflect.DeepEqual(got, want) {
			t.Errorf("legacy=%v: required = %v, want %v", legacy, got, want)
		}
		if _, ok := outer.Value.Properties["email"]; !ok {
			t.Errorf("legacy=%v: the field the struct did not have is merged", legacy)
		}
	}
}

// diamonds makes a schema of depth levels, each of which refers to the next one
// twice, the shape of a graph of types that share types: the paths to the bottom
// are 2^depth, the schemas depth.
func diamonds(depth int) *openapi3.SchemaRef {
	bottom := openapi3.NewObjectSchema().NewRef()
	bottom.Value.Title = "example.Bottom"
	level := bottom
	for i := 0; i < depth; i++ {
		upper := openapi3.NewObjectSchema()
		upper.Properties["a"] = level
		upper.Properties["b"] = level
		upper.Title = fmt.Sprint("example.Level", i)
		level = upper.NewRef()
	}
	return level
}

// within fails the test when f takes more than the time a walk of each schema once takes,
// which is a few milliseconds, by a long way.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); f() }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s did not finish in 20 s: it walks every path to a schema, and there are 2^60 of them", what)
	}
}

func TestNarrowingOneOfVisitsEachSchemaOnce(t *testing.T) {
	root := diamonds(60)
	hider := &oneOfHider{done: map[hiddenOneOf]hiddenOneOfResult{}}
	within(t, "narrowing oneOf", func() { hider.hide(root, "") })
}

func TestSimplifyingTitlesVisitsEachSchemaOnce(t *testing.T) {
	doc := &openapi3.T{Paths: openapi3.NewPaths(), Components: &openapi3.Components{Schemas: openapi3.Schemas{"example.Root": diamonds(60)}}}
	within(t, "simplifying titles", func() { simplifyTitles(doc) })
	if got := doc.Components.Schemas["example.Root"].Value.Title; got != "Level59" {
		t.Errorf("title = %q", got)
	}
}

// A schema that docgen could not describe is said to be one, since nothing in a
// document says it is not a description.
func TestAPlaceholderInADocumentIsReported(t *testing.T) {
	log := &records{}
	engine.SetLogger(slog.New(log))
	t.Cleanup(func() { engine.SetLogger(nil) })

	placeholder := func() *openapi3.Schema { return &openapi3.Schema{Type: &openapi3.Types{"object"}, Pattern: "default"} }
	owner := openapi3.NewObjectSchema()
	owner.Properties["unknown"] = placeholder().NewRef()
	owner.Properties["fine"] = openapi3.NewStringSchema().NewRef()
	union := placeholder() // the marker of documents that keep the old way of writing, next to the alternatives
	union.OneOf = openapi3.SchemaRefs{{Ref: "#/components/schemas/example.Fine"}}
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"example.Owner": owner.NewRef(),
		"example.Lost":  placeholder().NewRef(),
		"example.Fine":  openapi3.NewObjectSchema().NewRef(),
		"example.Union": union.NewRef(),
	}}}
	warnAboutPlaceholders(config.Doc{Name: "internal"}, doc)

	var where []string
	for _, rec := range log.list {
		if rec.Level == slog.LevelWarn {
			where = append(where, attrOf(rec, "in"))
		}
	}
	if want := []string{"example.Lost", "example.Owner.unknown"}; !reflect.DeepEqual(where, want) {
		t.Errorf("reported %v, want %v", where, want)
	}
}

// The order of the fields lists the fields the schema has: a field of an embedded
// struct that its type hides from the document is not one of them.
func TestFieldOrdersListOnlyTheFieldsASchemaHas(t *testing.T) {
	schema := structSchema("a", "c")
	schema.Value.Extensions = map[string]any{"x-apifox-orders": []string{"a", "b", "c"}}
	empty := structSchema()
	empty.Value.Extensions = map[string]any{"x-apifox-orders": []string{"gone"}}
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{"S": schema, "E": empty}}}

	trimFieldOrders(doc)

	if got := schema.Value.Extensions["x-apifox-orders"]; !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("orders = %v", got)
	}
	if _, has := empty.Value.Extensions["x-apifox-orders"]; has {
		t.Error("a schema with no field has no order of them")
	}
}
