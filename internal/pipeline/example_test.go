package pipeline

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
)

// examples/petstore is a module of its own whose documents are checked in. These
// tests run the whole tool on it: loading a module, building the schemas and
// operations, filtering them per audience and writing the files.

const petstoreDir = "../../examples/petstore"

func petstoreConfig(t *testing.T, dir string) *config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(dir, "docgen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// records keeps what a run logged, so that a test can say it logged nothing
// worse than information.
type records struct {
	mu   sync.Mutex
	list []slog.Record
}

func (r *records) Enabled(context.Context, slog.Level) bool { return true }
func (r *records) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.list = append(r.list, rec)
	return nil
}
func (r *records) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *records) WithGroup(string) slog.Handler      { return r }

func TestPetstoreDocumentsAreCurrent(t *testing.T) {
	log := &records{}
	drifts, err := Check(Options{Dir: petstoreDir, Config: petstoreConfig(t, petstoreDir), Logger: slog.New(log)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drifts {
		t.Errorf("%s (%s) %s; regenerate it with: go run ./cmd/docgen -C examples/petstore", d.Output, d.Document, d.Reason)
	}
	for _, rec := range log.list {
		if rec.Level >= slog.LevelWarn {
			t.Errorf("generating the example logged %s: %s", rec.Level, rec.Message)
		}
	}
}

func loadExampleDoc(t *testing.T, name string) *openapi3.T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(petstoreDir, "docs/api", name))
	if err != nil {
		t.Fatal(err)
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(data)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("%s is not a valid OpenAPI document: %v", name, err)
	}
	return doc
}

// The checked-in documents are what the example is for; these tests say what it
// is supposed to show, so that a regenerated document that lost it fails even
// though regenerating made it "current".
func TestPetstoreInternalDocumentShowsEverything(t *testing.T) {
	doc := loadExampleDoc(t, "internal.yaml")

	for _, path := range []string{
		"/api/pet/get", "/api/pet/list", "/api/pet/search", "/api/pet/create", "/api/pet/uploadPhoto",
		"/api/pet/downloadPhoto", "/api/pet/audit", "/api/pet/import", "/api/store/order/place",
	} {
		if doc.Paths.Value(path) == nil {
			t.Errorf("missing operation %s", path)
		}
	}
	if doc.Paths.Value("/api/pet/setStore") != nil {
		t.Error("a method marked @apidoc: - is not an operation")
	}
	if got := doc.Paths.Len(); got != 9 {
		t.Errorf("paths = %d, want 9", got)
	}

	pet := doc.Components.Schemas["example.com.petstore.pet.domain.Pet"].Value
	if pet.Properties["notes"] == nil {
		t.Error(`a field tagged apidoc:"internal" is shown in the internal document`)
	}
	if pet.Properties["vetCost"] != nil {
		t.Error(`a field tagged apidoc:"hidden" is shown nowhere`)
	}
	if doc.Components.Schemas["example.com.petstore.pet.domain.InternalAuditEntry"] == nil {
		t.Error("a type named Internal* is shown in the internal document")
	}
	if doc.Components.Schemas["example.com.petstore.errors.NotFoundErr"] == nil {
		t.Error("the errors of errors.json are components")
	}
	if doc.Components.Schemas["example.com.petstore.pet.service.ImportReq"] == nil {
		t.Error("overlay.yaml adds ImportReq")
	}
}

func TestPetstorePublicDocumentShowsWhatCustomersMaySee(t *testing.T) {
	doc := loadExampleDoc(t, "public.yaml")

	var paths []string
	for p := range doc.Paths.Map() {
		paths = append(paths, p)
	}
	for _, want := range []string{"/api/pet/get", "/api/pet/list", "/api/pet/search", "/api/pet/downloadPhoto", "/api/store/order/place"} {
		if doc.Paths.Value(want) == nil {
			t.Errorf("the public document lacks %s; it has %v", want, paths)
		}
	}
	for _, staff := range []string{"/api/pet/create", "/api/pet/uploadPhoto", "/api/pet/audit", "/api/pet/import"} {
		if doc.Paths.Value(staff) != nil {
			t.Errorf("%s has no public tag and must not be in the public document", staff)
		}
	}

	pet := doc.Components.Schemas["example.com.petstore.pet.domain.Pet"].Value
	if pet.Properties["notes"] != nil || pet.Properties["vetCost"] != nil {
		t.Errorf("staff-only fields are in the public document: %v", pet.Properties)
	}
	for name := range doc.Components.Schemas {
		if strings.Contains(name, "InternalAuditEntry") || strings.Contains(name, ".errors.") || strings.HasSuffix(name, "ImportReq") {
			t.Errorf("component %s should not be in the public document", name)
		}
	}
	if pet.Title != "Pet" {
		t.Errorf("a public schema's title is the last segment of its name, got %q", pet.Title)
	}
}

