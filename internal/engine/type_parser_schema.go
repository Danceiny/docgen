package engine

import (
	"go/ast"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

func (p *TypeParser) handleSchema(typeSpec *ast.TypeSpec, fullKey string, schema *openapi3.SchemaRef, underlyingType string) {
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
