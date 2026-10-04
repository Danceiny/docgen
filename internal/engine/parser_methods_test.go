package engine

import (
	"testing"
)

// Which methods of a service are operations: exported ones, except Name, the
// ones the annotation "@apidoc: -" hides and the ones the configuration skips.
func TestFilterValidMethods(t *testing.T) {
	withSettings(t, Settings{SkipMethods: []string{"SetStore", "GetChildren"}}, "example.com/shop")

	methods := []*Method{
		{Name: "Get"},
		{Name: "list"},     // unexported
		{Name: "Name"},     // the name of the service
		{Name: "SetStore"}, // configured to be skipped
		{Name: "Hidden", Hidden: true},
		{Name: "GetChildren"}, // configured to be skipped
		{Name: ""},
		{Name: "Search"},
	}
	var got []string
	for _, m := range filterValidMethods(methods) {
		got = append(got, m.Name)
	}
	if len(got) != 2 || got[0] != "Get" || got[1] != "Search" {
		t.Fatalf("operations = %v, want [Get Search]", got)
	}

	// Without the configuration only Name and the hidden ones go.
	withSettings(t, Settings{}, "example.com/shop")
	got = nil
	for _, m := range filterValidMethods(methods) {
		got = append(got, m.Name)
	}
	if len(got) != 4 {
		t.Fatalf("operations = %v, want Get SetStore GetChildren Search", got)
	}
}
