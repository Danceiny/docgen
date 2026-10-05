package pipeline

import (
	"container/list"
	"errors"
	"fmt"

	"github.com/Danceiny/docgen/internal/engine"

	"github.com/getkin/kin-openapi/openapi3"
)

func GenerateAPIs(doc *openapi3.T, targetPkgIDs []string, sessions ...*engine.GenerationSession) error {
	engine.MountAllErrors(doc)
	if len(sessions) == 0 || sessions[0] == nil {
		return errors.New("API generation requires a GenerationSession")
	}
	pkgs := sessions[0].Packages()
	patterns := compilePatterns(targetPkgIDs)
	for _, pkg := range pkgs {
		// the path relative to the module
		relativePath := getRelativePath(pkg.ID)

		// does it match the target patterns?
		if matchesAnyPattern(relativePath, patterns) {
			services := engine.FindServiceImplementations(pkg)
			for _, service := range services {
				for _, method := range service.Methods {
					if hidden := sessions[0].HiddenTypeOf(method); hidden != "" {
						engine.Logger().Warn("operation is left out of the document: it uses a type that the document hides",
							"document", doc.Info.Title, "service", service.ServiceName, "method", method.Name, "type", hidden, "at", method.Pos)
						continue
					}
					engine.BuildPathItem(doc, service, method)
				}
			}
		}
	}
	return nil
}

func GenerateModels(doc *openapi3.T, inclusive, exclusive []string, sessions ...*engine.GenerationSession) error {
	if len(sessions) == 0 || sessions[0] == nil {
		return errors.New("model generation requires a GenerationSession")
	}
	// collect all models first
	pkgs := sessions[0].Packages()
	// filter the target packages
	// turn the wildcards into regular expressions
	inclusivePatterns := compilePatterns(inclusive)
	exclusivePatterns := compilePatterns(exclusive)
	defers := list.New()
	mergeTasks := list.New()

	// collect all type definitions
	sorted, err := orderPackages(pkgs)
	if err != nil {
		return fmt.Errorf("order packages by dependency: %w", err)
	}

	for _, pkgID := range sorted {
		for _, pkg := range pkgs {
			if pkg.ID == pkgID {
				// the path relative to the module
				relativePath := getRelativePath(pkg.ID)

				if matchesAnyPattern(relativePath, exclusivePatterns) {
					continue
				}
				// does it match the target patterns?
				if matchesAnyPattern(relativePath, inclusivePatterns) {
					d, m := engine.ProcessModelsWithSession(pkg, doc, sessions[0])
					defers.PushBackList(d)
					mergeTasks.PushBackList(m)
				}
			}
		}
	}

	// run the defer tasks until no new ones are produced (this resolves deeply nested embedding)
	for defers.Len() > 0 {
		// copy the current list of tasks and clear the original, so that new tasks are collected while these run
		currentDefers := list.New()
		currentDefers.PushBackList(defers)
		defers.Init()

		for v := currentDefers.Front(); v != nil; v = v.Next() {
			v.Value.(func())()
		}
	}

	runMergeTasks(doc, mergeTasks)
	return nil
}

// runMergeTasks gives a struct the fields of the structs it embeds, again and
// again until no pass adds one, so that fields are passed along embedding of any
// depth, whatever order the tasks are in. Every pass that changes something adds
// a property or a required field to a finite set, so the loop ends, cycles of
// embedding included.
func runMergeTasks(doc *openapi3.T, mergeTasks *list.List) {
	for changed := true; changed; {
		changed = false
		for v := mergeTasks.Front(); v != nil; v = v.Next() {
			task := v.Value.(engine.MergeTask)
			if mergeProperties(doc, task.TargetKey, task.SourceKey) {
				changed = true
			}
		}
	}
}

func mergeProperties(doc *openapi3.T, targetKey, sourceKey string) bool {
	targetRef, ok := doc.Components.Schemas[targetKey]
	if !ok || targetRef.Value == nil {
		return false
	}
	sourceRef, ok := doc.Components.Schemas[sourceKey]
	if !ok || sourceRef.Value == nil {
		return false
	}

	target := targetRef.Value
	source := sourceRef.Value
	changed := false

	// Merge properties
	if target.Properties == nil {
		target.Properties = make(openapi3.Schemas)
	}
	for k, v := range source.Properties {
		if _, exists := target.Properties[k]; !exists {
			target.Properties[k] = v
			changed = true
		}
	}

	// Merge required fields
	if len(source.Required) > 0 {
		oldLen := len(target.Required)
		merged := dedupe(append(append([]string{}, target.Required...), source.Required...))
		if len(merged) != oldLen {
			changed = true
		}
		target.Required = merged
	}
	return changed
}
