package pipeline

import (
	"container/list"
	"fmt"
	"testing"

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