func TestPetstoreHidesTheRetiredEnumValues(t *testing.T) {
	for _, name := range []string{"internal.yaml", "public.yaml"} {
		raw, err := os.ReadFile(filepath.Join(petstoreDir, "docs/api", name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "quarantine") || strings.Contains(string(raw), "StatusLegacy") {
			t.Errorf("%s documents a value the //apidoc: directive leaves out", name)
		}
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Check is the gate of a repository that checks its documents in: it must say
// which document is stale and why, and it must not write anything.
func TestCheckSaysWhichDocumentsAreStale(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, petstoreDir, dir)
	cfg := petstoreConfig(t, dir)

	internal := filepath.Join(dir, "docs/api/internal.yaml")
	data, err := os.ReadFile(internal)
	if err != nil {
		t.Fatal(err)
	}
	stale := strings.Replace(string(data), "Petstore internal API", "Petstore staff API", 1)
	if stale == string(data) {
		t.Fatal("the fixture edit changed nothing")
	}
	if err := os.WriteFile(internal, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "docs/api/public.yaml")); err != nil {
		t.Fatal(err)
	}

	drifts, err := Check(Options{Dir: dir, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 2 {
		t.Fatalf("drifts = %+v", drifts)
	}
	byDoc := map[string]Drift{}
	for _, d := range drifts {
		byDoc[d.Document] = d
	}
	if d := byDoc["internal"]; d.Output != "docs/api/internal.yaml" || !strings.Contains(d.Reason, "first at line") {
		t.Errorf("internal = %+v", d)
	}
	if d := byDoc["public"]; d.Output != "docs/api/public.yaml" || d.Reason != "does not exist" {
		t.Errorf("public = %+v", d)
	}

	// Nothing was written: the stale file is still stale and the missing one is still missing.
	after, _ := os.ReadFile(internal)
	if string(after) != stale {
		t.Error("Check changed a document")
	}
	if _, err := os.Stat(filepath.Join(dir, "docs/api/public.yaml")); !os.IsNotExist(err) {
		t.Error("Check wrote a document")
	}

	// Select only the current one and there is nothing to report.
	if err := os.WriteFile(internal, data, 0o644); err != nil {
		t.Fatal(err)
	}
	drifts, err = Check(Options{Dir: dir, Config: cfg, Docs: []string{"internal"}})
	if err != nil || len(drifts) != 0 {
		t.Fatalf("a current document: %+v, %v", drifts, err)
	}
}

func TestDescribeDifference(t *testing.T) {
	for _, tc := range []struct {
		onDisk, fresh, want string
	}{
		{"a\nb\nc\n", "a\nX\nc\n", "first at line 2"},
		{"a\nb\n", "a\nb\nc\n", "first at line 3"}, // the file ends where generation goes on
		{"a\nb", "a\nb\nc", "has 2 lines and generation gives 3"},
		{"a\nb\nc\nd", "a\nb", "has 4 lines and generation gives 2"},
	} {
		if got := describeDifference([]byte(tc.onDisk), []byte(tc.fresh)); !strings.Contains(got, tc.want) {
			t.Errorf("describeDifference(%q, %q) = %q, want it to contain %q", tc.onDisk, tc.fresh, got, tc.want)
		}
	}
}

// Generating the same module again must give the same bytes. A hash table that
// is walked in whatever order it likes shows up here as a document that changes
// from one run to the next, which would make a drift gate cry wolf.
func TestPetstoreGenerationIsDeterministic(t *testing.T) {
	cfg := petstoreConfig(t, petstoreDir)
	var first map[string]string
	for run := 0; run < 8; run++ {
		out := t.TempDir()
		if err := Run(Options{Dir: petstoreDir, Config: cfg, OutputDir: out}); err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, d := range cfg.Docs {
			data, err := os.ReadFile(filepath.Join(out, d.Output))
			if err != nil {
				t.Fatal(err)
			}
			got[d.Name] = string(data)
		}
		if first == nil {
			first = got
			continue
		}
		for name, doc := range got {
			if doc != first[name] {
				t.Fatalf("run %d wrote a different %s document than run 0 (%s)", run, name, describeDifference([]byte(first[name]), []byte(doc)))
			}
		}
	}
}

// attrOf is the value of an attribute of a record, or "".
func attrOf(rec slog.Record, key string) string {
	value := ""
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			value = a.Value.String()
			return false
		}
		return true
	})
	return value
}

// A key of the configuration that names a type the module does not declare is a
// typo that would otherwise do nothing, without a word: the type is described
// as it is declared, and nobody learns that the entry was meant for it.
func TestConfigKeysThatNameNoTypeAreReported(t *testing.T) {
	cfg := petstoreConfig(t, petstoreDir)
	cfg.TypeMap["example.com.petstore.pet.domain.Pricee"] = config.TypeSpec{Type: "string"}
	cfg.TypeMap["github.com.shopspring.decimal.Decimal"] = config.TypeSpec{Type: "number"} // another module: not checked
	cfg.Request.Query["example.com.petstore.pet.protocol.ListPetReq"] = []config.QueryField{{Name: "x"}}
	cfg.Docs[1].ForceKeep = []string{"example.com.petstore.pet.domain.Pet", "example.com.petstore.pet.domain.Pat"}

	log := &records{}
	out := t.TempDir()
	if err := Run(Options{Dir: petstoreDir, Config: cfg, OutputDir: out, Logger: slog.New(log)}); err != nil {
		t.Fatal(err)
	}

	guesses := map[string]string{} // the key that names no type -> the key it was probably meant to be
	for _, rec := range log.list {
		if rec.Level == slog.LevelWarn && attrOf(rec, "key") != "" {
			guesses[attrOf(rec, "key")] = attrOf(rec, "didYouMean")
		}
	}
	want := map[string]string{
		"example.com.petstore.pet.domain.Pricee":       "example.com.petstore.pet.domain.Price",
		"example.com.petstore.pet.protocol.ListPetReq": "example.com.petstore.pet.protocol.ListPetsReq",
		"example.com.petstore.pet.domain.Pat":          "example.com.petstore.pet.domain.Pet",
	}
	if !reflect.DeepEqual(guesses, want) {
		t.Errorf("reported %v, want %v", guesses, want)
	}
}
