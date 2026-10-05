package engine

import (
	"encoding/json"
	"go/ast"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/suggest"
)

// parseStruct parses a struct type (the core of it).
func (p *TypeParser) parseStruct(st *ast.StructType, ctx *ParseContext) *openapi3.SchemaRef {
	schema := openapi3.NewObjectSchema()
	fields := structFields(st, p.typesInfo())
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
			fieldNames = appendNames(fieldNames, embeddedFieldNames...)

			if embeddedRef := p.parse(ft, fieldCtx); embeddedRef != nil {
				if embeddedRef.Value != nil && settings.CompatLegacyOutput {
					schema.Properties = mergeMaps(schema.Properties, embeddedRef.Value.Properties)
					requiredFields = append(requiredFields, embeddedRef.Value.Required...)
				} else if embeddedRef.Value != nil {
					// A field of the struct itself shadows one of the same name that it
					// embeds, as encoding/json has it, whichever of them is written first.
					for name, property := range embeddedRef.Value.Properties {
						if _, own := schema.Properties[name]; !own {
							schema.Properties[name] = property
						}
					}
					for _, name := range embeddedRef.Value.Required {
						if schema.Properties[name] == embeddedRef.Value.Properties[name] {
							requiredFields = append(requiredFields, name)
						}
					}
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
		if !settings.CompatLegacyOutput {
			fieldSchema = p.describeReference(field, fieldSchema, example, defaultVal)
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
					if !settings.CompatLegacyOutput {
						p.setExampleAndDefault(field, jsonName, fieldSchema.Value, example, defaultVal, fieldType)
						if wrapper := fieldSchema.Value; wrapper.Nullable && wrapper.Type == nil && len(wrapper.AllOf) == 1 {
							// OpenAPI 3.0 lets nullable have an effect next to a type only, and the
							// type is that of what the reference refers to.
							// A list needs its items next to the type, and says what it is
							// through the reference; the others are one word.
							if described := p.getRealSchemaFromDoc(strings.TrimPrefix(wrapper.AllOf[0].Ref, "#/components/schemas/")); described != nil && described.Value != nil && !described.Value.Type.Is(openapi3.TypeArray) {
								wrapper.Type = described.Value.Type
							}
						}
						if wrapper := fieldSchema.Value; len(wrapper.AllOf) == 1 && wrapper.Description == "" && !wrapper.Nullable &&
							wrapper.Example == nil && wrapper.Default == nil && schema.Properties[jsonName] == fieldSchema {
							// Nothing is left to say of the field: its example was one its type does
							// not allow. It is the reference itself, as a field that says nothing is.
							schema.Properties[jsonName] = wrapper.AllOf[0]
						}
						return
					}
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
						warnAt("generic candidate of a field has no schema",
							"candidate", v, "field", jsonName, "fieldType", types.ExprString(ft), "key", ctx.FullKey, "at", p.at(field))
					}
				}
				titles = dedupe(titles)
				if fieldSchema.Value.Title != "" {
					fieldSchema.Value.Title = fieldSchema.Value.Title + "[" + strings.Join(titles, ",") + "]"
				}
			})
		}

		schema.Properties[jsonName] = fieldSchema
		fieldNames = appendNames(fieldNames, jsonName)
		if !settings.CompatLegacyOutput {
			// What the field itself says of being required is what counts, not what a
			// field of the same name that it shadows said.
			requiredFields = slices.DeleteFunc(requiredFields, func(name string) bool { return name == jsonName })
		}
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

	for _, field := range structFields(st, p.typesInfo()) {
		// embedded fields
		if len(field.Names) == 0 {
			// the field names of the embedded field, recursively
			embeddedFieldNames := p.getEmbeddedFieldNamesFromAST(field.Type, ctx)
			fieldNames = appendNames(fieldNames, embeddedFieldNames...)
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

		fieldNames = appendNames(fieldNames, jsonName)
	}

	return fieldNames
}

