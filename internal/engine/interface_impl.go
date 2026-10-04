package engine

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// findImplementations finds the structs that implement the given interface.

func findImplementations(pkg *packages.Package, iface *ast.InterfaceType) []string {
	var implementations []string

	// 1. the list of the methods of the interface
	var ifaceMethods []string
	for _, method := range iface.Methods.List {
		if len(method.Names) > 0 {
			ifaceMethods = append(ifaceMethods, method.Names[0].Name)
		}
	}
	// Unexported sealed-union marker methods are not reliably discoverable via
	// types.MethodSet across package boundaries; collect their concrete
	// receivers directly from the package AST as a declaration-level fallback.
	for _, want := range ifaceMethods {
		if len(want) == 0 || want[0] >= 'A' && want[0] <= 'Z' {
			continue
		}
		seen := map[string]bool{}
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name.Name != want || fn.Recv == nil || len(fn.Recv.List) == 0 {
					continue
				}
				recv := fn.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if ident, ok := recv.(*ast.Ident); ok && !seen[ident.Name] {
					implementations = append(implementations, ident.Name)
					seen[ident.Name] = true
				}
			}
		}
	}
	if len(implementations) > 0 {
		return implementations
	}

	// 2. go through all types of the package
	for _, file := range pkg.Syntax {

		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				// 3. only check struct types
				if _, ok := typeSpec.Type.(*ast.StructType); !ok {
					continue
				}

				// 4. get the type object
				obj := pkg.TypesInfo.Defs[typeSpec.Name]
				if obj == nil {
					continue
				}

				// 5. check that all methods of the interface are implemented
				methodSet := types.NewMethodSet(obj.Type())
				if obj, ok := obj.(*types.TypeName); ok {
					// also check the method set of the pointer type
					methodSet = types.NewMethodSet(types.NewPointer(obj.Type()))
				}
				implementsAll := true
				for _, methodName := range ifaceMethods {
					if methodSet.Lookup(pkg.Types, methodName) == nil {
						implementsAll = false
						break
					}
				}

				if implementsAll {
					implementations = append(implementations, typeSpec.Name.Name)
				}
			}
		}
	}

	return implementations
}
