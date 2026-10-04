package engine

import "slices"

// The audiences a document can be written for. A document's visibility rules
// (apidoc tags, //apidoc: directives, the Internal name prefix) decide per
// audience which types, fields and enum values it shows.
const (
	AudienceInternal = "internal"
	AudiencePublic   = "public"
)

// Audience says who reads a generated document.
type Audience struct {
	// Name is AudienceInternal or AudiencePublic. Any other value, including the
	// zero value, is a neutral audience that matches neither.
	Name string
	// LegacyFieldTokens are the values of apidoc:"<value>" struct tags that make
	// a field visible to this audience. A field whose apidoc value is neither "-",
	// a visibility keyword nor one of these tokens is hidden.
	LegacyFieldTokens []string
	// HiddenTypePrefixes are name prefixes: a type whose name starts with one of
	// them, and that has no //apidoc: directive, is hidden from this audience.
	HiddenTypePrefixes []string
}

func (a Audience) isPublic() bool   { return a.Name == AudiencePublic }
func (a Audience) isInternal() bool { return a.Name == AudienceInternal }

func (a Audience) listsFieldToken(token string) bool {
	return slices.Contains(a.LegacyFieldTokens, token)
}
