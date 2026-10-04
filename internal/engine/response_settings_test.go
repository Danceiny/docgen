package engine

import (
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestRoutesStartWithTheConfiguredPrefix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prefix   string
		wantPath string
		wantID   string
	}{
		{"default", "", "/api/pet/get", "pet/get"},
		{"configured", "/v1", "/v1/pet/get", "pet/get"},
		{"nested", "/shop/v2", "/shop/v2/pet/get", "pet/get"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSettings(t, Settings{APIPrefix: tc.prefix}, "example.com/shop")
			doc := &openapi3.T{Paths: &openapi3.Paths{}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
			BuildPathItem(doc, ServiceInterface{ServiceName: "pet"}, &Method{Name: "Get", Doc: "Get a pet."})

			item := doc.Paths.Value(tc.wantPath)
			if item == nil || item.Post == nil {
				t.Fatalf("no POST operation at %s; paths = %v", tc.wantPath, doc.Paths.Map())
			}
			if item.Post.OperationID != tc.wantID {
				t.Errorf("operationId = %q, want %q", item.Post.OperationID, tc.wantID)
			}
		})
	}
}

func TestAPathTagThatStartsWithThePrefixIsTakenAsWritten(t *testing.T) {
	withSettings(t, Settings{APIPrefix: "/v1"}, "example.com/shop")
	doc := &openapi3.T{Paths: &openapi3.Paths{}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	BuildPathItem(doc, ServiceInterface{ServiceName: "pet"}, &Method{Name: "Get", Doc: "Get a pet.\n@path: /v1/legacy/pet"})
	if doc.Paths.Value("/v1/legacy/pet") == nil {
		t.Fatalf("paths = %v", doc.Paths.Map())
	}
}

func dataSpec(fullKey, description string) ResponseSpec {
	return ResponseSpec{Code: "200", Description: description, DataType: &TypeDescriptor{FullKey: fullKey}}
}

func TestEnvelopeNamesItsProperties(t *testing.T) {
	withSettings(t, Settings{
		Envelope: &Envelope{Code: "status", Message: "info", Data: "result"},
		TypeMap:  map[string]*openapi3.Schema{"example.com.shop.types.ID": openapi3.NewStringSchema()},
	}, "example.com/shop")
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"example.com.shop.domain.Pet": {Value: &openapi3.Schema{Title: "example.com.shop.domain.Pet", Type: &openapi3.Types{"object"}}},
	}}}

	ref := buildSuccessDataSchema(dataSpec("example.com.shop.domain.Pet", "The pet"), doc)
	s := ref.Value
	if len(s.Properties) != 3 || s.Properties["status"] == nil || s.Properties["info"] == nil {
		t.Fatalf("envelope properties = %v", s.Properties)
	}
	if !s.Properties["status"].Value.Type.Is("integer") || !s.Properties["info"].Value.Type.Is("string") {
		t.Errorf("status and info must be an integer and a string")
	}
	if got := s.Properties["result"]; got == nil || got.Ref != "#/components/schemas/example.com.shop.domain.Pet" {
		t.Errorf("result = %+v, want a reference to the Pet component", got)
	}
	if s.Title != "example.com.shop.domain.Pet" || s.Description != "The pet" {
		t.Errorf("title %q description %q", s.Title, s.Description)
	}
	if len(s.Extensions) != 0 {
		t.Errorf("extensions without VendorExtensions: %v", s.Extensions)
	}

	basic := buildSuccessDataSchema(dataSpec("example.com.shop.types.ID", ""), doc).Value
	if got := basic.Properties["result"]; got == nil || !got.Value.Type.Is("string") {
		t.Errorf("a configured basic type is the data inline: %+v", got)
	}

	none := buildSuccessDataSchema(dataSpec("example.com.shop.domain.Missing", ""), doc).Value
	if _, ok := none.Properties["result"]; ok || len(none.Properties) != 2 {
		t.Errorf("a data type with no schema leaves the envelope with no data property: %v", none.Properties)
	}
}

