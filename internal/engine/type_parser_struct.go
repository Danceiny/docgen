package engine

import (
	"go/ast"
	"go/types"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

// parseStruct parses a struct type (the core of it).
func (p *TypeParser) parseStruct(st *ast.StructType, ctx *ParseContext) *openapi3.SchemaRef {
	schema := openapi3.NewObjectSchema()
	fieldCnt := len(st.Fields.List)

	var fieldNames []string // the order of the fields, used for the extension field
	var requiredFields []string
	for _, field := range st.Fields.List {
		field := field // shadow

		// the context of the field
		fieldCtx := &ParseContext{
			importAlias:  ctx.importAlias,
			Doc:          field.Doc,
			Comment:      field.Comment,
			Field:        field,
			GenericValue: ctx.GenericValue,
		}
		// the name of the field and whether to skip it (a tag on an embedded field skips it whole)
		jsonName, example, defaultVal, skip := getAPIFieldName(field, p.audience)
		if skip {
			continue
		}

		var tagdocGenerics []string
		genericTypeArg := ExtractGenericTypeArg(field)
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

		// type Example struct {
		//    A, B, C int // several names in one field is not supported, please do not write this
		//}
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

	for _, field := range st.Fields.List {
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
