package engine

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// at says where in the source a node is, for the diagnostics that point at code.
func (p *TypeParser) at(node ast.Node) string {
	if p == nil {
		return ""
	}
	return positionOf(p.pkg, node)
}

// atField says where the field that is being parsed is, or "" when it is not a
// field that is.
func (p *TypeParser) atField(ctx *ParseContext) string {
	if ctx == nil || ctx.Field == nil {
		return ""
	}
	return p.at(ctx.Field)
}

// positionOf says where a node of a package is, as file:line, or "".
func positionOf(pkg *packages.Package, node ast.Node) string {
	if pkg == nil || pkg.Fset == nil || node == nil {
		return ""
	}
	pos := pkg.Fset.Position(node.Pos())
	if !pos.IsValid() {
		return ""
	}
	return fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
}

type ParseContext struct {
	importAlias map[string]string

	// for description
	Doc     *ast.CommentGroup
	Comment *ast.CommentGroup
	Field   *ast.Field

	GenericValue ast.Expr
	GenericTypes []string

	FullKey string
}

func NewSchemaRefFromFullKey(fk string) *openapi3.SchemaRef {
	return NewSchemaRef(NewRefFromFullKey(fk))
}

func NewRefFromFullKey(fk string) string {
	return "#/components/schemas/" + fk
}

func NewSchemaRef(ref string) *openapi3.SchemaRef {
	return openapi3.NewSchemaRef(ref, defaultSchema())
}

// defaultSchemaRef is a reference to the default schema.
func defaultSchemaRef() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{
		Value: defaultSchema(),
	}
}

// defaultSchema is the default schema: an object with the pattern "default", which marks a schema that has not been filled in.
func defaultSchema() *openapi3.Schema {
	return &openapi3.Schema{
		Type:    &openapi3.Types{openapi3.TypeObject},
		Pattern: defaultPattern,
	}
}

const (
	defaultPattern = "default"
)

// IsPlaceholder reports whether a schema is the one docgen puts where it has not
// described a type: an object with the pattern "default".
func IsPlaceholder(v *openapi3.Schema) bool { return isDefaultSchema(v) }

func isDefaultSchema(v *openapi3.Schema) bool {
	if v == nil {
		return false
	}
	return v.Type.Is(openapi3.TypeObject) && v.Pattern == defaultPattern
}

func mergeMaps(m1, m2 map[string]*openapi3.SchemaRef) map[string]*openapi3.SchemaRef {
	for k, v := range m2 {
		m1[k] = v
	}
	return m1
}

var (
	// \[([^\]]+)\] matches any characters inside square brackets (not the brackets themselves)
	extractGenericTypeRe = regexp.MustCompile(`\[([^\]]+)\]`)
)