func TestEnvelopeExtensionsFollowTheEnvelopeNames(t *testing.T) {
	withSettings(t, Settings{
		Envelope:         &Envelope{Code: "status", Message: "info", Data: "result"},
		VendorExtensions: true,
	}, "example.com/shop")
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"example.com.shop.domain.Pet": {Value: &openapi3.Schema{Title: "example.com.shop.domain.Pet", Type: &openapi3.Types{"object"}}},
	}}}

	s := buildSuccessDataSchema(dataSpec("example.com.shop.domain.Pet", ""), doc).Value
	orders, _ := s.Extensions["x-apifox-orders"].([]string)
	if len(orders) != 3 || orders[0] != "status" || orders[1] != "info" || orders[2] != "result" {
		t.Errorf("x-apifox-orders = %v", s.Extensions["x-apifox-orders"])
	}
	if s.Extensions["x-primary-property"] != "result" || s.Extensions["x-display-name"] != "example.com.shop.domain.Pet" {
		t.Errorf("extensions = %v", s.Extensions)
	}
}

// Without an envelope the body of a response is the data type itself.
func TestWithoutAnEnvelopeTheDataTypeIsTheBody(t *testing.T) {
	withSettings(t, Settings{
		TypeMap: map[string]*openapi3.Schema{"example.com.shop.types.ID": openapi3.NewStringSchema()},
	}, "example.com/shop")
	doc := &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"example.com.shop.domain.Pet": {Value: &openapi3.Schema{Title: "example.com.shop.domain.Pet", Type: &openapi3.Types{"object"}}},
	}}}

	if ref := buildSuccessDataSchema(dataSpec("example.com.shop.domain.Pet", "The pet"), doc); ref == nil || ref.Ref != "#/components/schemas/example.com.shop.domain.Pet" {
		t.Errorf("a component is a reference: %+v", ref)
	}
	if ref := buildSuccessDataSchema(dataSpec("example.com.shop.types.ID", ""), doc); ref == nil || ref.Value == nil || !ref.Value.Type.Is("string") {
		t.Errorf("a basic type is inline: %+v", ref)
	}
	if ref := buildSuccessDataSchema(dataSpec("example.com.shop.domain.Missing", ""), doc); ref != nil {
		t.Errorf("a type with no schema has no body schema: %+v", ref)
	}
}

func responsesOf(t *testing.T, doc string, results ...ResponseSpec) *openapi3.Responses {
	t.Helper()
	method := &Method{Name: "Get", Doc: doc, Responses: results}
	return buildResponses(method, &openapi3.T{Components: &openapi3.Components{Schemas: openapi3.Schemas{
		"example.com.shop.domain.Pet": {Value: &openapi3.Schema{Title: "example.com.shop.domain.Pet", Type: &openapi3.Types{"object"}}},
	}}})
}

func TestDefaultStatusesAreOnEveryOperation(t *testing.T) {
	withSettings(t, Settings{DefaultStatuses: map[string]string{"401": "Unauthorized", "429": "Too Many Requests"}}, "example.com/shop")

	r := responsesOf(t, "", dataSpec("example.com.shop.domain.Pet", ""))
	if r.Len() != 3 {
		t.Fatalf("responses = %v", r.Map())
	}
	if got := *r.Value("401").Value.Description; got != "Unauthorized" {
		t.Errorf("401 = %q", got)
	}
	if r.Value("200") == nil || r.Value("429") == nil {
		t.Errorf("responses = %v", r.Map())
	}
}

func TestNoDefaultStatusesWhenNoneAreConfigured(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	r := responsesOf(t, "", dataSpec("example.com.shop.domain.Pet", ""))
	if r.Len() != 1 || r.Value("200") == nil {
		t.Fatalf("responses = %v", r.Map())
	}
}

// A response object has at least one response; an operation that has nothing to
// document and no default status says 200 with no content.
func TestAnOperationWithNoResultStillHasAResponse(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	r := responsesOf(t, "")
	if r.Len() != 1 || r.Value("200") == nil || r.Value("200").Value.Content != nil {
		t.Fatalf("responses = %v", r.Map())
	}
	if got := *r.Value("200").Value.Description; got != "OK" {
		t.Errorf("description = %q", got)
	}
}

