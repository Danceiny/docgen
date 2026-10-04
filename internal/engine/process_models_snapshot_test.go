package engine

import (
	"bytes"
	"container/list"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// TestProcessModels_SchemaSnapshot locks the output of ProcessModels:
// it must match the committed golden for a fixture package.
// Drift fails closed; refresh with UPDATE_SCHEMA_SNAPSHOT=1.
//
// Negative control (AC-2): changing SnapshotOrder fields without refreshing
// the golden must fail this test — verified by the golden containing the
// fixture field names below; remove a property from the golden manually and
// re-run to observe fail-closed behavior.
func TestProcessModels_SchemaSnapshot(t *testing.T) {
	withSettings(t, Settings{VendorExtensions: true}, ModuleName)
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixtureDir := filepath.Join(filepath.Dir(thisFile), "testdata", "snapshotmodels")
	goldenPath := filepath.Join(fixtureDir, "schemas.golden.json")

	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:   "snapshot",
			Version: "snapshot",
		},
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{},
		},
	}

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedModule |
			packages.NeedFiles,
		Dir: fixtureDir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		t.Fatalf("package errors: %v", pkg.Errors)
	}
	if pkg.Name != "snapshotmodels" {
		t.Fatalf("unexpected package name %q", pkg.Name)
	}

	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	got, err := serializeSchemaSnapshot(doc)
	if err != nil {
		t.Fatalf("serializeSchemaSnapshot: %v", err)
	}

	if os.Getenv("UPDATE_SCHEMA_SNAPSHOT") == "1" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden at %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (set UPDATE_SCHEMA_SNAPSHOT=1 to create)", goldenPath, err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("ProcessModels schema snapshot drifted.\n"+
			"Refresh intentionally with: UPDATE_SCHEMA_SNAPSHOT=1 go test ./build/api/internal/engine -run TestProcessModels_SchemaSnapshot\n"+
			"--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// Lock fixture surface area so an empty/truncated golden cannot silently pass.
	gotStr := string(got)
	for _, needle := range []string{
		"SnapshotOrder",
		"SnapshotMeta",
		"SnapshotStatus",
		`"id"`,
		`"status"`,
		`"amount"`,
		`"tags"`,
		`"meta"`,
		`"source"`,
	} {
		if !bytes.Contains(got, []byte(needle)) {
			t.Fatalf("snapshot missing expected fixture surface %q; got:\n%s", needle, gotStr)
		}
	}
}

// The x- extensions are for the tools that read them; a document written without
// asking for them has none, whatever the types are.
func TestProcessModels_AddsNoVendorExtensionsByDefault(t *testing.T) {
	pkg := loadFixture(t, "testdata/snapshotmodels")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "plain"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	got, err := serializeSchemaSnapshot(doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(`"x-`)) {
		t.Fatalf("a vendor extension was written without VendorExtensions:\n%s", got)
	}
	// What the extensions repeated is still there: the enum values and their names
	// are in the schema and its description.
	for _, needle := range []string{"SnapshotStatusPending", `"enum"`, `"pending"`} {
		if !bytes.Contains(got, []byte(needle)) {
			t.Errorf("snapshot without extensions lost %q", needle)
		}
	}
}

func TestProcessModels_SchemaSnapshot_DriftFailsClosed(t *testing.T) {
	// AC-2: serializeSchemaSnapshot must not treat unequal docs as equal.
	docA := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"A": {Value: &openapi3.Schema{Title: "A", Description: "one"}},
	}}}
	docB := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"A": {Value: &openapi3.Schema{Title: "A", Description: "two"}},
	}}}
	a, err := serializeSchemaSnapshot(docA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := serializeSchemaSnapshot(docB)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("serializeSchemaSnapshot must fail-closed on description drift")
	}
}

// A type that contains itself must stay a reference to its own component: the
// generator must not expand it forever, nor change the model it describes.
func TestProcessModels_RecursiveTypeChildrenUsesRecursiveRef(t *testing.T) {
	pkg := loadFixture(t, "testdata/recursivemodels")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "recursive"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	node := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.recursivemodels.Node"]
	require.NotNil(t, node)
	require.NotNil(t, node.Value)
	children := node.Value.Properties["children"]
	require.NotNil(t, children)
	require.NotNil(t, children.Value)
	require.NotNil(t, children.Value.Items)
	assert.Equal(t, "#/components/schemas/github.com.Danceiny.docgen.internal.engine.testdata.recursivemodels.Node", children.Value.Items.Ref)
}

