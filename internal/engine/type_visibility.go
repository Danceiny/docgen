package engine

import (
	"go/ast"
	"go/types"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
)

// A type is hidden from a document by its //apidoc: directive or, when it has
// none, by its name: the document hides the types whose names start with one of
// its prefixes (hide_type_prefixes). Hiding a type leaves out the type, every
// field and every operation that refers to it. An enum is the exception: it is
// shown with its values hidden, as documents have always shown it.

// typeDeclaration finds the declaration of a named type, with the package that
// declares it: the package being parsed, or one the session loaded.
func (p *TypeParser) typeDeclaration(named *types.Named) (*ast.TypeSpec, *ast.GenDecl, *packages.Package) {
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return nil, nil, nil
	}
	pkg := p.pkg
	if p.pkg == nil || obj.Pkg().Path() != p.pkg.PkgPath {
		pkg = nil
		if p.session != nil {
			pkg = p.session.ImportedPackage(obj.Pkg().Path())
		}
	}
	if pkg == nil {
		return nil, nil, nil
	}
	spec, decl := findTypeDeclaration(pkg, obj.Name())
	return spec, decl, pkg
}

// namedTypeOf returns the named type an expression refers to, through pointers,
// lists, arrays and maps (of their values), or nil.
func (p *TypeParser) namedTypeOf(expr ast.Expr) *types.Named {
	if p.pkg == nil {
		return nil
	}
	return namedTypeIn(p.pkg.TypesInfo, expr)
}

// namedTypeIn is namedTypeOf for an expression of the package that the type
// information is of.
func namedTypeIn(info *types.Info, expr ast.Expr) *types.Named {
	if info == nil || expr == nil {
		return nil
	}
	t := info.TypeOf(expr)
	for t != nil {
		switch u := types.Unalias(t).(type) {
		case *types.Pointer:
			t = u.Elem()
		case *types.Slice:
			t = u.Elem()
		case *types.Array:
			t = u.Elem()
		case *types.Map:
			t = u.Elem()
		case *types.Named:
			return u
		default:
			return nil
		}
	}
	return nil
}

// hiddenNamed reports whether a named type is hidden from the document: because it
// is declared as a type that is, type S InternalState, or a list or a map of one,
// which leaves it nothing to be described by, whatever it says about itself; or by
// what it declares about itself; or, when it declares nothing, by its name.
func (p *TypeParser) hiddenNamed(named *types.Named, depth int) bool {
	spec, decl, pkg := p.typeDeclaration(named)
	if spec != nil && pkg != nil && depth < 16 && !isEnumType(pkg, spec) {
		if next := namedTypeIn(pkg.TypesInfo, spec.Type); next != nil && next != named && p.hiddenNamed(next, depth+1) {
			return true
		}
	}
	if spec != nil {
		if visibility := declaredVisibility(spec, decl); visibility != nil {
			if isEnumType(pkg, spec) {
				return false // shown, with its values hidden
			}
			return shouldHideByVisibilityOfType(visibility, spec.Name.Name, p.audience)
		}
	}
	return shouldHideByDefault(named.Obj().Name(), p.audience)
}

// typeDocs lists the comments that can carry the directive of a type: the
// comment of the declaration, then the one of the type in a group.
func typeDocs(spec *ast.TypeSpec, decl *ast.GenDecl) []*ast.CommentGroup {
	var docs []*ast.CommentGroup
	if decl != nil && decl.Doc != nil {
		docs = append(docs, decl.Doc)
	}
	if spec != nil && spec.Doc != nil {
		docs = append(docs, spec.Doc)
	}
	return docs
}

// declaredVisibility returns the visibility a type declares with an //apidoc:
// directive, or nil when it declares none.
func declaredVisibility(spec *ast.TypeSpec, decl *ast.GenDecl) *Visibility {
	for _, doc := range typeDocs(spec, decl) {
		if v := parseTypeVisibility(doc); v.Declared {
			return v
		}
	}
	return nil
}

