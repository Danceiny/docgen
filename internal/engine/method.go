package engine

import (
	"go/ast"
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
	// Hidden is set by "@apidoc: -": the method is neither routed nor documented.
	Hidden bool
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

	// 3. apply the overrides from the comments
	if commentSpec := m.parseParamComment(m.Doc, i); commentSpec != nil {
		param = *commentSpec
	}

	return param, true
}
