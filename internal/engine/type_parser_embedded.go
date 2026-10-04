package engine

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

// getEmbeddedFieldNamesFromAST returns the field names of an embedded field, from the AST alone.
func (p *TypeParser) getEmbeddedFieldNamesFromAST(expr ast.Expr, ctx *ParseContext) []string {
	var fieldNames []string

	// the different kinds of embedded fields
	switch t := expr.(type) {
	case *ast.Ident:
		// a type embedded by name, such as type User struct { BaseModel }
		fieldNames = p.getEmbeddedFieldNamesFromIdent(t, ctx)
	case *ast.SelectorExpr:
		// package.Type, such as type User struct { common.BaseModel }
		fieldNames = p.getEmbeddedFieldNamesFromSelector(t, ctx)
	case *ast.StarExpr:
		// a pointer type, such as type User struct { *BaseModel }
		fieldNames = p.getEmbeddedFieldNamesFromAST(t.X, ctx)
	case *ast.MapType:
		// a map type, such as type User struct { map[string]string }
		fieldNames = p.getEmbeddedFieldNamesFromAST(t.Key, ctx)
	case *ast.ArrayType:
		// an array type, such as type User struct { []string }
		fieldNames = p.getEmbeddedFieldNamesFromAST(t.Elt, ctx)
	case *ast.StructType:
		// a struct type, such as type User struct { struct { BaseModel } }
		fieldNames = p.extractFieldNamesFromStruct(t, ctx)
	}

	return fieldNames
}

// getEmbeddedFieldNamesFromIdent returns the field names of an embedded field that is an identifier.
func (p *TypeParser) getEmbeddedFieldNamesFromIdent(ident *ast.Ident, ctx *ParseContext) []string {
	var fieldNames []string

	// look for the type declaration in the current package
	if importAlias, targetType := findTypeRecursive(p.pkg, ident.Name); targetType != nil {
		if structType, ok := targetType.(*ast.StructType); ok {
			// a new context with the right imports
			newCtx := &ParseContext{
				importAlias:  importAlias,
				Doc:          ctx.Doc,
				Comment:      ctx.Comment,
				Field:        ctx.Field,
				GenericValue: ctx.GenericValue,
			}
			fieldNames = p.extractFieldNamesFromStruct(structType, newCtx)
		}
	} else {
		// try the external packages
		// go through the import aliases of the current context to find the type declaration
		for _, importPath := range ctx.importAlias {
			if isOwnImportPath(importPath) {
				// for a package of the module, try to load it and find the type declaration
				if externalPkg := p.loadExternalPackage(importPath); externalPkg != nil {
					if _, targetType := findTypeRecursive(externalPkg, ident.Name); targetType != nil {
						if structType, ok := targetType.(*ast.StructType); ok {
							// a new context with the imports of the external package
							externalCtx := &ParseContext{
								importAlias:  parseFileImports(externalPkg.Syntax[0]),
								Doc:          ctx.Doc,
								Comment:      ctx.Comment,
								Field:        ctx.Field,
								GenericValue: ctx.GenericValue,
							}
							fieldNames = p.extractFieldNamesFromStruct(structType, externalCtx)
							break
						}
					}
				}
			}
		}
	}

	return fieldNames
}

// getEmbeddedFieldNamesFromSelector returns the field names of an embedded field that is a selector expression.
func (p *TypeParser) getEmbeddedFieldNamesFromSelector(sel *ast.SelectorExpr, ctx *ParseContext) []string {
	var fieldNames []string

	// the package name and the type name
	if pkgIdent, ok := sel.X.(*ast.Ident); ok {
		pkgName := pkgIdent.Name
		typeName := sel.Sel.Name

		// look for the imported package
		if importPath, found := ctx.importAlias[pkgName]; found {
			// is it a package of the module (does its path start with the module name)?
			if isOwnImportPath(importPath) {
				// for a package of the module, look for the type declaration in the current package
				fieldNames = p.getEmbeddedFieldNamesFromInternalPackage(importPath, typeName, ctx)
			} else {
				// for an external package, use the existing logic that loads external packages
				fieldNames = p.getEmbeddedFieldNamesFromExternalPackage(importPath, typeName, ctx)
			}
		}
	}

	return fieldNames
}

// getEmbeddedFieldNamesFromInternalPackage returns the field names of an embedded type of a package of the module.
func (p *TypeParser) getEmbeddedFieldNamesFromInternalPackage(importPath, typeName string, ctx *ParseContext) []string {
	var fieldNames []string

	// for a package of the module the package has to be loaded to get the type declaration
	// use the existing logic that loads external packages, for a package of the module
	if internalPkg := p.loadExternalPackage(importPath); internalPkg != nil {
		fieldNames = p.extractFieldNamesFromExternalPackageCached(internalPkg, typeName, ctx)
	}

	return fieldNames
}

// getEmbeddedFieldNamesFromExternalPackage returns the field names of an embedded type of an external package.
func (p *TypeParser) getEmbeddedFieldNamesFromExternalPackage(importPath, typeName string, ctx *ParseContext) []string {
	var fieldNames []string

	// correctness first: the external package has to be loaded to get the field names in the right order
	// reuse the existing logic that loads external packages
	if externalPkg := p.loadExternalPackage(importPath); externalPkg != nil {
		fieldNames = p.extractFieldNamesFromExternalPackageCached(externalPkg, typeName, ctx)
	}

	return fieldNames
}

// extractFieldNamesFromExternalPackageCached extracts the field names from an external package that is already cached.
func (p *TypeParser) extractFieldNamesFromExternalPackageCached(externalPkg *packages.Package, typeName string, ctx *ParseContext) []string {
	var fieldNames []string

	// look for the type declaration in the external package
	if _, targetType := findTypeRecursive(externalPkg, typeName); targetType != nil {
		if structType, ok := targetType.(*ast.StructType); ok {
			// a new context with the imports of the external package
			externalCtx := &ParseContext{
				importAlias:  parseFileImports(externalPkg.Syntax[0]), // the imports of the external package
				Doc:          ctx.Doc,
				Comment:      ctx.Comment,
				Field:        ctx.Field,
				GenericValue: ctx.GenericValue,
			}

			// temporarily make the external package the current one, so that embedded fields resolve correctly
			originalPkg := p.pkg
			p.pkg = externalPkg
			fieldNames = p.extractFieldNamesFromStruct(structType, externalCtx)
			p.pkg = originalPkg
		}
	}

	return fieldNames
}
