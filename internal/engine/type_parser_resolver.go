package engine

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

// parseSelector handles selector expressions (such as pkg.Type).
func (p *TypeParser) parseSelector(sel *ast.SelectorExpr, ctx *ParseContext) *openapi3.SchemaRef {
	pkgAlias, ok := sel.X.(*ast.Ident)
	if !ok {
		warnAt("selector expression has an unsupported qualifier", "selector", sel.Sel.Name, "at", p.at(sel))
		return defaultSchemaRef() // an expression that cannot be parsed
	}

	typeName := sel.Sel.Name
	return p.Select(pkgAlias.Name, typeName, ctx)
}

func (p *TypeParser) Select(alias, tn string, ctx *ParseContext) *openapi3.SchemaRef {
	// the import path of the alias
	importPath := ctx.importAlias[alias]
	if importPath == "" {
		// for example alias = time, tn = Time
		importPath = alias
	}

	// basic types
	fk := fullComponentName(importPath, tn) // override it
	if fk == "unknown" || strings.HasPrefix(fk, "unknown.") {
		return nil
	}
	if schema := getBasicTypeSchema(fk); schema != nil {
		return updateDescription(schema.NewRef(), ctx.Doc, ctx.Comment)
	}
	// return it if it exists
	if v := p.getRealSchemaFromDoc(fk); v != nil {
		return updateDescription(v, ctx.Doc, ctx.Comment)
	}

	// A selector is package-qualified, so resolve it from that package rather
	// than looking up the selected name in the current package first. The latter
	// loops forever for aliases such as `type Config = other.Config`: a
	// same-named local alias is found and parsed back into the same selector.
	// Resolve both module-owned and third-party packages on demand. In-module
	// types must not fall through to an unresolved ref merely because their
	// package was outside the current model target set.
	if externalPkg := p.loadExternalPackage(importPath); externalPkg != nil {
		if p.session != nil && !p.session.EnterResolving(fk) {
			if !strings.HasPrefix(fk, ownKeyPrefix()) {
				return recursiveSchemaRef()
			}
			return NewSchemaRefFromFullKey(fk)
		}
		if p.session != nil {
			defer p.session.LeaveResolving(fk)
		}
		externalParser := NewTypeParser(externalPkg, p.doc, p.session)
		if v := externalParser.findTypeAndParse(fk, tn, ctx); v != nil {
			p.updateSchemaInDoc(fk, v)
			for e := externalParser.defers.Front(); e != nil; e = e.Next() {
				p.defers.PushBack(e.Value)
			}
			for e := externalParser.mergeTasks.Front(); e != nil; e = e.Next() {
				p.mergeTasks.PushBack(e.Value)
			}
			return v
		}
		if hasNoSchema(externalPkg, tn) {
			return nil
		}
	} else {
		if strings.HasPrefix(fk, "sync") {
			return nil
		}
		warnAt("external package could not be loaded",
			"importPath", importPath, "type", tn, "alias", alias, "key", fk, "at", p.atField(ctx))
	}

	if !strings.HasPrefix(fk, ownKeyPrefix()) {
		// A type of another module that has no declaration to read, such as
		// unsafe.Pointer. No component will ever describe it, so a reference to
		// it would point at nothing.
		warnAt("Go type has no schema and is left out", "type", fk, "at", p.atField(ctx))
		return nil
	}
	ref := NewSchemaRefFromFullKey(fk)
	return ref
}

// recursiveSchemaRef stands for a type of another module where it is reached
// again from inside itself. Such a type has no component of its own: it is
// written out where it is used, so the second time it can only be an object.
func recursiveSchemaRef() *openapi3.SchemaRef {
	return openapi3.NewObjectSchema().NewRef()
}

// hasNoSchema reports whether a type declared in the package is one that no
// schema describes: a function or a channel type. A field of such a type has no
// property, like a field that is declared with the function type itself.
func hasNoSchema(pkg *packages.Package, typeName string) bool {
	spec, _ := findTypeDeclaration(pkg, typeName)
	if spec == nil {
		return false
	}
	switch spec.Type.(type) {
	case *ast.FuncType, *ast.ChanType:
		return true
	}
	return false
}

func (p *TypeParser) findTypeAndParse(fk, tn string, ctx *ParseContext) *openapi3.SchemaRef {
	// look for the target type
	if importAlias, targetType := findTypeRecursive(p.pkg, tn); targetType != nil {
		ctx2 := *ctx
		ctx2.importAlias = importAlias
		// A type can be reached through a field in another package. In that
		// case the field context does not carry the declaration annotations
		// (notably @autowire), so restore the target declaration comments before
		// parsing it. Without this, a sealed interface is first parsed correctly
		// and then replaced by a default placeholder during cross-package
		// resolution.
		if typeSpec, genDecl := findTypeDeclaration(p.pkg, tn); typeSpec != nil {
			if genDecl != nil {
				ctx2.Doc = genDecl.Doc
			}
			ctx2.Comment = typeSpec.Doc
			if isEnumType(p.pkg, typeSpec) {
				underlyingType := "string"
				if ident, ok := typeSpec.Type.(*ast.Ident); ok {
					underlyingType = ident.Name
				}
				ref := generateEnumSchemaWithVisibility(p.pkg, typeSpec, underlyingType, p.audience)
				p.updateSchemaInDoc(fk, ref)
				return ref
			}
		}
		// targetType: Ident, int,
		if !strings.HasPrefix(fk, ownKeyPrefix()) {
			// A type of another module is written out where it is used, and only a
			// type that is being written out can contain itself.
			p.activeMu.Lock()
			already := p.activeTypes[fk]
			p.activeTypes[fk] = true
			p.activeMu.Unlock()
			if !already {
				defer func() {
					p.activeMu.Lock()
					delete(p.activeTypes, fk)
					p.activeMu.Unlock()
				}()
			}
		}
		return p.parse(targetType, &ctx2)
	}
	return nil
}

