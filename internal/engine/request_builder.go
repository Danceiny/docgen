package engine

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// IsMultipartRequest reports whether the request type is configured as a
// multipart/form-data upload (Settings.Multipart).
func IsMultipartRequest(typeFullKey string) bool {
	_, ok := settings.Multipart[typeFullKey]
	return ok
}

// IsQueryStringRequest reports whether the request type is configured as bound
// from the URL query string (Settings.QueryString).
func IsQueryStringRequest(typeFullKey string) bool {
	_, ok := settings.QueryString[typeFullKey]
	return ok
}

func buildRequestBody(method *Method, doc *openapi3.T) *openapi3.RequestBodyRef {
	if IsQueryStringRequest(firstBodyTypeKey(method)) {
		return nil // Query-string DTOs bind from the URL query; no request body.
	}
	// 0) Multipart (request.multipart in the configuration): emit
	//    multipart/form-data with a binary "file" part plus scalar fields.
	//    Method.RequestType is not filled in, so the request type is found
	//    through the TypeDescriptor.FullKey of the first body param.
	if bodyTypeKey := firstBodyTypeKey(method); bodyTypeKey != "" && IsMultipartRequest(bodyTypeKey) {
		return buildMultipartRequestBody(bodyTypeKey)
	}
	// 1) JSON (default).
	for _, p := range method.Params {
		switch p.In {
		case "body":
			schema := getContractSchemaRef(method.APIPath, p.Types[0], doc)
			return &openapi3.RequestBodyRef{
				Value: openapi3.NewRequestBody().
					WithDescription(p.Description).
					WithRequired(true).
					WithContent(openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: schema, // use the SchemaRef as it is
						},
					}),
			}
		case "header":
			// header parameters
			param := openapi3.NewHeaderParameter(p.Name)
			param.Description = p.Description
			param.Required = p.Required
			param.Schema = GetSchemaRef(p.Types[0])
		}
	}
	return nil
}

// firstBodyTypeKey returns the FullKey of the first body parameter, or "" if
// the method has no body param. The request types that are configured as
// multipart or as a query string are looked up by it, since Method.RequestType
// is not filled in.
func firstBodyTypeKey(method *Method) string {
	if method == nil {
		return ""
	}
	for _, p := range method.Params {
		if p.In != "body" || len(p.Types) == 0 {
			continue
		}
		if p.Types[0] == nil {
			continue
		}
		return p.Types[0].FullKey
	}
	return ""
}

// buildMultipartRequestBody turns the wire fields that a multipart request allows into the
// multipart/form-data form of OpenAPI. The result is explicit: 1) the file field exists and its schema is
// type=string/format=binary; 2) every scalar field appears in properties under the same wire name;
// 3) required combines decoder and service requirements from the allowlist.
// When the DTO type is not registered in Settings.Multipart, buildRequestBody has already
// sent the request down the JSON path; this function does not check that a second time.
func buildMultipartRequestBody(typeFullKey string) *openapi3.RequestBodyRef {
	fields, ok := settings.Multipart[typeFullKey]
	if !ok {
		return nil
	}
	properties := make(map[string]*openapi3.SchemaRef, len(fields))
	required := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.Required {
			required = append(required, f.Name)
		}
		switch f.Kind {
		case "file":
			binary := openapi3.NewStringSchema()
			binary.Format = "binary"
			properties[f.Name] = openapi3.NewSchemaRef("", binary)
		case "scalar":
			var sch *openapi3.Schema
			switch f.ScalarTo {
			case "integer":
				sch = openapi3.NewInt64Schema()
			default:
				sch = openapi3.NewStringSchema()
			}
			properties[f.Name] = openapi3.NewSchemaRef("", sch)
		default:
			// The configuration is validated, so an unknown kind is a programmer error.
			panic(fmt.Sprintf("engine: unknown multipart field kind %q for %s", f.Kind, f.Name))
		}
	}
	wrapper := openapi3.NewObjectSchema()
	wrapper.Properties = properties
	wrapper.Required = required
	return &openapi3.RequestBodyRef{
		Value: openapi3.NewRequestBody().
			WithRequired(true).
			WithContent(openapi3.Content{
				"multipart/form-data": &openapi3.MediaType{
					Schema: openapi3.NewSchemaRef("", wrapper),
				},
			}),
	}
}

// getContractSchemaRef never emits an unresolved component reference for
// runtime-only bindings (callbacks, interfaces, and opaque service objects:
// request.runtime_only in the configuration). Such inputs remain visible as
// routes but are explicitly unsatisfiable as JSON, instead of masquerading as an
// unconstrained object.
func getContractSchemaRef(path string, desc *TypeDescriptor, doc *openapi3.T) *openapi3.SchemaRef {
	if desc != nil {
		_, runtimeOnly := settings.RuntimeOnly[RuntimeInputKey{Path: path, TypeKey: desc.FullKey}]
		if runtimeOnly {
			return runtimeOnlySchemaRef(path, desc.FullKey)
		}
	}
	if desc != nil && isBasicType(desc.FullKey) {
		return GetSchemaRef(desc)
	}
	if desc != nil && doc != nil && doc.Components != nil {
		if _, ok := doc.Components.Schemas[desc.FullKey]; ok {
			return GetSchemaRef(desc)
		}
	}
	// A missing component is a generator defect. Preserve its exact reference
	// so GenerateYAML's normalization pass fails closed instead of silently
	// changing the request contract to not: {}.
	if desc == nil {
		return GetSchemaRef(&TypeDescriptor{FullKey: "invalid"})
	}
	return GetSchemaRef(desc)
}

