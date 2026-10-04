package engine

type ParamSpec struct {
	In          string // path/query/body
	Name        string
	Example     interface{}
	Types       []*TypeDescriptor
	Required    bool
	Description string
}
