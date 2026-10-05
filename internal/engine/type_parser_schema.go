package engine

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

func (p *TypeParser) handleSchema(typeSpec *ast.TypeSpec, fullKey string, schema *openapi3.SchemaRef, underlyingType string) {
	if !settings.CompatLegacySchemaShapes {
		// What the type is made of is what the compiler says, not what the
		// declaration happens to call it: byte and uint8 are the same, and a type
		// declared as another type of the module is made of what that is made of.
		if name := underlyingBasicName(p.pkg, typeSpec); name != "" {
			underlyingType = name
		}
	}
	// should the enum be hidden?
	if shouldHideType(p.pkg, typeSpec, p.audience) {
		schema = generateHiddenSchema(underlyingType)
	} else if isEnumType(p.pkg, typeSpec) && (settings.CompatLegacySchemaShapes || settings.TypeMap[fullKey] == nil) {
		// make the enum schema with visibility control; a schema that the
		// configuration gives the type (type_map) is the one it has, as for any other type
		schema = generateEnumSchemaWithVisibility(p.pkg, typeSpec, underlyingType, p.audience)
	}
	p.updateSchemaInDocForce(fullKey, schema)
}

func updateDescriptionForce(ref *openapi3.SchemaRef, doc, comment *ast.CommentGroup) *openapi3.SchemaRef {
	ref.Value.Description = extractDescription(doc, comment)
	return ref
}

func updateDescription(ref *openapi3.SchemaRef, doc, comment *ast.CommentGroup) *openapi3.SchemaRef {
	schemaCopy := *ref.Value
	desc := extractDescription(doc, comment)
	// the schema of any has no type
	isBasic := schemaCopy.Type != nil && len(*schemaCopy.Type) > 0 && isBasicType((*schemaCopy.Type)[0])

	switch {
	case desc != "" && isBasic:
		schemaCopy.Description = desc + "; " + schemaCopy.Description
	case desc != "":
		schemaCopy.Description = desc
	case !isBasic:
		schemaCopy.Description = ""
	}

	return &openapi3.SchemaRef{
		Value: &schemaCopy,
		Ref:   ref.Ref,
	}
}

func CopyRef(ref *openapi3.SchemaRef) *openapi3.SchemaRef {
	schemaCopy := *ref.Value // don't use deep copy
	return &openapi3.SchemaRef{
		Ref:   ref.Ref,
		Value: &schemaCopy,
	}
}

func (p *TypeParser) occupySchema(fk string) {
	p.schemaMu.Lock()
	defer p.schemaMu.Unlock()
	p.doc.Components.Schemas[fk] = openapi3.NewSchemaRef("", defaultSchema())
}
func (p *TypeParser) deleteSchema(key string) {
	p.schemaMu.Lock()
	defer p.schemaMu.Unlock()
	delete(p.doc.Components.Schemas, key)
}
func (p *TypeParser) updateSchemaInDocForce(key string, ref *openapi3.SchemaRef) {
	if ref == nil {
		return
	}

	if ref.Value != nil {
		p.schemaMu.Lock()
		defer p.schemaMu.Unlock()
		if ref.Value.Title == "" {
			ref.Value.Title = key
		}
		p.doc.Components.Schemas[key] = ref
	}
}
func (p *TypeParser) updateSchemaInDoc(key string, ref *openapi3.SchemaRef) {
	if ref == nil {
		return
	}

	if ref.Value != nil {
		p.schemaMu.Lock()
		defer p.schemaMu.Unlock()
		if existing, ok := p.doc.Components.Schemas[key]; ok && existing != nil {
			if isDefaultSchema(existing.Value) {
				if ref.Value.Title == "" {
					ref.Value.Title = key
				}
				p.doc.Components.Schemas[key] = ref
			}
		}
	}
}

func (p *TypeParser) getRealSchemaFromDoc(key string) *openapi3.SchemaRef {
	p.schemaMu.Lock()
	defer p.schemaMu.Unlock()
	v := p.doc.Components.Schemas[key]
	if v == nil {
		return nil
	}
	if isDefaultSchema(v.Value) {
		return nil
	}
	return v
}

func (p *TypeParser) searchSchemaFromDoc(key string) *openapi3.SchemaRef {
	p.schemaMu.Lock()
	defer p.schemaMu.Unlock()
	return searchSchemaFromDoc(p.doc, key)
}

func searchSchemaFromDoc(doc *openapi3.T, key string) *openapi3.SchemaRef {
	v := doc.Components.Schemas[key]
	if v != nil {
		return v
	}
	// Several components can end with the key (two packages that both declare a
	// type of that name). The smallest full key is the answer, so it does not
	// depend on how the map iterates.
	key = "." + key
	best := ""
	for k := range doc.Components.Schemas {
		if strings.HasSuffix(k, key) && (best == "" || k < best) {
			best = k
		}
	}
	if best == "" {
		return nil
	}
	return doc.Components.Schemas[best]
}

// underlyingBasicName is the name of the basic type that a declared type is made
// of, with the names that are other names for it resolved (byte is uint8, rune
// is int32), or "" when it is made of something else or the package has no type
// information.
func underlyingBasicName(pkg *packages.Package, typeSpec *ast.TypeSpec) string {
	if pkg == nil || pkg.TypesInfo == nil {
		return ""
	}
	obj, ok := pkg.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
	if !ok {
		return ""
	}
	basic, ok := obj.Type().Underlying().(*types.Basic)
	if !ok {
		return ""
	}
	switch kind := basic.Kind(); kind {
	case types.Uintptr:
		return "uint64"
	case types.UnsafePointer, types.Complex64, types.Complex128, types.Invalid:
		return ""
	default:
		return types.Typ[kind].Name()
	}
}
