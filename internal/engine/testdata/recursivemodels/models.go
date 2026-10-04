// Package recursivemodels is a fixture: a type that contains itself.
package recursivemodels

// Node is a tree: its children are Nodes.
type Node struct {
	Name     string `json:"name"`
	Children []Node `json:"children"`
}
