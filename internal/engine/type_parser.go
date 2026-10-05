package engine

import (
	"container/list"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

var (
	pkgCache     sync.Map // importPath -> *packages.Package
	pkgLoadError sync.Map // importPath -> struct{}; fail once per process
	typeKeyCache sync.Map // ast.Expr -> type key
)

type TypeParser struct {
	doc      *openapi3.T
	pkg      *packages.Package
	session  *GenerationSession
	audience Audience

	schemaMu sync.RWMutex // protects the writes to doc
	defers   *list.List
	// mergeTasks are the MergeTasks, to run until they change nothing, and then the
	// FinalTasks, in the order they were made.
	mergeTasks *list.List
	valueCheck *valueChecker // for a parser that has no session
	// onDemand is set on the parser of a package that no pattern of models matches,
	// which describes the types of the package that a type of another package reaches
	// and no more: nothing else reads the rest of it.
	onDemand    bool
	activeTypes map[string]bool
	activeMu    sync.Mutex
	// embedding holds the structs whose field names are being collected.
	embedding map[*ast.StructType]bool
}

type MergeTask struct {
	TargetKey string
	SourceKey string
}

// FinalTask is what is left to do once every type is described and the fields of
// the structs that are embedded have been passed on to those that embed them.
type FinalTask func()

func NewTypeParser(pkg *packages.Package, doc *openapi3.T, sessions ...*GenerationSession) *TypeParser {
	var session *GenerationSession
	var audience Audience
	if len(sessions) > 0 && sessions[0] != nil {
		session = sessions[0]
		audience = session.audience
	}
	return &TypeParser{
		doc:         doc,
		pkg:         pkg,
		session:     session,
		audience:    audience,
		defers:      list.New(),
		mergeTasks:  list.New(),
		activeTypes: make(map[string]bool),
		embedding:   make(map[*ast.StructType]bool),
	}
}

// ParseAllDecls parses all type declarations of the package.
func (p *TypeParser) ParseAllDecls() {
	for _, file := range p.pkg.Syntax {
		pkgAliases := parseFileImportsOf(p.pkg, file)

		for _, decl := range file.Decls {
			// type declarations (such as type User struct)
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						ctx := &ParseContext{
							importAlias: pkgAliases,
							Doc:         genDecl.Doc,
							Comment:     typeSpec.Comment,
						}
						p.warnAboutDirectiveMistakes(typeSpec, genDecl)
						p.parseTypeSpec(typeSpec, ctx) // parse the type declaration
					}
				}
			}
		}
	}
}

// parseTypeSpec handles a type declaration (such as type User struct).
func (p *TypeParser) parseTypeSpec(typeSpec *ast.TypeSpec, ctx *ParseContext) {
	typeName := typeSpec.Name.Name

	fk := p.generateStructKey(typeName)
	if p.declarationHidden(typeSpec) {
		// No schema: nothing refers to a type that is hidden, and the operations
		// that use it are left out.
		p.deleteSchema(fk)
		p.markHidden(fk)
		return
	}
	if v := p.getRealSchemaFromDoc(fk); v != nil {
		p.updateSchemaInDocForce(fk, updateDescriptionForce(v, ctx.Doc, ctx.Comment))
	} else {
		p.occupySchema(fk) // placeholder
		p.parseTypeSpecOccupied(typeSpec, ctx, fk)
	}
}

