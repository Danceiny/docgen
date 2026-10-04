package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
)

// generateFixture generates the documents of a fixture module and loads the one
// with the given name back, which also validates it as an OpenAPI document.
func generateFixture(t *testing.T, dir, name string) *openapi3.T {
	t.Helper()
	cfg, err := config.Load(filepath.Join(dir, "docgen.yaml"))
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
