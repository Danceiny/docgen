package engine

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// parseIndexExpr handles generic type expressions.
func (p *TypeParser) parseIndexExpr(fk string, ide *ast.IndexExpr, ctx *ParseContext) (string, *openapi3.SchemaRef) {
	// leave it to when the concrete type is known
	ctx2 := *ctx
	if ctx.GenericValue == nil {
		ctx2.GenericValue = ide.Index
	}
	return fk, p.parse(ide.X, &ctx2)
}

func (p *TypeParser) parseMap(expr *ast.MapType, ctx *ParseContext) *openapi3.SchemaRef {
	if p.referenceHidden(expr.Value, p.generateTypeKey(expr.Value), &ParseContext{}) {
		return nil // a map of a type that the document hides
	}
	valueSchema := p.parse(expr.Value, &ParseContext{importAlias: ctx.importAlias, GenericValue: ctx.GenericValue})
	if valueSchema == nil && p.isFuncOrChan(expr.Value) {
		return nil // a map of functions has nothing to describe
	}
	if valueSchema == nil && !settings.CompatLegacyOutput && p.isUnrepresentable(expr.Value) {
		return nil // nor has a map of what JSON has no value for, and the value said so
	}
	// Create a standards-compliant map schema. Keep the value type in
	// additionalProperties; putting a key schema into an extension caused the
	// generator to manufacture an `unknown` component ref for runtime maps.
	if valueSchema == nil {
		valueSchema = defaultSchemaRef()
	}
	additional := additionalPropertiesOf(valueSchema)
	if settings.CompatLegacyOutput {
		// kin-openapi writes the flag and not the schema when both are set, which
		// is how documents always lost the type of the values of a map.
		additional = openapi3.AdditionalProperties{Has: boolPtr(true), Schema: valueSchema}
	}
	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:                 &openapi3.Types{"object"},
			AdditionalProperties: additional,
		},
	}
}

// additionalPropertiesOf describes the values of a map: the schema of the value,
// or just true when it is any value, which says the same as the empty schema in
// fewer words.
func additionalPropertiesOf(value *openapi3.SchemaRef) openapi3.AdditionalProperties {
	if isAnyValue(value) {
		return openapi3.AdditionalProperties{Has: boolPtr(true)}
	}
	return openapi3.AdditionalProperties{Schema: value}
}

// isAnyValue reports whether a schema says nothing about a value: it has no type
// and no shape, or it is the placeholder of a schema that was never filled in.
func isAnyValue(ref *openapi3.SchemaRef) bool {
	if ref == nil || ref.Value == nil {
		return true
	}
	if ref.Ref != "" {
		return false
	}
	v := ref.Value
	if isDefaultSchema(v) {
		return true
	}
	return v.Type == nil && len(v.Properties) == 0 && len(v.AnyOf) == 0 && len(v.OneOf) == 0 &&
		len(v.AllOf) == 0 && v.Items == nil && len(v.Enum) == 0 && v.AdditionalProperties.Schema == nil &&
		v.AdditionalProperties.Has == nil
}

func boolPtr(v bool) *bool { return &v }

func (p *TypeParser) parseArray(expr *ast.ArrayType, ctx *ParseContext) *openapi3.SchemaRef {
	if !settings.CompatLegacyOutput && p.isByteSlice(expr) {
		// encoding/json writes a slice of bytes as a base64 string.
		return &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:     &openapi3.Types{openapi3.TypeString},
			Format:   "byte",
			Nullable: isNullableFromField(ctx.Field),
		}}
	}
	ctx2 := *ctx
	if !settings.CompatLegacyOutput {
		// The comment and the tags of a field are those of the list, not of what
		// it is a list of.
		ctx2.Where = p.atField(ctx) // what the elements are of is still where the field is
		ctx2.Doc, ctx2.Comment, ctx2.Field = nil, nil, nil
	}
	if p.referenceHidden(expr.Elt, p.generateTypeKey(expr.Elt), &ParseContext{}) {
		return nil // a list of a type that the document hides
	}
	elementRef := p.parse(expr.Elt, &ctx2)
	if elementRef == nil {
		// try to handle a type alias: if the element is an identifier, look for its type declaration in the current package
		if ident, ok := expr.Elt.(*ast.Ident); ok {
			if importAlias, targetType := findTypeRecursive(p.pkg, ident.Name); targetType != nil {
				ctx3 := ctx2
				ctx3.importAlias = importAlias
				// parse the underlying type recursively
				elementRef = p.parse(targetType, &ctx3)
			}
		}

		if elementRef == nil && p.isFuncOrChan(expr.Elt) {
			return nil // a list of functions has nothing to describe
		}
		if elementRef == nil && !settings.CompatLegacyOutput && p.isUnrepresentable(expr.Elt) {
			return nil // nor has a list of what JSON has no value for, and the element said so
		}
		if elementRef == nil && p.isTypeParam(expr.Elt) {
			// The element of a list in a generic declaration is only known where the
			// declaration is instantiated, and what an instance makes of it is not
			// read: the list is a list of anything. Documents that keep the old way of
			// writing have the placeholder of a type that was not described.
			elementRef = defaultSchemaRef()
			if !settings.CompatLegacyOutput {
				elementRef = &openapi3.SchemaRef{Value: anySchema()}
			}
		}
		if elementRef == nil {
			warnAt("array element type is unknown, using the default schema",
				"element", types.ExprString(expr.Elt), "at", p.at(expr))
			elementRef = defaultSchemaRef()
		}
	}
	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:     &openapi3.Types{openapi3.TypeArray},
			Nullable: isNullableFromField(ctx.Field),
			Items:    elementRef,
		},
	}
}

