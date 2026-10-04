package engine

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

// isEnumType reports whether the type is an enum: constants are declared with it.
func isEnumType(pkg *packages.Package, typeSpec *ast.TypeSpec) bool {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}

			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if ok && valueSpec.Type != nil {
					if ident, ok := valueSpec.Type.(*ast.Ident); ok && ident.Name == typeSpec.Name.Name {
						return true
					}
				}
			}
		}
	}
	return false
}

// generateEnumSchemaWithVisibility makes the schema of an enum, leaving out the values that are not visible.
func generateEnumSchemaWithVisibility(pkg *packages.Package, typeSpec *ast.TypeSpec, underlyingType string, aud Audience) *openapi3.SchemaRef {
	// the visible enum values
	enumValues := collectVisibleEnumEntries(pkg, typeSpec.Name.Name, aud)
	return generateEnumSchemaFromEntry(enumValues, underlyingType, "type", typeSpec.Name.Name, "at", positionOf(pkg, typeSpec)).NewRef()
}

type EnumEntry struct {
	Name    string // name of the constant, such as StatusOK
	Value   string // value, such as "OK"
	Doc     string // doc comment (the comment before the declaration)
	Comment string // comment, such as "the status is normal"
}

// where are attributes that say which enum it is, for the diagnostics.
func generateEnumSchemaFromEntry(entries []EnumEntry, underlyingType string, where ...any) *openapi3.Schema {
	// take all enum values and convert them according to the underlying type
	enumValues := make([]any, len(entries))
	for i, entry := range entries {
		// convert the string value to the right type for the underlying type
		switch underlyingType {
		case "int", "int8", "int16", "int32", "int64":
			if intVal, err := strconv.ParseInt(entry.Value, 10, 64); err == nil {
				enumValues[i] = intVal
			} else {
				// if the conversion fails, keep the string
				enumValues[i] = entry.Value
			}
		case "uint", "uint8", "uint16", "uint32", "uint64":
			if uintVal, err := strconv.ParseUint(entry.Value, 10, 64); err == nil {
				enumValues[i] = uintVal
			} else {
				// if the conversion fails, keep the string
				enumValues[i] = entry.Value
			}
		case "float32", "float64":
			if floatVal, err := strconv.ParseFloat(entry.Value, 64); err == nil {
				enumValues[i] = floatVal
			} else {
				// if the conversion fails, keep the string
				enumValues[i] = entry.Value
			}
		case "bool":
			if boolVal, err := strconv.ParseBool(entry.Value); err == nil {
				enumValues[i] = boolVal
			} else {
				// if the conversion fails, keep the string
				enumValues[i] = entry.Value
			}
		default:
			// for string and other types, keep the string
			enumValues[i] = entry.Value
		}
	}

	// build the Markdown description
	var descBuilder strings.Builder
	descBuilder.WriteString("Enums \n")
	for _, entry := range entries {
		fmt.Fprintf(&descBuilder,
			"- `%s` (`%s`): %s\n",
			entry.Name,
			entry.Value,
			entry.Comment,
		)
	}

	// create the schema and fill in the fields
	ori := getBasicTypeSchema(underlyingType)
	if ori == nil {
		Logger().Warn("enum underlying type has no schema, using object", append([]any{"underlyingType", underlyingType}, where...)...)
		ori = getBasicTypeSchema("object")
	}
	schema := *ori
	schema.Enum = enumValues
	schema.Description = descBuilder.String()
	// add the extension fields (optional)
	var varNames []any
	var comments []any
	for _, entry := range entries {
		varNames = append(varNames, entry.Name)
		comments = append(comments, entry.Comment)
	}
	// the old fields are kept, and the Apifox extension is added
	apifox := make([]map[string]any, 0, len(entries))
	for i, entry := range entries {
		apifox = append(apifox, map[string]any{
			"value":       enumValues[i],
			"name":        entry.Name,
			"description": entry.Comment,
		})
	}
	if settings.VendorExtensions {
		schema.Extensions = map[string]any{
			"x-enum-varnames": varNames,
			"x-enum-comments": comments,
			"x-apifox-enum":   apifox,
		}
	}

	return &schema
}

