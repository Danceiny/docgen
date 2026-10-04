package engine

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/errcat"
)

func buildResponses(method *Method, doc *openapi3.T) *openapi3.Responses {
	responses := openapi3.NewResponses()
	for status, description := range settings.DefaultStatuses {
		responses.Set(status, &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription(description)})
	}
	// group the responses by status code
	responsesByCode := make(map[string][]ResponseSpec)
	sort.Slice(method.Responses, func(i, j int) bool {
		return method.Responses[i].Code < method.Responses[j].Code
	})
	for _, resp := range method.Responses {
		responsesByCode[resp.Code] = append(responsesByCode[resp.Code], resp)
	}

	// A response with a status code is written in the order of the codes, so that
	// when two of them set the same status the later one wins every time: the
	// 200 of a binary response sets the statuses of its errors, and an @response
	// annotation of the operation, which has a higher code, replaces them.
	codes := make([]string, 0, len(responsesByCode))
	for code := range responsesByCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		respSpecs := responsesByCode[code]
		if len(respSpecs) == 0 {
			continue
		}
		// A response type that writes raw bytes (an image, a file) instead of the
		// normal JSON envelope is documented with its configured media types.
		if code == "200" && len(respSpecs) == 1 && respSpecs[0].DataType != nil {
			if bin, ok := settings.BinaryResponses[respSpecs[0].DataType.FullKey]; ok {
				responses.Set(code, &openapi3.ResponseRef{Value: &openapi3.Response{
					Description: stringPtr(bin.Description),
					Content:     openapi3.NewContentWithSchema(openapi3.NewStringSchema().WithFormat("binary"), bin.ContentTypes),
				}})
				for status, description := range bin.Errors {
					responses.Set(status, &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription(description)})
				}
				continue
			}
		}
		// build the schema of that status code
		var content *openapi3.SchemaRef
		if len(respSpecs) == 1 {
			// a single response: use it as it is
			content = buildSuccessDataSchema(respSpecs[0], doc)
		} else {
			// several responses: use oneOf
			union := &openapi3.Schema{}
			for _, resp := range respSpecs {
				if v := buildSuccessDataSchema(resp, doc); v != nil {
					union.OneOf = append(union.OneOf, v)
				}
			}
			content = &openapi3.SchemaRef{Value: union}
		}

		// With an envelope the description of the response is the one of the
		// envelope; without one the schema is the data type itself, whose own
		// description describes the type, not the response.
		respDesc := ""
		if settings.Envelope != nil && content != nil && content.Value != nil {
			respDesc = content.Value.Description
		}
		if respDesc == "" {
			respDesc = respSpecs[0].Description
		}
		if respDesc == "" {
			respDesc = "OK"
		}

		response := &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: stringPtr(respDesc),
				Headers:     doc.Components.Headers,
			},
		}
		if content != nil {
			response.Value.Content = openapi3.NewContentWithSchemaRef(content, []string{"application/json"})
		}
		responses.Set(code, response)
	}
	// A response object must have at least one response. An operation with no
	// result to document and no default status answers 200 with no content.
	if responses.Len() == 0 {
		responses.Set("200", &openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("OK")})
	}
	return responses
}

// WithDescription sets the description of a schema and returns it.
func WithDescription(s *openapi3.Schema, desc string) *openapi3.Schema {
	s.Description = desc
	return s
}

// isBasicType reports whether all the types are basic types.
func isBasicType(types ...string) bool {
	for _, t := range types {
		if getBasicTypeSchema(t) != nil {
			continue
		}
		return false
	}
	return true
}

// getBasicTypeSchema returns the fixed schema of a basic type: the configured
// type map first, so that a module can restate the schema of a type the engine
// knows, then the Go types the engine knows.
func getBasicTypeSchema(name string) *openapi3.Schema {
	if s := settings.TypeMap[name]; s != nil {
		return s
	}
	if !settings.CompatLegacySchemaShapes {
		switch name {
		case "any":
			return anySchema()
		case "byte":
			return BasicTypeSchemas["uint8"] // a byte on its own is a number in JSON
		}
	}
	return BasicTypeSchemas[name]
}

// anySchema is the schema of any and of interface{}: the schema that has no
// type, which every JSON value is valid against. Documents generated before it
// was fixed describe any as an object and interface{} as a string, an integer
// or an object, which a client would refuse a boolean, a list or a fraction for.
func anySchema() *openapi3.Schema {
	return &openapi3.Schema{}
}

// dateTimeSchema is the schema of a time.Time: a date-time string.
func dateTimeSchema() *openapi3.Schema {
	s := openapi3.NewDateTimeSchema()
	s.Example = "2024-01-02T15:04:05Z"
	return s
}