// referenceHidden reports whether the type that an expression refers to is
// hidden from the document, so that whatever refers to it is left out. fk is the
// key of the type.
func (p *TypeParser) referenceHidden(expr ast.Expr, fk string, ctx *ParseContext) bool {
	if ctx != nil && ctx.FullKey == "" {
		// A reference to a type, as opposed to the declaration of one: what the
		// type declares about itself has the last word over its name.
		if named := p.namedTypeOf(expr); named != nil {
			if !settings.CompatLegacyOutput {
				// The type it is made of, not the key of the list or the map that holds
				// it: a map of maps of pointers to it is as hidden as it is.
				return p.hiddenNamed(named, 0)
			}
			if spec, decl, pkg := p.typeDeclaration(named); spec != nil {
				if visibility := declaredVisibility(spec, decl); visibility != nil {
					if isEnumType(pkg, spec) {
						return false // shown, with its values hidden
					}
					return shouldHideByVisibilityOfType(visibility, spec.Name.Name, p.audience)
				}
			}
		}
	}
	last := fk[strings.LastIndex(fk, ".")+1:]
	return shouldHideByDefault(last, p.audience)
}

// spacedDirective matches a comment that was meant to be an //apidoc: directive
// and has a space after the slashes, which makes it an ordinary comment.
var spacedDirective = regexp.MustCompile(`^//\s+apidoc:`)

// warnAboutDirectiveMistakes reports the two mistakes in an //apidoc: directive
// that make it do nothing or something else, without a word: a space after the
// slashes, and a scope that is none of the known ones.
func (p *TypeParser) warnAboutDirectiveMistakes(spec *ast.TypeSpec, decl *ast.GenDecl) {
	for _, doc := range typeDocs(spec, decl) {
		for _, c := range doc.List {
			if spacedDirective.MatchString(c.Text) {
				warnAt("write the directive with no space after the slashes, //apidoc:...; as written it is an ordinary comment and is ignored",
					"type", spec.Name.Name, "at", p.at(spec))
			}
		}
	}
	if v := declaredVisibility(spec, decl); v != nil {
		switch v.Scope {
		case "public", "internal", "hidden", "custom":
		default:
			warnAt("the //apidoc: directive of a type has a scope that is not public, internal or hidden, so the type is shown as if it had none",
				"scope", v.Scope, "type", spec.Name.Name, "at", p.at(spec))
		}
	}
}

// declarationHidden reports whether a type declaration is hidden from the
// document and is not an enum: such a type has no schema at all.
func (p *TypeParser) declarationHidden(spec *ast.TypeSpec) bool {
	if isEnumType(p.pkg, spec) {
		if settings.CompatLegacyOutput {
			return false
		}
		// A directive shows the type with its values hidden. An enum that is hidden
		// by its name, with no directive to say otherwise, is hidden like any type:
		// it has no schema, and nothing that refers to it is in the document.
		if _, decl := findTypeDeclaration(p.pkg, spec.Name.Name); declaredVisibility(spec, decl) != nil {
			return false
		}
		return shouldHideByDefault(spec.Name.Name, p.audience)
	}
	if shouldHideType(p.pkg, spec, p.audience) {
		return true
	}
	return !settings.CompatLegacyOutput && p.declaredFromHidden(spec)
}

// declaredFromHidden reports whether a type is declared as a type that is hidden,
// type S InternalState, or a list or a map of one. It has nothing to be described
// by, and is hidden as whatever refers to it is, whatever it says about itself.
func (p *TypeParser) declaredFromHidden(spec *ast.TypeSpec) bool {
	if p.pkg == nil {
		return false
	}
	next := namedTypeIn(p.pkg.TypesInfo, spec.Type)
	if next == nil || !p.hiddenNamed(next, 0) {
		return false
	}
	if _, decl := findTypeDeclaration(p.pkg, spec.Name.Name); declaredVisibility(spec, decl) != nil {
		warnAt("a type that is declared as a type that the document hides is hidden too, whatever its //apidoc: directive says: nothing is left to describe it by",
			"type", spec.Name.Name, "declaredAs", next.Obj().Name(), "at", p.at(spec))
	}
	return true
}

// markHidden records that a type of the module is hidden from the document, so
// that the operations that use it can be left out.
func (p *TypeParser) markHidden(key string) {
	if p.session != nil {
		p.session.markHidden(key)
	}
}
