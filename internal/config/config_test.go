package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const valid = `
version: 1
generic_titles: [example.Page, example.Envelope]
type_map:
  example.types.ID: {type: string, description: id}
  example.types.IDs: {type: array, items: {type: string}}
headers:
  default: Base
  types: {Base: example.protocol.Base, Admin: example.protocol.Admin}
docs:
  - name: internal
    audience: internal
    output: docs/api/api.yaml
    info: {title: Example API, version: 1.2.3, description: An example.}
    servers:
      - {url: "http://localhost:8080", description: Local}
    models: ["*/domain", "common/types"]
    services: ["*/service"]
    legacy_field_tokens: [Staff]
  - name: public
    audience: public
    output: docs/api/openapi.yaml
    info: {title: Example Public API}
    models: ["*/domain"]
    services: ["*/service"]
    force_keep: [example.Webhook]
`

func TestParseValid(t *testing.T) {
	cfg, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Docs) != 2 {
		t.Fatalf("docs = %d", len(cfg.Docs))
	}
	d := cfg.Docs[0]
	if d.Name != "internal" || d.Audience != AudienceInternal || d.Output != "docs/api/api.yaml" ||
		d.Info.Title != "Example API" || d.Info.Version != "1.2.3" || len(d.Servers) != 1 ||
		d.Servers[0].URL != "http://localhost:8080" || len(d.Models) != 2 || d.Services[0] != "*/service" ||
		len(d.LegacyFieldTokens) != 1 || d.LegacyFieldTokens[0] != "Staff" {
		t.Fatalf("doc decoded wrongly: %+v", d)
	}
}

func TestParseReadsGenericTitlesAndForceKeep(t *testing.T) {
	cfg, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.GenericTitles) != 2 || cfg.GenericTitles[1] != "example.Envelope" {
		t.Errorf("generic_titles = %v", cfg.GenericTitles)
	}
	if len(cfg.Docs[0].ForceKeep) != 0 {
		t.Errorf("force_keep is per document, got %v on the first", cfg.Docs[0].ForceKeep)
	}
	if got := cfg.Docs[1].ForceKeep; len(got) != 1 || got[0] != "example.Webhook" {
		t.Errorf("force_keep = %v", got)
	}
}

func TestParseDefaultsTheInfoVersion(t *testing.T) {
	cfg, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Docs[0].Info.Version; got != "1.2.3" {
		t.Errorf("an explicit version must be kept, got %q", got)
	}
	if got := cfg.Docs[1].Info.Version; got != DefaultInfoVersion {
		t.Errorf("a missing version must default to %q, got %q", DefaultInfoVersion, got)
	}
}

// A file with nothing in it is the first mistake of a new user: say what it needs
// instead of the "EOF" of the decoder.
func TestParseSaysWhatAnEmptyConfigurationNeeds(t *testing.T) {
	for _, in := range []string{"", "\n\n", "# only a comment\n"} {
		_, err := Parse([]byte(in))
		if err == nil || !strings.Contains(err.Error(), "the configuration is empty") || !strings.Contains(err.Error(), "docs:") {
			t.Errorf("Parse(%q) = %v", in, err)
		}
	}
}

func TestParseRejects(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(valid, old, new, 1) }
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"unknown key", replace("audience: internal", "audience: internal\n    audiance: x"), "audiance"},
		{"wrong version", replace("version: 1\n", "version: 2\n"), "want 1, got 2"},
		{"no docs", "version: 1\ndocs: []\n", "at least one document"},
		{"missing name", replace("name: internal", "name: \"\""), "docs[0].name: required"},
		{"duplicate name", replace("name: public", "name: internal"), "duplicate document name"},
		{"bad audience", replace("audience: public", "audience: partner"), "docs[1].audience"},
		{"absolute output", replace("docs/api/api.yaml", "/etc/api.yaml"), "inside the module directory"},
		{"output escapes the module", replace("docs/api/api.yaml", "../api.yaml"), "inside the module directory"},
		{"missing title", replace("title: Example Public API", "title: \"\""), "docs[1].info.title"},
		{"no models", replace("models: [\"*/domain\"]\n    services: [\"*/service\"]\n", "models: []\n    services: [\"*/service\"]\n"), "docs[1].models"},
		{"no services", replace("services: [\"*/service\"]\n    legacy_field_tokens", "services: []\n    legacy_field_tokens"), "docs[0].services"},
		{"server without url", replace("url: \"http://localhost:8080\"", "url: \"\""), "servers[0].url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseReportsEveryProblemAtOnce(t *testing.T) {
	_, err := Parse([]byte("version: 1\ndocs:\n  - {name: a}\n"))
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"audience", "output", "info.title", "models", "services"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docgen.yaml")
	if err := os.WriteFile(path, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("a missing file must be an error")
	}
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatalf("an invalid file's error must name the file, got %v", err)
	}
}

