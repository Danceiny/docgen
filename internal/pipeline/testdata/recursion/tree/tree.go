// Package tree declares types that contain themselves.
package tree

// Node is a tree.
type Node struct {
	// Name of the node.
	Name string `json:"name"`
	// Children are the nodes below this one.
	Children []*Node `json:"children"`
	// Parent is the node above this one.
	Parent *Node `json:"parent"`
	// Attributes of the node by name.
	Attributes map[string]*Node `json:"attributes"`
	// Siblings are the nodes beside this one.
	Siblings []Node `json:"siblings"`
}

// Folder and File contain each other.
type Folder struct {
	// Files in the folder.
	Files []*File `json:"files"`
	// Size of everything in the folder.
	Size int64 `json:"size"`
}

// File is in a folder.
type File struct {
	// Folder the file is in.
	Folder *Folder `json:"folder"`
	// Name of the file.
	Name string `json:"name"`
}

// GetReq asks for a tree.
type GetReq struct {
	// ID of the tree.
	ID string `json:"id"`
}
