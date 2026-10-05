package pipeline

import (
	"container/list"
	"fmt"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

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
