package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

// A field's description is its comment above and its comment on the same line,
// joined; a field with neither has none.
func TestExtractDescriptionOfStructFields(t *testing.T) {
	src := "package protocol\n" +
		"type ButtonEntrance struct {\n" +
		"\tID       string `json:\"id\"`       // agreed with the front end, which acts on it\n" +
		"\tDisabled bool   `json:\"disabled\"` // visible to the user but not clickable\n" +
		"\t// hint text\n" +
		"\tHint string `json:\"hint\"`\n" +
		"\t// above\n" +
		"\tBoth string `json:\"both\"` // beside\n" +
		"\tNone string `json:\"none\"`\n" +
		"}\n"
	node, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	ast.Inspect(node, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "ButtonEntrance" {
			return true
		}
		for _, field := range ts.Type.(*ast.StructType).Fields.List {
			got[field.Names[0].Name] = extractDescription(field.Doc, field.Comment)
		}
		return false
	})

	want := map[string]string{
		"ID":       "agreed with the front end, which acts on it",
		"Disabled": "visible to the user but not clickable",
		"Hint":     "hint text",
		"Both":     "above\nbeside",
		"None":     "",
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("description of %s = %q, want %q", name, got[name], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("fields = %v", got)
	}
}

func TestExtractTagMultiLine(t *testing.T) {
	tests := []struct {
		name     string
		comments string
		tagName  string
		expected string
	}{
		{
			name:     "single line desc",
			comments: `@desc: Cancel an order with reliable state management.`,
			tagName:  "desc",
			expected: "Cancel an order with reliable state management.",
		},
		{
			name: "multi line desc",
			comments: `@desc: Cancel an order with reliable state management.

This endpoint handles order cancellation with state validation,
audit logging and retries. It supports both full and
partial cancellation based on the merchant's policy and the order state.

**Prerequisites:**
- **Order ID**: Must be provided to identify the target order
- **JWT Authentication**: Bearer token required in Authorization header
- **Valid Order State**: Order must be in a cancellable state`,
			tagName: "desc",
			expected: `Cancel an order with reliable state management.

This endpoint handles order cancellation with state validation,
audit logging and retries. It supports both full and
partial cancellation based on the merchant's policy and the order state.

**Prerequisites:**
- **Order ID**: Must be provided to identify the target order
- **JWT Authentication**: Bearer token required in Authorization header
- **Valid Order State**: Order must be in a cancellable state`,
		},
		{
			name: "multi line desc with other tags",
			comments: `@desc: Cancel an order with reliable state management.

This endpoint handles order cancellation with state validation,
audit logging and retries.

@method: POST
@response: 429,RateLimitErr,Too many requests`,
			tagName: "desc",
			expected: `Cancel an order with reliable state management.

This endpoint handles order cancellation with state validation,
audit logging and retries.`,
		},
		{
			name: "multi line desc with empty lines",
			comments: `@desc: Cancel an order with reliable state management.

This endpoint handles order cancellation with state validation,

audit logging and retries.`,
			tagName: "desc",
			expected: `Cancel an order with reliable state management.

This endpoint handles order cancellation with state validation,

audit logging and retries.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTag(tt.comments, tt.tagName)
			if result != tt.expected {
				t.Errorf("ExtractTag() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractTagMetadataDoesNotConsumeContinuation(t *testing.T) {
	comments := "@permission:read\n- |4- invalid continuation"
	if got := ExtractTag(comments, "permission"); got != "read" {
		t.Fatalf("permission tag consumed continuation: %q", got)
	}
}

func TestExtractTagPathDoesNotConsumeContinuation(t *testing.T) {
	comments := []string{"@path: /api/example", "continuation must not become path", "@method: POST"}
	if got := ExtractTagFromComments(comments, "path"); got != "/api/example" {
		t.Fatalf("path tag consumed continuation: %q", got)
	}
}

// A @param line is "<in> <name> <type> <required> [<description>]"; the
// description is optional, and a line that is too short is not an annotation.
func TestParamCommentWithAndWithoutADescription(t *testing.T) {
	m := &Method{}
	doc := "@param:query limit int required \"how many\"\n" +
		"@param:query offset int required\n" +
		"@param:query short int\n"

	withDescription := m.parseParamComment(doc, "limit")
	if withDescription == nil || withDescription.In != "query" || !withDescription.Required || withDescription.Description != "how many" {
		t.Fatalf("limit = %+v", withDescription)
	}
	withoutDescription := m.parseParamComment(doc, "offset")
	if withoutDescription == nil || withoutDescription.Description != "" || !withoutDescription.Required {
		t.Fatalf("offset = %+v", withoutDescription)
	}
	if got := m.parseParamComment(doc, "short"); got != nil {
		t.Fatalf("a line with fewer than four fields is no annotation: %+v", got)
	}
	if got := m.parseParamComment(doc, "absent"); got != nil {
		t.Fatalf("absent = %+v", got)
	}
}

// The types an annotation lists are separated by a comma, with a space after it or
// not; what follows a semicolon goes with them, and the lines after the annotation
// are not part of it. Documents that keep the old way of writing read the line as
// it is.
func TestTheValuesOfAnAnnotationAreTrimmedAndEndAtTheSemicolon(t *testing.T) {
	src := "package p\n" +
		"type T struct {\n" +
		"\t// @generic: Product, Order\n" +
		"\tSpace any\n" +
		"\t// @generic: Product,Order\n" +
		"\tNoSpace any\n" +
		"\t// @generic: Product,  Order ,Refund; the data of the answer\n" +
		"\tSemicolon any\n" +
		"\t// @generic: Product, Order\n" +
		"\t// and more of what is said about it\n" +
		"\tMore any\n" +
		"\t// @generic: ,Product,,\n" +
		"\tEmpties any\n" +
		"\t// @autowire: true; the types that implement it\n" +
		"\tUnion any\n" +
		"}\n"
	node, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]*ast.Field{}
	ast.Inspect(node, func(n ast.Node) bool {
		if st, ok := n.(*ast.StructType); ok {
			for _, field := range st.Fields.List {
				fields[field.Names[0].Name] = field
			}
		}
		return true
	})

	want := map[string][]string{
		"Space":     {"Product", "Order"},
		"NoSpace":   {"Product", "Order"},
		"Semicolon": {"Product", "Order", "Refund"},
		"More":      {"Product", "Order"},
		"Empties":   {"Product"},
	}
	for name, values := range want {
		got := extractTagValueFromDocComments(fields[name].Doc, fields[name].Comment, "generic")
		if !reflect.DeepEqual(got, values) {
			t.Errorf("%s: %q, want %q", name, got, values)
		}
	}
	if got := extractTagValueFromDocComments(fields["Union"].Doc, nil, "autowire"); !reflect.DeepEqual(got, []string{"true"}) {
		t.Errorf("autowire: %q, want [true]", got)
	}

	withSettings(t, Settings{CompatLegacyOutput: true}, "example.com/p")
	if got := extractTagValueFromDocComments(fields["Space"].Doc, nil, "generic"); !reflect.DeepEqual(got, []string{"Product", " Order"}) {
		t.Errorf("legacy: %q, want the line as it is written", got)
	}
}
