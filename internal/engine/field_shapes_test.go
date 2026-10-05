package engine

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fieldShapesKey = "github.com.Danceiny.docgen.internal.engine.testdata.fieldshapes."

func fieldShapes(t *testing.T, s Settings) *openapi3.T {
	t.Helper()
	withSettings(t, s, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/fieldshapes")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "fieldshapes"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	return doc
}

func propertyNames(doc *openapi3.T, name string) []string {
	schema := doc.Components.Schemas[fieldShapesKey+name]
	if schema == nil || schema.Value == nil {
		return nil
	}
	var names []string
	for p := range schema.Value.Properties {
		names = append(names, p)
	}
	sort.Strings(names)
	return names
}

func requiredOf(doc *openapi3.T, name string) []string {
	required := append([]string(nil), doc.Components.Schemas[fieldShapesKey+name].Value.Required...)
	sort.Strings(required)
	return required
}

// A struct is documented as encoding/json reads it.
func TestFieldShapesAreReadAsEncodingJSONReadsThem(t *testing.T) {
	doc := fieldShapes(t, Settings{})

	assert.Equal(t, []string{"Lat", "Lng"}, propertyNames(doc, "Point"), "every name of a declaration is a field, and a blank one is none")
	assert.Equal(t, []string{"base", "name"}, propertyNames(doc, "Nested"), "an embedded field that has a json name is a field of that name")
	base := doc.Components.Schemas[fieldShapesKey+"Nested"].Value.Properties["base"]
	require.NotNil(t, base)
	assert.Equal(t, fieldShapesKey+"Base", base.Value.Title, "the schema of the embedded type is the field")
	assert.Equal(t, []string{"id", "name"}, propertyNames(doc, "Flat"), "an embedded field with no name is flattened")
	assert.Equal(t, []string{"quiet"}, propertyNames(doc, "Quiet"), "the type of the embedded field need not be exported")

	assert.Equal(t, []string{"binding", "plain", "several", "spaced", "tagged"}, requiredOf(doc, "Rules"),
		"a rule among others is a rule: required,email")
	assert.Nil(t, doc.Components.Schemas[fieldShapesKey+"Raw2"].Value.Properties["body"].Value.Type, "raw JSON is any JSON")

	anything := doc.Components.Schemas[fieldShapesKey+"Anything"].Value.Properties
	for _, name := range []string{"any", "empty"} {
		require.NotNil(t, anything[name], name)
		assert.Nil(t, anything[name].Value.Type, "%s takes any JSON value, so its schema has no type", name)
		assert.Empty(t, anything[name].Value.AnyOf, name)
	}
	assert.Nil(t, anything["list"].Value.Items.Value.Type, "the items of a list of any are any value")

	// A type declared as any is any value too, and is no reason to stop.
	for _, name := range []string{"Payload", "Alias", "Raw"} {
		if schema := doc.Components.Schemas[fieldShapesKey+name]; schema != nil {
			assert.Nil(t, schema.Value.Type, "%s takes any JSON value", name)
		}
	}
	holders := doc.Components.Schemas[fieldShapesKey+"Holders"].Value.Properties
	assert.Equal(t, "P is a payload.", holders["p"].Value.Description, "the comment of a field of the type is its description")
	assert.Len(t, holders, 4)

	collections := doc.Components.Schemas[fieldShapesKey+"Collections"].Value.Properties
	tags := collections["tags"].Value.AdditionalProperties
	require.NotNil(t, tags.Schema, "the values of a map have their schema")
	assert.True(t, tags.Schema.Value.Type.Is("string"))
	assert.Equal(t, fieldShapesKey+"Base", collections["owners"].Value.AdditionalProperties.Schema.Value.Title)
	any := collections["any"].Value.AdditionalProperties
	assert.Nil(t, any.Schema, "a map of any is any value, which is just true")
	require.NotNil(t, any.Has)
	assert.True(t, *any.Has)
	for _, name := range []string{"data", "raw"} {
		assert.True(t, collections[name].Value.Type.Is("string"), "%s is written as a string by encoding/json", name)
		assert.Equal(t, "byte", collections[name].Value.Format, name)
	}
	assert.True(t, collections["hash"].Value.Type.Is("array"), "an array of bytes is an array of numbers")
	assert.True(t, collections["hash"].Value.Items.Value.Type.Is("integer"))
	assert.True(t, collections["one"].Value.Type.Is("integer"), "a byte is a number")
}

// legacy_output keeps what documents generated before the fix have.
func TestFieldShapesOfLegacyDocuments(t *testing.T) {
	doc := fieldShapes(t, Settings{CompatLegacyOutput: true})

	assert.Equal(t, []string{"Lat", "_"}, propertyNames(doc, "Point"), "the first name of a declaration, and the blank field as a property")
	assert.True(t, doc.Components.Schemas[fieldShapesKey+"Raw2"].Value.Properties["body"].Value.Type.Is("string"), "raw JSON is a string")
	assert.Equal(t, []string{"id", "name"}, propertyNames(doc, "Nested"), "an embedded field is flattened whatever its json name")
	assert.Equal(t, []string{"plain", "tagged"}, requiredOf(doc, "Rules"), "required only as the whole tag")

	anything := doc.Components.Schemas[fieldShapesKey+"Anything"].Value.Properties
	assert.True(t, anything["any"].Value.Type.Is("object"), "any is an object")
	assert.Len(t, anything["empty"].Value.AnyOf, 3, "interface{} is one of three types")

	collections := doc.Components.Schemas[fieldShapesKey+"Collections"].Value.Properties
	encoded, err := json.Marshal(collections["tags"].Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"object","additionalProperties":true}`, string(encoded), "a map has lost the type of its values")
	assert.True(t, collections["data"].Value.Type.Is("array"), "a slice of bytes is an array of strings")
	assert.True(t, collections["one"].Value.Type.Is("string"), "a byte is a string")
}

// The example and the default of a field are read as what its type says they are,
// and a value its type does not allow is left out with a warning that names the
// field, instead of making the whole document invalid.
func TestTagValuesAreReadByTheTypeOfTheField(t *testing.T) {
	logs := captureLogs(t)
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/tagvalues")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "tags", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	props := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.tagvalues.Thing"].Value.Properties
	assert.EqualValues(t, int64(3600000000000), props["timeout"].Value.Example, "a pointer to a duration is read as a duration")
	assert.EqualValues(t, int64(30000000000), props["timeout"].Value.Default)
	assert.EqualValues(t, int64(600000000000), props["plain"].Value.Example)
	assert.Equal(t, "123", props["pointer"].Value.Example, "a string is a string, though it looks like a number")
	assert.Equal(t, "456", props["custom"].Value.Example)
	assert.EqualValues(t, int64(3), props["level"].Value.Example)
	assert.EqualValues(t, int64(1234567890123456789), props["big"].Value.Example)
	assert.Equal(t, true, props["ok"].Value.Default)
	assert.Equal(t, 0.5, props["ratio"].Value.Example)
	assert.Equal(t, []any{"a", "b"}, props["names"].Value.Example)
	assert.Equal(t, []any{float64(0), float64(1)}, props["modes"].Value.Example, "the items of a list are checked against their enum")
	assert.NotEmpty(t, props["dropped"].Ref, "a field whose example its type does not allow, and that has nothing else to say, is the reference")
	assert.Empty(t, props["dropped"].Value.AllOf)

	for _, name := range []string{"badCount", "badFlag"} {
		assert.Nil(t, props[name].Value.Example, name)
		assert.Nil(t, props[name].Value.Default, name)
	}
	assert.NotEqual(t, "2024-01-02", props["badWhen"].Value.Example, "the example of the type stays when the one of the field is not a date-time")
	fields := map[string]string{}
	for _, rec := range logs.records {
		if rec.Level == slog.LevelWarn {
			attrs := attrsOf(rec)
			fields[attrs["field"]] = attrs["value"]
		}
	}
	assert.Equal(t, map[string]string{"badCount": "abc", "badWhen": "2024-01-02", "badFlag": "maybe", "dropped": "nonsense"}, fields)
}

// A name that stands for a type of the module is described as the type, whether
// it is declared before the type or after it, and so are pointers to it, to
// pointers, to lists and to lists of it.
func TestAliasesAndPointersToThemAreDescribedAsWhatTheyStandFor(t *testing.T) {
	logs := captureLogs(t)
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/aliases")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "aliases", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	const prefix = "github.com.Danceiny.docgen.internal.engine.testdata.aliases."

	for _, name := range []string{"Early", "Late"} {
		alias := doc.Components.Schemas[prefix+name]
		require.NotNil(t, alias, name)
		assert.NotEqual(t, "default", alias.Value.Pattern, "%s is described, not left as the placeholder of a type that was not parsed yet", name)
		assert.NotEmpty(t, alias.Value.Properties, name)
	}

	user := doc.Components.Schemas[prefix+"User"].Value.Properties
	for _, name := range []string{"a", "b", "c", "d", "h", "i", "k"} {
		assert.Contains(t, user[name].Ref, prefix, "%s refers to a component, not to a bare object", name)
	}
	for _, name := range []string{"f", "g"} {
		assert.True(t, doc.Components.Schemas[prefix+"Name"].Value.Type.Is("string"))
		assert.Contains(t, user[name].Ref, prefix+"Name", name)
	}
	for _, name := range []string{"e", "j", "l"} {
		assert.True(t, user[name].Value.Type.Is("array"), "%s is a list", name)
	}
	for _, rec := range logs.records {
		assert.Less(t, rec.Level, slog.LevelWarn, "%s %v", rec.Message, attrsOf(rec))
	}
}

// A type that is declared as another type of the package is described as that
// type, whatever order a chain of them is declared in, and what has methods is any
// value, not an object with a placeholder.
func TestTypesDeclaredAsOtherTypesAreDescribed(t *testing.T) {
	logs := captureLogs(t)
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/chains")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "chains", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	const prefix = "github.com.Danceiny.docgen.internal.engine.testdata.chains."

	for _, name := range []string{"A1", "A2", "A3", "Money2", "Money3", "Level2", "Names2", "Dict2"} {
		component := doc.Components.Schemas[prefix+name]
		require.NotNil(t, component, name)
		assert.False(t, IsPlaceholder(component.Value), "%s is described, not left as the placeholder of a type that was not parsed yet", name)
	}
	for _, name := range []string{"A1", "A2", "A3", "Money2", "Money3"} {
		assert.Contains(t, doc.Components.Schemas[prefix+name].Value.Properties, "amount", name)
	}
	assert.Equal(t, []any{int64(0), int64(1)}, doc.Components.Schemas[prefix+"Level2"].Value.Enum)
	assert.True(t, doc.Components.Schemas[prefix+"Names2"].Value.Type.Is("array"))
	assert.True(t, doc.Components.Schemas[prefix+"Dict2"].Value.Type.Is("object"))

	holder := doc.Components.Schemas[prefix+"Holder"].Value.Properties
	for _, name := range []string{"a", "m", "l", "n", "d"} {
		// a reference is written as one, whatever the schema behind it is when it is made
		assert.True(t, holder[name].Ref != "" || !IsPlaceholder(holder[name].Value), name)
	}
	for _, name := range []string{"s", "t", "u"} {
		assert.Nil(t, holder[name].Value.Type, "%s has methods, and any value can have them", name)
		assert.False(t, IsPlaceholder(holder[name].Value), name)
	}
	assert.EqualValues(t, 1, holder["l"].Value.Example, "the example is read by the enum")
	for _, rec := range logs.records {
		assert.Less(t, rec.Level, slog.LevelWarn, "%s %v", rec.Message, attrsOf(rec))
	}
}

// type_map gives a type its schema whatever the type is declared as, a struct
// too, and whether it is declared before the types that use it or after them.
func TestTypeMapDescribesAStructAsConfigured(t *testing.T) {
	configured := &openapi3.Schema{Type: &openapi3.Types{"string"}, Description: "configured description"}
	doc := fieldShapes(t, Settings{TypeMap: map[string]*openapi3.Schema{fieldShapesKey + "Mapped": configured}})

	mapped := doc.Components.Schemas[fieldShapesKey+"Mapped"]
	require.NotNil(t, mapped)
	assert.True(t, mapped.Value.Type.Is("string"))
	assert.Equal(t, "configured description", mapped.Value.Description)
	assert.Empty(t, mapped.Value.Properties)
	assert.Nil(t, configured.Properties, "the schema of the configuration is not changed by being used")
	for _, name := range []string{"BeforeMapped", "AfterMapped"} {
		field := doc.Components.Schemas[fieldShapesKey+name].Value.Properties["m"]
		require.NotNil(t, field, name)
		assert.True(t, field.Ref != "" || field.Value.Type.Is("string"), "%s: a reference to it, or what it is", name)
	}
}

// A field of a struct shadows a field of the same name of a struct it embeds,
// whichever is written first, as encoding/json has it.
func TestAFieldShadowsAnEmbeddedOneInEitherOrder(t *testing.T) {
	doc := fieldShapes(t, Settings{VendorExtensions: true})
	for _, name := range []string{"OuterFirst", "OuterLast"} {
		schema := doc.Components.Schemas[fieldShapesKey+name].Value
		assert.True(t, schema.Properties["name"].Value.Type.Is("integer"), "%s: the field of the struct, not the embedded one", name)
		assert.NotContains(t, schema.Required, "name", "%s: the embedded field is shadowed, and it was the required one", name)
		assert.Equal(t, []string{"name", "kept"}, schema.Extensions["x-apifox-orders"], "%s: each name once", name)
	}
}

// A type that the configuration says is a string, a duration written as one, has
// the example and the default of a field as the text they are.
func TestExampleOfATypeThatTheConfigurationMakesAStringIsText(t *testing.T) {
	duration := &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: "duration", Example: "1h30m"}
	withSettings(t, Settings{TypeMap: map[string]*openapi3.Schema{"time.Duration": duration}}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/tagvalues")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "tags", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	written := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.tagvalues.Thing"].Value.Properties["written"].Value
	assert.Equal(t, "30s", written.Example)
	assert.Equal(t, "5m", written.Default)
}

// The line that says which types a field of a type parameter may be is not part of
// the description of the field.
func TestTheCandidatesOfAGenericFieldAreNotItsDescription(t *testing.T) {
	doc := fieldShapes(t, Settings{})
	data := doc.Components.Schemas[fieldShapesKey+"Box"].Value.Properties["data"]
	require.NotNil(t, data)
	assert.Equal(t, "Data is the payload.", data.Value.Description)

	spaced := doc.Components.Schemas[fieldShapesKey+"Box2"].Value.Properties["data"]
	require.NotNil(t, spaced)
	var candidates []string
	for _, alternative := range spaced.Value.OneOf {
		candidates = append(candidates, strings.TrimPrefix(alternative.Ref, "#/components/schemas/"+fieldShapesKey))
	}
	assert.Equal(t, []string{"Base", "Inner", "OuterFirst"}, candidates, "a space after a comma does not lose a candidate")
	assert.Equal(t, "the payload", spaced.Value.Description, "and the text after the semicolon is the description")
}

func TestAnEmbeddedTypeThatIsNotAStructIsAFieldNamedAfterIt(t *testing.T) {
	doc := fieldShapes(t, Settings{})
	assert.Equal(t, []string{"NamesList", "other"}, propertyNames(doc, "EmbedsList"))
	assert.True(t, doc.Components.Schemas[fieldShapesKey+"EmbedsList"].Value.Properties["NamesList"].Value.Type.Is("array"))

	legacy := fieldShapes(t, Settings{CompatLegacyOutput: true})
	assert.Equal(t, []string{"other"}, propertyNames(legacy, "EmbedsList"), "documents generated before leave it out")
}

// A pointer to a type that has no schema is left out with a warning, as the type
// itself is, and not an object that says nothing.
func TestAPointerToATypeWithNoSchemaIsLeftOut(t *testing.T) {
	logs := captureLogs(t)
	doc := fieldShapes(t, Settings{})
	assert.Equal(t, []string{"d"}, propertyNames(doc, "Pointers"))
	warned := false
	for _, rec := range logs.records {
		warned = warned || (rec.Level == slog.LevelWarn && attrsOf(rec)["type"] == "complex64")
	}
	assert.True(t, warned, "it is said that complex64 has no schema")
}

// A field whose type is declared further down the package than the field has the
// example and the default of its tags judged by that type, as one whose type is
// declared above it: what the field refers to is looked up when every type is
// described, and not taken from the placeholder the type was when the field was
// read. The struct that a type embeds, declared further down still, has been
// passed on by then.
func TestExamplesOfFieldsWhoseTypesAreDeclaredLaterAreJudgedByThoseTypes(t *testing.T) {
	logs := captureLogs(t)
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/laterdecl")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "later", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	props := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.laterdecl.Order"].Value.Properties
	assert.Equal(t, "active", props["status"].Value.Example)
	assert.Equal(t, "new", props["status"].Value.Default)
	assert.EqualValues(t, int64(2), props["level"].Value.Example, "a number of an enum of numbers")
	assert.True(t, props["level"].Value.Type.Is("integer"), "and the nullable reference has the type of the enum")
	assert.Equal(t, map[string]any{"city": "Dubai"}, props["addr"].Value.Example)
	assert.Equal(t, []any{"active"}, props["tags"].Value.Example, "the items of a list are judged by their type")
	assert.Equal(t, map[string]any{"a": "new"}, props["byKey"].Value.Example, "so are the values of a map")
	assert.Equal(t, "active", props["derived"].Value.Example, "a type declared as another type has what that one allows")
	assert.Equal(t, map[string]any{"id": "x"}, props["child"].Value.Example)

	for _, name := range []string{"bad", "badAddr", "noCity", "badChild"} {
		assert.NotEmpty(t, props[name].Ref, "%s says nothing of itself but the type it has, so it is the reference", name)
		assert.Nil(t, props[name].Value.Example, name)
	}
	assert.Nil(t, props["badTags"].Value.Example)
	rejected := map[string]string{}
	for _, rec := range logs.records {
		if rec.Level == slog.LevelWarn {
			attrs := attrsOf(rec)
			rejected[attrs["field"]] = attrs["value"]
		}
	}
	assert.Equal(t, map[string]string{
		"bad":      "bogus",
		"badAddr":  `{"city":1}`,
		"noCity":   "{}",
		"badTags":  `["bogus"]`,
		"badChild": `{"name":"n"}`,
		"status":   "bogus",
	}, rejected, "and each is said, with the field")

	// A field that says nothing of itself once its example is left out is the plain
	// reference in the struct that has it and in the struct that embeds it.
	for _, name := range []string{"Kid", "Parent"} {
		status := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.laterdecl."+name].Value.Properties["status"]
		require.NotNil(t, status, name)
		assert.NotEmpty(t, status.Ref, "%s: the reference itself", name)
		assert.Empty(t, status.Value.AllOf, "%s: not a wrapper of it that says nothing", name)
	}
}

// A union, an interface that says @autowire: true, is made of the types that
// implement it, whether a field that uses it comes before or after its
// declaration, and however many do: a use that comes after the declaration must
// not leave the union with nothing to add the alternatives to.
func TestAUnionHasItsAlternativesWhateverTheOrderOfItsUses(t *testing.T) {
	logs := captureLogs(t)
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/autowire")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "union", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	const prefix = "github.com.Danceiny.docgen.internal.engine.testdata.autowire."

	for name, want := range map[string][]string{
		"Early": {prefix + "Alpha", prefix + "Beta"},
		"Late":  {prefix + "Delta", prefix + "Gamma"},
	} {
		union := doc.Components.Schemas[prefix+name]
		require.NotNil(t, union, name)
		var got []string
		for _, alternative := range union.Value.OneOf {
			got = append(got, strings.TrimPrefix(alternative.Ref, "#/components/schemas/"))
		}
		sort.Strings(got)
		assert.Equal(t, want, got, name)
		assert.Nil(t, union.Value.Type, "%s is the union, not an object", name)
		assert.Empty(t, union.Value.Pattern, "%s is not the placeholder of a type docgen could not fill in", name)
		assert.Equal(t, name+" is a union that is declared "+map[string]string{"Early": "before", "Late": "after"}[name]+" the struct that uses it.", union.Value.Description,
			"the line that makes it a union is not part of what it says")
	}

	lonely := doc.Components.Schemas[prefix+"Lonely"]
	require.NotNil(t, lonely)
	assert.True(t, IsPlaceholder(lonely.Value), "a union that nothing implements is the placeholder it always was")
	for _, rec := range logs.records {
		assert.NotContains(t, rec.Message, "no schema", "the implementations of a union have schemas")
	}

	legacy := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "union", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	withSettings(t, Settings{CompatLegacyOutput: true}, "github.com/Danceiny/docgen")
	defers, mergeTasks = processModelsFor(pkg, legacy, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, legacy)
	union := legacy.Components.Schemas[prefix+"Late"]
	require.NotNil(t, union)
	assert.Len(t, union.Value.OneOf, 2)
	assert.Equal(t, "default", union.Value.Pattern, "documents that keep the old way of writing have the marker next to the alternatives")
	assert.Contains(t, union.Value.Description, "@autowire: true", "and the line")
}
