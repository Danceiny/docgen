package engine

import (
	"log/slog"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// @method lists the HTTP methods of an operation, separated by commas; a space
// after the comma is allowed, and every method OpenAPI has an operation for is.
func TestBuildPathItemHonoursEveryHTTPMethod(t *testing.T) {
	logs := captureLogs(t)
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	service := ServiceInterface{ServiceName: "pet"}
	method := &Method{Name: "Edit", Doc: "Edit a pet.\n\n@method: GET, post,PATCH, head ,OPTIONS,TRACE,DELETE,PUT,FETCH"}

	BuildPathItem(doc, service, method)

	item := doc.Paths.Value("/api/pet/edit")
	if item == nil {
		t.Fatalf("no path; the paths are %v", doc.Paths.Map())
	}
	got := map[string]bool{
		"GET": item.Get != nil, "POST": item.Post != nil, "PATCH": item.Patch != nil, "HEAD": item.Head != nil,
		"OPTIONS": item.Options != nil, "TRACE": item.Trace != nil, "DELETE": item.Delete != nil, "PUT": item.Put != nil,
	}
	for name, ok := range got {
		if !ok {
			t.Errorf("the operation has no %s", name)
		}
	}

	warned := false
	for _, rec := range logs.records {
		if attrsOf(rec)["httpMethod"] == "FETCH" && rec.Level == slog.LevelWarn {
			warned = true
		}
	}
	if !warned {
		t.Error("an HTTP method that cannot be an operation is not reported")
	}
}
