package engine

import (
	"go/ast"
	"go/types"
)

type Method struct {
	Name           string
	APIPath        string // parsed from doc or converted from name
	Doc            string
	RequestType    *TypeDescriptor // the type of the request parameter (such as "example.com.petstore.pet.protocol.CreatePetReq")
	ResponseType   *TypeDescriptor // the type of the response data (such as "example.com.petstore.pet.protocol.CreatePetResp")
	Params         []ParamSpec     // the descriptions of the parameters
	Responses      []ResponseSpec  // the descriptions of the responses
	Source         *ast.FuncDecl
	ImportAlias    map[string]string
	CurrentPkgPath string
	ServiceName    string
	HeaderType     string // the header type that was given, such as "AuthHeader"
	Folder         string // the value of the @folder annotation
	Pos            string // where the method is declared, as file:line, for diagnostics
	Info           *types.Info
	// Hidden is set by "@apidoc: -": the method is neither routed nor documented.
	Hidden bool
}

// warnUnmatchedParamAnnotations says so when a @param annotation names a
// parameter that the method does not have: it describes nothing, and the
// parameter it was meant for is read as a JSON body.
func (m *Method) warnUnmatchedParamAnnotations(fields []*ast.Field) {
	known := map[string]bool{}
	for i, field := range fields {
		known[generateParamName(i)] = true
		for _, name := range field.Names {
			known[name.Name] = true
		}
	}
	for _, name := range paramAnnotationNames(m.Doc) {
		if !known[name] {
			Logger().Warn("@param names a parameter that the method does not have, ignoring it; name a Go parameter, or param1, param2 and so on by position",
				"method", m.Name, "param", name, "at", m.Pos)
		}
	}
}

func (m *Method) parseParam(i int, field *ast.Field) (ParamSpec, bool) {
	paramType := m.parseTypeExpr(field.Type)

	// 1. A struct-typed parameter is the request body, kept as one typed value.
	// Request types declared next to the service, outside the packages whose
	// types become schemas, are intentionally kept as typed bodies instead of
	// being dropped.
	if isStructType(field.Type, m.ImportAlias) {
		return ParamSpec{
			Name:        generateParamName(i),
			Types:       []*TypeDescriptor{paramType},
			Required:    true,
			Description: extractFieldDescription(field),
			In:          "body",
		}, true
	}

	// 2. handle parameters of basic types
	param := ParamSpec{
		Name:        generateParamName(i),
		Types:       []*TypeDescriptor{paramType},
		Required:    true, // the default body parameter is required
		Description: extractFieldDescription(field),
		In:          "body",
	}

	// 3. apply the overrides from the comments: a @param annotation names the Go
	// parameter, or its position as param1, param2 and so on
	identifiers := []any{generateParamName(i)}
	for _, name := range field.Names {
		identifiers = append([]any{name.Name}, identifiers...)
	}
	for _, id := range identifiers {
		if commentSpec := m.parseParamComment(m.Doc, id); commentSpec != nil {
			param = *commentSpec
			break
		}
	}

	return param, true
}
