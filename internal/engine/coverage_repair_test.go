package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

func TestGenerationSessionLifecycleAndIndex(t *testing.T) {
	leaf := &packages.Package{ID: "leaf", PkgPath: "example/leaf"}
	root := &packages.Package{ID: "root", PkgPath: "example/root", Imports: map[string]*packages.Package{"example/leaf": leaf}}
	s := &GenerationSession{moduleDir: "/tmp", imports: map[string]*packages.Package{}, resolving: map[string]bool{}}
	s.index(root, map[string]bool{})
	if s.ImportedPackage("example/root") != root || s.ImportedPackage("example/leaf") != leaf {
		t.Fatal("package index missing entries")
	}
	if !s.EnterResolving("x") || s.EnterResolving("x") {
		t.Fatal("resolving guard failed")
	}
	s.LeaveResolving("x")
	if !s.EnterResolving("x") {
		t.Fatal("resolving key was not released")
	}
	if s.ModuleDir() != "/tmp" || len(s.Packages()) != 0 {
		t.Fatal("session accessors wrong")
	}
	s.Close()
	if s.packages != nil || s.imports != nil {
		t.Fatal("Close must release package references")
	}
}

func TestNewGenerationSessionLoadsModule(t *testing.T) {
	s, err := NewGenerationSession("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Packages()) == 0 || s.ModuleDir() == "" {
		t.Fatal("module session has no packages")
	}
	s.Close()
}

func TestRuntimeRequestAndMethodParsingBranches(t *testing.T) {
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{"model.Request": {Value: openapi3.NewObjectSchema()}}}}
	known := &TypeDescriptor{FullKey: "model.Request"}
	if getContractSchemaRef("/api/x", known, doc).Value == nil {
		t.Fatal("known contract schema missing")
	}
	withSettings(t, Settings{RuntimeOnly: map[RuntimeInputKey]struct{}{
		{Path: "/api/cron/setStore", TypeKey: "example.cron.JobStore"}: {},
	}}, "example.com/app")
	runtime := getContractSchemaRef("/api/cron/setStore", &TypeDescriptor{FullKey: "example.cron.JobStore"}, doc)
	if runtime.Value == nil || runtime.Value.Extensions["x-runtime-unserializable"] != true {
		t.Fatal("runtime-only contract was not marked")
	}
	if getContractSchemaRef("/api/x", nil, doc).Value == nil {
		t.Fatal("nil descriptor should retain invalid ref")
	}
	m := &Method{CurrentPkgPath: "example.app", ImportAlias: map[string]string{"model": "example.app.model"}}
	if got := m.parseBaseType("model.Request"); got != "example.app.model.Request" {
		t.Fatalf("alias type=%s", got)
	}
	if got := m.parseBaseType("Request"); got != "example.app.Request" {
		t.Fatalf("local type=%s", got)
	}
	if got := m.parseBaseType("[]*string"); got != "string" {
		t.Fatalf("a basic type has no package: %s", got)
	}
	if p, d := parseTypeModifiers("**[][]int"); p != 2 || d != 2 {
		t.Fatalf("modifiers=%d,%d", p, d)
	}
	body := ParamSpec{Name: "request", In: "body", Types: []*TypeDescriptor{known}, Required: true}
	if buildRequestBody(&Method{APIPath: "/api/x", Params: []ParamSpec{body}}, doc) == nil {
		t.Fatal("request body missing")
	}
}

func TestTypeParserSyntheticCollections(t *testing.T) {
	p := NewTypeParser(&packages.Package{ID: "fixture", PkgPath: "fixture", TypesInfo: &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}}, &openapi3.T{Info: &openapi3.Info{Title: "fixture"}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}})
	mapRef := p.parseMap(&ast.MapType{Value: ast.NewIdent("string")}, &ParseContext{})
	if mapRef.Value == nil || mapRef.Value.AdditionalProperties.Schema == nil {
		t.Fatal("map value schema missing")
	}
	arrayRef := p.parseArray(&ast.ArrayType{Elt: ast.NewIdent("Unknown")}, &ParseContext{})
	if arrayRef.Value == nil || arrayRef.Value.Items == nil {
		t.Fatal("array item schema missing")
	}
	iface := p.parseInterface(&ast.InterfaceType{Methods: &ast.FieldList{}}, &ParseContext{})
	if iface.Value == nil || iface.Value.Type != nil || len(iface.Value.AnyOf) != 0 {
		t.Fatalf("the empty interface is the schema that has no type: %+v", iface.Value)
	}
	withSettings(t, Settings{CompatLegacySchemaShapes: true}, "")
	legacy := p.parseInterface(&ast.InterfaceType{Methods: &ast.FieldList{}}, &ParseContext{})
	if legacy.Value == nil || len(legacy.Value.AnyOf) != 3 {
		t.Fatal("a configuration that keeps legacy_schema_shapes gets the union of a string, an integer and an object")
	}
}

func TestFindImplementationsSealedMarker(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture
type Contract interface { sealed() }
type One struct{}
type Two struct{}
func (*One) sealed() {}
func (Two) sealed() {}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	iface := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.InterfaceType)
	got := findImplementations(&packages.Package{Syntax: []*ast.File{file}}, iface)
	seen := map[string]bool{}
	for _, name := range got {
		seen[name] = true
	}
	if len(got) != 2 || !seen["One"] || !seen["Two"] {
		t.Fatalf("implementations=%v", got)
	}
}
