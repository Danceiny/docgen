package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/Danceiny/docgen/internal/engine"

	"gopkg.in/yaml.v3"

	"github.com/getkin/kin-openapi/openapi3"
)

func generateInternalYAML(doc *openapi3.T, outputPath string, genericTitles []string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	// serialize with GenerateYAML, which is safe: it handles circular references
	yamlData, err := GenerateYAML(doc, genericTitles)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, yamlData, 0644)
}

// generatePublicYAML generates the public API document, with the slimming of the schemas
func generatePublicYAML(doc *openapi3.T, outputPath string, genericTitles, forceKeep []string, public publicOptions) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	// first the tags: remove the internal ones and keep the public ones
	optimizeTags(doc, public)

	// then the paths: keep only the operations that have the public tag
	filterPublicPaths(doc, public.tag)

	if !legacyOutput {
		// Documents were always written with the schemas that fields use copied
		// into place, because the titles that say which component a copy is of
		// were made short before anything looked at them. The references are made
		// first, so that what is used is known and nothing is copied.
		slimSchemas(doc, genericTitles)
	}

	// last, remove the schemas that are no longer used (judged by the filtered paths)
	removeUnusedSchemas(doc, forceKeep)

	simplifyTitles(doc)

	// order anyOf/allOf so that the error responses come last
	if public.errorsLast {
		optimizeAnyOfAllOfOrder(doc)
	}

	// log the statistics of the optimization
	printOptimizationStats(doc)
	yamlData, err := GenerateYAML(doc, genericTitles)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, yamlData, 0644)
}

// GenerateYAML serializes the document with a wrapper of our own.
func GenerateYAML(doc *openapi3.T, genericTitles []string) ([]byte, error) {
	slimSchemas(doc, genericTitles)
	if err := normalizeLocalSchemaRefs(doc); err != nil {
		return nil, err
	}
	if err := checkSchemasHaveNoCycles(doc); err != nil {
		return nil, err
	}

	// Marshal before validation so validation observes the exact wire contract,
	// including recursive local refs and all transformer changes.
	type safeDoc openapi3.T
	yamlData, err := yaml.Marshal((*safeDoc)(doc))
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAPI document: %w", err)
	}
	// Comments can contain tabs, which yaml.v3 preserves inside literal
	// blocks. Expand them in the wire document so generated files remain
	// whitespace-clean and portable across YAML parsers.
	yamlData = bytes.ReplaceAll(yamlData, []byte{'\t'}, []byte("    "))
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	parsed, err := loader.LoadFromData(yamlData)
	if err != nil {
		return nil, fmt.Errorf("load marshaled OpenAPI document: %w", err)
	}
	if err := parsed.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("validate marshaled OpenAPI document: %w", err)
	}
	return yamlData, nil
}

// slimSchemas cleans up the references of the components, and turns the schema
// of a field that says which component it is a copy of into a reference to it.
func slimSchemas(doc *openapi3.T, genericTitles []string) {
	// 1. clean up the invalid references
	hider := &oneOfHider{genericTitles: genericTitles, done: map[hiddenOneOf]hiddenOneOfResult{}}
	for _, k := range sortedKeys(doc.Components.Schemas) {
		v := doc.Components.Schemas[k]
		if v != nil && v.Ref == engine.NewRefFromFullKey(k) {
			// A named slice/alias may be emitted as a self-ref. The component
			// already carries its parsed value, so retain that value and avoid
			// serializing an immediately recursive root definition.
			copyRef := *v
			copyRef.Ref = ""
			v = &copyRef
		}
		v = processSchemaRef(doc, v)
		if vv, updated := hider.hide(v, ""); updated {
			doc.Components.Schemas[k] = vv
		} else {
			doc.Components.Schemas[k] = v
		}
	}
	for _, k := range sortedKeys(doc.Components.Schemas) {
		v := doc.Components.Schemas[k]
		if v != nil && v.Ref == engine.NewRefFromFullKey(k) {
			copyRef := *v
			copyRef.Ref = ""
			v = &copyRef
		}
		doc.Components.Schemas[k] = slimComponent(doc, k, v)
	}
}

// normalizeLocalSchemaRefs verifies that every local schema reference has a
// component. A missing reference is a generator defect: erasing it (or
// replacing it with an object) changes the contract and hides schema drift.
func normalizeLocalSchemaRefs(doc *openapi3.T) error {
	seen := make(map[*openapi3.Schema]bool)
	return documentSchemas(doc, func(_ string, ref *openapi3.SchemaRef) error {
		return normalizeSchemaRef(doc, ref, seen)
	})
}

