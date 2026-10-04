package engine

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/Danceiny/docgen/route"

	"golang.org/x/tools/go/packages"
)

func (m *Method) parseParams(params *ast.FieldList) {
	m.Params = nil // reset the list of parameters
	if params == nil || len(params.List) == 0 {
		m.warnUnmatchedParamAnnotations(nil)
		return
	}

	// skip the context parameter
	startIndex := 0
	if isContextType(params.List[0].Type) {
		startIndex = 1
	}
	m.warnUnmatchedParamAnnotations(params.List[startIndex:])

	// go through all parameters (including the request body)
	for i, field := range params.List[startIndex:] {
		param, ok := m.parseParam(i, field)
		if !ok {
			continue
		}
		m.Params = append(m.Params, param)
	}
	// add a header parameter for the @header:type tag
	headerFullKey := headerTypeKey(m.HeaderType)
	if headerFullKey == "" {
		return // no header type is configured
	}

	m.Params = append(m.Params, ParamSpec{
		Name:        "Headers",
		In:          "header",
		Required:    true,
		Description: fmt.Sprintf("request headers (%s)", m.HeaderType),
		Types: []*TypeDescriptor{
			{
				FullKey:    headerFullKey,
				IsPointer:  false,
				Dimensions: 0,
			},
		},
	})
}

func isStructType(expr ast.Expr, alias map[string]string) bool {
	switch t := expr.(type) {
	case *ast.StructType: // a struct literal
		return true

	case *ast.StarExpr: // a pointer to a struct
		return isStructType(t.X, alias)

	case *ast.SelectorExpr: // a struct of another package (pkg.User)
		exprType := parseSelectorExpr(t, alias)
		return exprType == "struct"

	case *ast.Ident: // a struct of the current package
		if obj := t.Obj; obj != nil {
			typeSpec, ok := obj.Decl.(*ast.TypeSpec)
			if ok {
				_, isStruct := typeSpec.Type.(*ast.StructType)
				return isStruct
			}
		}
		return false

	default:
		return false
	}
}

// generateParamName returns the default name of an anonymous parameter.
func generateParamName(index int) string {
	return fmt.Sprintf("param%d", index+1)
}

// parseStringType parses types written as text: a list of types separated by |,
// each of which gives a TypeDescriptor, so "[]*int|string" is parsed into two.
func (m *Method) parseStringType(s string) []*TypeDescriptor {
	types := strings.Split(s, "|")
	descriptors := make([]*TypeDescriptor, 0, len(types))

	for _, t := range types {
		desc := m.parseSingleType(strings.TrimSpace(t))
		if desc != nil {
			descriptors = append(descriptors, desc)
		}
	}

	return descriptors
}

func (m *Method) parseResults(results *ast.FieldList) {
	if results == nil || len(results.List) == 0 {
		return
	}

	// clear the defaults
	m.ResponseType = nil
	m.Responses = nil

	// the custom status codes of the comments
	for _, e := range extractResponseCode(m.Doc) {
		statusCode := e[0]
		errName := e[1]
		description := e[2]

		// check the status code of the comment against the HTTP code of the error in the catalog
		if catalogErr, ok := settings.Errors.Find(errName); ok {
			actualHTTPCode := strconv.Itoa(int(catalogErr.HTTPCode))
			if statusCode != actualHTTPCode {
				Logger().Warn("status code in a @response comment differs from the HTTP code of its error, using the error's",
					"commentCode", statusCode, "error", errName, "errorCode", actualHTTPCode, "method", m.Name, "at", m.Pos)
				statusCode = actualHTTPCode
			}
		} else if settings.Errors != nil && len(settings.Errors.Entries()) > 0 {
			Logger().Warn("@response names an error that the error catalog does not have, so the response has no schema",
				"error", errName, "method", m.Name, "at", m.Pos)
		}

		resp := ResponseSpec{
			Code: statusCode,
			DataType: &TypeDescriptor{
				FullKey:     errorKey(errName),
				Description: statusCode,
			},
			Description: description,
		}

		m.Responses = append(m.Responses, resp)
	}

	l := []string{"error", "context.Context"}
	// go through all results
	for _, field := range results.List {
		resultType := m.parseTypeExpr(field.Type)

		// error handling
		if slices.Contains(l, resultType.FullKey) {
			continue
		}

		m.ResponseType = resultType
		// success response handling
		resp := ResponseSpec{
			Code:     "200", // default success status code
			DataType: resultType,
		}
		m.Responses = append(m.Responses, resp)
	}
}

