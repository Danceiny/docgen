package engine

import (
	"go/ast"
	"log/slog"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/errcat"
)

const shopErrors = `[
  {"name": "NotFoundErr", "code": 100000404, "message": "not found", "httpCode": 404},
  {"name": "RateLimitErr", "code": 100000429, "message": "slow down", "httpCode": 429}
]`

// withErrorCatalog configures a catalog and the prefix its errors are keyed by,
// for a module called example.com/shop, for one test.
func withErrorCatalog(t *testing.T, catalog, prefix string) {
	t.Helper()
	c, err := errcat.Parse([]byte(catalog))
	if err != nil {
		t.Fatal(err)
	}
	withSettings(t, Settings{Errors: c, ErrorPrefix: prefix}, "example.com/shop")
}

func newDoc() *openapi3.T {
	return &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
}

func TestMountAllErrorsAddsOneComponentPerCatalogEntry(t *testing.T) {
	withErrorCatalog(t, shopErrors, "example.com.shop.errors.")
	doc := newDoc()
	MountAllErrors(doc)

	if len(doc.Components.Schemas) != 2 {
		t.Fatalf("components = %d, want 2", len(doc.Components.Schemas))
	}
	ref := doc.Components.Schemas["example.com.shop.errors.NotFoundErr"]
	if ref == nil || ref.Value == nil {
		t.Fatalf("no component for NotFoundErr: %v", doc.Components.Schemas)
	}
	s := ref.Value
	if s.Title != "example.com.shop.errors.NotFoundErr" || s.Description != "NotFoundErr" || !s.Type.Is("object") {
		t.Errorf("component = title %q description %q type %v", s.Title, s.Description, s.Type)
	}
	code, message := s.Properties["code"], s.Properties["message"]
	if code == nil || !code.Value.Type.Is("integer") || code.Value.Example != int32(100000404) {
		t.Errorf("code = %+v", code)
	}
	if message == nil || !message.Value.Type.Is("string") || message.Value.Example != "not found" {
		t.Errorf("message = %+v", message)
	}
}

func TestMountAllErrorsWithNoCatalogAddsNothing(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	doc := newDoc()
	MountAllErrors(doc)
	if len(doc.Components.Schemas) != 0 {
		t.Fatalf("components = %v", doc.Components.Schemas)
	}
}

func TestErrorPrefixDefaultsToTheModuleKey(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	if got := errorPrefix(); got != "example.com.shop.errors." {
		t.Fatalf("default prefix = %q", got)
	}
	if got := errorKey("NotFoundErr"); got != "example.com.shop.errors.NotFoundErr" {
		t.Fatalf("errorKey = %q", got)
	}

	withSettings(t, Settings{ErrorPrefix: "example.com.shop.faults."}, "example.com/shop")
	if got := errorKey("NotFoundErr"); got != "example.com.shop.faults.NotFoundErr" {
		t.Fatalf("configured errorKey = %q", got)
	}
}

// A @response annotation names an error; its status is the HTTP status the
// catalog gives that error, and an annotation that says otherwise is corrected
// with a warning. A name the catalog lacks is kept as written.
func TestResponseAnnotationsAreCheckedAgainstTheCatalog(t *testing.T) {
	withErrorCatalog(t, shopErrors, "example.com.shop.errors.")
	rec := captureLogs(t)

	m := &Method{Doc: "@response:400,NotFoundErr,Order is missing\n@response:429,RateLimitErr,Too fast\n@response:503,Unlisted,Down"}
	m.parseResults(&ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}, {Type: ast.NewIdent("error")}}})

	type got struct{ code, key, description, comment string }
	var have []got
	for _, r := range m.Responses {
		have = append(have, got{r.Code, r.DataType.FullKey, r.DataType.Description, r.Description})
	}
	want := []got{
		// 400 is corrected to the catalog's 404
		{"404", "example.com.shop.errors.NotFoundErr", "404", "Order is missing"},
		// agrees with the catalog, untouched
		{"429", "example.com.shop.errors.RateLimitErr", "429", "Too fast"},
		// not in the catalog, kept as written
		{"503", "example.com.shop.errors.Unlisted", "503", "Down"},
		// the method's own result
		{"200", "string", "", ""},
	}
	if len(have) != len(want) {
		t.Fatalf("responses = %+v", have)
	}
	for i := range want {
		if have[i] != want[i] {
			t.Errorf("response %d = %+v, want %+v", i, have[i], want[i])
		}
	}

	if len(rec.records) != 1 {
		t.Fatalf("got %d diagnostics, want one warning for the corrected status: %v", len(rec.records), rec.records)
	}
	w := rec.records[0]
	if w.Level != slog.LevelWarn {
		t.Errorf("level = %v", w.Level)
	}
	attrs := attrsOf(w)
	if attrs["commentCode"] != "400" || attrs["error"] != "NotFoundErr" || attrs["errorCode"] != "404" {
		t.Errorf("attributes = %v", attrs)
	}
}