// documentSchemas calls visit for every schema the document declares or uses
// directly: the components, then the parameters, request bodies and responses
// of every operation. It goes through them in an order that does not depend on
// how a map iterates, goes on after a failure, and returns the failures joined,
// each saying where it happened.
func documentSchemas(doc *openapi3.T, visit func(where string, ref *openapi3.SchemaRef) error) error {
	var all error
	visitIn := func(where string, ref *openapi3.SchemaRef) {
		if err := visit(where, ref); err != nil {
			if strings.HasPrefix(where, "path ") {
				err = fmt.Errorf("%w; a method that is not part of the API is left out of the documents by the annotation \"@apidoc: -\"", err)
			}
			all = errors.Join(all, fmt.Errorf("%s: %w", where, err))
		}
	}
	visitContent := func(where string, content openapi3.Content) {
		for _, mediaType := range slices.Sorted(maps.Keys(content)) {
			if media := content[mediaType]; media != nil {
				visitIn(where, media.Schema)
			}
		}
	}
	for _, key := range sortedKeys(doc.Components.Schemas) {
		visitIn("component "+key, doc.Components.Schemas[key])
	}
	paths := doc.Paths.Map()
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		for _, op := range paths[path].Operations() {
			for _, param := range op.Parameters {
				if param != nil && param.Value != nil {
					visitIn("path "+path+" method parameter", param.Value.Schema)
				}
			}
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				visitContent("path "+path+" request body", op.RequestBody.Value.Content)
			}
			if op.Responses != nil {
				responses := op.Responses.Map()
				for _, status := range slices.Sorted(maps.Keys(responses)) {
					if response := responses[status]; response != nil && response.Value != nil {
						visitContent("path "+path+" response", response.Value.Content)
					}
				}
			}
		}
	}
	return all
}

// schemaEdge is a schema contained in another one, and what it is there.
type schemaEdge struct {
	name string
	ref  *openapi3.SchemaRef
}

// schemaChildren lists the schemas a schema contains, in an order that does not
// depend on how a map iterates.
func schemaChildren(schema *openapi3.Schema) []schemaEdge {
	var edges []schemaEdge
	add := func(name string, ref *openapi3.SchemaRef) {
		if ref != nil {
			edges = append(edges, schemaEdge{name, ref})
		}
	}
	for _, named := range []struct {
		kind    string
		schemas openapi3.Schemas
	}{
		{"properties", schema.Properties},
		{"patternProperties", schema.PatternProperties},
		{"dependentSchemas", schema.DependentSchemas},
		{"$defs", schema.Defs},
	} {
		for _, name := range sortedKeys(named.schemas) {
			add(named.kind+"."+name, named.schemas[name])
		}
	}
	for _, list := range []struct {
		kind    string
		schemas openapi3.SchemaRefs
	}{
		{"allOf", schema.AllOf},
		{"anyOf", schema.AnyOf},
		{"oneOf", schema.OneOf},
		{"prefixItems", schema.PrefixItems},
	} {
		for i, child := range list.schemas {
			add(fmt.Sprintf("%s[%d]", list.kind, i), child)
		}
	}
	add("items", schema.Items)
	add("not", schema.Not)
	add("contains", schema.Contains)
	add("propertyNames", schema.PropertyNames)
	add("if", schema.If)
	add("then", schema.Then)
	add("else", schema.Else)
	add("contentSchema", schema.ContentSchema)
	add("additionalProperties", schema.AdditionalProperties.Schema)
	add("unevaluatedItems", schema.UnevaluatedItems.Schema)
	add("unevaluatedProperties", schema.UnevaluatedProperties.Schema)
	return edges
}