// loadFixture loads the package of a directory under internal/engine.
func loadFixture(t *testing.T, dir string) *packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedImports | packages.NeedDeps | packages.NeedModule | packages.NeedFiles,
		Dir: dir,
	}
	pkgs, err := packages.Load(cfg, ".")
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	require.Empty(t, pkgs[0].Errors)
	return pkgs[0]
}

func TestProcessModels_PackageQualifiedAliasUsesImportedType(t *testing.T) {
	pkg := loadFixture(t, "testdata/aliasmodels")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "alias"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	external := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.aliasexternal.Collision"]
	require.NotNil(t, external)
	require.NotNil(t, external.Value)
	value := external.Value.Properties["value"]
	require.NotNil(t, value)
	require.NotNil(t, value.Value)
	assert.True(t, value.Value.Type.Is("string"))
}

func runProcessModelsTasks(t *testing.T, defers, mergeTasks *list.List, doc *openapi3.T) {
	t.Helper()
	for round := 0; round < 20 && defers.Len() > 0; round++ {
		current := list.New()
		current.PushBackList(defers)
		defers.Init()
		for v := current.Front(); v != nil; v = v.Next() {
			v.Value.(func())()
		}
	}
	if defers.Len() > 0 {
		t.Fatalf("deferred ProcessModels tasks did not drain (%d left)", defers.Len())
	}

	for i := 0; i < 10; i++ {
		changed := false
		for v := mergeTasks.Front(); v != nil; v = v.Next() {
			task := v.Value.(MergeTask)
			if mergeSnapshotProperties(doc, task.TargetKey, task.SourceKey) {
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func mergeSnapshotProperties(doc *openapi3.T, targetKey, sourceKey string) bool {
	targetRef, ok := doc.Components.Schemas[targetKey]
	if !ok || targetRef == nil || targetRef.Value == nil {
		return false
	}
	sourceRef, ok := doc.Components.Schemas[sourceKey]
	if !ok || sourceRef == nil || sourceRef.Value == nil {
		return false
	}
	changed := false
	if targetRef.Value.Properties == nil {
		targetRef.Value.Properties = openapi3.Schemas{}
	}
	for k, v := range sourceRef.Value.Properties {
		if _, exists := targetRef.Value.Properties[k]; !exists {
			targetRef.Value.Properties[k] = v
			changed = true
		}
	}
	return changed
}

// serializeSchemaSnapshot produces a deterministic JSON ledger of
// Components.Schemas for golden comparison (sorted keys, indented).
func serializeSchemaSnapshot(doc *openapi3.T) ([]byte, error) {
	if doc == nil || doc.Components == nil {
		return []byte("{}\n"), nil
	}
	keys := make([]string, 0, len(doc.Components.Schemas))
	for k := range doc.Components.Schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]any, len(keys))
	for _, k := range keys {
		ref := doc.Components.Schemas[k]
		raw, err := json.Marshal(ref)
		if err != nil {
			return nil, err
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, err
		}
		out[k] = decoded
	}
	return json.MarshalIndent(out, "", "  ")
}

// processModelsFor parses a package for one audience without a session.
func processModelsFor(pkg *packages.Package, doc *openapi3.T, audience Audience) (*list.List, *list.List) {
	parser := NewTypeParser(pkg, doc)
	parser.audience = audience
	parser.ParseAllDecls()
	return parser.defers, parser.mergeTasks
}

// A field whose type is a named function or channel type has no property, as a
// field declared with the function type itself has none, instead of a reference
// to a component that does not exist.
func TestProcessModels_FieldsOfFunctionAndChannelTypesAreLeftOut(t *testing.T) {
	logs := captureLogs(t)
	pkg := loadFixture(t, "testdata/functypes")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "functypes"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	options := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.functypes.Options"]
	require.NotNil(t, options)
	var names []string
	for name := range options.Value.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"name", "retries"}, names)

	for key := range doc.Components.Schemas {
		assert.NotContains(t, key, "Handler")
		assert.NotContains(t, key, "Callback")
		assert.NotContains(t, key, "Events")
	}

	// Leaving them out is the rule for these types, not a problem to report.
	for _, rec := range logs.records {
		assert.Less(t, rec.Level, slog.LevelWarn, "logged %q %v", rec.Message, attrsOf(rec))
	}
}

