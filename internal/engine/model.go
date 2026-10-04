package engine

import (
	"container/list"
	"go/ast"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

func ProcessModelsWithSession(pkg *packages.Package, doc *openapi3.T, session *GenerationSession) (*list.List, *list.List) {
	parser := NewTypeParser(pkg, doc, session)
	parser.ParseAllDecls()
	return parser.defers, parser.mergeTasks
}

// replaceSlashes replaces the path separators safely.
func replaceSlashes(s string) string {
	return strings.ReplaceAll(s, "/", ".")
}

// cloneSchemaRef makes a deep clone of a SchemaRef.
func cloneSchemaRef(ref *openapi3.SchemaRef) *openapi3.SchemaRef {
	if ref == nil {
		return nil
	}

	cloned := &openapi3.SchemaRef{
		Ref:   ref.Ref,
		Value: cloneSchema(ref.Value),
	}
	return cloned
}

// CloneSchemaPreservedFields lists OpenAPI fields cloneSchema is required to
// retain. Adding a name here without updating cloneSchema must fail
// TestCloneSchema_KnownGaps.
var CloneSchemaPreservedFields = []string{
	"Type", "Title", "Format", "Description", "Nullable",
	"Enum", "Default", "Example",
	"ReadOnly", "WriteOnly", "AllowEmptyValue", "Deprecated",
	"ExclusiveMin", "ExclusiveMax", "Min", "Max", "MultipleOf",
	"MinLength", "MaxLength", "Pattern",
	"MinItems", "MaxItems", "UniqueItems", "Items",
	"Required", "Properties", "MinProps", "MaxProps", "AdditionalProperties",
	"OneOf", "AnyOf", "AllOf", "Not", "Discriminator",
	"Extensions", "ExternalDocs", "XML",
}

// CloneSchemaKnownGaps lists Schema fields intentionally NOT deep-copied yet.
// Most are OpenAPI ≥3.1-only or unused by the generator. Do not silently drop a
// preserved field into this list: reviewers treat gaps as explicit debt, not as
// accidental omission.
var CloneSchemaKnownGaps = []string{
	"Origin", // kin-openapi parse metadata; not part of emitted contract
	"Const", "Examples", "PrefixItems", "Contains", "MinContains", "MaxContains",
	"PatternProperties", "DependentSchemas", "PropertyNames",
	"UnevaluatedItems", "UnevaluatedProperties",
	"If", "Then", "Else", "DependentRequired",
	"Defs", "SchemaDialect", "Comment",
	"SchemaID", "Anchor", "DynamicRef", "DynamicAnchor",
	"ContentMediaType", "ContentEncoding", "ContentSchema",
}

// cloneSchema clones a Schema (a deep copy of the contract fields; the known gaps are in CloneSchemaKnownGaps).
func cloneSchema(schema *openapi3.Schema) *openapi3.Schema {
	if schema == nil {
		return new(openapi3.Schema)
	}

	cloned := &openapi3.Schema{
		Type:            schema.Type,
		Title:           schema.Title,
		Format:          schema.Format,
		Nullable:        schema.Nullable,
		Description:     schema.Description,
		Default:         schema.Default,
		Example:         schema.Example,
		ReadOnly:        schema.ReadOnly,
		WriteOnly:       schema.WriteOnly,
		AllowEmptyValue: schema.AllowEmptyValue,
		Deprecated:      schema.Deprecated,
		UniqueItems:     schema.UniqueItems,
		ExclusiveMin:    cloneExclusiveBound(schema.ExclusiveMin),
		ExclusiveMax:    cloneExclusiveBound(schema.ExclusiveMax),
		Min:             cloneFloat64Ptr(schema.Min),
		Max:             cloneFloat64Ptr(schema.Max),
		MultipleOf:      cloneFloat64Ptr(schema.MultipleOf),
		MinLength:       schema.MinLength,
		MaxLength:       cloneUint64Ptr(schema.MaxLength),
		Pattern:         schema.Pattern,
		MinItems:        schema.MinItems,
		MaxItems:        cloneUint64Ptr(schema.MaxItems),
		MinProps:        schema.MinProps,
		MaxProps:        cloneUint64Ptr(schema.MaxProps),
		Properties:      make(openapi3.Schemas, len(schema.Properties)),
		Required:        append([]string{}, schema.Required...),
		XML:             schema.XML,
		ExternalDocs:    schema.ExternalDocs,
	}

	if len(schema.Enum) > 0 {
		cloned.Enum = append([]any{}, schema.Enum...)
	}

	// Items
	if schema.Items != nil {
		cloned.Items = cloneSchemaRef(schema.Items)
	}
	if schema.Not != nil {
		cloned.Not = cloneSchemaRef(schema.Not)
	}
	// AllOf / OneOf / AnyOf
	if len(schema.AllOf) > 0 {
		cloned.AllOf = make(openapi3.SchemaRefs, len(schema.AllOf))
		for i, sr := range schema.AllOf {
			cloned.AllOf[i] = cloneSchemaRef(sr)
		}
	}
	if len(schema.OneOf) > 0 {
		cloned.OneOf = make(openapi3.SchemaRefs, len(schema.OneOf))
		for i, sr := range schema.OneOf {
			cloned.OneOf[i] = cloneSchemaRef(sr)
		}
	}
	if len(schema.AnyOf) > 0 {
		cloned.AnyOf = make(openapi3.SchemaRefs, len(schema.AnyOf))
		for i, sr := range schema.AnyOf {
			cloned.AnyOf[i] = cloneSchemaRef(sr)
		}
	}

	// Properties
	for k, v := range schema.Properties {
		cloned.Properties[k] = cloneSchemaRef(v)
	}

	// AdditionalProperties (bool Has and/or nested Schema)
	if schema.AdditionalProperties.Has != nil {
		has := *schema.AdditionalProperties.Has
		cloned.AdditionalProperties.Has = &has
	}
	if schema.AdditionalProperties.Schema != nil {
		cloned.AdditionalProperties.Schema = cloneSchemaRef(schema.AdditionalProperties.Schema)
	}

	if schema.Discriminator != nil {
		cloned.Discriminator = cloneDiscriminator(schema.Discriminator)
	}

	// Extensions (a shallow copy of the keys and values is enough)
	if len(schema.Extensions) > 0 {
		ext := make(map[string]any, len(schema.Extensions))
		for k, v := range schema.Extensions {
			ext[k] = v
		}
		cloned.Extensions = ext
	}

	return cloned
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneUint64Ptr(v *uint64) *uint64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneExclusiveBound(bound openapi3.ExclusiveBound) openapi3.ExclusiveBound {
	out := openapi3.ExclusiveBound{}
	if bound.Bool != nil {
		b := *bound.Bool
		out.Bool = &b
	}
	if bound.Value != nil {
		out.Value = cloneFloat64Ptr(bound.Value)
	}
	return out
}

func cloneDiscriminator(d *openapi3.Discriminator) *openapi3.Discriminator {
	if d == nil {
		return nil
	}
	out := &openapi3.Discriminator{
		PropertyName: d.PropertyName,
	}
	if len(d.Extensions) > 0 {
		ext := make(map[string]any, len(d.Extensions))
		for k, v := range d.Extensions {
			ext[k] = v
		}
		out.Extensions = ext
	}
	if len(d.Mapping) > 0 {
		out.Mapping = make(openapi3.StringMap[openapi3.MappingRef], len(d.Mapping))
		for k, ref := range d.Mapping {
			cloned := cloneSchemaRef((*openapi3.SchemaRef)(&ref))
			if cloned == nil {
				out.Mapping[k] = openapi3.MappingRef{}
				continue
			}
			out.Mapping[k] = openapi3.MappingRef(*cloned)
		}
	}
	return out
}

func parseTypeExpr(expr ast.Expr, currentPkgPath string, importAlias map[string]string) *TypeDescriptor {
	// defensive check
	if expr == nil {
		return &TypeDescriptor{FullKey: "invalid"}
	}

	// create the base descriptor
	var desc *TypeDescriptor
	switch t := expr.(type) {
	case *ast.StarExpr:
		desc = parseTypeExpr(t.X, currentPkgPath, importAlias)
		desc.IsPointer = true

	case *ast.ArrayType:
		desc = parseTypeExpr(t.Elt, currentPkgPath, importAlias)
		desc.Dimensions++

	case *ast.SelectorExpr:
		fullKey := formatComponentKey(parseSelectorExpr(t, importAlias))
		desc = &TypeDescriptor{FullKey: fullKey}

	case *ast.MapType, *ast.InterfaceType:
		// An object: what a map holds is not described, as in a field of a struct.
		if settings.CompatLegacyOperationTypes {
			return &TypeDescriptor{FullKey: "unknown"}
		}
		desc = &TypeDescriptor{FullKey: "object"}

	case *ast.IndexExpr:
		// An instantiated generic type, Page[Order]: the generic type itself,
		// which is described as it is declared.
		if settings.CompatLegacyOperationTypes {
			return &TypeDescriptor{FullKey: "unknown"}
		}
		desc = parseTypeExpr(t.X, currentPkgPath, importAlias)

	case *ast.IndexListExpr:
		if settings.CompatLegacyOperationTypes {
			return &TypeDescriptor{FullKey: "unknown"}
		}
		desc = parseTypeExpr(t.X, currentPkgPath, importAlias)

	case *ast.Ident:
		fullKey := t.Name
		if getBasicTypeSchema(fullKey) == nil {
			fullKey = fullComponentName(currentPkgPath, t.Name)
		}
		desc = &TypeDescriptor{FullKey: fullKey}

	default:
		desc = &TypeDescriptor{FullKey: "unknown"}
	}

	return desc
}

// extractFieldDescription returns the description of a struct field.
func extractFieldDescription(field *ast.Field) string {
	// priority 1: the desc property of the field tag
	if desc := getTagValue(field, "desc"); desc != "" {
		return desc
	}

	// priority 2: the comment of the field (format: // user ID)
	if field.Doc != nil && len(field.Doc.List) > 0 {
		return strings.TrimSpace(field.Doc.List[0].Text[2:])
	}

	return ""
}