// structFields lists the fields of a struct as encoding/json sees them, each
// with the one name it has: a declaration of several names (Lat, Lng float64)
// becomes a field for each, a blank name (_) is no field, and an embedded field
// that has a json name (Base `json:"base"`) is a field of that name, not a
// flattened one. A configuration that keeps legacy_output gets what
// documents always had: the first name of a declaration, and the embedded field
// flattened.
func structFields(st *ast.StructType, info *types.Info) []*ast.Field {
	if st == nil || st.Fields == nil {
		return nil
	}
	if settings.CompatLegacyOutput {
		return st.Fields.List
	}
	fields := make([]*ast.Field, 0, len(st.Fields.List))
	for _, f := range st.Fields.List {
		switch {
		case len(f.Names) == 0:
			if embeddedJSONName(f) != "" || embeddedNonStruct(f, info) {
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

// typesInfo is the type information of the package the parser reads, or nil.
func (p *TypeParser) typesInfo() *types.Info {
	if p.pkg == nil {
		return nil
	}
	return p.pkg.TypesInfo
}

// embeddedNonStruct reports whether an embedded field is of a named type that is
// not a struct: a list, a map, a basic type or an interface, which encoding/json
// writes as a field named after the type, not as the fields of what it embeds.
func embeddedNonStruct(f *ast.Field, info *types.Info) bool {
	if info == nil {
		return false
	}
	t := info.TypeOf(f.Type)
	if t == nil {
		return false
	}
	if pointer, ok := t.(*types.Pointer); ok {
		t = pointer.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || !named.Obj().Exported() {
		return false
	}
	_, isStruct := named.Underlying().(*types.Struct)
	return !isStruct
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

// setExampleAndDefault gives a field the example and the default of its tags,
// read as what the schema of the field says it is: a number for an integer, the
// text as it is for a string, a list or an object as JSON or YAML. A value that is
// not one the schema allows is left out, with a warning that names the field and
// the tag: it would make the whole document invalid, and the error would not say
// which field it is.
func (p *TypeParser) setExampleAndDefault(field *ast.Field, name string, schema *openapi3.Schema, example, defaultValue string, goType reflect.Type) {
	set := func(tag, raw string, assign func(any)) {
		if raw == "" {
			return
		}
		value := valueOfTag(schema, raw, goType)
		if value == nil {
			return
		}
		if err := validationCopy(effectiveSchema(schema)).VisitJSON(value); err != nil {
			warnAt("the "+tag+" tag of a field is not a value that the type of the field allows, so it is left out",
				"field", name, "value", raw, "reason", firstLine(err.Error()), "at", p.at(field))
			return
		}
		assign(value)
	}
	set("example", example, func(v any) { schema.Example = v })
	set("default", defaultValue, func(v any) { schema.Default = v })
}

// valueOfTag reads the text of an example or default tag as a value of the schema.
func valueOfTag(schema *openapi3.Schema, raw string, goType reflect.Type) any {
	for goType != nil && goType.Kind() == reflect.Pointer {
		goType = goType.Elem()
	}
	schema = effectiveSchema(schema)
	if schema.Type.Is(openapi3.TypeString) {
		// text is text, a duration that the configuration wrote as one included
		return raw
	}
	if goType == reflect.TypeOf(time.Duration(0)) {
		// a duration is written as one, 10m, and is its nanoseconds in JSON
		if d, err := time.ParseDuration(raw); err == nil {
			return int64(d)
		}
		return raw
	}
	switch {
	case schema.Type.Is(openapi3.TypeInteger):
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return v
		}
	case schema.Type.Is(openapi3.TypeNumber):
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return v
		}
	case schema.Type.Is(openapi3.TypeBoolean):
		if v, err := strconv.ParseBool(raw); err == nil {
			return v
		}
	default:
		// an array, an object, or anything: JSON. A plain word or a date is the text
		// it is, not what YAML would take it for.
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			return value
		}
	}
	return raw
}

// firstLine is the first line of a message, which is where an error says what is wrong.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// describeReference gives a field whose type is a component what the field says
// about itself. OpenAPI 3.0 has no place for it next to a $ref, so the reference
// is the only member of an allOf, which can have a description, a nullable flag,
// an example and a default of its own. A field that says nothing keeps the plain
// reference.
func (p *TypeParser) describeReference(field *ast.Field, ref *openapi3.SchemaRef, example, defaultValue string) *openapi3.SchemaRef {
	if ref.Ref == "" {
		// A type that was parsed already is not a reference yet but a copy of its
		// component, which says which one it is by its title.
		if ref.Value == nil || ref.Value.Title == "" || p.doc == nil || p.doc.Components == nil || p.doc.Components.Schemas[ref.Value.Title] == nil {
			return ref
		}
		ref = &openapi3.SchemaRef{Ref: NewRefFromFullKey(ref.Value.Title), Value: ref.Value}
	}
	description := extractDescription(field.Doc, field.Comment)
	nullable := isNullableFromField(field)
	if description == "" && !nullable && example == "" && defaultValue == "" {
		return ref
	}
	return &openapi3.SchemaRef{Value: &openapi3.Schema{
		AllOf:       openapi3.SchemaRefs{ref},
		Description: description,
		Nullable:    nullable,
	}}
}

// effectiveSchema is the schema a value of a field has to be a value of: a
// reference that has a description of its own is the only member of an allOf, and
// the value is a value of what it refers to.
func effectiveSchema(schema *openapi3.Schema) *openapi3.Schema {
	for schema.Type == nil && len(schema.AllOf) == 1 && schema.AllOf[0] != nil && schema.AllOf[0].Value != nil {
		schema = schema.AllOf[0].Value
	}
	return schema
}

// validationCopy is the schema, and everything it is made of, with the enums
// written as the numbers of a JSON document, which is what the validation of a
// value compares with: the values of an enum of integers are integers of Go until
// the document is written. The enum of the items of a list is one of them.
func validationCopy(schema *openapi3.Schema) *openapi3.Schema {
	return copyForValidation(schema, map[*openapi3.Schema]*openapi3.Schema{})
}

func copyForValidation(schema *openapi3.Schema, done map[*openapi3.Schema]*openapi3.Schema) *openapi3.Schema {
	if schema == nil {
		return nil
	}
	if copied, ok := done[schema]; ok {
		return copied
	}
	c := *schema
	done[schema] = &c
	if len(schema.Enum) > 0 {
		c.Enum = make([]any, len(schema.Enum))
		for i, v := range schema.Enum {
			switch n := v.(type) {
			case int:
				c.Enum[i] = float64(n)
			case int64:
				c.Enum[i] = float64(n)
			case uint64:
				c.Enum[i] = float64(n)
			default:
				c.Enum[i] = v
			}
		}
	}
	ref := func(r *openapi3.SchemaRef) *openapi3.SchemaRef {
		if r == nil {
			return nil
		}
		return &openapi3.SchemaRef{Ref: r.Ref, Value: copyForValidation(r.Value, done)}
	}
	c.Items = ref(schema.Items)
	c.Not = ref(schema.Not)
	if schema.AdditionalProperties.Schema != nil {
		c.AdditionalProperties.Schema = ref(schema.AdditionalProperties.Schema)
	}
	if len(schema.Properties) > 0 {
		c.Properties = make(openapi3.Schemas, len(schema.Properties))
		for name, p := range schema.Properties {
			c.Properties[name] = ref(p)
		}
	}
	for _, of := range []struct{ from, to *openapi3.SchemaRefs }{{&schema.AllOf, &c.AllOf}, {&schema.OneOf, &c.OneOf}, {&schema.AnyOf, &c.AnyOf}} {
		if len(*of.from) > 0 {
			*of.to = make(openapi3.SchemaRefs, len(*of.from))
			for i, r := range *of.from {
				(*of.to)[i] = ref(r)
			}
		}
	}
	return &c
}

// appendNames adds names to the order of the fields of a struct. A name that is
// there already, a field that shadows an embedded one, keeps its place, unless the
// configuration keeps documents as they were, which have it twice.
func appendNames(names []string, more ...string) []string {
	if settings.CompatLegacyOutput {
		return append(names, more...)
	}
	for _, name := range more {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}