// parseInterface handles interface types.
func (p *TypeParser) parseInterface(expr *ast.InterfaceType, ctx *ParseContext) *openapi3.SchemaRef {
	// the empty interface, interface{}
	if expr.Methods == nil || len(expr.Methods.List) == 0 {
		if !settings.CompatLegacyOutput {
			empty := anySchema()
			if settings.VendorExtensions {
				empty.Extensions = map[string]interface{}{"x-go-interface": "interface{}"}
			}
			return &openapi3.SchemaRef{Value: empty}
		}
		empty := &openapi3.Schema{
			Description: "empty interface",
			AnyOf: []*openapi3.SchemaRef{
				openapi3.NewStringSchema().NewRef(),
				openapi3.NewIntegerSchema().NewRef(),
				openapi3.NewObjectSchema().NewRef(),
			},
		}
		if settings.VendorExtensions {
			empty.Extensions = map[string]interface{}{"x-go-interface": "interface{}"}
		}
		return &openapi3.SchemaRef{Value: empty}
	}
	autowire := extractTagValueFromDocComments(ctx.Doc, ctx.Comment, "autowire")
	if !settings.CompatLegacyOutput && len(autowire) == 0 {
		// Whatever has the methods can be written as any JSON value.
		return &openapi3.SchemaRef{Value: anySchema()}
	}
	ref := defaultSchemaRef()
	if len(autowire) > 0 && strings.TrimSpace(strings.SplitN(autowire[0], "\n", 2)[0]) == "true" {
		impls := findImplementations(p.pkg, expr)
		if !settings.CompatLegacyOutput && len(impls) > 0 {
			// The union is not a placeholder: a field that refers to the interface
			// after it is declared and before the alternatives are added would take
			// it for one, parse the type again, and leave the alternatives with
			// nothing to be added to.
			ref = &openapi3.SchemaRef{Value: &openapi3.Schema{}}
		}
		if p.onDemand && !settings.CompatLegacyOutput {
			// Nothing else reads this package: the alternatives of the union are
			// described here, as what they are made of is.
			for _, impl := range impls {
				p.parseDeclared(p.generateStructKey(impl), impl)
			}
		}
		for _, impl := range impls {
			impl := impl
			p.defers.PushFront(func() {
				if ref.Value == nil {
					return
				}
				if p.searchSchemaFromDoc(p.generateStructKey(impl)) != nil {
					// Keep the union closed while preserving component identity. An
					// inline copy loses the dependency edge, so public pruning can
					// delete AmendPayload and later re-materialize a default object.
					ref.Value.OneOf = append(ref.Value.OneOf, NewSchemaRefFromFullKey(p.generateStructKey(impl)))
				} else if !settings.CompatLegacyOutput && p.session != nil && p.session.Hides(p.generateStructKey(impl)) {
					Logger().Debug("an alternative of an autowired interface is hidden from the document, and is left out", "impl", impl)
				} else {
					Logger().Warn("implementation of an autowired interface has no schema", "impl", impl)
				}
			})

		}
	}
	return ref
}

// isFuncOrChan reports whether the type expression is a function or a channel
// type, written out or declared with a name: the types no schema describes.
func (p *TypeParser) isFuncOrChan(expr ast.Expr) bool {
	if p.pkg == nil || p.pkg.TypesInfo == nil {
		return false
	}
	t := p.pkg.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Signature, *types.Chan:
		return true
	}
	return false
}

// isUnrepresentable reports whether a type is one that JSON has no value for: a
// complex number or an unsafe pointer. Describing the type says so.
func (p *TypeParser) isUnrepresentable(expr ast.Expr) bool {
	if p.pkg == nil || p.pkg.TypesInfo == nil {
		return false
	}
	t := p.pkg.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch basic.Kind() {
	case types.Complex64, types.Complex128, types.UnsafePointer:
		return true
	}
	return false
}

// isByteSlice reports whether a type is a slice, not an array, of bytes, or of a
// type whose kind is that of a byte: the slices that encoding/json writes as a
// string.
func (p *TypeParser) isByteSlice(expr *ast.ArrayType) bool {
	if expr.Len != nil || p.pkg == nil || p.pkg.TypesInfo == nil {
		return false
	}
	elem := p.pkg.TypesInfo.TypeOf(expr.Elt)
	if elem == nil {
		return false
	}
	basic, ok := elem.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

// isTypeParam reports whether the type expression is a type parameter of a
// generic declaration.
func (p *TypeParser) isTypeParam(expr ast.Expr) bool {
	if p.pkg == nil || p.pkg.TypesInfo == nil {
		return false
	}
	_, ok := p.pkg.TypesInfo.TypeOf(expr).(*types.TypeParam)
	return ok
}
