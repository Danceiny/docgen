// Package groupeddecl is a fixture: types declared in a group, with the directive
// above each type and not above the group.
package groupeddecl

type (
	// Mode is an enum in a group, and only some of its values are public.
	//
	//apidoc:public:ModeA,ModeNope
	Mode int

	// Secret is a struct in the same group, which is not for the public.
	//
	//apidoc:internal
	Secret struct {
		Key string `json:"key"`
	}

	// Plain has no directive.
	Plain struct {
		Name string `json:"name"`
	}
)

// The modes.
const (
	ModeA Mode = 1
	ModeB Mode = 2
)

// Holder uses them.
type Holder struct {
	Mode   Mode   `json:"mode"`
	Secret Secret `json:"secret"`
	Plain  Plain  `json:"plain"`
}
