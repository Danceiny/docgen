package engine

import (
	"log/slog"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// An operation id names one operation: each verb of the route has its own.
	ids := map[string]bool{}
	for _, op := range item.Operations() {
		if ids[op.OperationID] {
			t.Errorf("the operation id %q is used twice", op.OperationID)
		}
		ids[op.OperationID] = true
	}
	if item.Get.OperationID != "pet/edit_get" || item.Post.OperationID != "pet/edit_post" {
		t.Errorf("operation ids = %q and %q", item.Get.OperationID, item.Post.OperationID)
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

// A route that nothing can be put under is not in the document as an empty path.
func TestBuildPathItemWithNoValidHTTPMethodAddsNoPath(t *testing.T) {
	captureLogs(t)
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	BuildPathItem(doc, ServiceInterface{ServiceName: "pet"}, &Method{Name: "Fetch", Doc: "Fetch.\n\n@method: FETCH"})
	if got := doc.Paths.Len(); got != 0 {
		t.Fatalf("paths = %v", doc.Paths.Map())
	}
}

// Two methods that answer one route with different verbs keep their operation
// ids apart, and a route with one operation keeps the id it always had.
func TestOperationsOfOneRouteWithDifferentVerbsHaveDifferentIDs(t *testing.T) {
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	service := ServiceInterface{ServiceName: "pet"}
	BuildPathItem(doc, service, &Method{Name: "Read", Doc: "Read.\n\n@method: GET\n@path: /item"})
	if got := doc.Paths.Value("/api/pet/item").Get.OperationID; got != "pet/item" {
		t.Fatalf("a route with one operation has the id %q", got)
	}
	BuildPathItem(doc, service, &Method{Name: "Remove", Doc: "Remove.\n\n@method: DELETE\n@path: /item"})
	item := doc.Paths.Value("/api/pet/item")
	if item.Get.OperationID != "pet/item_get" || item.Delete.OperationID != "pet/item_delete" {
		t.Fatalf("ids = %q and %q", item.Get.OperationID, item.Delete.OperationID)
	}
}

// Two methods that answer one route with one verb cannot both be in the document;
// the later one is, and the log says which route it was.
func TestTwoMethodsOnOneRouteAndVerbAreReported(t *testing.T) {
	logs := captureLogs(t)
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	service := ServiceInterface{ServiceName: "pet"}
	BuildPathItem(doc, service, &Method{Name: "Get", Doc: "First.\n\n@path: /same"})
	BuildPathItem(doc, service, &Method{Name: "Other", Doc: "Second.\n\n@path: /same"})

	if got := doc.Paths.Value("/api/pet/same").Post.Summary; got != "Second." {
		t.Fatalf("the document has %q, want the later method", got)
	}
	reported := false
	for _, rec := range logs.records {
		if rec.Level == slog.LevelWarn && attrsOf(rec)["route"] == "/api/pet/same" && attrsOf(rec)["replaces"] == "First." {
			reported = true
		}
	}
	if !reported {
		t.Error("the collision is not reported")
	}
}

// The refusal of the document says only that a path parameter is missing; the
// warning says what to add, and for which of the two it is.
func TestARouteAndItsPathParametersAreSaidToDisagree(t *testing.T) {
	logs := captureLogs(t)
	op := &openapi3.Operation{Parameters: openapi3.Parameters{{Value: openapi3.NewPathParameter("other")}}}
	warnAboutPathParameters(&Method{Name: "ByID", APIPath: "/api/s/byId/{id}", Pos: "s.go:1"}, op)

	var messages []string
	for _, rec := range logs.records {
		messages = append(messages, rec.Message)
	}
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0], "{id}")
	assert.Contains(t, messages[0], "@param:path id")
	assert.Contains(t, messages[1], "path parameter other")
	assert.Contains(t, messages[1], "{other}")

	logs.records = nil
	op = &openapi3.Operation{Parameters: openapi3.Parameters{{Value: openapi3.NewPathParameter("id")}}}
	warnAboutPathParameters(&Method{Name: "ByID", APIPath: "/api/s/byId/{id}"}, op)
	assert.Empty(t, logs.records, "a route and its parameters that agree say nothing")
}
