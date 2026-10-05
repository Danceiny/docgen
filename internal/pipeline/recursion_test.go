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

// referenceOf is the component a field refers to: a field that has a comment of
// its own refers to it from the only member of an allOf, which can have the
// description.
func referenceOf(ref *openapi3.SchemaRef) string {
	if ref == nil {
		return ""
	}
	if ref.Ref == "" && ref.Value != nil && len(ref.Value.AllOf) == 1 {
		return ref.Value.AllOf[0].Ref
	}
	return ref.Ref
}

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
			if got := referenceOf(props["parent"]); got != recursionPrefix+"tree.Node" {
				t.Errorf("parent refers to %q, want Node", got)
			}
			if got := props["parent"].Value.Description; got != "Parent is the node above this one." {
				t.Errorf("the description of parent = %q: it is written next to the reference", got)
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
			// The folder a file is in is a reference to Folder, with the description of
			// the field next to it.
			folderOfFile := file.Properties["folder"]
			if got := referenceOf(folderOfFile); got != recursionPrefix+"tree.Folder" {
				t.Errorf("the folder of a file = %+v: not a reference to Folder", folderOfFile)
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
	value := schema.Value
	if len(value.AllOf) == 1 && value.AllOf[0].Value != nil {
		value = value.AllOf[0].Value // a field with a comment of its own refers to its type from an allOf
	}
	for name := range value.Properties {
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
	assert.Nil(t, public.Paths.Value("/api/shop/level"), "an operation that returns an enum that is hidden by its name is left out too")

	// The internal document hides only what is hidden from everybody.
	internalOrder := internal.Components.Schemas[hidingPrefix+"Order"]
	require.NotNil(t, internalOrder)
	assert.Equal(t, []string{"after", "afterPtrList", "before", "beforeList", "beforeMap", "beforeNested", "beforePtrList", "beforeValue", "id", "level", "note", "shown"},
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

	for _, path := range []string{"/api/shop/anything", "/api/shop/whatever"} {
		anything := responseSchema(t, doc, path)
		assert.Nil(t, anything.Value.Type, "%s: an interface holds any JSON value", path)
	}

	// The summary of an operation is the first line of its comment, unless that is
	// an annotation: then it is the name of the method.
	assert.Equal(t, "Annotated", doc.Paths.Value("/api/shop/annotated").Post.Summary)
	assert.Equal(t, "One returns a product.", doc.Paths.Value("/api/shop/one").Post.Summary)
}

// compat.legacy_operation_types keeps the documents that were generated before
// the shape of a result was described as they were: a list is its element, and a
// map or an instantiated generic type has no content.
func TestLegacyOperationTypesKeepTheOldResults(t *testing.T) {
	doc := generateFixtureWith(t, "testdata/results", "docgen-legacy.yaml", "plain")
	product := "#/components/schemas/example.com.results.m.Product"

	assert.Equal(t, product, responseSchema(t, doc, "/api/shop/list").Ref, "a list is its element")
	assert.Equal(t, product, responseSchema(t, doc, "/api/shop/pointers").Ref)

	for _, path := range []string{"/api/shop/counts", "/api/shop/paged", "/api/shop/anything"} {
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

const mapsPrefix = "example.com.maps.m."

// The values of a map are in the document: a reference to the component of a
// struct, the schema of anything else, and just true for any value.
func TestTheValuesOfAMapAreDescribed(t *testing.T) {
	for _, name := range []string{"internal", "public"} {
		t.Run(name, func(t *testing.T) {
			doc := generateFixture(t, "testdata/maps", name)
			pet := doc.Components.Schemas[mapsPrefix+"Pet"]
			require.NotNil(t, pet)
			props := pet.Value.Properties

			tags := props["tags"].Value.AdditionalProperties
			require.NotNil(t, tags.Schema, "tags: the values have a schema")
			assert.True(t, tags.Schema.Value.Type.Is("string"))

			owners := props["owners"].Value.AdditionalProperties
			require.NotNil(t, owners.Schema, "owners: the values have a schema")
			assert.Equal(t, "#/components/schemas/"+mapsPrefix+"Owner", owners.Schema.Ref, "a struct is a reference, in the public document too")

			extra := props["extra"].Value.AdditionalProperties
			assert.Nil(t, extra.Schema, "a map of any says no more than true")
			require.NotNil(t, extra.Has)
			assert.True(t, *extra.Has)

			photo := props["photo"].Value
			assert.True(t, photo.Type.Is("string"), "a slice of bytes is written as a string")
			assert.Equal(t, "byte", photo.Format)
		})
	}
}

// compat.legacy_output keeps the documents that were generated before.
func TestLegacyOutputKeepTheMapsAsTheyWere(t *testing.T) {
	doc := generateFixtureWith(t, "testdata/maps", "docgen-legacy.yaml", "internal")
	props := doc.Components.Schemas[mapsPrefix+"Pet"].Value.Properties

	owners := props["owners"].Value.AdditionalProperties
	assert.Nil(t, owners.Schema, "the values of a map are not in the document")
	require.NotNil(t, owners.Has)
	assert.True(t, *owners.Has)
	assert.True(t, props["photo"].Value.Type.Is("array"), "a slice of bytes is an array of strings")
}

const commentsPrefix = "example.com.comments.m."

// What a field says about itself is written with the field, and the same way
// whatever order the types are declared in: a field of a type that is a component
// refers to it from the only member of an allOf, which can have a description, a
// flag, an example and a default, and a field that says nothing is the reference. The
// comment of a type is the description of its component, and a list is described
// once, not once for each level it is made of.
func TestWhatAFieldSaysAboutItselfIsWrittenWithTheField(t *testing.T) {
	for _, name := range []string{"internal", "public"} {
		t.Run(name, func(t *testing.T) {
			doc := generateFixture(t, "testdata/comments", name)
			pet := doc.Components.Schemas[commentsPrefix+"Pet"]
			require.NotNil(t, pet)
			assert.Equal(t, "Pet is an animal, with fields of types declared before and after it.", pet.Value.Description)
			props := pet.Value.Properties

			for field, want := range map[string]string{
				"home":   "Home is of a type declared after Pet.",
				"before": "Before is of a type declared before Pet.",
				"plain":  "Plain says nothing about its type.",
			} {
				p := props[field]
				require.NotNil(t, p, field)
				assert.Empty(t, p.Ref, field)
				require.Len(t, p.Value.AllOf, 1, "%s: a reference with a description of its own", field)
				assert.Equal(t, want, p.Value.Description, field)
				assert.Contains(t, p.Value.AllOf[0].Ref, commentsPrefix, field)
			}
			assert.Equal(t, "#/components/schemas/"+commentsPrefix+"Birth", props["bare"].Ref, "a field with nothing to say is the reference, in either order")

			kind := props["kind"].Value
			require.Len(t, kind.AllOf, 1)
			assert.Equal(t, "Kind is what the pet is.", kind.Description)
			assert.EqualValues(t, 1, kind.Example, "the example is a value of the enum it refers to")
			assert.EqualValues(t, 0, kind.Default)

			maybe := props["maybe"].Value
			assert.True(t, maybe.Nullable, "nullable is said next to the reference")
			assert.Len(t, maybe.AllOf, 1)

			others := props["others"].Value
			assert.Equal(t, "Others own it too.", others.Description)
			assert.Equal(t, "#/components/schemas/"+commentsPrefix+"Birth", others.Items.Ref, "the elements are the reference, with no comment of the field next to it")

			matrix := props["matrix"].Value
			assert.Equal(t, "Matrix is a matrix of numbers.", matrix.Description)
			assert.Empty(t, matrix.Items.Value.Description)
			assert.Empty(t, matrix.Items.Value.Items.Value.Description)
			assert.Empty(t, props["tags"].Value.Items.Value.Description)

			birth := doc.Components.Schemas[commentsPrefix+"Birth"]
			require.NotNil(t, birth, "the public document keeps the component the references are to")
			assert.Equal(t, "Birth is when and where.", birth.Value.Description)
			kindComponent := doc.Components.Schemas[commentsPrefix+"Kind"]
			require.NotNil(t, kindComponent)
			assert.True(t, strings.HasPrefix(kindComponent.Value.Description, "Kind says what a pet is.\n\nEnums"), "the comment of the type first, then its values: %q", kindComponent.Value.Description)
		})
	}
}

// compat.legacy_output keeps documents the way they were: a comment on a field
// of a type that is declared after the struct copies the type into the field.
func TestLegacyOutputKeepTheCommentsAsTheyWere(t *testing.T) {
	doc := generateFixtureWith(t, "testdata/comments", "docgen-legacy.yaml", "internal")
	props := doc.Components.Schemas[commentsPrefix+"Pet"].Value.Properties

	assert.Empty(t, doc.Components.Schemas[commentsPrefix+"Pet"].Value.Description, "a type's comment is not its description")
	assert.Empty(t, props["home"].Value.AllOf)
	assert.NotEmpty(t, props["home"].Value.Properties, "a type declared after the struct is copied into a field that has a comment")
	assert.Equal(t, "#/components/schemas/"+commentsPrefix+"Early", props["before"].Ref, "one declared before is a reference, and the comment is lost")
}

// A type that an overlay replaces may have had a type that nothing else uses; the
// public document drops that type with the rest of what nothing refers to, and
// is written. The fields that refer to the replaced type refer to the new schema.
func TestAnOverlayThatReplacesATypeLeavesNoDanglingReferences(t *testing.T) {
	for _, name := range []string{"internal", "public"} {
		t.Run(name, func(t *testing.T) {
			doc := generateFixture(t, "testdata/replaced", name)
			owner := doc.Components.Schemas["example.com.replaced.m.Owner"]
			require.NotNil(t, owner)
			assert.Contains(t, owner.Value.Properties, "name", "the schema of the overlay")
			assert.NotContains(t, owner.Value.Properties, "address")
			if name == "public" {
				assert.NotContains(t, doc.Components.Schemas, "example.com.replaced.m.Address", "nothing refers to it any more")
			}
		})
	}
}

const ondemandPrefix = "example.com.ondemand.other."

// A type of the module in a package that no pattern of models matches is described
// by its own declaration, as it is in a package that does match: its description
// is its comment, not that of the first field that uses it, it is not nullable
// because a field of it is, and its enums are made of what they are made of.
func TestATypeOutsideModelsIsDescribedByItsDeclaration(t *testing.T) {
	doc := generateFixture(t, "testdata/ondemand", "internal")

	thing := doc.Components.Schemas[ondemandPrefix+"Thing"]
	require.NotNil(t, thing)
	assert.Equal(t, "Thing is a thing in another package.", thing.Value.Description)
	assert.False(t, thing.Value.Nullable, "a field of it is nullable, the type is not")

	holder := doc.Components.Schemas["example.com.ondemand.m.Holder"].Value.Properties
	assert.True(t, holder["first"].Value.Nullable, "the field says so, next to the reference")
	assert.Equal(t, "First is the first use of the thing, and it is nullable.", holder["first"].Value.Description)
	assert.Equal(t, "Second is the second use.", holder["second"].Value.Description)
	assert.Equal(t, "#/components/schemas/"+ondemandPrefix+"Thing", holder["plain"].Ref)

	raw := doc.Components.Schemas[ondemandPrefix+"Raw"].Value
	assert.True(t, raw.Type.Is("integer"))
	assert.Equal(t, []any{float64(97), float64(98)}, raw.Enum)
	dur := doc.Components.Schemas[ondemandPrefix+"Dur"].Value
	assert.True(t, dur.Type.Is("integer"), "a duration is a number in JSON")
	assert.True(t, strings.HasPrefix(dur.Description, "Dur is an enum of durations."))
}