func normalizeSchemaRef(doc *openapi3.T, ref *openapi3.SchemaRef, seen map[*openapi3.Schema]bool) error {
	if ref == nil {
		return nil
	}
	if strings.HasPrefix(ref.Ref, "#/components/schemas/") {
		key := strings.TrimPrefix(ref.Ref, "#/components/schemas/")
		if _, ok := doc.Components.Schemas[key]; !ok {
			if key == "unknown" || strings.HasPrefix(key, "unknown.") {
				return fmt.Errorf("missing local schema reference %q: the type is not a named type of the module: a map, a list of lists or another unnamed type cannot be what an operation takes or returns directly; wrap it in a struct", ref.Ref)
			}
			return fmt.Errorf("missing local schema reference %q: no schema was generated for that type; the package that declares it has to match a pattern of \"models\" in the configuration, or the type has to be declared in the module", ref.Ref)
		}
	}
	if ref.Value == nil || seen[ref.Value] {
		return nil
	}
	seen[ref.Value] = true
	schema := ref.Value
	if len(schema.Enum) > 0 && schema.Example != nil {
		validExample := false
		for _, value := range schema.Enum {
			if reflect.DeepEqual(value, schema.Example) {
				validExample = true
				break
			}
		}
		if !validExample {
			schema.Example = nil
		}
	}
	for _, edge := range schemaChildren(schema) {
		if err := normalizeSchemaRef(doc, edge.ref, seen); err != nil {
			return err
		}
	}
	return nil
}

// checkSchemasHaveNoCycles fails when a schema contains itself other than
// through a $ref. Such a document cannot be written: the YAML encoder would
// follow the cycle until the process ran out of memory. A type that contains
// itself is described once, as a component, and referred to by name.
func checkSchemasHaveNoCycles(doc *openapi3.T) error {
	onPath := make(map[*openapi3.Schema]bool)
	done := make(map[*openapi3.Schema]bool)
	var walk func(where string, ref *openapi3.SchemaRef) error
	walk = func(where string, ref *openapi3.SchemaRef) error {
		// A reference ends the description: what it names is described where it
		// is declared.
		if ref == nil || ref.Ref != "" || ref.Value == nil || done[ref.Value] {
			return nil
		}
		schema := ref.Value
		if onPath[schema] {
			return fmt.Errorf("%s contains itself; a type that contains itself must be a $ref", strings.TrimPrefix(where, "."))
		}
		onPath[schema] = true
		defer func() {
			delete(onPath, schema)
			done[schema] = true
		}()
		for _, edge := range schemaChildren(schema) {
			if err := walk(where+"."+edge.name, edge.ref); err != nil {
				return err
			}
		}
		return nil
	}
	return documentSchemas(doc, func(_ string, ref *openapi3.SchemaRef) error {
		return walk("", ref)
	})
}

func parseTitleGenericTypes(title string) []string {
	return filter(strings.Split(engine.ExtractGenericType(title), ","), func(s string) bool {
		return s != ""
	})
}

// processSchemaRef cleans up the reference of one SchemaRef, and turns a title into a reference.
func processSchemaRef(doc *openapi3.T, schemaRef *openapi3.SchemaRef) *openapi3.SchemaRef {
	if schemaRef == nil {
		return schemaRef
	}

	// is schemaRef.Value nil?
	if schemaRef.Value == nil {
		return schemaRef
	}

	// cleaning up references: a reference with the pattern "default" and a description is replaced by the content it refers to
	if schemaRef.Ref != "" && schemaRef.Value.Pattern == "default" && schemaRef.Value.Description != "" {
		key := strings.TrimPrefix(schemaRef.Ref, "#/components/schemas/")
		if v, ok := doc.Components.Schemas[key]; ok && v != schemaRef {
			desc := schemaRef.Value.Description
			schemaRef.Value = engine.CopyRef(v).Value
			schemaRef.Value.Description = desc
			schemaRef.Ref = "" // clear ref
		}
		// Otherwise the unresolved ref is preserved, so that normalizeLocalSchemaRefs
		// can fail closed instead of silently changing the contract.
	} else if strings.HasPrefix(schemaRef.Ref, "#/components/schemas/") {
		// A Go type alias can intentionally point at a non-exported package
		// component. If its parsed value is already embedded, keep the public
		// document self-contained instead of emitting an unresolved local ref.
		// Keep local refs intact; the final normalization pass reports a missing
		// component with its exact reference.
		return schemaRef
	} else if schemaRef.Value.Title != "" {
		// turning a title into a reference: if the title names a schema of the components, use a reference
		if v, ok := doc.Components.Schemas[schemaRef.Value.Title]; ok && v != schemaRef {
			// fields of a struct that go through select (referenced from an external package) are duplicated, so they are handled specially here
			schemaRef.Ref = engine.NewRefFromFullKey(schemaRef.Value.Title)
		}
	}

	return schemaRef
}

