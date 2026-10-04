package engine

import (
	"fmt"
	"go/ast"
	"go/types"
	"strconv"
	"strings"
)

// generateStructKey makes the full component key of a struct.
func (p *TypeParser) generateStructKey(typeName string) string {
	return fullComponentName(p.pkg.PkgPath, typeName)
}

func getGenericFullKey(fk string, expr ast.Expr) string {
	if expr == nil {
		return fk
	}
	switch v := expr.(type) {
	case *ast.Ident:
		if v.Name == "any" || v.Name == "T" {
			return fk
		}
		return fk + v.Name
	case *ast.SelectorExpr:
		if v.Sel.Name == "any" || v.Sel.Name == "T" {
			return fk
		}
		return fk + v.Sel.Name
	default:
		return fk
	}
}

// generateTypeKey makes the unique identity of a type.
// This is the enhanced type key generation: it needs the type information.
func (p *TypeParser) generateTypeKey(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	// check the cache
	if cached, ok := typeKeyCache.Load(expr); ok {
		return cached.(string)
	}

	// the actual generation
	key := p.generateTypeKeyUncached(expr)
	typeKeyCache.Store(expr, key)
	keyTypeCache.Store(key, expr)
	return key
}

func (p *TypeParser) generateTypeKeyUncached(expr ast.Expr) (out string) {
	defer func() {
		out = strings.ReplaceAll(out, "/", ".")
		out = strings.ReplaceAll(out, "*", "pointer.")
		out = strings.ReplaceAll(out, "[]", "array.")
	}()
	// prefer the information of the type system
	if p.pkg.TypesInfo != nil {
		if typeInfo := p.pkg.TypesInfo.TypeOf(expr); typeInfo != nil {
			switch t := typeInfo.(type) {
			case *types.Named:
				return p.fullyQualifiedName(t)
			case *types.Basic:
				return t.Name()
			case *types.Pointer:
				return p.generateTypeKeyForType(t.Elem())
			case *types.Slice:
				return "array_of_" + p.generateTypeKeyForType(t.Elem())
			case *types.Array:
				return "array_of_" + strconv.FormatInt(t.Len(), 10) + "_" + p.generateTypeKeyForType(t.Elem())
			case *types.Map:
				return fmt.Sprintf("map_%s_%s",
					p.generateTypeKeyForType(t.Key()),
					p.generateTypeKeyForType(t.Elem()),
				)
			case *types.Struct:
				return p.generateAnonymousStructKey(t)
			case *types.Interface: // TypeParam.Name = "T"
				return "interface"
			case *types.Alias:
				// for a type alias, return the full name of the alias
				if obj := t.Obj(); obj != nil {
					if pkg := obj.Pkg(); pkg != nil {
						return fmt.Sprintf("%s.%s",
							strings.ReplaceAll(pkg.Path(), "/", "."),
							obj.Name(),
						)
					}
					return obj.Name()
				}
				return "alias"
			case *types.TypeParam:
				return "typeParam"
			case *types.Signature:
				return "signature"
			case *types.Chan:
				return "chan"
			case *types.Union:
				// The type set of a constraint, such as ~int | ~string: never the
				// type of a value, so there is nothing to describe.
				return "constraint"
			default:
				Logger().Error("go/types type has no key rule", "type", goType(t), "at", p.at(expr))
			}
		}
	}

	// the case where the AST has no type information
	switch t := expr.(type) {
	case *ast.StarExpr:
		return p.generateTypeKeyUncached(t.X)
	case *ast.ArrayType:
		return "array_of_" + p.generateTypeKeyUncached(t.Elt)
	case *ast.SelectorExpr:
		return p.generateSelectorKey(t)
	case *ast.StructType:
		return p.generateAnonymousStructKey(nil)
	case *ast.Ident:
		return p.generateIdentKey(t)
	case *ast.MapType:
		return fmt.Sprintf("map_%s_%s",
			p.generateTypeKeyUncached(t.Key),
			p.generateTypeKeyUncached(t.Value),
		)
	case *ast.InterfaceType:
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return "object"
		}
		return fmt.Sprintf("array.%d", len(t.Methods.List))
	case *ast.FuncType:
		return "function"
	case *ast.ChanType:
		return "chan"
	case *ast.BinaryExpr, *ast.UnaryExpr:
		// A union of types in a constraint, such as int | string, or a term of it,
		// such as ~string.
		return "constraint"
	default:
		Logger().Error("expression has no key rule", "expr", goType(expr), "at", p.at(expr))
		return fmt.Sprintf("unhandled_%T", expr)
	}
}

// generateTypeKeyForType makes the key of a go/types type.
func (p *TypeParser) generateTypeKeyForType(t types.Type) string {
	if named, ok := t.(*types.Named); ok {
		return p.fullyQualifiedName(named)
	}

	v := t.String() // basic types are returned as they are
	if isBasicType(v) {
		return v
	}
	return "object"
}

func (p *TypeParser) fullyQualifiedName(named *types.Named) string {
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return named.Obj().Name() // a built-in type
	}
	return fmt.Sprintf("%s.%s",
		strings.ReplaceAll(pkg.Path(), "/", "."),
		named.Obj().Name(),
	)
}

func (p *TypeParser) generateSelectorKey(sel *ast.SelectorExpr) string {
	if pkgIdent, ok := sel.X.(*ast.Ident); ok {
		if obj := p.pkg.TypesInfo.ObjectOf(pkgIdent); obj != nil {
			if pkgName, ok := obj.(*types.PkgName); ok {
				return fmt.Sprintf("%s.%s",
					strings.ReplaceAll(pkgName.Imported().Path(), "/", "."),
					sel.Sel.Name,
				)
			}
		}
	}
	return fmt.Sprintf("%s.%s", sel.X, sel.Sel.Name) // fall back
}

func (p *TypeParser) generateIdentKey(ident *ast.Ident) string {
	if obj := p.pkg.TypesInfo.ObjectOf(ident); obj != nil {
		if pkg := obj.Pkg(); pkg != nil {
			return fullComponentName(pkg.Path(), obj.Name())
		}
		return obj.Name() // a built-in identifier
	}
	return ident.Name // last resort
}

func (p *TypeParser) generateAnonymousStructKey(t *types.Struct) string {
	if t == nil {
		return "anonStruct@<ast>" // the AST has no type information
	}
	// make a unique hash for a struct of the type system
	return fmt.Sprintf("anonStruct@%p", t)
}
