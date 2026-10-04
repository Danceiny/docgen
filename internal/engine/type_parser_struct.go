package engine

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/suggest"
)

// parseStruct parses a struct type (the core of it).
func (p *TypeParser) parseStruct(st *ast.StructType, ctx *ParseContext) *openapi3.SchemaRef {
	schema := openapi3.NewObjectSchema()
	fields := structFields(st)
	fieldCnt := len(fields)

	var fieldNames []string // the order of the fields, used for the extension field
	var requiredFields []string
	for _, field := range fields {
		field := field // shadow

		// the context of the field
		fieldCtx := &ParseContext{
			importAlias:  ctx.importAlias,
			Doc:          field.Doc,
			Comment:      field.Comment,
			Field:        field,
			GenericValue: ctx.GenericValue,
		}
		p.warnAboutFieldTagNobodyReads(field)
		// the name of the field and whether to skip it (a tag on an embedded field skips it whole)
		jsonName, example, defaultVal, skip := getAPIFieldName(field, p.audience)
		if skip {
			continue
		}

		var tagdocGenerics []string
		genericTypeArg := ExtractGenericTypeArg(field)
		if genericTypeArg == "T" && p.realTypeNamedT(field.Type) {
			genericTypeArg = "" // a type of the module that is called T, a field like any other
		}
		if genericTypeArg != "" { // parse it automatically, the same as a comment
			vs := strings.Split(genericTypeArg, ".")
			suffixKey := vs[len(vs)-1]
			if suffixKey == "T" || suffixKey == "any" {
				tagdocGenerics = dedupe(extractTagValueFromDocComments(field.Doc, field.Comment, "generic"))
			} else {
				tagdocGenerics = []string{suffixKey}
			}
		}
		if fieldCnt == 1 && len(tagdocGenerics) > 0 {
			p.defers.PushFront(func() {
				gtk := tagdocGenerics[0]
				if ref := p.searchSchemaFromDoc(gtk); ref != nil {
					schema.Title = ctx.FullKey + "[" + gtk + "]"
				}
			})
		}

		fieldCtx.GenericTypes = tagdocGenerics //important
		gtk := p.generateTypeKey(fieldCtx.GenericValue)
		genericComm := filter(append(tagdocGenerics, gtk),
			func(s string) bool {
				return s != "" && s != "any" && s != "T"
			})

		ft := field.Type
		isGeneric := genericTypeArg == "T"
		if isGeneric {
			fieldCtx.GenericTypes = ctx.GenericTypes
			genericComm = append(genericComm, ctx.GenericTypes...)
			ft = fieldCtx.GenericValue
		}

		genericComm = dedupe(genericComm)

		// embedded fields (embedded structs)
		if len(field.Names) == 0 {
			if p.referenceHidden(ft, p.generateTypeKey(ft), fieldCtx) {
				continue
			}
			// for an embedded field the order of its fields has to be kept:
			// its fields are added where the embedded field is in the struct

			// the field names of the embedded field (only for fieldNames, they do not depend on the schema)
			embeddedFieldNames := p.getEmbeddedFieldNamesFromAST(ft, fieldCtx)
			fieldNames = append(fieldNames, embeddedFieldNames...)

			if embeddedRef := p.parse(ft, fieldCtx); embeddedRef != nil {
				if embeddedRef.Value != nil {
					schema.Properties = mergeMaps(schema.Properties, embeddedRef.Value.Properties)
					requiredFields = append(requiredFields, embeddedRef.Value.Required...)
				}
				// Add a persistent MergeTask to ensure deep merging of embedded structs
				fk := p.generateTypeKey(field.Type)
				p.mergeTasks.PushBack(MergeTask{
					TargetKey: ctx.FullKey,
					SourceKey: fk,
				})
			}
			continue
		}

		if unicode.IsLower(rune(field.Names[0].Name[0])) {
			continue
		}

		fieldSchema := p.parse(ft, fieldCtx)
		if fieldSchema == nil {
			Logger().Debug("field has no schema and is left out", "field", field.Names[0].Name, "fieldType", types.ExprString(ft), "at", p.at(field))
			continue
		}

		// set the description of the field, with care for how placeholders are updated:
		// only set Description when the schema is not the default placeholder, so that the update of placeholders is not broken
		if fieldSchema.Value != nil {
			p.defers.PushBack(func() {
				// is the current schema still the default placeholder?
				// if it is not, it has been updated, and Description can be set safely
				if !isDefaultSchema(fieldSchema.Value) {
					if desc := extractDescription(field.Doc, field.Comment); desc != "" {
						fieldSchema.Value.Description = desc
					}
					fieldType := inferReflectTypeFromAST(ft)
					// the full type name of the field
					fieldTypeKey := p.generateTypeKey(ft)
					if example != "" {
						// is the type mapped to string in BasicTypeSchemas?
						if basicSchema := getBasicTypeSchema(fieldTypeKey); basicSchema != nil && basicSchema.Type != nil && basicSchema.Type.Is("string") {
							// mapped to string in BasicTypeSchemas: use the string value as it is
							fieldSchema.Value.Example = example
						} else {
							// otherwise use the usual type inference
							if exampleValue := parseExampleToInterface(example, fieldType); exampleValue != nil {
								fieldSchema.Value.Example = exampleValue
							}
						}
					}
					if defaultVal != "" {

						// is the type mapped to string in BasicTypeSchemas?
						if basicSchema := getBasicTypeSchema(fieldTypeKey); basicSchema != nil && basicSchema.Type != nil && basicSchema.Type.Is("string") {
							// mapped to string in BasicTypeSchemas: use the string value as it is
							fieldSchema.Value.Default = defaultVal
						} else {
							// otherwise use the usual type inference
							if defaultValValue := parseExampleToInterface(defaultVal, fieldType); defaultValValue != nil {
								fieldSchema.Value.Default = defaultValValue
							}
						}
					}
				}
			})
		}

		if len(tagdocGenerics) > 0 {
			if fieldSchema.Value.Title != "" {
				fieldSchema.Value.Title = fieldSchema.Value.Title + "[" + strings.Join(tagdocGenerics, ",") + "]"
			}
		}
		if isGeneric && len(genericComm) > 0 {
			p.defers.PushFront(func() {
				if strings.Contains(fieldSchema.Value.Title, "[") {
					cv := *fieldSchema.Value
					fieldSchema.Value = &cv
				}
				var titles []string
				for _, v := range genericComm {
					if ref := p.searchSchemaFromDoc(v); ref != nil && !isDefaultSchema(ref.Value) {
						titles = append(titles, v)
						fieldSchema.Value.OneOf = append(fieldSchema.Value.OneOf, NewSchemaRefFromFullKey(ref.Value.Title))
					} else {
						Logger().Warn("generic candidate of a field has no schema",
							"candidate", v, "field", jsonName, "fieldType", types.ExprString(ft), "key", ctx.FullKey)
					}
				}
				titles = dedupe(titles)
				if fieldSchema.Value.Title != "" {
					fieldSchema.Value.Title = fieldSchema.Value.Title + "[" + strings.Join(titles, ",") + "]"
				}
			})
		}

		schema.Properties[jsonName] = fieldSchema
		fieldNames = append(fieldNames, jsonName)
		if isRequiredField(field) {
			requiredFields = append(requiredFields, jsonName)
		}
	}

	schema.Required = dedupe(requiredFields)
	// the extension with the order of the fields
	// x-apifox-orders gives the original order of the fields (compatible with Apifox)
	if len(fieldNames) > 0 && settings.VendorExtensions {
		if schema.Extensions == nil {
			schema.Extensions = make(map[string]interface{})
		}
		schema.Extensions["x-apifox-orders"] = fieldNames
	}

	return schema.NewRef()
}

