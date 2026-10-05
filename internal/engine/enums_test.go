package engine

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enumValues(entries []EnumEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name+"="+e.Value)
	}
	return out
}

// The values of an enum are what the compiler says they are, however they are
// declared, and all the constants of the type are there, the ones that repeat
// the type implicitly too.
func TestEnumValuesAreWhatTheCompilerComputes(t *testing.T) {
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/enums")

	assert.Equal(t, []string{"Low=0", "Medium=1", "High=2"}, enumValues(collectEnumEntries(pkg, "Priority")))
	assert.Equal(t, []string{"Unknown=-1", "Base=10", "Next=15"}, enumValues(collectEnumEntries(pkg, "Level")))
	assert.Equal(t, []string{"Read=1", "Write=2", "Exec=4"}, enumValues(collectEnumEntries(pkg, "Mask")))
	assert.Equal(t, []string{"NameA=x-a", "NameB=b"}, enumValues(collectEnumEntries(pkg, "Name")), "a blank constant is no value")
	assert.Empty(t, collectEnumEntries(pkg, "Plain"))

	priority := collectEnumEntries(pkg, "Priority")
	require.Len(t, priority, 3)
	assert.Equal(t, "is the first and has the iota.", priority[0].Comment)
	assert.Equal(t, "is the third.", priority[2].Comment)

	// A name that is an alias is a name for string: the constants that are declared
	// with it are its values, not every string constant of the package.
	assert.Equal(t, []string{"AliasA=a"}, enumValues(collectEnumEntries(pkg, "Alias")))
}

// legacy_schema_shapes keeps what documents generated before the fix have: the
// constants declared with the type and a literal, and an empty value for the rest.
func TestEnumValuesOfLegacyDocuments(t *testing.T) {
	withSettings(t, Settings{CompatLegacySchemaShapes: true}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/enums")

	assert.Equal(t, []string{"Low="}, enumValues(collectEnumEntries(pkg, "Priority")), "only the first, and its iota is no literal")
	assert.Equal(t, []string{"Unknown=", "Base=10", "Next="}, enumValues(collectEnumEntries(pkg, "Level")))
	assert.Equal(t, []string{"NameA=", "NameB=b", "_=skipped"}, enumValues(collectEnumEntries(pkg, "Name")))
}

// A configuration that gives a type its schema has given it, an enum too: a type
// whose JSON is its name and not its number says so in type_map. A configuration
// that keeps legacy_schema_shapes has always had the enum win.
func TestTypeMapGivesAnEnumItsSchema(t *testing.T) {
	const key = "github.com.Danceiny.docgen.internal.engine.testdata.enums.Priority"
	byName := map[string]*openapi3.Schema{key: {Type: &openapi3.Types{"string"}, Description: "the name of the priority"}}

	for _, tc := range []struct {
		name   string
		legacy bool
		want   string
	}{
		{"default", false, "string"},
		{"legacy", true, "integer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, Settings{TypeMap: byName, CompatLegacySchemaShapes: tc.legacy}, "github.com/Danceiny/docgen")
			pkg := loadFixture(t, "testdata/enums")
			doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "enums", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
			defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
			runProcessModelsTasks(t, defers, mergeTasks, doc)

			priority := doc.Components.Schemas[key]
			require.NotNil(t, priority)
			assert.True(t, priority.Value.Type.Is(tc.want), "Priority is %v", priority.Value.Type)
			if tc.want == "string" {
				assert.Empty(t, priority.Value.Enum, "the schema of type_map has no values it was not given")
			}
		})
	}
}

func enumSchema(t *testing.T, name string) *openapi3.Schema {
	t.Helper()
	withSettings(t, Settings{}, "github.com/Danceiny/docgen")
	pkg := loadFixture(t, "testdata/enums")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "enums", Version: "1"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	ref := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.enums."+name]
	require.NotNil(t, ref, name)
	return ref.Value
}

// An enum is made of what the compiler says it is made of: a byte, a rune, a
// pointer-sized integer, a duration and another enum of the module all give the
// numbers or the strings they are, and the values of an enum are different.
func TestEnumsOfOrdinaryDeclarations(t *testing.T) {
	for name, want := range map[string]struct {
		typ    string
		values []any
	}{
		"Raw":     {"integer", []any{uint64(97), uint64(2)}},
		"Letter":  {"integer", []any{int64(122)}},
		"Up":      {"integer", []any{uint64(1)}},
		"Dur":     {"integer", []any{int64(1000000000), int64(3600000000000)}},
		"Wrapped": {"string", []any{"w1"}},
		"Dup":     {"integer", []any{int64(0), int64(1)}},
	} {
		t.Run(name, func(t *testing.T) {
			schema := enumSchema(t, name)
			assert.True(t, schema.Type.Is(want.typ), "%s is %v", name, schema.Type)
			assert.Equal(t, want.values, schema.Enum)
		})
	}
}