// slimComponent slims the schema of the component with the given key. The
// component counts as being expanded while its schema is walked, so that a
// reference to it from inside itself stays a reference.
func slimComponent(doc *openapi3.T, key string, rootRef *openapi3.SchemaRef) *openapi3.SchemaRef {
	return slimPropertiesWithVisited(doc, rootRef, map[string]bool{engine.NewRefFromFullKey(key): true})
}

// slimChild slims a schema contained in the one being walked. A reference to a
// component that is being expanded, because the walk is inside it, stays a
// reference. Without that, a type that contains itself (a tree of nodes, a
// folder of folders) would be expanded into itself: processSchemaRef puts the
// component's schema in place of a reference that has a description, and the
// copy it makes shares its properties with the component.
func slimChild(doc *openapi3.T, child *openapi3.SchemaRef, visited map[string]bool) *openapi3.SchemaRef {
	ref := ""
	if child != nil {
		ref = child.Ref
	}
	if ref != "" && visited[ref] {
		return &openapi3.SchemaRef{Ref: ref}
	}
	expanded := processSchemaRef(doc, child)
	if !legacyOutput && expanded != nil && expanded.Ref != "" {
		// A reference is written as one: what it refers to is a component, which is
		// slimmed on its own turn. Going down into it again for every path that leads
		// to it makes the walk as long as the number of paths, which is what a graph
		// of types that share types has far more of than types. (Documents that were
		// generated before depend on where the walk cut the cycles it met on the
		// way, so a configuration that keeps legacy_output keeps the walk.)
		return expanded
	}
	if ref != "" && expanded != nil && expanded.Ref == "" {
		// The schema of the component is in place of the reference: walking it is
		// walking inside the component.
		visited[ref] = true
		defer delete(visited, ref)
	}
	return slimPropertiesWithVisited(doc, expanded, visited)
}

func slimPropertiesWithVisited(doc *openapi3.T, rootRef *openapi3.SchemaRef, visited map[string]bool) *openapi3.SchemaRef {
	root := rootRef.Value
	if root == nil {
		return rootRef
	}

	// the unique identity of this node
	nodeID := getSchemaNodeID(rootRef)

	// has this node been visited already? That would be a circular reference
	if visited[nodeID] {
		// return a reference instead of going on
		if rootRef.Ref != "" {
			return &openapi3.SchemaRef{Ref: rootRef.Ref}
		}
		// for a cycle with no reference, return a simplified schema
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:        root.Type,
				Description: root.Description + " (circular reference detected)",
			},
		}
	}

	// mark this node as visited
	visited[nodeID] = true

	sort.Strings(root.Required) // so that the order is the same every time the document is generated
	// the properties
	for k, prop := range root.Properties {
		if prop == nil {
			continue
		}
		root.Properties[k] = slimChild(doc, prop, visited)
	}

	// the array items
	if root.Items != nil {
		root.Items = slimChild(doc, root.Items, visited)
	}

	// the values of a map; documents generated before they were written are
	// left as they were
	if !legacyOutput && root.AdditionalProperties.Schema != nil {
		root.AdditionalProperties.Schema = slimChild(doc, root.AdditionalProperties.Schema, visited)
	}

	// AllOf
	for i, subSchema := range root.AllOf {
		if subSchema != nil {
			root.AllOf[i] = slimChild(doc, subSchema, visited)
		}
	}

	// OneOf
	for i, subSchema := range root.OneOf {
		if subSchema != nil {
			root.OneOf[i] = slimChild(doc, subSchema, visited)
		}
	}

	// AnyOf
	for i, subSchema := range root.AnyOf {
		if subSchema != nil {
			root.AnyOf[i] = slimChild(doc, subSchema, visited)
		}
	}

	// Stabilize composition order so regen drift gates are deterministic.
	sortSchemaRefsByKey(root.AllOf)
	sortSchemaRefsByKey(root.OneOf)
	sortSchemaRefsByKey(root.AnyOf)
	// when done, remove this node from visited, so that it can be visited again by another path
	delete(visited, nodeID)

	if len(root.OneOf) == 1 {
		return root.OneOf[0]
	}
	return rootRef
}