func (p *TypeParser) parseTypeSpecOccupied(typeSpec *ast.TypeSpec, ctx *ParseContext, fullKey string) {
	p.activeMu.Lock()
	if p.activeTypes[fullKey] {
		p.activeMu.Unlock()
		return
	}
	p.activeTypes[fullKey] = true
	p.activeMu.Unlock()
	defer func() {
		p.activeMu.Lock()
		delete(p.activeTypes, fullKey)
		p.activeMu.Unlock()
	}()
	var (
		schema *openapi3.SchemaRef
		// the expression of the underlying type (such as "string" or "int")
		underlyingType = "default"
	)
	if typeSpec.TypeParams != nil {
		gt := typeSpec.TypeParams.List[0].Type // only the first type parameter is supported for now
		ctx.GenericValue = gt
	}
	if typeSpec.Doc != nil {
		ctx.Doc = typeSpec.Doc
	}
	ctx.FullKey = fullKey
	if configured := settings.TypeMap[fullKey]; configured != nil && !settings.CompatLegacyOutput {
		// What the configuration says the type is, is what it is, whatever it is
		// declared as: a struct too.
		copied := *configured
		schema = &openapi3.SchemaRef{Value: &copied}
		if copied.Description == "" {
			copied.Description = extractDescription(ctx.Doc, ctx.Comment)
		}
		p.handleSchema(typeSpec, fullKey, schema, underlyingType, ctx)
		return
	}
	// Go alias declarations (`type Alias = Target`) must resolve the target
	// schema without treating the alias component as the target's recursion
	// key. Preserve the alias component identity while copying the resolved
	// target shape.
	if typeSpec.Assign.IsValid() {
		aliasCtx := *ctx
		aliasCtx.FullKey = ""
		schema = p.parse(typeSpec.Type, &aliasCtx)
		underlyingType = "alias"
		if ident, ok := typeSpec.Type.(*ast.Ident); ok {
			underlyingType = ident.Name
		}
		p.handleSchema(typeSpec, fullKey, schema, underlyingType, ctx)
		p.describeWhenTheTypeIs(typeSpec, fullKey, schema)
		return
	}
	switch t := typeSpec.Type.(type) {
	case *ast.StructType:
		underlyingType = "StructType"
		schema = p.parseStruct(t, ctx)
		if !settings.CompatLegacyOutput && schema != nil && schema.Value != nil && schema.Value.Description == "" {
			schema.Value.Description = extractDescription(ctx.Doc, ctx.Comment)
		}
	case *ast.Ident:
		// type definitions (such as type T string)
		underlyingType = t.Name
		schema = p.parse(t, ctx)
		defer p.describeWhenTheTypeIs(typeSpec, fullKey, schema)
	case *ast.SelectorExpr:
		// external types (such as time.Time)
		underlyingType = fmt.Sprintf("%s.%s", t.X, t.Sel.Name)
		schema = p.parse(t, ctx)
	default:
		schema = p.parse(t, ctx)
	}

	p.handleSchema(typeSpec, fullKey, schema, underlyingType, ctx)
}

