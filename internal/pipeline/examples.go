package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// checkExamples fails when the example or the default of a schema is not a value
// that the schema allows, with what the schema refers to followed. A field of a
// type of the module has been checked when it was read, and a value its type does
// not allow is left out of the document with a warning; the values that no field
// has, those of an overlay and of the configuration, are what this looks at. A
// document with such a value is one that other tools refuse, and the error says
// where it is.
//
// It has to be done before the references are cut (see cutReferences). Every
// schema is looked at once.
func checkExamples(doc *openapi3.T) error {
	seen := map[*openapi3.Schema]bool{}
	var walk func(where string, ref *openapi3.SchemaRef) error
	walk = func(where string, ref *openapi3.SchemaRef) error {
		if ref == nil || ref.Ref != "" || ref.Value == nil || seen[ref.Value] {
			return nil
		}
		schema := ref.Value
		seen[schema] = true
		for _, value := range []struct {
			kind string
			what any
		}{{"example", schema.Example}, {"default", schema.Default}} {
			if value.what == nil {
				continue
			}
			if err := schema.VisitJSON(value.what); err != nil {
				at := "the schema"
				if where != "" {
					at = strings.TrimPrefix(where, ".")
				}
				written, _ := json.Marshal(value.what)
				return fmt.Errorf("the %s of %s, %s, is not a value that the schema allows: %s", value.kind, at, written, firstLine(err.Error()))
			}
		}
		for _, edge := range schemaChildren(schema) {
			if err := walk(where+"."+edge.name, edge.ref); err != nil {
				return err
			}
		}
		return nil
	}
	return documentSchemas(doc, func(_ string, ref *openapi3.SchemaRef) error {
		return walk("", ref)
	})
}

// firstLine is the first line of a message, which is where an error says what is wrong.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
