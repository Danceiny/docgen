package pipeline

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Danceiny/docgen/internal/config"
)

// generateFixture generates the documents of a fixture module and loads the one
// with the given name back, which also validates it as an OpenAPI document.
func generateFixture(t *testing.T, dir, name string) *openapi3.T {
	t.Helper()
	return generateFixtureWith(t, dir, "docgen.yaml", name)
}

// generateFixtureWith is generateFixture with the configuration file of the
// fixture that is named.
func generateFixtureWith(t *testing.T, dir, configFile, name string) *openapi3.T {
	t.Helper()
	cfg, err := config.Load(filepath.Join(dir, configFile))
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Run(Options{Dir: dir, Config: cfg, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	selected, err := cfg.Select([]string{name})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, selected[0].Output))
	if err != nil {
		t.Fatal(err)
	}
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		t.Fatalf("load the generated document: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("the generated document is not valid: %v", err)
	}
	return doc
}

const recursionPrefix = "#/components/schemas/example.com.recursion."

// A type that contains itself, directly or through another type, is described
// with a $ref to itself. Describing it inline would never end: the document
// would have to contain a copy of the type inside the copy of the type.
func TestATypeThatContainsItselfIsAReference(t *testing.T) {
	for _, name := range []string{"internal", "public"} {
		t.Run(name, func(t *testing.T) {
			doc := generateFixture(t, "testdata/recursion", name)

			node := doc.Components.Schemas["example.com.recursion.tree.Node"]
			if node == nil || node.Value == nil {
				t.Fatalf("no component for Node; there are %d components", len(doc.Components.Schemas))
			}
			props := node.Value.Properties

			children := props["children"]
			if children == nil || children.Value == nil || children.Value.Items == nil {
				t.Fatalf("children = %+v", children)
			}
			if got := children.Value.Items.Ref; got != recursionPrefix+"tree.Node" {
				t.Errorf("the items of children refer to %q, want Node", got)
			}
			if got := children.Value.Description; got != "Children are the nodes below this one." {
				t.Errorf("the description of children = %q: a field's description stays on the field", got)
			}
			if got := props["parent"].Ref; got != recursionPrefix+"tree.Node" {
				t.Errorf("parent refers to %q, want Node", got)
			}
			if got := props["siblings"].Value.Items.Ref; got != recursionPrefix+"tree.Node" {
				t.Errorf("the items of siblings refer to %q, want Node", got)
			}
			if props["attributes"] == nil {
				t.Error("attributes is missing")
			}

			folder := doc.Components.Schemas["example.com.recursion.tree.Folder"].Value
			file := doc.Components.Schemas["example.com.recursion.tree.File"].Value
			if got := folder.Properties["files"].Value.Items.Ref; got != recursionPrefix+"tree.File" {
				t.Errorf("the files of a folder refer to %q, want File", got)
			}
			// A field that has a description may carry a copy of its type instead of
			// a reference to it. Either way the files of that folder refer to File
			// and the copy does not go on for ever.
			folderOfFile := file.Properties["folder"]
			switch {
			case folderOfFile.Ref == recursionPrefix+"tree.Folder":
			case folderOfFile.Value != nil && folderOfFile.Value.Properties["files"] != nil &&
				folderOfFile.Value.Properties["files"].Value.Items.Ref == recursionPrefix+"tree.File":
			default:
				t.Errorf("the folder of a file = %+v: neither a reference to Folder nor a copy of it", folderOfFile)
			}
		})
	}
}

// A model package that imports nothing and that no other package imports has
// no place in the dependency order unless it is put there; its types are in the
// document all the same.
func TestAModelPackageWithNoNeighboursIsDocumented(t *testing.T) {
	doc := generateFixture(t, "testdata/recursion", "internal")
	if doc.Components.Schemas["example.com.recursion.lonely.Lonely"] == nil {
		t.Fatalf("the type of a package that imports nothing is missing; the components are %d", len(doc.Components.Schemas))
	}
}

const hidingPrefix = "example.com.hiding.domain."