// parseSelectorExpr resolves a selector expression to a component name.
func parseSelectorExpr(expr *ast.SelectorExpr, aliases map[string]string) string {
	var parts []string

	// resolve the parent expression recursively
	if x, ok := expr.X.(*ast.SelectorExpr); ok {
		parent := parseSelectorExpr(x, aliases)
		parts = strings.Split(parent, ".")
	} else if ident, ok := expr.X.(*ast.Ident); ok {
		// apply the alias conversion and the path formatting
		if realPath, exists := aliases[ident.Name]; exists {
			parts = strings.Split(formatComponentKey(realPath), ".")
		} else {
			parts = strings.Split(formatComponentKey(ident.Name), ".")
		}
	}

	// join the final component name
	parts = append(parts, expr.Sel.Name)
	return strings.Join(parts, ".")
}

// parseTypeExpr resolves a type expression of the method.
func (m *Method) parseTypeExpr(expr ast.Expr) *TypeDescriptor {
	return parseTypeExpr(expr, m.CurrentPkgPath, m.ImportAlias)
}

// parseFileImports parses the imports of a file and returns the map from alias to full path.
func parseFileImports(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, imp := range file.Imports {
		// the full import path (without the quotes)
		fullPath := strings.Trim(imp.Path.Value, `"`)

		// an import with an alias (such as import ud "user.domain")
		var alias string
		if imp.Name != nil {
			alias = imp.Name.Name
		} else {
			// by default the alias is the last segment of the full path
			// for example: user.domain → domain
			parts := strings.Split(fullPath, "/")
			alias = parts[len(parts)-1]
		}

		// store the full path (user.domain)
		aliases[alias] = fullPath
	}
	return aliases
}

// parseFileImportsOf is parseFileImports with the names that the imported
// packages really have: a package whose name is not the last element of its
// import path (package models in .../model, package proto in .../proto/v2) is
// used in the file by the name it declares.
func parseFileImportsOf(pkg *packages.Package, file *ast.File) map[string]string {
	aliases := parseFileImports(file)
	if pkg == nil {
		return aliases
	}
	for _, imp := range file.Imports {
		if imp.Name != nil {
			continue // an alias that was written is the name
		}
		fullPath := strings.Trim(imp.Path.Value, `"`)
		if imported := pkg.Imports[fullPath]; imported != nil && imported.Name != "" {
			if _, taken := aliases[imported.Name]; !taken {
				aliases[imported.Name] = fullPath
			}
		}
	}
	return aliases
}

// extractResponseCode returns the @response annotations of a doc comment as
// [status, error name, description], for example @response:201,UserCreated,LongDescription.
func extractResponseCode(doc string) (out [][3]string) {
	lines := strings.Split(doc, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "@response:") {
			parts := strings.SplitN(line[len("@response:"):], ",", 3)
			if len(parts) == 1 {
				parts = strings.Split(strings.TrimSpace(parts[0]), " ")
			}
			if len(parts) < 2 {
				parts = append(parts, "")
			}
			if len(parts) < 3 {
				parts = append(parts, "")
			}
			out = append(out, [3]string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])})
		}
	}
	return out
}

// isContextType reports whether the expression is context.Context.
func isContextType(expr ast.Expr) bool {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if x, ok := sel.X.(*ast.Ident); ok && x.Name == "context" && sel.Sel.Name == "Context" {
			return true
		}
	}
	return false
}