// Every basic Go type has a property, except the complex numbers, which JSON has
// no number for: a field that disappears from the document without a word is
// the worst way for a type to be unsupported.
func TestProcessModels_EveryBasicTypeHasAPropertyOrAWarning(t *testing.T) {
	logs := captureLogs(t)
	pkg := loadFixture(t, "testdata/basictypes")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "basictypes"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	basics := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.basictypes.Basics"]
	require.NotNil(t, basics)

	type shape struct{ typ, format string }
	want := map[string]shape{
		"bool":    {"boolean", ""},
		"string":  {"string", ""},
		"int":     {"integer", ""},
		"int8":    {"integer", ""},
		"int16":   {"integer", ""},
		"int32":   {"integer", "int32"},
		"int64":   {"integer", "int64"},
		"uint":    {"integer", "int64"},
		"uint8":   {"integer", ""},
		"uint16":  {"integer", "int32"},
		"uint32":  {"integer", "int64"},
		"uint64":  {"integer", "int64"},
		"uintptr": {"integer", "int64"},
		"rune":    {"integer", "int32"},
		"float32": {"number", "float"},
		"float64": {"number", ""},
		"timeout": {"integer", "int64"}, // the JSON of a Duration is its nanoseconds
		"created": {"string", "date-time"},
	}
	for name, w := range want {
		prop := basics.Value.Properties[name]
		if prop == nil || prop.Value == nil {
			t.Errorf("the property %s is missing", name)
			continue
		}
		if got := (shape{prop.Value.Type.Slice()[0], prop.Value.Format}); got != w {
			t.Errorf("%s is %+v, want %+v", name, got, w)
		}
		if len(prop.Value.Enum) != 0 {
			t.Errorf("%s has the enum %v: a basic type is not an enum", name, prop.Value.Enum)
		}
	}
	assert.Len(t, basics.Value.Properties, len(want), "only the complex numbers are left out")

	// A Duration is the number JSON writes, and no enum: the constants of the time
	// package that make it look like one are not its values. Its default and its
	// example are written as durations in the tags and are that number here.
	timeout := basics.Value.Properties["timeout"].Value
	assert.Empty(t, timeout.Enum)
	assert.EqualValues(t, int64(5*time.Second), timeout.Default)
	assert.EqualValues(t, int64(10*time.Minute), timeout.Example)

	warned := map[string]bool{}
	for _, rec := range logs.records {
		if rec.Message == "Go type has no schema and is left out" {
			warned[attrsOf(rec)["type"]] = true
		}
	}
	assert.Equal(t, map[string]bool{"complex64": true, "complex128": true}, warned)
}

// A type of another module is written out where it is used, so a type that
// contains itself, or two that contain each other, cannot be: reached again from
// inside itself it is an object. Without that the generator ran until the stack
// of the process overflowed. A type that has nothing to read, such as
// unsafe.Pointer, has no property, and says so.
func TestProcessModels_TypesOfOtherModulesThatContainThemselves(t *testing.T) {
	logs := captureLogs(t)
	pkg := loadFixture(t, "testdata/externalmodels")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "externalmodels"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	holder := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.externalmodels.Holder"]
	require.NotNil(t, holder)
	props := holder.Value.Properties

	require.NotNil(t, props["scope"], "the scope is written out where it is used")
	outer := props["scope"].Value.Properties["Outer"]
	require.NotNil(t, outer, "a scope has an outer scope")
	assert.Empty(t, outer.Ref, "the second scope is written out, not referred to: no component describes it")
	assert.True(t, outer.Value.Type.Is("object"))
	assert.Empty(t, outer.Value.Properties, "the second scope is an object and nothing more")

	require.NotNil(t, props["request"])
	assert.True(t, props["request"].Value.Type.Is("object"))
	assert.NotEmpty(t, props["request"].Value.Properties)

	assert.NotContains(t, props, "raw")
	var warned []string
	for _, rec := range logs.records {
		if rec.Message == "Go type has no schema and is left out" {
			warned = append(warned, attrsOf(rec)["type"])
		}
	}
	assert.Equal(t, []string{"unsafe.Pointer"}, warned)

	for key, schema := range doc.Components.Schemas {
		assert.False(t, strings.HasPrefix(key, "go.ast.") || strings.HasPrefix(key, "net.http."), "%s: a type of another module has no component", key)
		assert.NotEmpty(t, schema.Value.Type)
	}
}

// A generic declaration that nothing instantiates is documented with what it
// does say: the fields of a type parameter are not described, and that is the
// way generic types are, not a problem to report.
func TestProcessModels_GenericDeclarationsAreNotReportedAsProblems(t *testing.T) {
	logs := captureLogs(t)
	pkg := loadFixture(t, "testdata/generics")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "generics"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	page := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.generics.Page"]
	require.NotNil(t, page)
	assert.Contains(t, page.Value.Properties, "total")
	assert.Contains(t, page.Value.Properties, "items")

	pair := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.generics.Pair"]
	require.NotNil(t, pair)
	assert.Contains(t, pair.Value.Properties, "count")

	for _, rec := range logs.records {
		assert.Less(t, rec.Level, slog.LevelWarn, "logged %q %v", rec.Message, attrsOf(rec))
	}
}