// collectEnumEntries collects the names, the values and the comments of the constants of an enum.
func collectEnumEntries(pkg *packages.Package, typeName string) []EnumEntry {
	if !settings.CompatLegacySchemaShapes {
		if entries, ok := collectEnumEntriesByType(pkg, typeName); ok {
			return entries
		}
	}
	return collectEnumEntriesFromSyntax(pkg, typeName)
}

// collectEnumEntriesByType collects the constants of the enum type, whichever way
// they are declared: the value is what the compiler computed, so a constant
// declared with iota or an expression has its value, and one that repeats the
// type implicitly (the B of `A Status = iota` followed by `B`) is there. It
// reports false when the package has no type information to ask.
func collectEnumEntriesByType(pkg *packages.Package, typeName string) ([]EnumEntry, bool) {
	if pkg == nil || pkg.Types == nil || pkg.TypesInfo == nil {
		return nil, false
	}
	typeObj, ok := pkg.Types.Scope().Lookup(typeName).(*types.TypeName)
	if !ok || typeObj.IsAlias() {
		// An alias, type Status = string, is the type it stands for, and every
		// constant of that type would be one of its values: only the declarations
		// that name it can be told.
		return nil, false
	}
	var entries []EnumEntry
	// in the order of the source: the files of the package, then the declarations
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			blockDocComments := extractComments(genDecl.Doc)
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range valueSpec.Names {
					c, ok := pkg.TypesInfo.Defs[name].(*types.Const)
					if !ok || name.Name == "_" || !types.Identical(c.Type(), typeObj.Type()) {
						continue
					}
					entries = append(entries, EnumEntry{
						Name:    name.Name,
						Value:   constantText(c.Val()),
						Doc:     blockDocComments,
						Comment: strings.TrimPrefix(extractDescription(valueSpec.Doc, valueSpec.Comment), name.Name+" "),
					})
				}
			}
		}
	}
	return entries, true
}

// constantText is the value of a constant as the text a schema parses it from: a
// string without quotes, a number in decimal.
func constantText(v constant.Value) string {
	switch v.Kind() {
	case constant.String:
		return constant.StringVal(v)
	case constant.Bool:
		return strconv.FormatBool(constant.BoolVal(v))
	case constant.Float:
		f, _ := constant.Float64Val(v)
		return strconv.FormatFloat(f, 'g', -1, 64)
	default:
		return v.ExactString()
	}
}

// collectEnumEntriesFromSyntax collects the constants that are declared with the
// type of the enum and a literal; the value of any other is empty.
func collectEnumEntriesFromSyntax(pkg *packages.Package, typeName string) []EnumEntry {
	var entries []EnumEntry
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}

			// does the const block have a constant of the target type?
			hasTargetType := false
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || valueSpec.Type == nil {
					continue
				}
				ident, ok := valueSpec.Type.(*ast.Ident)
				if ok && ident.Name == typeName {
					hasTargetType = true
					break
				}
			}

			if !hasTargetType {
				continue
			}

			// the doc comment of the whole const block
			blockDocComments := extractComments(genDecl.Doc)

			// go through all ValueSpecs
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || valueSpec.Type == nil {
					continue
				}

				// is the type the target enum type?
				ident, ok := valueSpec.Type.(*ast.Ident)
				if !ok || ident.Name != typeName {
					continue
				}

				// the name and the value of each constant
				for i, name := range valueSpec.Names {
					if i >= len(valueSpec.Values) {
						continue // guard against an index out of range
					}

					// extract the value
					value := ""
					if basicLit, ok := valueSpec.Values[i].(*ast.BasicLit); ok {
						// the Kind of the BasicLit says whether the quotes have to be removed:
						// for numbers (INT, FLOAT and so on) the value is used as it is,
						// only for STRING are the quotes removed
						if basicLit.Kind == token.STRING {
							value = strings.Trim(basicLit.Value, `"`)
						} else {
							// numbers (INT, FLOAT and so on) are used as they are
							value = basicLit.Value
						}
					}

					// the comment before the line and the comment at the end of it (only of this ValueSpec)
					lineComment := strings.TrimPrefix(extractDescription(valueSpec.Doc, valueSpec.Comment), name.Name+" ")

					entries = append(entries, EnumEntry{
						Name:    name.Name,
						Value:   value,
						Doc:     blockDocComments, // the comment of the whole const block
						Comment: lineComment,
					})
				}
			}
		}
	}
	return entries
}