// extractFieldNamesFromStruct returns the field names of a struct type.
func (p *TypeParser) extractFieldNamesFromStruct(st *ast.StructType, ctx *ParseContext) []string {
	// A struct that embeds itself, directly or through other structs, has the
	// names of its fields once.
	if p.embedding[st] {
		return nil
	}
	if p.embedding == nil {
		p.embedding = make(map[*ast.StructType]bool)
	}
	p.embedding[st] = true
	defer delete(p.embedding, st)

	var fieldNames []string

	for _, field := range structFields(st) {
		// embedded fields
		if len(field.Names) == 0 {
			// the field names of the embedded field, recursively
			embeddedFieldNames := p.getEmbeddedFieldNamesFromAST(field.Type, ctx)
			fieldNames = append(fieldNames, embeddedFieldNames...)
			continue
		}

		// should the field be skipped?
		jsonName, _, _, skip := getAPIFieldName(field, p.audience)
		if skip {
			continue
		}

		// skip unexported fields
		if unicode.IsLower(rune(field.Names[0].Name[0])) {
			continue
		}

		fieldNames = append(fieldNames, jsonName)
	}

	return fieldNames
}

// structFields lists the fields of a struct as encoding/json sees them, each
// with the one name it has: a declaration of several names (Lat, Lng float64)
// becomes a field for each, a blank name (_) is no field, and an embedded field
// that has a json name (Base `json:"base"`) is a field of that name, not a
// flattened one. A configuration that keeps legacy_schema_shapes gets what
// documents always had: the first name of a declaration, and the embedded field
// flattened.
func structFields(st *ast.StructType) []*ast.Field {
	if st == nil || st.Fields == nil {
		return nil
	}
	if settings.CompatLegacySchemaShapes {
		return st.Fields.List
	}
	fields := make([]*ast.Field, 0, len(st.Fields.List))
	for _, f := range st.Fields.List {
		switch {
		case len(f.Names) == 0:
			if embeddedJSONName(f) != "" {
				named := *f
				named.Names = []*ast.Ident{{NamePos: f.Pos(), Name: exported(embeddedTypeName(f.Type))}}
				fields = append(fields, &named)
				continue
			}
			fields = append(fields, f)
		case len(f.Names) == 1 && f.Names[0].Name != "_":
			fields = append(fields, f)
		default:
			for _, name := range f.Names {
				if name.Name == "_" {
					continue
				}
				named := *f
				named.Names = []*ast.Ident{name}
				fields = append(fields, &named)
			}
		}
	}
	return fields
}