type RuntimeInputKey struct {
	Path    string
	TypeKey string
}

func runtimeOnlySchemaRef(path, typeKey string) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: &openapi3.Schema{
		Description: "runtime-unserializable input; no JSON value can satisfy this binding",
		Not:         openapi3.NewSchemaRef("", &openapi3.Schema{}),
		Extensions: map[string]interface{}{
			"x-runtime-unserializable": true,
			"x-runtime-path":           path,
			"x-runtime-type-key":       typeKey,
		},
	}}
}

// parseSingleType parses a single complex type expression.
// Supported forms: []*[]*pkg.Type, [][]int, ***model.Data and so on.
func (m *Method) parseSingleType(typeStr string) *TypeDescriptor {
	if v, ok := typeDescCache.Load(typeStr); ok {
		return v.(*TypeDescriptor)
	}

	// step 1: take the pointer and array markers
	ptrCount, dims := parseTypeModifiers(typeStr)

	// step 2: parse the name of the base type
	baseType := m.parseBaseType(typeStr)

	out := &TypeDescriptor{
		FullKey:    baseType,
		IsPointer:  ptrCount > 0,
		Dimensions: dims,
	}
	typeDescCache.Store(typeStr, out)
	return out
}
func (m *Method) parseBaseType(typeStr string) string {
	// step 1: remove all modifiers (pointers and arrays)
	base := strings.ReplaceAll(typeStr, "*", "")
	base = strings.ReplaceAll(base, "[]", "")

	// a basic type has no package
	if getBasicTypeSchema(base) != nil {
		return base
	}

	// step 2: handle import aliases (such as model.User → example.com.pet.model.User)
	if parts := strings.Split(base, "."); len(parts) > 1 {
		if realPkg, ok := m.ImportAlias[parts[0]]; ok {
			// convert the alias to the real package path
			formattedPkg := formatComponentKey(realPkg)
			base = fmt.Sprintf("%s.%s", formattedPkg, strings.Join(parts[1:], "."))
		}
	}

	// step 3: complete the package path of local types
	if !strings.Contains(base, ".") {
		base = fmt.Sprintf("%s.%s", m.CurrentPkgPath, base)
	}

	return formatComponentKey(base)
}

// a regular expression that matches the type modifiers
var (
	typeModifierRe = regexp.MustCompile(`^(\*|\[\])+`)
)

// parseTypeModifiers parses the pointer and array modifiers of a type written as text.
func parseTypeModifiers(typeStr string) (ptrCount, dims int) {
	matches := typeModifierRe.FindAllStringSubmatch(typeStr, -1)
	if len(matches) == 0 {
		return 0, 0
	}

	modifiers := matches[0][0]
	for _, c := range modifiers {
		switch c {
		case '*':
			ptrCount++
		case '[': // a [ adds one array dimension
			dims++
		}
	}
	return
}

// queryValueSchema is the schema of a query parameter of the given type, a string
// when the type is not one of the others.
func queryValueSchema(typ string) *openapi3.Schema {
	switch typ {
	case "integer":
		return openapi3.NewInt64Schema()
	case "number":
		return openapi3.NewFloat64Schema()
	case "boolean":
		return openapi3.NewBoolSchema()
	}
	return openapi3.NewStringSchema()
}

func buildParameters(method *Method, doc *openapi3.T) openapi3.Parameters {
	var params openapi3.Parameters
	// Query-string DTOs (request.query in the configuration): emit the
	// configured fields as query parameters.
	if fields, ok := settings.QueryString[firstBodyTypeKey(method)]; ok {
		for _, f := range fields {
			param := openapi3.NewQueryParameter(f.Name)
			param.Required = f.Required
			param.Description = f.Description
			param.Schema = queryValueSchema(f.Type).NewRef()
			params = append(params, &openapi3.ParameterRef{Value: param})
		}
	}

	for _, p := range method.Params {
		// skip the body parameter (it is handled by RequestBody)
		if p.In == "body" {
			continue
		}
		// Query-string DTO routes skip the header parameter: anonymous clients
		// call them with a plain GET without auth headers.
		if p.In == "header" && IsQueryStringRequest(firstBodyTypeKey(method)) {
			continue
		}

		// create a Parameter of the kind that matches the location of the parameter
		var param *openapi3.Parameter
		switch p.In {
		case "path":
			param = openapi3.NewPathParameter(p.Name)
		case "query":
			param = openapi3.NewQueryParameter(p.Name)
		case "header":
			param = openapi3.NewHeaderParameter(p.Name)
		case "cookie":
			param = openapi3.NewCookieParameter(p.Name)
		default:
			continue // ignore unknown kinds
		}

		// set the properties they have in common
		param.Description = p.Description
		param.Required = p.Required
		param.Schema = GetSchemaRef(p.Types[0])

		params = append(params, &openapi3.ParameterRef{
			Value: param,
		})
	}

	return params
}
