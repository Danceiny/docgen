// Package domain is a fixture: types that are hidden from the public document in
// each of the ways there are, declared before and after the type that uses them.
package domain

// InternalBefore is hidden from the public document by its name, and is declared
// before the type that uses it.
type InternalBefore struct {
	// Secret is not for customers.
	Secret string `json:"secret"`
}

// StaffNote is hidden from the public document by its directive.
//
//apidoc:internal
type StaffNote struct {
	// Text is the note.
	Text string `json:"text"`
}

// Draft is hidden from every document.
//
//apidoc:hidden
type Draft struct {
	// Body is unfinished.
	Body string `json:"body"`
}

// InternalButPublic has a hidden name and says it is shown.
//
//apidoc:public
type InternalButPublic struct {
	// Note is shown.
	Note string `json:"note"`
}

// Order is what a customer bought; it uses every hidden type in every way.
type Order struct {
	// ID of the order.
	ID string `json:"id"`

	// Before is a pointer to a type declared above.
	Before *InternalBefore `json:"before,omitempty"`
	// BeforeValue is the type itself.
	BeforeValue InternalBefore `json:"beforeValue"`
	// BeforeList is a list of the type.
	BeforeList []InternalBefore `json:"beforeList"`
	// BeforePtrList is a list of pointers to the type.
	BeforePtrList []*InternalBefore `json:"beforePtrList"`
	// BeforeMap is a map of the type.
	BeforeMap map[string]*InternalBefore `json:"beforeMap"`
	// BeforeNested is a map of maps of pointers to a hidden type.
	BeforeNested map[string]map[string]*InternalBefore `json:"beforeNested"`

	// After is a pointer to a type declared below.
	After *InternalAfter `json:"after,omitempty"`
	// AfterPtrList is a list of pointers to the type.
	AfterPtrList []*InternalAfter `json:"afterPtrList"`

	// Note is hidden by its directive.
	Note *StaffNote `json:"note,omitempty"`
	// Draft is hidden everywhere.
	Draft *Draft `json:"draft,omitempty"`
	// Shown has a hidden name and an override.
	Shown *InternalButPublic `json:"shown,omitempty"`

	InternalAfter `json:"-"`
	// Level is an enum with a hidden name: it is shown, without its values.
	Level InternalLevel `json:"level"`
}

// InternalAfter is hidden from the public document by its name, and is declared
// after the type that uses it.
type InternalAfter struct {
	// Cost is not for customers.
	Cost int `json:"cost"`
}

// InternalLevel is an enum.
type InternalLevel string

const (
	// LevelLow is low.
	LevelLow InternalLevel = "low"
	// LevelHigh is high.
	LevelHigh InternalLevel = "high"
)
