package pipeline

import (
	"fmt"
	"sort"

	"golang.org/x/tools/go/packages"
)

// orderPackages returns the IDs of the packages so that every package comes
// after the packages it imports. A package that imports nothing and that no
// loaded package imports is not in the dependency graph, and is not in its
// order either; it comes last, so that the types of a package are documented
// whether or not it has neighbours.
func orderPackages(pkgs []*packages.Package) ([]string, error) {
	sorted, err := topologicalSort(buildDependencyGraph(pkgs))
	if err != nil {
		return nil, err
	}
	placed := make(map[string]bool, len(sorted))
	for _, id := range sorted {
		placed[id] = true
	}
	var alone []string
	for _, pkg := range pkgs {
		if !placed[pkg.ID] {
			alone = append(alone, pkg.ID)
		}
	}
	sort.Strings(alone)
	return append(sorted, alone...), nil
}

// buildDependencyGraph builds the dependency graph of the packages.
func buildDependencyGraph(pkgs []*packages.Package) map[string][]string {
	graph := make(map[string][]string)
	for _, pkg := range pkgs {
		for _, imp := range pkg.Imports {
			graph[pkg.ID] = append(graph[pkg.ID], imp.ID)
		}
	}
	for id := range graph {
		sort.Strings(graph[id])
	}
	return graph
}

// topologicalSort sorts topologically.
func topologicalSort(graph map[string][]string) ([]string, error) {
	var sorted []string
	visited := make(map[string]bool)
	temp := make(map[string]bool)

	var visit func(node string) error
	visit = func(node string) error {
		if temp[node] {
			return fmt.Errorf("circular dependency detected: %s", node)
		}
		if !visited[node] {
			temp[node] = true
			for _, neighbor := range graph[node] {
				if err := visit(neighbor); err != nil {
					return err
				}
			}
			temp[node] = false
			visited[node] = true
			sorted = append(sorted, node)
		}
		return nil
	}

	nodes := make([]string, 0, len(graph))
	for node := range graph {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if err := visit(node); err != nil {
			return nil, err
		}
	}

	return sorted, nil
}