func TestResponseAnnotationsAreNotCheckedWithoutACatalog(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	rec := captureLogs(t)

	m := &Method{Doc: "@response:400,NotFoundErr,Order is missing"}
	m.parseResults(&ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}})

	if len(m.Responses) != 1 || m.Responses[0].Code != "400" {
		t.Fatalf("responses = %+v", m.Responses)
	}
	if len(rec.records) != 0 {
		t.Fatalf("diagnostics = %v", rec.records)
	}
}

// An error response is the error itself, not the envelope around a data type; a
// type that merely lives under the prefix is not an error and stays an envelope.
// The error names its properties as the envelope does.
func TestErrorResponseIsTheCatalogEntry(t *testing.T) {
	withSettings(t, Settings{
		Errors:      mustCatalog(t, shopErrors),
		ErrorPrefix: "example.com.shop.errors.",
		Envelope:    &Envelope{Code: "code", Message: "msg", Data: "data"},
	}, "example.com/shop")
	doc := newDoc()

	ref := buildSuccessDataSchema(ResponseSpec{
		Code:     "404",
		DataType: &TypeDescriptor{FullKey: "example.com.shop.errors.NotFoundErr", Description: "404"},
	}, doc)
	s := ref.Value
	if s.Title != "NotFoundErr" {
		t.Errorf("title = %q, want the error's name", s.Title)
	}
	code, msg := s.Properties["code"], s.Properties["msg"]
	if code == nil || code.Value.Example != int32(100000404) || msg == nil || msg.Value.Example != "not found" {
		t.Errorf("properties = %+v", s.Properties)
	}
	if _, ok := s.Properties["data"]; ok {
		t.Error("an error response has no data property")
	}

	other := buildSuccessDataSchema(ResponseSpec{
		Code:        "200",
		Description: "a result type in the errors' package",
		DataType:    &TypeDescriptor{FullKey: "example.com.shop.errors.BatchResult"},
	}, doc).Value
	if other.Title != "" || other.Properties["code"].Value.Example != nil {
		t.Errorf("a type under the prefix that is not in the catalog became %+v", other)
	}
	if other.Description != "a result type in the errors' package" {
		t.Errorf("description = %q", other.Description)
	}
}

// Without an envelope an error is an object with a code and a message.
func TestErrorResponseWithoutAnEnvelopeHasCodeAndMessage(t *testing.T) {
	withSettings(t, Settings{Errors: mustCatalog(t, shopErrors), ErrorPrefix: "example.com.shop.errors."}, "example.com/shop")

	s := buildSuccessDataSchema(ResponseSpec{
		Code:     "404",
		DataType: &TypeDescriptor{FullKey: "example.com.shop.errors.NotFoundErr"},
	}, newDoc()).Value
	if s.Title != "NotFoundErr" || s.Properties["code"] == nil || s.Properties["message"] == nil || len(s.Properties) != 2 {
		t.Fatalf("error without an envelope = %+v", s)
	}
}

func mustCatalog(t *testing.T, data string) *errcat.Catalog {
	t.Helper()
	c, err := errcat.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