func TestSelect(t *testing.T) {
	cfg, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	all, err := cfg.Select(nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %v, %v", all, err)
	}
	pub, err := cfg.Select([]string{"public"})
	if err != nil || len(pub) != 1 || pub[0].Name != "public" {
		t.Fatalf("public = %v, %v", pub, err)
	}
	if _, err := cfg.Select([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "available: internal, public") {
		t.Fatalf("unknown name must list the available ones, got %v", err)
	}
}

func TestParseReadsTypeMapAndHeaders(t *testing.T) {
	cfg, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	id := cfg.TypeMap["example.types.ID"]
	if id.Type != "string" || id.Description != "id" {
		t.Errorf("ID = %+v", id)
	}
	ids := cfg.TypeMap["example.types.IDs"]
	if ids.Type != "array" || ids.Items == nil || ids.Items.Type != "string" {
		t.Errorf("IDs = %+v", ids)
	}
	if cfg.Headers.Default != "Base" || cfg.Headers.Types["Admin"] != "example.protocol.Admin" {
		t.Errorf("headers = %+v", cfg.Headers)
	}
}

func TestParseRejectsBadTypeMapAndHeaders(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(valid, old, new, 1) }
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"unknown schema type", replace("example.types.ID: {type: string, description: id}", "example.types.ID: {type: text}"), "type_map.example.types.ID.type"},
		{"array without items", replace("example.types.IDs: {type: array, items: {type: string}}", "example.types.IDs: {type: array}"), "an array needs its element type"},
		{"items on a string", replace("example.types.ID: {type: string, description: id}", "example.types.ID: {type: string, items: {type: string}}"), "only an array has items"},
		{"bad element type", replace("items: {type: string}}", "items: {type: blob}}"), "type_map.example.types.IDs.items.type"},
		{"default not among the types", replace("default: Base", "default: Other"), "headers.default"},
		{"empty header type", replace("Admin: example.protocol.Admin", "Admin: \"\""), "headers.types"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseAcceptsNoTypeMapAndNoHeaders(t *testing.T) {
	data := "version: 1\ndocs:\n  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}\n"
	cfg, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TypeMap) != 0 || cfg.Headers.Default != "" || len(cfg.Headers.Types) != 0 {
		t.Fatalf("a stranger's minimal config must need neither: %+v %+v", cfg.TypeMap, cfg.Headers)
	}
}

const withRequestAndResponse = `
version: 1
request:
  multipart:
    example.UploadReq:
      - {name: file, kind: file, required: true}
      - {name: owner, kind: scalar, scalar_to: integer}
  query:
    example.GetAssetReq:
      - {name: ref, required: true, description: Storage reference}
  runtime_only:
    - {path: /api/cron/setStore, type: example.JobStore}
response:
  binary:
    example.GetAssetResp:
      description: Public image
      content_types: [image/png, image/jpeg]
      errors:
        "404": Not found
docs:
  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}
`

func TestParseReadsRequestAndResponse(t *testing.T) {
	cfg, err := Parse([]byte(withRequestAndResponse))
	if err != nil {
		t.Fatal(err)
	}
	up := cfg.Request.Multipart["example.UploadReq"]
	if len(up) != 2 || up[0].Kind != "file" || !up[0].Required || up[1].ScalarTo != "integer" {
		t.Errorf("multipart = %+v", up)
	}
	q := cfg.Request.Query["example.GetAssetReq"]
	if len(q) != 1 || q[0].Name != "ref" || !q[0].Required || q[0].Description != "Storage reference" {
		t.Errorf("query = %+v", q)
	}
	if len(cfg.Request.RuntimeOnly) != 1 || cfg.Request.RuntimeOnly[0].Path != "/api/cron/setStore" || cfg.Request.RuntimeOnly[0].Type != "example.JobStore" {
		t.Errorf("runtime_only = %+v", cfg.Request.RuntimeOnly)
	}
	b := cfg.Response.Binary["example.GetAssetResp"]
	if b.Description != "Public image" || len(b.ContentTypes) != 2 || b.Errors["404"] != "Not found" {
		t.Errorf("binary = %+v", b)
	}
}

