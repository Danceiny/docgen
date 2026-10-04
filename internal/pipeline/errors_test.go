package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
	"github.com/Danceiny/docgen/internal/engine"
)

func TestLoadErrorCatalog(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := `[{"name": "NotFoundErr", "code": 404, "message": "not found", "httpCode": 404}]`
	if err := os.WriteFile(filepath.Join(dir, "build", "errors.json"), []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := loadErrorCatalog(dir, config.Errors{})
	if c != nil || err != nil {
		t.Fatalf("no file configured: got %v, %v; want neither", c, err)
	}

	c, err = loadErrorCatalog(dir, config.Errors{File: "build/errors.json"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Find("NotFoundErr"); !ok {
		t.Fatalf("the file is read relative to the module directory, found %+v", c.Entries())
	}

	if _, err := loadErrorCatalog(dir, config.Errors{File: "build/missing.json"}); err == nil || !strings.Contains(err.Error(), "errors.file") {
		t.Fatalf("a missing catalog must be reported as errors.file: %v", err)
	}
}

func TestEngineSettingsCarryTheErrorPrefix(t *testing.T) {
	s := engineSettings(&config.Config{Errors: config.Errors{File: "e.json", ComponentPrefix: "shop.errors."}})
	if s.ErrorPrefix != "shop.errors." {
		t.Fatalf("ErrorPrefix = %q", s.ErrorPrefix)
	}
}

// A catalog that cannot be read stops the run before anything is generated, so a
// missing file is not mistaken for a module without errors.
func TestRunStopsOnAnUnreadableErrorCatalog(t *testing.T) {
	oldRoot := rootDir
	t.Cleanup(func() {
		rootDir = oldRoot
		engine.Configure(engine.Settings{})
	})

	cfg := &config.Config{Errors: config.Errors{File: "nope/errors.json"}}
	err := Run(Options{Dir: t.TempDir(), Config: cfg})
	if err == nil || !strings.Contains(err.Error(), "errors.file") {
		t.Fatalf("Run = %v, want an errors.file error", err)
	}
}

// Run reads the catalog and hands it, with the prefix its errors are keyed by, to
// the engine, which mounts one component per error.
func TestRunHandsTheCatalogToTheEngine(t *testing.T) {
	oldRoot := rootDir
	t.Cleanup(func() {
		rootDir = oldRoot
		engine.Configure(engine.Settings{})
	})
	dir := t.TempDir()
	catalog := `[{"name": "NotFoundErr", "code": 404, "message": "not found", "httpCode": 404}]`
	if err := os.WriteFile(filepath.Join(dir, "errors.json"), []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Errors: config.Errors{File: "errors.json", ComponentPrefix: "shop.errors."}}
	if err := Run(Options{Dir: dir, Config: cfg}); err != nil {
		t.Fatal(err)
	}

	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	engine.MountAllErrors(doc)
	if _, ok := doc.Components.Schemas["shop.errors.NotFoundErr"]; !ok || len(doc.Components.Schemas) != 1 {
		t.Fatalf("components = %v, want shop.errors.NotFoundErr alone", doc.Components.Schemas)
	}
}

func TestEngineSettingsCarryTheResponseAndAPISettings(t *testing.T) {
	s := engineSettings(&config.Config{
		API:              config.API{Prefix: "/v1"},
		VendorExtensions: true,
		Response: config.Response{
			Envelope:        &config.Envelope{Code: "status", Message: "info", Data: "result"},
			DefaultStatuses: map[string]string{"401": "Unauthorized"},
		},
	})
	if s.APIPrefix != "/v1" || !s.VendorExtensions {
		t.Fatalf("settings = %+v", s)
	}
	if s.Envelope == nil || s.Envelope.Code != "status" || s.Envelope.Message != "info" || s.Envelope.Data != "result" {
		t.Fatalf("envelope = %+v", s.Envelope)
	}
	if s.DefaultStatuses["401"] != "Unauthorized" {
		t.Fatalf("default statuses = %v", s.DefaultStatuses)
	}

	// No envelope in the configuration is no envelope in the settings.
	if bare := engineSettings(&config.Config{}); bare.Envelope != nil || bare.VendorExtensions || bare.APIPrefix != "" {
		t.Fatalf("a configuration without them must not invent them: %+v", bare)
	}
}