func propertyNames(schema *openapi3.SchemaRef) []string {
	var names []string
	for name := range schema.Value.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// A type that a document hides is not in it, and neither is anything that refers
// to it, whatever the shape of the reference, and wherever the type is declared:
// it is the same for a type that is declared before the type that uses it and for
// one declared after. What a type says about itself is stronger than its name. An
// operation that uses a hidden type is left out, with a warning.
func TestAHiddenTypeIsLeftOutWithEverythingThatRefersToIt(t *testing.T) {
	internal := generateFixture(t, "testdata/hiding", "internal")
	public := generateFixture(t, "testdata/hiding", "public")

	// The public document: the customer sees ID and the types that are shown.
	order := public.Components.Schemas[hidingPrefix+"Order"]
	require.NotNil(t, order)
	assert.Equal(t, []string{"id", "shown"}, propertyNames(order),
		"every field of a hidden type is gone, in every shape; the override stays")
	assert.Contains(t, propertyNames(order.Value.Properties["shown"]), "note", "a directive overrules the name")

	for _, hidden := range []string{"InternalBefore", "InternalAfter", "StaffNote", "Draft"} {
		assert.NotContains(t, public.Components.Schemas, hidingPrefix+hidden, "%s is hidden from the public document", hidden)
	}
	assert.NotContains(t, public.Components.Schemas, hidingPrefix+"InternalLevel", "an enum with a hidden name is hidden by it")

	assert.NotNil(t, public.Paths.Value("/api/shop/get"))
	assert.Nil(t, public.Paths.Value("/api/shop/risk"), "an operation that returns a hidden type is left out")
	assert.Nil(t, public.Paths.Value("/api/shop/note"), "an operation that takes a hidden type is left out")

	// The internal document hides only what is hidden from everybody.
	internalOrder := internal.Components.Schemas[hidingPrefix+"Order"]
	require.NotNil(t, internalOrder)
	assert.Equal(t, []string{"after", "afterPtrList", "before", "beforeList", "beforeMap", "beforePtrList", "beforeValue", "id", "level", "note", "shown"},
		propertyNames(internalOrder))
	assert.NotContains(t, internal.Components.Schemas, hidingPrefix+"Draft")
	assert.Contains(t, internal.Components.Schemas, hidingPrefix+"StaffNote")
	assert.NotNil(t, internal.Paths.Value("/api/shop/risk"))
}

// responseSchema returns the schema of the 200 response of an operation.
func responseSchema(t *testing.T, doc *openapi3.T, path string) *openapi3.SchemaRef {
	t.Helper()
	item := doc.Paths.Value(path)
	require.NotNil(t, item, path)
	response := item.Post.Responses.Value("200")
	require.NotNil(t, response, path)
	media := response.Value.Content.Get("application/json")
	require.NotNil(t, media, "%s has no content: the result is not described", path)
	return media.Schema
}

// The result of an operation is what the Go method returns: a list of products is
// a list, whatever its elements are, and a map or an instantiated generic type is
// something rather than nothing.
func TestTheResultOfAnOperationKeepsItsShape(t *testing.T) {
	doc := generateFixture(t, "testdata/results", "plain")
	product := "#/components/schemas/example.com.results.m.Product"

	one := responseSchema(t, doc, "/api/shop/one")
	assert.Equal(t, product, one.Ref)

	for _, path := range []string{"/api/shop/list", "/api/shop/pointers"} {
		list := responseSchema(t, doc, path)
		require.NotNil(t, list.Value, path)
		assert.True(t, list.Value.Type.Is("array"), "%s is a list", path)
		assert.Equal(t, product, list.Value.Items.Ref, path)
	}

	grid := responseSchema(t, doc, "/api/shop/grid")
	require.True(t, grid.Value.Type.Is("array"))
	require.True(t, grid.Value.Items.Value.Type.Is("array"), "a list of lists")
	assert.Equal(t, product, grid.Value.Items.Value.Items.Ref)

	names := responseSchema(t, doc, "/api/shop/names")
	require.True(t, names.Value.Type.Is("array"))
	assert.True(t, names.Value.Items.Value.Type.Is("string"))

	counts := responseSchema(t, doc, "/api/shop/counts")
	assert.True(t, counts.Value.Type.Is("object"))

	paged := responseSchema(t, doc, "/api/shop/paged")
	assert.Equal(t, "#/components/schemas/example.com.results.m.Page", paged.Ref)
}

// compat.legacy_operation_types keeps the documents that were generated before
// the shape of a result was described as they were: a list is its element, and a
// map or an instantiated generic type has no content.
func TestLegacyOperationTypesKeepTheOldResults(t *testing.T) {
	doc := generateFixtureWith(t, "testdata/results", "docgen-legacy.yaml", "plain")
	product := "#/components/schemas/example.com.results.m.Product"

	assert.Equal(t, product, responseSchema(t, doc, "/api/shop/list").Ref, "a list is its element")
	assert.Equal(t, product, responseSchema(t, doc, "/api/shop/pointers").Ref)

	for _, path := range []string{"/api/shop/counts", "/api/shop/paged"} {
		response := doc.Paths.Value(path).Post.Responses.Value("200")
		require.NotNil(t, response, path)
		assert.Nil(t, response.Value.Content.Get("application/json"), "%s: a map and a generic type have no content", path)
	}
}

// parameterOf finds the parameter with the name of an operation.
func parameterOf(t *testing.T, op *openapi3.Operation, name string) *openapi3.Parameter {
	t.Helper()
	for _, p := range op.Parameters {
		if p.Value.Name == name {
			return p.Value
		}
	}
	t.Fatalf("%s %s has no parameter %q; it has %d", op.OperationID, op.Summary, name, len(op.Parameters))
	return nil
}

// A @param annotation turns a parameter of a basic type into a query, header or
// path parameter: it names the Go parameter, or its position as param1, param2.
// One that names no parameter says so instead of being ignored.
func TestParamAnnotationsDescribeTheParametersOfBasicTypes(t *testing.T) {
	cfg, err := config.Load("testdata/params/docgen.yaml")
	require.NoError(t, err)
	log := &records{}
	out := t.TempDir()
	require.NoError(t, Run(Options{Dir: "testdata/params", Config: cfg, OutputDir: out, Logger: slog.New(log)}))
	data, err := os.ReadFile(filepath.Join(out, "out/all.yaml"))
	require.NoError(t, err)
	doc, err := openapi3.NewLoader().LoadFromData(data)
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))

	count := doc.Paths.Value("/api/shop/count").Get
	status := parameterOf(t, count, "status")
	assert.Equal(t, "query", status.In)
	assert.True(t, status.Required)
	assert.Equal(t, "the state of the items", status.Description)
	assert.True(t, status.Schema.Value.Type.Is("string"))
	assert.Nil(t, count.RequestBody, "a parameter that is described is not a body")

	byID := doc.Paths.Value("/api/shop/byId/{id}").Get
	id := parameterOf(t, byID, "id")
	assert.Equal(t, "path", id.In)
	assert.True(t, id.Required, "a path parameter is always required")

	many := doc.Paths.Value("/api/shop/many").Get
	limit := parameterOf(t, many, "limit")
	assert.Equal(t, "query", limit.In)
	assert.False(t, limit.Required)
	assert.True(t, limit.Schema.Value.Type.Is("integer"))
	tags := parameterOf(t, many, "tags")
	assert.True(t, tags.Schema.Value.Type.Is("array"))
	assert.Equal(t, "header", parameterOf(t, many, "token").In)

	assert.Equal(t, "query", parameterOf(t, doc.Paths.Value("/api/shop/pos").Get, "param1").In, "by position")

	unmatched := doc.Paths.Value("/api/shop/unmatched").Post
	assert.NotNil(t, unmatched.RequestBody)
	assert.Empty(t, unmatched.Parameters)
	warned := false
	for _, rec := range log.list {
		if rec.Level == slog.LevelWarn && strings.Contains(rec.Message, "@param names a parameter that the method does not have") {
			warned = true
		}
	}
	assert.True(t, warned, "an annotation that matches nothing is reported")

	assert.NotNil(t, doc.Paths.Value("/api/shop/plain").Post.RequestBody, "an undescribed basic type is the body, as before")
}

// A package is used by the name it declares, which is not always the last element
// of its import path: package models in model/, package proto in proto/v2.
func TestAPackageNameThatIsNotTheLastElementOfItsPath(t *testing.T) {
	doc := generateFixture(t, "testdata/pkgnames", "all")
	ask := doc.Paths.Value("/api/shop/ask").Post
	assert.Equal(t, "#/components/schemas/example.com.pkgnames.model.Req", ask.RequestBody.Value.Content.Get("application/json").Schema.Ref)
	assert.Equal(t, "#/components/schemas/example.com.pkgnames.proto.v2.Reply", responseSchema(t, doc, "/api/shop/ask").Ref)
	answer := doc.Paths.Value("/api/shop/answer").Post
	assert.Equal(t, "#/components/schemas/example.com.pkgnames.proto.v2.Reply", answer.RequestBody.Value.Content.Get("application/json").Schema.Ref)
	assert.Equal(t, "#/components/schemas/example.com.pkgnames.model.Resp", responseSchema(t, doc, "/api/shop/answer").Ref)
}