func schemaRefSortKey(ref *openapi3.SchemaRef) string {
	if ref == nil {
		return ""
	}
	if ref.Ref != "" {
		return "ref:" + ref.Ref
	}
	if ref.Value == nil {
		return "schema:empty"
	}
	// Title and type are useful primary keys, but are not unique: composition
	// branches commonly share both. Always append canonical structural JSON so
	// reverse input order cannot affect the emitted document.
	structural := ""
	if encoded, err := json.Marshal(ref.Value); err == nil {
		structural = string(encoded)
	}
	if ref.Value.Title != "" {
		return "title:" + ref.Value.Title + "\x00" + structural
	}
	if ref.Value.Type != nil && len(*ref.Value.Type) > 0 {
		return "type:" + strings.Join([]string(*ref.Value.Type), ",") + "\x00" + structural
	}
	// encoding/json sorts map keys and serializes pointed-to values, yielding a
	// stable structural key without process-specific pointer addresses.
	if structural != "" {
		return "schema:" + structural
	}
	return "schema:unserializable"
}

func sortSchemaRefsByKey(refs openapi3.SchemaRefs) {
	if len(refs) < 2 {
		return
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return schemaRefSortKey(refs[i]) < schemaRefSortKey(refs[j])
	})
}

// getSchemaNodeID makes the unique identity of a schema node.
func getSchemaNodeID(schemaRef *openapi3.SchemaRef) string {
	if schemaRef == nil {
		return "nil"
	}

	// a reference is the identity by preference
	if schemaRef.Ref != "" {
		return schemaRef.Ref
	}

	// a schema with no reference is identified by its memory address
	if schemaRef.Value != nil {
		return fmt.Sprintf("schema@%p", schemaRef.Value)
	}

	return fmt.Sprintf("schemaref@%p", schemaRef)
}

// oneOfHider narrows the oneOf of the schemas of generic types by the type
// arguments in their titles. What it makes of a schema depends on the schema and
// the title it is under, and on nothing else, so it is made once: the schemas
// that types share are reached by every path that leads to them, and the paths
// of a graph of types are far more than its types.
type oneOfHider struct {
	genericTitles []string
	done          map[hiddenOneOf]hiddenOneOfResult
}

type hiddenOneOf struct {
	schema      *openapi3.SchemaRef
	parentTitle string
}

type hiddenOneOfResult struct {
	ref     *openapi3.SchemaRef
	updated bool
}

func (h *oneOfHider) hide(rootRef *openapi3.SchemaRef, parentTitle string) (*openapi3.SchemaRef, bool) {
	key := hiddenOneOf{rootRef, parentTitle}
	if r, ok := h.done[key]; ok {
		return r.ref, r.updated
	}
	ref, updated := h.hideOnce(rootRef, parentTitle)
	h.done[key] = hiddenOneOfResult{ref, updated}
	return ref, updated
}

func (h *oneOfHider) hideOnce(rootRef *openapi3.SchemaRef, parentTitle string) (*openapi3.SchemaRef, bool) {
	genericTitles := h.genericTitles
	root := rootRef.Value
	if root == nil {
		return rootRef, false
	}

	title := root.Title
	if slices.Contains(genericTitles, title) {
		return rootRef, false
	}
	vs := parseTitleGenericTypes(title)
	if len(vs) == 0 {
		title = parentTitle
		vs = parseTitleGenericTypes(title)
	}
	if len(vs) > 0 && len(root.OneOf) > 0 {
		ofs := make(openapi3.SchemaRefs, 0, len(vs))
		for _, of := range root.OneOf {
			for _, vv := range vs {
				if strings.HasSuffix(of.Ref, vv) {
					ofs = append(ofs, of)
					break
				}
			}
		}

		if removed := len(root.OneOf) - len(ofs); removed > 0 {
			if len(ofs) == 1 {
				return ofs[0], true
			}
			cop := *root
			cop.OneOf = ofs
			return &openapi3.SchemaRef{
				Ref:   rootRef.Ref,
				Value: &cop,
			}, true
		}
	}

	overwritePM := make(map[string]*openapi3.SchemaRef)
	for k, v := range root.Properties {
		if v == nil || v.Value == nil {
			continue
		}

		if vv, ok := h.hide(v, title); ok {
			overwritePM[k] = vv
		}
	}
	if len(overwritePM) > 0 {
		cop := *root
		cop.Properties = make(map[string]*openapi3.SchemaRef)
		for k, v := range root.Properties {
			cop.Properties[k] = v
		}
		for k, v := range overwritePM {
			cop.Properties[k] = v
		}

		tmp := *rootRef
		tmp.Value = &cop
		return &tmp, true
	}

	if root.Items != nil {
		if vv, ok := h.hide(root.Items, title); ok {
			cop := *root
			cop.Items = vv
			tmp := *rootRef
			tmp.Value = &cop
			return &tmp, true
		}
	}
	return rootRef, false
}