// parse is the core of parsing, without locks.
func (p *TypeParser) parse(expr ast.Expr, ctx *ParseContext) (schema *openapi3.SchemaRef) {
	if expr == nil {
		// A field of a type parameter that no type argument was given for (a
		// generic declaration that nothing instantiates), or of a type that is
		// named T, which is read as a type parameter: there is nothing to describe.
		if ctx != nil && ctx.Field != nil {
			Logger().Debug("field of a type parameter has no type argument and is left out", "at", p.at(ctx.Field))
		}
		return nil
	}
	// the actual parsing (no locks here)
	fk := ctx.FullKey
	if fk == "" {
		fk = p.generateTypeKey(expr)
	}
	if ctx.GenericValue != nil {
		fk = getGenericFullKey(fk, ctx.GenericValue)
		ctx.FullKey = fk
	}
	defer func() {
		if schema != nil && schema.Value != nil && ctx != nil {
			if settings.CompatLegacyOutput || schema.Ref == "" {
				// nullability only matters in the context of a field
				schema.Value.Nullable = isNullableFromField(ctx.Field)
				// A comment is the description; without one, what the schema has
				// already stays. A type that type_map gives a description has that
				// one, whatever its own comment says.
				desc := extractDescription(ctx.Doc, ctx.Comment)
				described := ctx.FullKey != "" && settings.TypeMap[ctx.FullKey] != nil && schema.Value.Description != ""
				if settings.CompatLegacyOutput || !described && (desc != "" || schema.Value.Description == "") {
					schema.Value.Description = desc
				}
			}
			// A reference has no description or nullability of its own: that of the
			// field that uses it is written with it (see describeReference), since
			// what it refers to is shared with every other use.
		}
		// nil has to be allowed to update here too!
		// only some types need their schema written
		if strings.HasPrefix(fk, ownKeyPrefix()) {
			p.updateSchemaInDoc(fk, schema)
		} else {
			p.deleteSchema(fk)
		}
		if schema == nil {
			p.deleteSchema(fk)
		}
	}()

	// Should it be hidden? This comes before the cache: a type that was described
	// before this reference is reached must not be copied into it.
	if p.referenceHidden(expr, fk, ctx) {
		return nil
	}

	if cached := p.getRealSchemaFromDoc(fk); cached != nil {
		// updateDescription cannot be used here: it would pollute the comments of the structure
		v := CopyRef(cached)
		return v
	}

	vs := strings.Split(fk, ".")
	last := vs[len(vs)-1]

	if v := getBasicTypeSchema(last); v != nil {
		return CopyRef(v.NewRef())
	} else if v := getBasicTypeSchema(fk); v != nil {
		return CopyRef(v.NewRef())
	}

	p.occupySchema(fk)
	switch t := expr.(type) {
	case *ast.Ident:
		schema = p.parseIdent(t, ctx)
	case *ast.IndexExpr:
		fk, schema = p.parseIndexExpr(fk, t, ctx)
	case *ast.IndexListExpr:
		warnAt("generic type with several type arguments is not supported", "key", fk, "at", p.at(t))
	case *ast.StarExpr:
		schema = p.parsePointer(t, ctx)
	case *ast.ArrayType:
		ctx.FullKey = ""
		schema = p.parseArray(t, ctx)
	case *ast.StructType:
		ctx.FullKey = ""
		schema = p.parseStruct(t, ctx)
	case *ast.SelectorExpr:
		schema = p.parseSelector(t, ctx)
	case *ast.MapType:
		ctx.FullKey = ""
		schema = p.parseMap(t, ctx)
	case *ast.InterfaceType:
		ctx.FullKey = ""
		schema = p.parseInterface(t, ctx)
	case *ast.FuncType, *ast.ChanType:
		// No schema describes a function or a channel.
		return nil
	default:
		warnAt("type expression has no rule", "key", fk, "expr", goType(t), "at", p.at(t))
		return nil
	}
	// copyRef cannot be used here
	return schema
}

// describeWhenTheTypeIs gives the component of a type that is declared as another
// type of the package, type Money2 Money or type Alias = Money, the schema of that
// type as soon as it is described, when it is declared further down and is not yet.
// The type it ends in is looked for, not the one on its right, which can itself be
// declared as another, so that it does not matter in what order a chain is declared.
func (p *TypeParser) describeWhenTheTypeIs(typeSpec *ast.TypeSpec, fullKey string, schema *openapi3.SchemaRef) {
	if settings.CompatLegacyOutput || schema == nil || schema.Ref == "" || !isDefaultSchema(schema.Value) {
		return
	}
	target := p.rootTypeKey(typeSpec)
	p.defers.PushBack(func() {
		if described := p.getRealSchemaFromDoc(target); described != nil && described.Value != nil {
			p.updateSchemaInDocForce(fullKey, CopyRef(described))
		}
	})
}

// rootTypeKey is the key of the type that a chain of declarations of the package
// ends in: Money for type A = B; type B Money.
func (p *TypeParser) rootTypeKey(spec *ast.TypeSpec) string {
	for range 32 {
		ident, ok := spec.Type.(*ast.Ident)
		if !ok || p.pkg == nil || p.pkg.TypesInfo == nil {
			break
		}
		obj, ok := p.pkg.TypesInfo.Uses[ident].(*types.TypeName)
		if !ok || obj.Pkg() == nil || obj.Pkg().Path() != p.pkg.PkgPath {
			break
		}
		next, _ := findTypeDeclaration(p.pkg, obj.Name())
		if next == nil {
			break
		}
		spec = next
	}
	return p.generateStructKey(spec.Name.Name)
}
