package engine

import (
	"go/ast"
	"go/types"
	"regexp"
	"strings"

	"github.com/Danceiny/docgen/internal/suggest"
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

// warnAboutMalformedParams says so for each @param annotation that is not well
// formed: it describes nothing, and the parameter it was meant for is read as a
// JSON body without a word.
func (m *Method) warnAboutMalformedParams() {
	for _, line := range strings.Split(m.Doc, "\n") {
		if _, problem, ok := parseParamForm(line); problem != "" && !ok {
			warnAt("a @param annotation is not well formed, so it is ignored: "+problem,
				"annotation", strings.TrimSpace(line), "method", m.Name, "at", m.Pos)
		}
	}
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
			warnAt("@param names a parameter that the method does not have, ignoring it; name a Go parameter, or param1, param2 and so on by position",
				"method", m.Name, "param", name, "at", m.Pos)
		}
	}
}

// knownAnnotations are the annotations of a method that docgen reads.
var knownAnnotations = []string{"apidoc", "autowire", "desc", "doc", "folder", "generic", "headerType", "method", "param", "path", "permission", "response", "tags"}

// annotationLine matches a line that starts an annotation: @name:
var annotationLine = regexp.MustCompile(`^@([A-Za-z][A-Za-z0-9]*):`)

// warnAboutMistypedAnnotations reports an annotation that is not one docgen
// reads but is one letter away from one that it does, @respone or @Response.
// Comments carry annotations for other readers too (a router has its own), so an
// annotation that is not close to any known one is left alone.
func (m *Method) warnAboutMistypedAnnotations() {
	for _, line := range strings.Split(m.Doc, "\n") {
		match := annotationLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		name := match[1]
		known := false
		for _, k := range knownAnnotations {
			if name == k {
				known = true
			}
		}
		if known {
			continue
		}
		// One edit, not two: @auth is two from @path and is somebody else's.
		guess := suggest.ClosestWithin(name, knownAnnotations, 1)
		if guess == "" {
			guess = suggest.ClosestWithin(strings.ToLower(name), knownAnnotations, 1)
		}
		if guess != "" {
			warnAt("annotation is not one docgen reads; it is ignored", "annotation", "@"+name, "didYouMean", "@"+guess, "method", m.Name, "at", m.Pos)
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