func findTypeDeclaration(pkg *packages.Package, typeName string) (*ast.TypeSpec, *ast.GenDecl) {
	if pkg == nil {
		return nil, nil
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && typeSpec.Name.Name == typeName {
					return typeSpec, genDecl
				}
			}
		}
	}
	return nil, nil
}

func (p *TypeParser) parsePointer(expr *ast.StarExpr, ctx *ParseContext) *openapi3.SchemaRef {
	return p.parse(expr.X, ctx)
}

func (p *TypeParser) parseIdent(ident *ast.Ident, ctx *ParseContext) *openapi3.SchemaRef {
	// 1. basic types
	if schema := getBasicTypeSchema(ident.Name); schema != nil {
		return updateDescription(schema.NewRef(), ctx.Doc, ctx.Comment)
	}
	if p.isPredeclaredType(ident) {
		// complex64 and complex128: JSON has no number with two parts.
		warnAt("Go type has no schema and is left out", "type", ident.Name, "at", p.at(ident))
		return nil
	}

	fullKey := ctx.FullKey
	if fullKey == "" {
		// 2. the full type key (including the package path)
		fullKey = p.generateTypeKey(ident)
	}
	// The parser uses the synthetic "unknown" package when an unexported
	// runtime dependency cannot be resolved from an API contract. Never emit a
	// dangling component reference for that sentinel.
	if strings.HasPrefix(fullKey, "unknown.") || fullKey == "unknown" {
		return nil
	}
	if strings.HasSuffix(fullKey, ".T") {
		return nil
	}
	// A self-referential field (for example []*Node) reaches
	// this resolver while its parent schema is still being built. Return a
	// real component reference to the occupied schema instead of descending
	// into the same AST forever or replacing the field with a generic object.
	p.activeMu.Lock()
	active := p.activeTypes[fullKey]
	p.activeMu.Unlock()
	if active {
		if !strings.HasPrefix(fullKey, ownKeyPrefix()) {
			return recursiveSchemaRef()
		}
		return NewSchemaRefFromFullKey(fullKey)
	}

	// 3. check whether it is in the cache
	cached := p.getRealSchemaFromDoc(fullKey)
	if cached != nil {
		return updateDescription(cached, ctx.Doc, ctx.Comment)
	}
	if !strings.HasPrefix(fullKey, ownKeyPrefix()) {
		vs := strings.Split(fullKey, ".")
		k := vs[len(vs)-1]
		if unicode.IsLower(rune(k[0])) {
			return nil
		}
		return p.findTypeAndParse(fullKey, k, ctx)
	}
	if hasNoSchema(p.pkg, ident.Name) {
		return nil
	}
	return NewSchemaRefFromFullKey(fullKey)
}

// loadExternalPackage loads an external package.
func (p *TypeParser) loadExternalPackage(importPath string) *packages.Package {
	if p.session != nil {
		if pkg := p.session.ImportedPackage(importPath); pkg != nil {
			return pkg
		}
	}
	// check the cache
	if val, ok := pkgCache.Load(importPath); ok {
		return val.(*packages.Package)
	}
	if _, failed := pkgLoadError.Load(importPath); failed {
		return nil
	}
	if pkg := findImportedPackage(p.pkg, importPath, make(map[string]bool)); pkg != nil {
		pkgCache.Store(importPath, pkg)
		return pkg
	}

	// load the package with packages.Load
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedModule,
		Dir: func() string {
			if p.session != nil {
				return p.session.ModuleDir()
			}
			return ""
		}(),
	}

	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		pkgLoadError.Store(importPath, struct{}{})
		Logger().Error("failed to load package", "importPath", importPath, "err", err)
		return nil
	}

	if len(pkgs) == 0 {
		pkgLoadError.Store(importPath, struct{}{})
		return nil
	}

	pkg := pkgs[0]
	// cache the result
	pkgCache.Store(importPath, pkg)

	return pkg
}

func findImportedPackage(pkg *packages.Package, importPath string, seen map[string]bool) *packages.Package {
	if pkg == nil || seen[pkg.ID] {
		return nil
	}
	seen[pkg.ID] = true
	for path, dep := range pkg.Imports {
		if path == importPath || dep.ID == importPath {
			return dep
		}
		if found := findImportedPackage(dep, importPath, seen); found != nil {
			return found
		}
	}
	return nil
}

// isPredeclaredType reports whether the identifier names a type of the universe
// scope, as opposed to a type that a package declares under that name.
func (p *TypeParser) isPredeclaredType(ident *ast.Ident) bool {
	if p.pkg == nil || p.pkg.TypesInfo == nil {
		return false
	}
	obj := p.pkg.TypesInfo.ObjectOf(ident)
	if obj == nil || obj.Pkg() != nil {
		return false
	}
	_, isType := obj.(*types.TypeName)
	return isType && types.Universe.Lookup(ident.Name) == obj
}
