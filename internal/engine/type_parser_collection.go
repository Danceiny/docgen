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
	valueSchema := p.parse(expr.Value, &ParseContext{importAlias: ctx.importAlias, GenericValue: ctx.GenericValue})
	if valueSchema == nil && p.isFuncOrChan(expr.Value) {
		return nil // a map of functions has nothing to describe
	}
	// Create a standards-compliant map schema. Keep the value type in
	// additionalProperties; putting a key schema into an extension caused the
	// generator to manufacture an `unknown` component ref for runtime maps.
	if valueSchema == nil {
		valueSchema = defaultSchemaRef()
	}
	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:                 &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{Has: boolPtr(true), Schema: valueSchema},
		},
	}
}

func boolPtr(v bool) *bool { return &v }

func (p *TypeParser) parseArray(expr *ast.ArrayType, ctx *ParseContext) *openapi3.SchemaRef {
	ctx2 := *ctx
	elementRef := p.parse(expr.Elt, &ctx2)
	if elementRef == nil {
		// try to handle a type alias: if the element is an identifier, look for its type declaration in the current package
		if ident, ok := expr.Elt.(*ast.Ident); ok {
			if importAlias, targetType := findTypeRecursive(p.pkg, ident.Name); targetType != nil {
				ctx3 := *ctx
				ctx3.importAlias = importAlias
				// parse the underlying type recursively
				elementRef = p.parse(targetType, &ctx3)
			}
		}

		if elementRef == nil && p.isFuncOrChan(expr.Elt) {
			return nil // a list of functions has nothing to describe
		}
		if elementRef == nil && p.isTypeParam(expr.Elt) {
			// The element of a list in a generic declaration is only known where the
			// declaration is instantiated.
			elementRef = defaultSchemaRef()
		}
		if elementRef == nil {
			Logger().Warn("array element type is unknown, using the default schema",
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
	ref := defaultSchemaRef()
	autowire := extractTagValueFromDocComments(ctx.Doc, ctx.Comment, "autowire")
	if len(autowire) > 0 && strings.TrimSpace(strings.SplitN(autowire[0], "\n", 2)[0]) == "true" {
		for _, impl := range findImplementations(p.pkg, expr) {
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

// isTypeParam reports whether the type expression is a type parameter of a
// generic declaration.
func (p *TypeParser) isTypeParam(expr ast.Expr) bool {
	if p.pkg == nil || p.pkg.TypesInfo == nil {
		return false
	}
	_, ok := p.pkg.TypesInfo.TypeOf(expr).(*types.TypeParam)
	return ok
}