// FindServiceImplementations finds the services of a package.
func FindServiceImplementations(pkg *packages.Package) []ServiceInterface {
	var services []ServiceInterface
	structMethods := make(map[string][]*Method) // key: the name of the struct
	// phase one: collect the methods of all structs
	for _, file := range pkg.Syntax {
		pkgAliases := parseFileImportsOf(pkg, file)

		ast.Inspect(file, func(n ast.Node) bool {
			// only method declarations
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				return true
			}

			// resolve the receiver type (only for the target package)
			structName := extractStructName(fn.Recv)
			if structName == "" {
				return true
			}

			// parse the metadata of the method
			method := parseMethod(fn, pkgAliases, pkg)
			structMethods[structName] = append(structMethods[structName], method)
			return true
		})
	}

	// phase two: find the structs that are services
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			typeSpec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			// only struct types
			if _, ok := typeSpec.Type.(*ast.StructType); !ok {
				return true
			}

			structName := typeSpec.Name.Name
			methods := structMethods[structName]

			// the key check: a service has a Name method that returns a string
			serviceName := extractServiceName(methods)
			if serviceName == "" {
				return true
			}

			services = append(services, ServiceInterface{
				ServiceName: serviceName,
				Methods:     filterValidMethods(methods),
			})
			return true
		})
	}

	return services
}

// filterValidMethods keeps the methods that are operations: the exported ones,
// except Name, the ones the annotation "@apidoc: -" hides and the ones that the
// configuration skips.
func filterValidMethods(methods []*Method) []*Method {
	var valid []*Method
	for _, m := range methods {
		// the first letter of the method name is upper case
		if len(m.Name) == 0 || !unicode.IsUpper(rune(m.Name[0])) {
			continue
		}
		if m.Name == "Name" || slices.Contains(settings.SkipMethods, m.Name) {
			continue
		}
		if m.Hidden {
			continue
		}
		valid = append(valid, m)
	}
	return valid
}

// extractStructName returns the name of the struct a method is declared on (it
// handles pointers and package names).
func extractStructName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	expr := recv.List[0].Type
	// pointer types
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	// selector expressions (such as service.User)
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	// plain identifiers (the current package)
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// extractServiceName returns the string that the Name() method of a service
// returns, or "" when the methods have no such method.
func extractServiceName(methods []*Method) string {
	for _, m := range methods {
		if m.Name == "Name" && m.ResponseType != nil && (m.ResponseType.FullKey == "string" || strings.HasSuffix(m.ResponseType.FullKey, ".string")) {
			// the string the method returns
			name := parseNameMethodBody(m.Source, m.Info)
			if name == "" {
				Logger().Warn("the Name method does not return a string literal or a constant, so the struct is not a service",
					"method", m.Name, "at", m.Pos)
			}
			return name
		}
	}
	return "" // no valid implementation found
}

// parseNameMethodBody safely parses the return value of the Name method.
func parseNameMethodBody(fn *ast.FuncDecl, info *types.Info) string {
	// 1. check the signature of the method
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return ""
	}
	if ident, ok := fn.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "string" {
		return ""
	}

	// 2. go through the body of the method to find the return statement
	var returnValue string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		retStmt, ok := n.(*ast.ReturnStmt)
		if !ok || len(retStmt.Results) != 1 {
			return true
		}

		// the type checker knows the value of any constant expression: a literal, a
		// constant of this file or of another package, a concatenation
		if info != nil {
			if tv, ok := info.Types[retStmt.Results[0]]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
				returnValue = constant.StringVal(tv.Value)
				return false
			}
		}

		// 3. handle string literals
		if lit, ok := retStmt.Results[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if unquoted, err := strconv.Unquote(lit.Value); err == nil {
				returnValue = unquoted
			}
			return false // stop when a valid return is found
		}

		// 4. handle constant expressions (such as a string defined with const)
		if ident, ok := retStmt.Results[0].(*ast.Ident); ok {
			if obj := ident.Obj; obj != nil && obj.Kind == ast.Con {
				if spec, ok := obj.Decl.(*ast.ValueSpec); ok && len(spec.Values) > 0 {
					if lit, ok := spec.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if unquoted, err := strconv.Unquote(lit.Value); err == nil {
							returnValue = unquoted
						}
					}
				}
			}
		}
		return true
	})

	return returnValue
}