func TestResponseDescription(t *testing.T) {
	// An envelope carries the description of the response.
	withSettings(t, Settings{Envelope: &Envelope{Code: "code", Message: "msg", Data: "data"}}, "example.com/shop")
	r := responsesOf(t, "", dataSpec("example.com.shop.domain.Pet", "The pet"))
	if got := *r.Value("200").Value.Description; got != "The pet" {
		t.Errorf("with an envelope: %q", got)
	}

	// Without one, the description is the annotation's, never the type's own.
	withSettings(t, Settings{TypeMap: map[string]*openapi3.Schema{
		"example.com.shop.types.ID": openapi3.NewStringSchema().WithFormat("uuid"),
	}}, "example.com/shop")
	idSchema := getBasicTypeSchema("example.com.shop.types.ID")
	idSchema.Description = "identifier of a pet"
	r = responsesOf(t, "", dataSpec("example.com.shop.types.ID", "The id"))
	if got := *r.Value("200").Value.Description; got != "The id" {
		t.Errorf("without an envelope: %q", got)
	}
	r = responsesOf(t, "", dataSpec("example.com.shop.types.ID", ""))
	if got := *r.Value("200").Value.Description; got != "OK" {
		t.Errorf("without an envelope and without an annotation: %q", got)
	}
}

// A status with no schema, such as a misspelt error name, is documented with its
// description and no content.
func TestAResponseWhoseTypeHasNoSchemaHasNoContent(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	r := responsesOf(t, "", ResponseSpec{Code: "404", Description: "No such pet", DataType: &TypeDescriptor{FullKey: "example.com.shop.errors.Typo"}})
	got := r.Value("404")
	if got == nil || got.Value.Content != nil || *got.Value.Description != "No such pet" {
		t.Fatalf("responses = %v", r.Map())
	}
}

func TestVendorExtensionsOfOperationsAndEnums(t *testing.T) {
	entries := []EnumEntry{{Name: "Open", Value: "open", Comment: "can be bought"}, {Name: "Sold", Value: "sold", Comment: "gone"}}

	withSettings(t, Settings{}, "example.com/shop")
	if s := generateEnumSchemaFromEntry(entries, "string"); len(s.Extensions) != 0 {
		t.Errorf("enum extensions without VendorExtensions: %v", s.Extensions)
	}
	op := buildOperation(&Method{Name: "Get", ServiceName: "pet", Folder: "Pets"}, newDoc())
	if len(op.Extensions) != 0 {
		t.Errorf("folder extension without VendorExtensions: %v", op.Extensions)
	}

	withSettings(t, Settings{VendorExtensions: true}, "example.com/shop")
	s := generateEnumSchemaFromEntry(entries, "string")
	for _, key := range []string{"x-enum-varnames", "x-enum-comments", "x-apifox-enum"} {
		if s.Extensions[key] == nil {
			t.Errorf("enum extension %s missing: %v", key, s.Extensions)
		}
	}
	op = buildOperation(&Method{Name: "Get", ServiceName: "pet", Folder: "Pets"}, newDoc())
	if op.Extensions["x-apifox-folder"] != "Pets" {
		t.Errorf("operation extensions = %v", op.Extensions)
	}
}

func TestExternalDocsFromTheDocAnnotation(t *testing.T) {
	withSettings(t, Settings{}, "example.com/shop")
	rec := captureLogs(t)

	got := buildExternalDocs(&Method{Name: "Get", Doc: "@doc: Pet guide: https://example.com/guide"}, nil)
	if got == nil || got.Description != "Pet guide" || got.URL != "https://example.com/guide" {
		t.Fatalf("external docs = %+v", got)
	}
	if buildExternalDocs(&Method{Name: "Get", Doc: "No annotation here."}, nil) != nil {
		t.Fatal("no annotation, no external docs")
	}
	if len(rec.records) != 0 {
		t.Fatalf("diagnostics = %v", rec.records)
	}

	// No colon: nothing to split, so nothing is documented, and the author hears of it.
	if got := buildExternalDocs(&Method{Name: "Get", Doc: "@doc: just words"}, nil); got != nil {
		t.Fatalf("external docs = %+v", got)
	}
	if len(rec.records) != 1 || rec.records[0].Level.String() != "WARN" {
		t.Fatalf("diagnostics = %v", rec.records)
	}
}