// the table of basic types
var BasicTypeSchemas = map[string]*openapi3.Schema{
	"object":                   openapi3.NewObjectSchema(),
	"int":                      openapi3.NewIntegerSchema(),
	"uint8":                    openapi3.NewIntegerSchema(),
	"uint":                     openapi3.NewInt64Schema(),
	"uintptr":                  openapi3.NewInt64Schema(),
	"rune":                     openapi3.NewInt32Schema(),
	"float32":                  {Type: &openapi3.Types{openapi3.TypeNumber}, Format: "float"},
	"int8":                     openapi3.NewIntegerSchema(),
	"int16":                    openapi3.NewIntegerSchema(),
	"byte":                     openapi3.NewStringSchema(),
	"int32":                    openapi3.NewInt32Schema(),
	"uint32":                   openapi3.NewInt64Schema(),
	"uint16":                   openapi3.NewInt32Schema(),
	"uint64":                   openapi3.NewInt64Schema(),
	"int64":                    openapi3.NewInt64Schema(),
	"string":                   openapi3.NewStringSchema(),
	"bool":                     openapi3.NewBoolSchema(),
	"float64":                  openapi3.NewFloat64Schema(),
	"map.string.interface{}":   openapi3.NewObjectSchema(),
	"any":                      openapi3.NewObjectSchema(),
	"time.Duration":            openapi3.NewInt64Schema(), // the JSON of a Duration is its number of nanoseconds
	"time.Time":                dateTimeSchema(),
	"time":                     dateTimeSchema(),
	"error":                    WithDescription(openapi3.NewStringSchema(), `error`),
	"encoding.json.RawMessage": WithDescription(openapi3.NewStringSchema(), `JSON`),
}

// convertErrorToSchema is the body of a response with an error of the catalog: an
// object with its code and its message, named as the envelope names them, or
// "code" and "message" when there is no envelope.
func convertErrorToSchema(e errcat.Entry) *openapi3.SchemaRef {
	codeName, messageName := "code", "message"
	if env := settings.Envelope; env != nil {
		codeName, messageName = env.Code, env.Message
	}
	code := openapi3.NewIntegerSchema()
	code.Example = e.Code
	message := openapi3.NewStringSchema()
	message.Example = e.Message
	schema := openapi3.NewObjectSchema().
		WithProperty(codeName, code).
		WithProperty(messageName, message)
	schema.Title = e.Name
	return schema.NewRef()
}

func buildSuccessDataSchema(spec ResponseSpec, doc *openapi3.T) *openapi3.SchemaRef {
	v := spec.DataType.FullKey
	schema := searchSchemaFromDoc(doc, v)

	// is it an error of the error catalog? (judged by the FullKey)
	if strings.HasPrefix(v, errorPrefix()) {
		if e, ok := settings.Errors.Find(v); ok {
			// an error is returned as it is, not wrapped in the data field
			return convertErrorToSchema(e)
		}
	}

	env := settings.Envelope
	if env == nil {
		return inLists(unwrappedDataSchema(v, schema), spec.DataType.Dimensions)
	}

	schemaRef := openapi3.NewObjectSchema().
		WithProperty(env.Code, openapi3.NewIntegerSchema()).
		WithProperty(env.Message, openapi3.NewStringSchema())

	// set the title
	if schema != nil && schema.Value != nil && schema.Value.Title != "" {
		schemaRef.Title = schema.Value.Title
	}
	schemaRef.Description = spec.Description

	// is it a basic type?
	if baseSchema := getBasicTypeSchema(v); baseSchema != nil {
		schemaRef.WithPropertyRef(env.Data, inLists(&openapi3.SchemaRef{Value: baseSchema}, spec.DataType.Dimensions))
	} else if schema != nil {
		// a reference rather than the schema, to save space
		schemaRef.WithPropertyRef(env.Data, inLists(NewSchemaRefFromFullKey(v), spec.DataType.Dimensions))
	}

	if settings.VendorExtensions {
		// add OpenAPI extension properties that improve how the document is displayed
		extensions := map[string]interface{}{
			"x-apifox-orders": []string{env.Code, env.Message, env.Data},
		}

		// add the x-display-name extension property, which shows the name of the inner schema
		if schema != nil && schema.Value != nil && schema.Value.Title != "" {
			extensions["x-display-name"] = schema.Value.Title
		}

		// add the x-primary-property extension property, which says the main property is data
		extensions["x-primary-property"] = env.Data

		schemaRef.Extensions = extensions
	}
	return schemaRef.NewRef()
}

// inLists wraps a schema in as many lists as a type has dimensions: a result of
// []Product is a list of Products. A nil schema stays nil.
func inLists(schema *openapi3.SchemaRef, dimensions int) *openapi3.SchemaRef {
	if schema == nil || settings.CompatLegacyOperationTypes {
		return schema
	}
	for i := 0; i < dimensions; i++ {
		schema = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeArray}, Items: schema}}
	}
	return schema
}

// unwrappedDataSchema is the body of a response that has no envelope: the data
// type itself, as a basic schema or a reference to its component. It is nil
// when the type has no schema (a hidden type, or no result at all), and the
// response then has a description and no content.
func unwrappedDataSchema(fullKey string, component *openapi3.SchemaRef) *openapi3.SchemaRef {
	if baseSchema := getBasicTypeSchema(fullKey); baseSchema != nil {
		return &openapi3.SchemaRef{Value: baseSchema}
	}
	if component != nil {
		return NewSchemaRefFromFullKey(fullKey)
	}
	return nil
}