// parseMethod for dispatch, not for doc
func parseMethod(fn *ast.FuncDecl, pkgAlias map[string]string, pkg *packages.Package) *Method {
	// Methods are not filtered here: all of them have to be parsed.
	// They are filtered in filterValidMethods.

	comments := extractCommentGroupText(fn.Doc)
	method := &Method{
		Name:           fn.Name.Name,
		Doc:            fn.Doc.Text(),
		Source:         fn,
		ImportAlias:    pkgAlias,
		CurrentPkgPath: pkg.PkgPath,
		Info:           pkg.TypesInfo,
		Hidden:         route.Skipped(strings.Split(comments, "\n")),
	}
	if pkg.Fset != nil {
		if pos := pkg.Fset.Position(fn.Pos()); pos.IsValid() {
			method.Pos = fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
		}
	}

	method.warnAboutMistypedAnnotations()

	// parse the @headerType tag
	if headerType := ExtractTag(method.Doc, "headerType"); headerType != "" {
		method.HeaderType = headerType
	} else {
		// use the configured default header by default
		method.HeaderType = settings.DefaultHeader
	}

	// parse the @folder tag
	if folder := ExtractTag(method.Doc, "folder"); folder != "" {
		method.Folder = folder
	}

	// parse the parameters
	method.parseParams(fn.Type.Params)
	// parse the return types
	method.parseResults(fn.Type.Results)

	return method
}

func getMultiTagValue(field *ast.Field, tagName string) []string {
	if field.Tag == nil {
		return nil
	}

	tag := strings.Trim(field.Tag.Value, "`")
	for _, part := range strings.Split(tag, " ") {
		if strings.HasPrefix(part, tagName+":") {
			value := strings.Trim(strings.TrimPrefix(part, tagName+":"), `"`)
			return strings.Split(value, ",")
		}
	}
	return nil
}

func getTagValue(field *ast.Field, tagName string) string {
	v := getMultiTagValue(field, tagName)
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// getAPIFieldName returns the JSON name of a field.
// It uses the unified visibility control.
func getAPIFieldName(field *ast.Field, aud Audience) (jsonKey string, example string, defaultValue string, hide bool) {
	// use the unified field visibility control
	if shouldHideField(field, aud) {
		return "", "", "", true
	}
	jsonKey, hide = getTagValueByOrderDefaultByName(field, "api.header", "json")

	example, _ = getRawTagValueByOrder(field, "example")
	defaultValue, _ = getRawTagValueByOrder(field, "default")
	if defaultValue == "" {
		defaultValue, _ = getJSONDefaultValue(field)
	}
	return jsonKey, example, defaultValue, hide
}

func getJSONDefaultValue(field *ast.Field) (string, bool) {
	if field == nil || field.Tag == nil {
		return "", false
	}
	tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
	splits := strings.Split(tag.Get("json"), ",")
	for i := 1; i < len(splits); i++ {
		if value, ok := strings.CutPrefix(splits[i], "default="); ok {
			return value, true
		}
	}
	return "", false
}
func getRawTagValueByOrder(field *ast.Field, tagName string) (string, bool) {
	if field == nil || field.Tag == nil {
		return "", false
	}
	tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
	v := tag.Get(tagName)
	if v == "-" {
		return "", false
	}
	return v, true
}

func getTagValueByOrderDefaultByName(field *ast.Field, tagNames ...string) (string, bool) {
	if field.Tag == nil {
		if len(field.Names) > 0 {
			return field.Names[0].Name, false
		}
		return "", false
	}

	tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
	for _, tagName := range tagNames {
		tagVal := tag.Get(tagName)
		if tagVal == "-" {
			return "", true
		}

		if commaIndex := strings.Index(tagVal, ","); commaIndex != -1 {
			tagVal = tagVal[:commaIndex]
		}

		if tagVal != "" {
			return tagVal, false
		}
	}
	if len(field.Names) > 0 {
		return field.Names[0].Name, false
	}
	return "", false
}

// isRequiredField reports whether a field is required.
func isRequiredField(field *ast.Field) bool {
	if getValueFromTag(field, "required") == "true" {
		return true
	}
	validate, binding := getValueFromTag(field, "validate"), getValueFromTag(field, "binding")
	if settings.CompatLegacyFieldShapes {
		return validate == "required" || binding == "required"
	}
	return hasRule(validate, "required") || hasRule(binding, "required")
}

// hasRule reports whether a validate or binding tag has a rule: they are
// separated by commas, required,email.
func hasRule(tag, rule string) bool {
	for _, r := range strings.Split(tag, ",") {
		if strings.TrimSpace(r) == rule {
			return true
		}
	}
	return false
}