func TestParseRejectsBadRequestAndResponse(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(withRequestAndResponse, old, new, 1) }
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"unknown field kind", replace("kind: file", "kind: blob"), "kind: want file or scalar"},
		{"scalar without a type", replace("scalar_to: integer", "scalar_to: \"\""), "scalar_to: want string or integer"},
		{"scalar of another type", replace("scalar_to: integer", "scalar_to: boolean"), "scalar_to: want string or integer"},
		{"file with a scalar type", replace("kind: file, required: true", "kind: file, scalar_to: string, required: true"), "only a scalar has one"},
		{"field without a name", replace("name: file", "name: \"\""), ".name: required"},
		{"duplicate multipart field", replace("name: owner", "name: file"), "duplicate field"},
		{"multipart without fields", replace("      - {name: file, kind: file, required: true}\n      - {name: owner, kind: scalar, scalar_to: integer}\n", "      []\n"), "at least one field"},
		{"query parameter without a name", replace("name: ref", "name: \"\""), "request.query"},
		{"runtime_only without a path", replace("path: /api/cron/setStore", "path: \"\""), "path and type are both required"},
		{"duplicate runtime_only", replace("    - {path: /api/cron/setStore, type: example.JobStore}\n", "    - {path: /api/cron/setStore, type: example.JobStore}\n    - {path: /api/cron/setStore, type: example.JobStore}\n"), "duplicate /api/cron/setStore"},
		{"binary without a description", replace("description: Public image", "description: \"\""), "description: required"},
		{"binary without media types", replace("content_types: [image/png, image/jpeg]", "content_types: []"), "at least one media type"},
		{"status 200 among the errors", replace("\"404\": Not found", "\"200\": Fine"), "other than 200"},
		{"status that is not a code", replace("\"404\": Not found", "\"4xx\": Not found"), "want a status code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

const withErrors = `
version: 1
errors:
  file: build/errors.json
  component_prefix: example.com.shop.errors.
docs:
  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}
`

func TestParseReadsErrors(t *testing.T) {
	cfg, err := Parse([]byte(withErrors))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Errors.File != "build/errors.json" || cfg.Errors.ComponentPrefix != "example.com.shop.errors." {
		t.Fatalf("errors = %+v", cfg.Errors)
	}

	// A stranger's minimal config needs no catalog.
	cfg, err = Parse([]byte("version: 1\ndocs:\n  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Errors != (Errors{}) {
		t.Fatalf("errors without a section = %+v", cfg.Errors)
	}

	// The prefix may be left to its default.
	if _, err := Parse([]byte(strings.Replace(withErrors, "  component_prefix: example.com.shop.errors.\n", "", 1))); err != nil {
		t.Fatalf("a file without a prefix: %v", err)
	}
}

func TestParseRejectsBadErrors(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(withErrors, old, new, 1) }
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"absolute file", replace("build/errors.json", "/etc/errors.json"), "errors.file: must be a path inside the module directory"},
		{"file outside the module", replace("build/errors.json", "../errors.json"), "errors.file: must be a path inside the module directory"},
		{"prefix without a trailing dot", replace("example.com.shop.errors.\n", "example.com.shop.errors\n"), "must name a package and end in a dot"},
		{"prefix that is only a dot", replace("example.com.shop.errors.\n", "\".\"\n"), "must name a package and end in a dot"},
		{"prefix without a file", replace("  file: build/errors.json\n", ""), "has no effect without errors.file"},
		{"unknown key", replace("  file:", "  path: x\n  file:"), "field path not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

const withOverlays = `
version: 1
docs:
  - name: a
    audience: internal
    output: a.yaml
    info: {title: A}
    models: [m]
    services: [s]
    overlay:
      - {file: build/overlay.yaml, stage: after_models}
      - {file: build/late.yaml, stage: after_apis}
`

func TestParseReadsOverlays(t *testing.T) {
	cfg, err := Parse([]byte(withOverlays))
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Docs[0].Overlay
	want := []OverlayFile{
		{File: "build/overlay.yaml", Stage: StageAfterModels},
		{File: "build/late.yaml", Stage: StageAfterAPIs},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("overlay = %+v, want %+v", got, want)
	}

	// A stranger's minimal config needs none.
	cfg, err = Parse([]byte("version: 1\ndocs:\n  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}\n"))
	if err != nil || len(cfg.Docs[0].Overlay) != 0 {
		t.Fatalf("no overlay: %+v, %v", cfg, err)
	}
}

func TestParseRejectsBadOverlays(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(withOverlays, old, new, 1) }
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"no file", replace("{file: build/overlay.yaml, stage: after_models}", "{stage: after_models}"), "docs[0].overlay[0].file: required"},
		{"absolute file", replace("build/overlay.yaml", "/etc/overlay.yaml"), "docs[0].overlay[0].file: must be a path inside the module directory"},
		{"file outside the module", replace("build/late.yaml", "../late.yaml"), "docs[0].overlay[1].file: must be a path inside the module directory"},
		{"unknown stage", replace("stage: after_apis", "stage: before_prune"), `docs[0].overlay[1].stage: want "after_models" or "after_apis", got "before_prune"`},
		{"no stage", replace("{file: build/late.yaml, stage: after_apis}", "{file: build/late.yaml}"), `docs[0].overlay[1].stage: want "after_models"`},
		{"the same file at the same stage twice", replace("build/late.yaml, stage: after_apis", "build/overlay.yaml, stage: after_models"), "build/overlay.yaml is already applied at after_models"},
		{"unknown key", replace("{file: build/overlay.yaml,", "{path: x, file: build/overlay.yaml,"), "field path not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseAllowsTheSameFileAtTwoStages(t *testing.T) {
	yaml := strings.Replace(withOverlays, "build/late.yaml, stage: after_apis", "build/overlay.yaml, stage: after_apis", 1)
	if _, err := Parse([]byte(yaml)); err != nil {
		t.Fatalf("the same file at two stages: %v", err)
	}
}

const withResponseSettings = `
version: 1
api:
  prefix: /v1
  skip_methods: [GetChildren, SetStore]
vendor_extensions: true
compat:
  legacy_operation_types: true
response:
  envelope: {code: status, message: info, data: result}
  default_statuses:
    "401": Unauthorized
    "429": Too Many Requests
docs:
  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}
`

func TestParseReadsAPIResponseAndExtensions(t *testing.T) {
	cfg, err := Parse([]byte(withResponseSettings))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.API.Prefix != "/v1" || !cfg.VendorExtensions {
		t.Fatalf("api %+v extensions %v", cfg.API, cfg.VendorExtensions)
	}
	if !cfg.Compat.LegacyOperationTypes {
		t.Fatal("compat.legacy_operation_types is not read")
	}
	if !slices.Equal(cfg.API.SkipMethods, []string{"GetChildren", "SetStore"}) {
		t.Fatalf("skip_methods = %v", cfg.API.SkipMethods)
	}
	env := cfg.Response.Envelope
	if env == nil || env.Code != "status" || env.Message != "info" || env.Data != "result" {
		t.Fatalf("envelope = %+v", env)
	}
	if got := cfg.Response.DefaultStatuses; len(got) != 2 || got["401"] != "Unauthorized" || got["429"] != "Too Many Requests" {
		t.Fatalf("default_statuses = %v", got)
	}

	// A stranger's minimal config has none of them.
	cfg, err = Parse([]byte("version: 1\ndocs:\n  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.API.Prefix != "" || cfg.VendorExtensions || cfg.Response.Envelope != nil || len(cfg.Response.DefaultStatuses) != 0 {
		t.Fatalf("defaults: %+v", cfg)
	}
}

func TestParseRejectsBadAPIAndResponseSettings(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(withResponseSettings, old, new, 1) }
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"prefix without a slash", replace("prefix: /v1", "prefix: v1"), "api.prefix"},
		{"prefix ending in a slash", replace("prefix: /v1", "prefix: /v1/"), "api.prefix"},
		{"prefix with a space", replace("prefix: /v1", `prefix: "/v 1"`), "api.prefix"},
		{"prefix with a query", replace("prefix: /v1", `prefix: "/v1?x=1"`), "api.prefix"},
		{"a skipped method that is not a name", replace("SetStore", "Set-Store"), `api.skip_methods: "Set-Store"`},
		{"an empty skipped method", replace("SetStore", `""`), `api.skip_methods: ""`},
		{"envelope without a code", replace("code: status, ", ""), "response.envelope.code: required"},
		{"envelope without a message", replace("message: info, ", ""), "response.envelope.message: required"},
		{"envelope without data", replace(", data: result", ""), "response.envelope.data: required"},
		{"envelope with one name twice", replace("data: result", "data: status"), "must be three different properties"},
		{"envelope with message and data alike", replace("data: result", "data: info"), "must be three different properties"},
		{"a status that is not three digits", replace(`"401"`, `"41"`), `response.default_statuses["41"]`},
		{"a status that is not a number", replace(`"401"`, `"4xx"`), `response.default_statuses["4xx"]`},
		{"a status out of range", replace(`"401"`, `"600"`), `response.default_statuses["600"]`},
		{"the status every operation has anyway", replace(`"401"`, `"200"`), `response.default_statuses["200"]`},
		{"a status with no description", replace("401\": Unauthorized", `401": ""`), "description required"},
		{"unknown envelope key", replace("{code: status,", "{extra: x, code: status,"), "field extra not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

const withPublicSection = `
version: 1
docs:
  - name: pub
    audience: public
    output: pub.yaml
    info: {title: P}
    models: [m]
    services: [s]
    hide_type_prefixes: [Internal, Private]
    public:
      tag: external
      strip_tags_containing: [".com", "staff"]
      errors_last: true
`

func TestParseReadsThePublicSectionAndHiddenTypePrefixes(t *testing.T) {
	cfg, err := Parse([]byte(withPublicSection))
	if err != nil {
		t.Fatal(err)
	}
	d := cfg.Docs[0]
	if len(d.HideTypePrefixes) != 2 || d.HideTypePrefixes[1] != "Private" {
		t.Fatalf("hide_type_prefixes = %v", d.HideTypePrefixes)
	}
	if d.Public == nil || d.Public.Tag != "external" || !d.Public.ErrorsLast || len(d.Public.StripTagsContaining) != 2 {
		t.Fatalf("public = %+v", d.Public)
	}

	// A public document needs none of it.
	cfg, err = Parse([]byte("version: 1\ndocs:\n  - {name: a, audience: public, output: a.yaml, info: {title: A}, models: [m], services: [s]}\n"))
	if err != nil || cfg.Docs[0].Public != nil || len(cfg.Docs[0].HideTypePrefixes) != 0 {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}
}

func TestParseRejectsBadPublicSections(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(withPublicSection, old, new, 1) }
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"a public section on an internal document", replace("audience: public", "audience: internal"), "only a document of the public audience has a public section"},
		{"an empty hidden prefix", replace("[Internal, Private]", `["", Private]`), "docs[0].hide_type_prefixes[0]: must not be empty"},
		{"an empty strip string", replace(`[".com", "staff"]`, `[".com", ""]`), "docs[0].public.strip_tags_containing[1]: must not be empty"},
		{"an unknown public key", replace("errors_last: true", "errors_last: true\n      colour: red"), "field colour not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseReadsTypeMapExamples(t *testing.T) {
	cfg, err := Parse([]byte(`
version: 1
type_map:
  example.Date: {type: string, format: date, example: "2024-01-02"}
  example.Count: {type: integer, example: 3}
docs:
  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.TypeMap["example.Date"].Example; got != "2024-01-02" {
		t.Errorf("date example = %#v", got)
	}
	if got := cfg.TypeMap["example.Count"].Example; got != 3 {
		t.Errorf("count example = %#v, want the integer 3", got)
	}
}

func TestQueryFieldTypes(t *testing.T) {
	yaml := func(typ string) string {
		return "version: 1\nrequest:\n  query:\n    example.Req:\n      - {name: n, type: " + typ + "}\ndocs:\n  - {name: a, audience: internal, output: a.yaml, info: {title: A}, models: [m], services: [s]}\n"
	}
	for _, ok := range []string{"string", "integer", "number", "boolean"} {
		if _, err := Parse([]byte(yaml(ok))); err != nil {
			t.Errorf("type %s: %v", ok, err)
		}
	}
	if _, err := Parse([]byte(yaml("date"))); err == nil || !strings.Contains(err.Error(), `request.query.example.Req[0].type: want string, integer, number or boolean, got "date"`) {
		t.Errorf("a query type that does not exist: %v", err)
	}
}