func ExtractGenericType(input string) string {
	// find the match
	matches := extractGenericTypeRe.FindStringSubmatch(input)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// inferReflectTypeFromAST infers the reflect.Type of an AST expression.
func inferReflectTypeFromAST(expr ast.Expr) reflect.Type {
	switch t := expr.(type) {
	case *ast.Ident:
		// basic types
		switch t.Name {
		case "string":
			return reflect.TypeOf("")
		case "int":
			return reflect.TypeOf(int(0))
		case "int64":
			return reflect.TypeOf(int64(0))
		case "int32":
			return reflect.TypeOf(int32(0))
		case "float64":
			return reflect.TypeOf(float64(0))
		case "float32":
			return reflect.TypeOf(float32(0))
		case "bool":
			return reflect.TypeOf(false)
		default:
			// for a custom type, interface{}
			return reflect.TypeOf((*interface{})(nil)).Elem()
		}
	case *ast.SelectorExpr:
		// time.Duration: its example and default are written as durations, 10m
		if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == "time" && t.Sel.Name == "Duration" {
			return reflect.TypeOf(time.Duration(0))
		}
	case *ast.StarExpr:
		// a pointer type: the base type, recursively
		baseType := inferReflectTypeFromAST(t.X)
		if baseType != nil {
			return reflect.PointerTo(baseType)
		}
	case *ast.ArrayType:
		// array and slice types
		elemType := inferReflectTypeFromAST(t.Elt)
		if elemType != nil {
			return reflect.SliceOf(elemType)
		}
	}
	// interface{} by default
	return reflect.TypeOf((*interface{})(nil)).Elem()
}

// parseExampleToInterface converts the string of an example (or default) tag to a value of the type of the field.
func parseExampleToInterface(exampleStr string, fieldType reflect.Type) interface{} {
	if exampleStr == "" {
		return nil
	}

	// time.Duration is a special case
	if fieldType == reflect.TypeOf(time.Duration(0)) {
		if duration, err := time.ParseDuration(exampleStr); err == nil {
			// the number of nanoseconds (int64)
			return int64(duration)
		}
		// if parsing fails, the original string
		return exampleStr
	}

	// convert according to the type of the field
	switch fieldType.Kind() {
	case reflect.String:
		return exampleStr
	case reflect.Int, reflect.Int64:
		if val, err := strconv.ParseInt(exampleStr, 10, 64); err == nil {
			// a big integer (outside the safe integer range of JavaScript) is returned as a string, to avoid scientific notation
			if val > 9007199254740991 || val < -9007199254740991 {
				return exampleStr
			}
			return val
		}
	case reflect.Float64:
		if val, err := strconv.ParseFloat(exampleStr, 64); err == nil {
			return val
		}
	case reflect.Bool:
		if val, err := strconv.ParseBool(exampleStr); err == nil {
			return val
		}
	default:
		// for a complex type, first check whether it should be handled as a string
		// is it a numeric string that may be a big integer?
		if isNumericString(exampleStr) {
			if val, err := strconv.ParseInt(exampleStr, 10, 64); err == nil {
				// a big integer is returned as a string
				if val > 9007199254740991 || val < -9007199254740991 {
					return exampleStr
				}
			}
		}

		// for a complex type, try to parse YAML/JSON (YAML is compatible with JSON)
		var result interface{}
		if err := yaml.Unmarshal([]byte(exampleStr), &result); err == nil {
			// if the result is a float64 and the original string is an integer, check for a big integer
			if floatVal, ok := result.(float64); ok && isNumericString(exampleStr) {
				// is it a big integer (outside the safe integer range of JavaScript)?
				if floatVal > 9007199254740991 || floatVal < -9007199254740991 {
					return exampleStr // return the original string to avoid scientific notation
				}
			}
			return result
		}
	}

	// if parsing fails, the original string
	return exampleStr
}

// isNumericString reports whether the string is purely digits.
func isNumericString(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// findTypeRecursive looks for a type declaration (all kinds of types are supported).
func findTypeRecursive(pkg *packages.Package, typeName string) (map[string]string, ast.Expr) {
	for _, file := range pkg.Syntax {
		if t := findTypeInFile(file, typeName); t != nil {
			return parseFileImportsOf(pkg, file), t
		}
	}
	return nil, nil
}

// findTypeInFile finds a type in a single file.
func findTypeInFile(file *ast.File, typeName string) ast.Expr {
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.Name == typeName {
					return typeSpec.Type
				}
			}
		}
	}
	return nil
}

// isNullableFromField reports whether the json tag of the field has the option
// nullable, as in `json:"name,nullable"`: the field may be null.
func isNullableFromField(f *ast.Field) bool {
	if f == nil {
		return false
	}
	jsonTag := getValueFromTag(f, "json")
	// parse the format of tags like `json:"field,nullable"`
	parts := strings.Split(jsonTag, ",")
	for _, p := range parts[1:] { // ignore the field name part
		if strings.TrimSpace(p) == "nullable" {
			return true
		}
	}
	return false
}
