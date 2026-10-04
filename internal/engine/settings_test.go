package engine

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// withSettings sets the run's settings and module name for one test.
func withSettings(t *testing.T, s Settings, module string) {
	t.Helper()
	oldSettings, oldModule := settings, ModuleName
	Configure(s)
	ModuleName = module
	t.Cleanup(func() { Configure(oldSettings); ModuleName = oldModule })
}

func TestConfiguredTypeIsABasicType(t *testing.T) {
	withSettings(t, Settings{TypeMap: map[string]*openapi3.Schema{
		"example.com.shop.types.ID": openapi3.NewStringSchema().WithFormat("uuid"),
	}}, "example.com/shop")

	schema := getBasicTypeSchema("example.com.shop.types.ID")
	if schema == nil || !schema.Type.Is("string") || schema.Format != "uuid" {
		t.Fatalf("configured type = %#v", schema)
	}
	if !isBasicType("example.com.shop.types.ID", "string") {
		t.Fatal("a configured type and a Go basic type are both basic")
	}
	if isBasicType("example.com.shop.types.Other") {
		t.Fatal("an unlisted type is not basic")
	}
	if getBasicTypeSchema("string") == nil {
		t.Fatal("the Go basic types stay known without configuration")
	}

	Configure(Settings{})
	if getBasicTypeSchema("example.com.shop.types.ID") != nil {
		t.Fatal("without the configuration the type is not basic")
	}
}

// The engine describes Go's basic types and nothing else; a module's own
// types with a fixed wire form are listed in its configuration.
func TestEngineKnowsNoDomainTypeWithoutConfiguration(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	for _, key := range []string{
		"example.com.shop.types.ID", "example.com.shop.types.IDs", "example.com.shop.types.Date",
		"example.com.shop.catalog.Currency", "example.com.shop.protocol.ActionID",
	} {
		if getBasicTypeSchema(key) != nil {
			t.Errorf("%s is built into the engine; it belongs in the configuration", key)
		}
	}
}

func TestHeaderTypeKey(t *testing.T) {
	withSettings(t, Settings{
		HeaderTypes:   map[string]string{"BaseHeader": "example.Base", "AdminHeader": "example.Admin"},
		DefaultHeader: "BaseHeader",
	}, "example")
	for name, want := range map[string]string{
		"AdminHeader": "example.Admin", // configured
		"BaseHeader":  "example.Base",
		"":            "example.Base", // no @headerType: the default
		"Unknown":     "example.Base", // unknown @headerType: the default
	} {
		if got := headerTypeKey(name); got != want {
			t.Errorf("headerTypeKey(%q) = %q, want %q", name, got, want)
		}
	}

	Configure(Settings{})
	if got := headerTypeKey("BaseHeader"); got != "" {
		t.Errorf("with no header types configured no header is added, got %q", got)
	}
	Configure(Settings{HeaderTypes: map[string]string{"A": "x.A"}})
	if got := headerTypeKey("B"); got != "" {
		t.Errorf("without a default, an unknown header adds none, got %q", got)
	}
}

// The module's own packages are recognised by the module path, not by a literal
// prefix. A multi-segment module path has dots of its own, and a sibling module
// may share its first segments.
func TestOwnPackagesAreRecognisedByTheModulePath(t *testing.T) {
	withSettings(t, Settings{}, "github.com/acme/shop")

	if got := ownKeyPrefix(); got != "github.com.acme.shop." {
		t.Fatalf("ownKeyPrefix = %q", got)
	}
	for path, want := range map[string]bool{
		"github.com/acme/shop":                true,
		"github.com/acme/shop/order/domain":   true,
		"github.com/acme/shopfront/order":     false, // another module that starts with the same text
		"github.com/acme/other":               false,
		"github.com/stretchr/testify/require": false,
		"example.com/other/types":             false,
		"":                                    false,
	} {
		if got := isOwnImportPath(path); got != want {
			t.Errorf("isOwnImportPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// A time.Time is a date-time string, and a module can say otherwise: the type
// map is looked at before the Go types the engine knows.
func TestTheTypeMapRestatesABuiltInType(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	builtin := getBasicTypeSchema("time.Time")
	if builtin == nil || !builtin.Type.Is("string") || builtin.Format != "date-time" || builtin.Example != "2024-01-02T15:04:05Z" {
		t.Fatalf("built-in time.Time = %+v", builtin)
	}
	if other := getBasicTypeSchema("time"); other == nil || other.Format != "date-time" {
		t.Fatalf("built-in time = %+v", other)
	}

	custom := openapi3.NewStringSchema().WithFormat("rfc3339")
	custom.Example = "now"
	withSettings(t, Settings{TypeMap: map[string]*openapi3.Schema{"time.Time": custom}}, "example.com/shop")
	if got := getBasicTypeSchema("time.Time"); got != custom {
		t.Fatalf("the configured schema must win: %+v", got)
	}
	if got := getBasicTypeSchema("time"); got == nil || got.Format != "date-time" {
		t.Fatalf("a type the map does not mention keeps its built-in schema: %+v", got)
	}
	if got := getBasicTypeSchema("string"); got == nil || !got.Type.Is("string") {
		t.Fatalf("string = %+v", got)
	}
}
