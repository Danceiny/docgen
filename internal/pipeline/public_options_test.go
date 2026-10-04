package pipeline

import (
	"reflect"
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/config"
)

func TestPublicSettingsDefaults(t *testing.T) {
	got := publicSettings(nil)
	if got.tag != "public" || len(got.stripTagsContaining) != 0 || got.errorsLast {
		t.Fatalf("defaults = %+v", got)
	}
	got = publicSettings(&config.Public{StripTagsContaining: []string{".com"}, ErrorsLast: true})
	if got.tag != "public" || !got.errorsLast || !reflect.DeepEqual(got.stripTagsContaining, []string{".com"}) {
		t.Fatalf("an unset tag keeps the default: %+v", got)
	}
	if got := publicSettings(&config.Public{Tag: "external"}); got.tag != "external" {
		t.Fatalf("tag = %q", got.tag)
	}
}

func docWithOperations(tagsByPath map[string][]string) *openapi3.T {
	doc := &openapi3.T{Paths: &openapi3.Paths{}, Components: &openapi3.Components{Schemas: openapi3.Schemas{}}}
	for path, tags := range tagsByPath {
		doc.Paths.Set(path, &openapi3.PathItem{Post: &openapi3.Operation{Tags: tags, Responses: openapi3.NewResponses()}})
	}
	return doc
}

func TestOnlyOperationsWithThePublicTagStay(t *testing.T) {
	build := func() *openapi3.T {
		return docWithOperations(map[string][]string{
			"/api/a": {"Pet", "openapi"},
			"/api/b": {"Pet", "OpenAPI"}, // the comparison ignores case
			"/api/c": {"Pet"},
			"/api/d": {"external"},
			"/api/e": nil,
		})
	}

	doc := build()
	filterPublicPaths(doc, "openapi")
	if got := sortedPathKeys(doc); !reflect.DeepEqual(got, []string{"/api/a", "/api/b"}) {
		t.Fatalf("with the default tag: %v", got)
	}

	doc = build()
	filterPublicPaths(doc, "external")
	if got := sortedPathKeys(doc); !reflect.DeepEqual(got, []string{"/api/d"}) {
		t.Fatalf("with a configured tag: %v", got)
	}
}

func sortedPathKeys(doc *openapi3.T) []string {
	var keys []string
	for k := range doc.Paths.Map() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestTagsContainingAStripStringAreRemovedExceptThePublicTag(t *testing.T) {
	doc := docWithOperations(map[string][]string{
		"/api/a": {"Pet", "intranet.example.COM", "openapi", "Billing"},
	})
	optimizeTags(doc, publicOptions{tag: "openapi", stripTagsContaining: []string{".com"}})
	if got := doc.Paths.Value("/api/a").Post.Tags; !reflect.DeepEqual(got, []string{"Pet", "openapi", "Billing"}) {
		t.Fatalf("tags = %v", got)
	}

	// A public tag that itself contains a strip string is kept.
	doc = docWithOperations(map[string][]string{"/api/a": {"public.com", "other.com"}})
	optimizeTags(doc, publicOptions{tag: "public.com", stripTagsContaining: []string{".com"}})
	if got := doc.Paths.Value("/api/a").Post.Tags; !reflect.DeepEqual(got, []string{"public.com"}) {
		t.Fatalf("tags = %v", got)
	}

	// Nothing to strip, nothing stripped.
	doc = docWithOperations(map[string][]string{"/api/a": {"x.com"}})
	optimizeTags(doc, publicOptions{tag: "openapi"})
	if got := doc.Paths.Value("/api/a").Post.Tags; !reflect.DeepEqual(got, []string{"x.com"}) {
		t.Fatalf("tags = %v", got)
	}
}

func TestErrorSchemasGoAfterTheOthers(t *testing.T) {
	plain := func(title string) *openapi3.SchemaRef {
		return &openapi3.SchemaRef{Value: &openapi3.Schema{Title: title}}
	}
	byCode := &openapi3.SchemaRef{Value: &openapi3.Schema{Properties: openapi3.Schemas{
		"code": {Value: &openapi3.Schema{Enum: []any{float64(404)}}},
	}}}

	in := []*openapi3.SchemaRef{plain("NotFoundErr"), plain("Pet"), byCode, plain("Order"), {Ref: "#/components/schemas/Other"}, plain("TIMEOUTERR")}
	got := reorderAnyOfAllOf(in)
	var titles []string
	for _, s := range got {
		switch {
		case s.Value == nil:
			titles = append(titles, "(ref)")
		case s.Value.Title == "":
			titles = append(titles, "(code enum)")
		default:
			titles = append(titles, s.Value.Title)
		}
	}
	want := []string{"Pet", "Order", "(ref)", "NotFoundErr", "(code enum)", "TIMEOUTERR"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("order = %v, want %v", titles, want)
	}

	if one := []*openapi3.SchemaRef{plain("NotFoundErr")}; !reflect.DeepEqual(reorderAnyOfAllOf(one), one) {
		t.Fatal("a single alternative stays")
	}
}