// exported capitalizes a name so that a field made of an embedded type that is
// not exported is not taken for a field that is not: encoding/json keeps an
// embedded struct that has a json name whether or not its type is exported.
func exported(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// embeddedJSONName returns the name a json tag gives an embedded field, or "".
func embeddedJSONName(f *ast.Field) string {
	name, _, _ := strings.Cut(getValueFromTag(f, "json"), ",")
	if name == "-" {
		return ""
	}
	return name
}

// embeddedTypeName is the name of an embedded field in Go: the name of its type.
func embeddedTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return embeddedTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return embeddedTypeName(t.X)
	case *ast.IndexListExpr:
		return embeddedTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return "_"
}

// realTypeNamedT reports whether a field of type T or *T is of a type that is
// really called T, which is not what T by convention is, the type parameter of a
// generic declaration. Such a field is a field like any other.
func (p *TypeParser) realTypeNamedT(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	if !ok || ident.Name != "T" || p.pkg == nil || p.pkg.TypesInfo == nil {
		return false
	}
	t := p.pkg.TypesInfo.TypeOf(ident)
	if t == nil {
		return false
	}
	_, isParam := t.(*types.TypeParam)
	return !isParam
}

// fieldScopes are the values of an apidoc tag that are scopes of their own.
var fieldScopes = []string{"-", "public", "internal", "hidden", "custom"}

// warnAboutFieldTagNobodyReads reports a field whose apidoc tag has a value that
// is neither a scope nor a token that some document lists. Such a field is hidden
// from every document, which is the quietest way for a typo (apidoc:"internl") or
// a description put in the wrong tag to make a field disappear.
func (p *TypeParser) warnAboutFieldTagNobodyReads(field *ast.Field) {
	value := getFieldApidocTag(field)
	if value == "" || isNewVisibilityFormat(value) || value == "-" || slices.Contains(settings.FieldTokens, value) {
		return
	}
	at := p.at(field)
	if !firstTime("apidoc tag", at) {
		return
	}
	name := ""
	if len(field.Names) > 0 {
		name = field.Names[0].Name
	}
	args := []any{"field", name, "value", value, "at", at}
	if guess := suggest.ClosestWithin(strings.ToLower(value), append(append([]string(nil), fieldScopes...), settings.FieldTokens...), 2); guess != "" {
		args = append(args, "didYouMean", guess)
	}
	Logger().Warn("the apidoc tag of a field has a value that is none of the scopes (-, public, internal, hidden) and that no document lists in legacy_field_tokens, so the field is hidden from every document", args...)
}