func TestOperationTags(t *testing.T) {
	method := &Method{Name: "Get", ServiceName: "pet", Doc: "Get a pet.\n@tags: public, reading"}

	withSettings(t, Settings{}, "example.com/shop")
	got := buildOperation(method, newDoc()).Tags
	if want := []string{"Pet", "Public", "Reading"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %v, want %v: the service, then @tags, no empty tag for the missing @permission", got, want)
	}

	// An operation with neither annotation has the tag of its service only.
	bare := &Method{Name: "Get", ServiceName: "pet", Doc: "Get a pet."}
	if got := buildOperation(bare, newDoc()).Tags; !reflect.DeepEqual(got, []string{"Pet"}) {
		t.Errorf("bare operation tags = %v", got)
	}

	// Documents generated before the option existed have the empty tag.
	withSettings(t, Settings{KeepEmptyTags: true}, "example.com/shop")
	if got := buildOperation(bare, newDoc()).Tags; !reflect.DeepEqual(got, []string{"", "Pet"}) {
		t.Errorf("tags with KeepEmptyTags = %v", got)
	}
	// Repeated tags are listed once, in order.
	dup := &Method{Name: "Get", ServiceName: "pet", Doc: "Get a pet.\n@tags: public\n@permission: public"}
	if got := buildOperation(dup, newDoc()).Tags; !reflect.DeepEqual(got, []string{"Pet", "Public"}) {
		t.Errorf("duplicate tags = %v", got)
	}
}

func TestQueryParametersHaveTheirType(t *testing.T) {
	withSettings(t, Settings{QueryString: map[string][]QueryField{
		"example.com.shop.protocol.ListReq": {
			{Name: "q"},
			{Name: "limit", Type: "integer", Required: true},
			{Name: "ratio", Type: "number"},
			{Name: "all", Type: "boolean"},
			{Name: "odd", Type: "unknown"},
		},
	}}, "example.com/shop")
	method := &Method{Name: "List", Params: []ParamSpec{{In: "body", Types: []*TypeDescriptor{{FullKey: "example.com.shop.protocol.ListReq"}}}}}

	got := map[string]string{}
	required := map[string]bool{}
	for _, p := range buildParameters(method, newDoc()) {
		got[p.Value.Name] = p.Value.Schema.Value.Type.Slice()[0]
		required[p.Value.Name] = p.Value.Required
	}
	want := map[string]string{"q": "string", "limit": "integer", "ratio": "number", "all": "boolean", "odd": "string"}
	for name, typ := range want {
		if got[name] != typ {
			t.Errorf("parameter %s has type %q, want %q", name, got[name], typ)
		}
	}
	if !required["limit"] || required["q"] {
		t.Errorf("required = %v", required)
	}
}

// A binary response lists the statuses of its errors, and an operation can
// annotate one of them with @response. The annotation says more than the list, so
// it wins, and it wins every time: the result must not depend on the order in
// which a map happens to be visited.
func TestAnnotatedErrorOfABinaryResponseWinsEveryTime(t *testing.T) {
	withSettings(t, Settings{
		BinaryResponses: map[string]BinaryResponse{
			"example.com.shop.protocol.PhotoResp": {
				Description:  "The photo",
				ContentTypes: []string{"image/png"},
				Errors:       map[string]string{"404": "No photo", "503": "Storage is down"},
			},
		},
		Errors:      mustCatalog(t, shopErrors),
		ErrorPrefix: "example.com.shop.errors.",
	}, "example.com/shop")

	for i := 0; i < 200; i++ {
		r := responsesOf(t, "",
			ResponseSpec{Code: "200", DataType: &TypeDescriptor{FullKey: "example.com.shop.protocol.PhotoResp"}},
			ResponseSpec{Code: "404", Description: "The pet has no photo", DataType: &TypeDescriptor{FullKey: "example.com.shop.errors.NotFoundErr"}},
		)
		notFound := r.Value("404")
		if notFound == nil || notFound.Value.Content == nil {
			t.Fatalf("run %d: the annotated 404 lost to the binary response's list: %+v", i, notFound)
		}
		if got := *notFound.Value.Description; got != "The pet has no photo" {
			t.Fatalf("run %d: 404 description = %q", i, got)
		}
		if unavailable := r.Value("503"); unavailable == nil || *unavailable.Value.Description != "Storage is down" {
			t.Fatalf("run %d: the listed 503 must stay: %+v", i, unavailable)
		}
	}
}