// A type named T is read as a type parameter, as the name T has always been in
// documents of generic types: a field of it has no property. That is a limit, and
// it is not reported as a problem, since T is by far the most common name of a
// type parameter.
func TestProcessModels_ATypeNamedTIsReadAsATypeParameter(t *testing.T) {
	logs := captureLogs(t)
	pkg := loadFixture(t, "testdata/typenamedt")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "typenamedt"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	holder := doc.Components.Schemas["github.com.Danceiny.docgen.internal.engine.testdata.typenamedt.Holder"]
	require.NotNil(t, holder)
	assert.Contains(t, holder.Value.Properties, "count")
	assert.NotContains(t, holder.Value.Properties, "item")

	for _, rec := range logs.records {
		assert.Less(t, rec.Level, slog.LevelWarn, "logged %q %v", rec.Message, attrsOf(rec))
	}
}

// A struct that embeds itself, or two that embed each other, is a legal Go type
// that encoding/json marshals; collecting the names of its fields must stop at
// the second visit, not overflow the stack. A json option called "default" with
// no value is not a default and not a crash.
func TestProcessModels_EmbeddedCyclesAndMalformedJSONDefaults(t *testing.T) {
	pkg := loadFixture(t, "testdata/embeddedcycles")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "embeddedcycles"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)

	key := func(name string) string {
		return "github.com.Danceiny.docgen.internal.engine.testdata.embeddedcycles." + name
	}
	names := func(name string) []string {
		schema := doc.Components.Schemas[key(name)]
		require.NotNil(t, schema, name)
		var out []string
		for prop := range schema.Value.Properties {
			out = append(out, prop)
		}
		sort.Strings(out)
		return out
	}
	assert.Equal(t, []string{"name"}, names("T"))
	assert.Contains(t, names("A"), "x")
	assert.Contains(t, names("B"), "y")

	defaults := doc.Components.Schemas[key("Defaults")]
	require.NotNil(t, defaults)
	assert.Nil(t, defaults.Value.Properties["plain"].Value.Default)
	assert.Equal(t, "x", defaults.Value.Properties["valued"].Value.Default)
}

// The name of a service is the string its Name method returns, wherever the
// constant it returns is declared; one that cannot be read, a variable or a
// function's result, is not a service, and the log says so rather than leaving
// the struct out in silence.
func TestServiceNamesAreConstantsWhereverTheyAreDeclared(t *testing.T) {
	logs := captureLogs(t)
	pkg := loadFixture(t, "testdata/servicenames")

	var names []string
	for _, service := range FindServiceImplementations(pkg) {
		names = append(names, service.ServiceName)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"literal", "order", "samefile", "stock", "store/joined"}, names)

	var unresolved int
	for _, rec := range logs.records {
		if rec.Level == slog.LevelWarn && rec.Message == "the Name method does not return a string literal or a constant, so the struct is not a service" {
			unresolved++
		}
	}
	assert.Equal(t, 2, unresolved, "the variable and the computed name are reported")
}

// A mistake that makes an annotation or a directive do nothing is said, with the
// right spelling when there is one; an annotation that is nothing like a known
// one belongs to another reader of the comment and is left alone.
func TestMistypedAnnotationsAndDirectivesAreReported(t *testing.T) {
	logs := captureLogs(t)
	pkg := loadFixture(t, "testdata/mistakes")
	doc := &openapi3.T{OpenAPI: "3.0.3", Info: &openapi3.Info{Title: "snapshot", Version: "mistakes"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	defers, mergeTasks := processModelsFor(pkg, doc, testInternal)
	runProcessModelsTasks(t, defers, mergeTasks, doc)
	FindServiceImplementations(pkg)

	guesses := map[string]string{} // annotation -> what it should be
	types := map[string]string{}   // type -> the mistake in its directive
	for _, rec := range logs.records {
		if rec.Level != slog.LevelWarn {
			continue
		}
		attrs := attrsOf(rec)
		switch {
		case attrs["annotation"] != "":
			guesses[attrs["annotation"]] = attrs["didYouMean"]
		case attrs["type"] != "":
			types[attrs["type"]] = rec.Message
		}
	}
	assert.Equal(t, map[string]string{"@respone": "@response", "@Tags": "@tags"}, guesses,
		"@auth is nothing like a known annotation and belongs to another reader of the comment")
	assert.Contains(t, types["Spaced"], "no space after the slashes")
	assert.Contains(t, types["Bogus"], "scope that is not public, internal or hidden")
	assert.NotContains(t, types, "Fine")
}
