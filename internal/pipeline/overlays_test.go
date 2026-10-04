package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
)

func writeOverlay(t *testing.T, dir, name, key, typ string) {
	t.Helper()
	data := "version: 1\nschemas:\n  " + key + ": {type: " + typ + "}\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func typeOf(doc *openapi3.T, key string) string {
	ref := doc.Components.Schemas[key]
	if ref == nil || ref.Value == nil || ref.Value.Type == nil {
		return ""
	}
	return ref.Value.Type.Slice()[0]
}

// An overlay is applied at the stage the configuration gives it, and the
// overlays of a stage in the order the configuration lists them, so a later one
// replaces an earlier one.
func TestApplyOverlaysAppliesOnlyTheStageAndInConfiguredOrder(t *testing.T) {
	dir := t.TempDir()
	writeOverlay(t, dir, "first.yaml", "a.X", "string")
	writeOverlay(t, dir, "second.yaml", "a.X", "integer")
	writeOverlay(t, dir, "late.yaml", "a.Y", "boolean")

	overlays, err := loadOverlays(dir, []config.OverlayFile{
		{File: "first.yaml", Stage: config.StageAfterModels},
		{File: "second.yaml", Stage: config.StageAfterModels},
		{File: "late.yaml", Stage: config.StageAfterAPIs},
	})
	if err != nil {
		t.Fatal(err)
	}

	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	applyOverlays(doc, overlays, config.StageAfterModels)
	if got := typeOf(doc, "a.X"); got != "integer" {
		t.Errorf("a.X after the models = %q, want the later overlay's integer", got)
	}
	if doc.Components.Schemas["a.Y"] != nil {
		t.Error("an overlay of a later stage was applied early")
	}

	applyOverlays(doc, overlays, config.StageAfterAPIs)
	if got := typeOf(doc, "a.Y"); got != "boolean" {
		t.Errorf("a.Y after the operations = %q", got)
	}
}

func TestLoadOverlaysNamesTheEntryThatFails(t *testing.T) {
	dir := t.TempDir()
	writeOverlay(t, dir, "ok.yaml", "a.X", "string")

	_, err := loadOverlays(dir, []config.OverlayFile{
		{File: "ok.yaml", Stage: config.StageAfterModels},
		{File: "missing.yaml", Stage: config.StageAfterAPIs},
	})
	if err == nil || !strings.Contains(err.Error(), "overlay[1]") || !strings.Contains(err.Error(), "read overlay") {
		t.Fatalf("a missing overlay: %v", err)
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("version: 1\nschemas:\n  a.X: {type: text}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = loadOverlays(dir, []config.OverlayFile{{File: "bad.yaml", Stage: config.StageAfterModels}})
	if err == nil || !strings.Contains(err.Error(), "overlay[0]") || !strings.Contains(err.Error(), "bad.yaml") {
		t.Fatalf("an invalid overlay: %v", err)
	}
}
