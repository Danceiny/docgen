package engine

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/tools/go/packages"
)

// Visibility is the visibility configuration of a type or a field.
type Visibility struct {
	Scope     string   // "public", "internal", "hidden", "custom"
	Whitelist []string // allowlist of values (custom scope only)
	Blacklist []string // blocklist of values
	Language  string   // supported languages, such as "zh", "en", "ar"
}

// EnumVisibility is the visibility configuration of an enum (kept for backward compatibility).
type EnumVisibility = Visibility

// parseTypeVisibility parses the visibility annotation of a type.
// The supported formats are:
// //apidoc:public - shown in the public document
// //apidoc:internal - shown in the internal document only
// //apidoc:hidden - never shown
// //apidoc:public,zh - shown in the public document, with Chinese comments
// //apidoc:public:StatusActive,StatusPending - only the listed values are shown in the public document
// //apidoc:internal:-StatusLegacy* - the values that start with StatusLegacy are left out of the internal document
// //apidoc:public,zh:StatusActive,StatusPending - the listed values are shown in the public document, with Chinese comments
func parseTypeVisibility(doc *ast.CommentGroup) *Visibility {
	if doc == nil {
		return &Visibility{Scope: "public"} // public by default
	}

	// look for an //apidoc:... tag
	apidocPattern := regexp.MustCompile(`//apidoc:([^\s]+)`)

	for _, comment := range doc.List {
		matches := apidocPattern.FindStringSubmatch(comment.Text)
		if len(matches) < 2 {
			continue
		}

		content := strings.TrimSpace(matches[1])
		return parseApidocContent(content)
	}

	return &Visibility{Scope: "public"} // public by default
}

// parseApidocContent parses the content of an apidoc annotation.
// The format is scope[,language][:values].
// For example: public,zh:StatusActive,StatusPending
func parseApidocContent(content string) *Visibility {
	// split at the colon to get the list of values
	parts := strings.Split(content, ":")
	scopeAndLang := strings.TrimSpace(parts[0])

	// parse the scope and the language
	scopeLangParts := strings.Split(scopeAndLang, ",")
	scope := strings.TrimSpace(scopeLangParts[0])

	visibility := &Visibility{
		Scope:     scope,
		Whitelist: []string{},
		Blacklist: []string{},
		Language:  "",
	}

	// is there a language?
	if len(scopeLangParts) > 1 {
		visibility.Language = strings.TrimSpace(scopeLangParts[1])
	}

	// parse the list of values, if there is one
	if len(parts) > 1 {
		values := strings.Split(parts[1], ",")
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}

			if strings.HasPrefix(value, "-") {
				// blocklist: starts with -
				visibility.Blacklist = append(visibility.Blacklist, strings.TrimPrefix(value, "-"))
			} else {
				// allowlist
				visibility.Whitelist = append(visibility.Whitelist, value)
			}
		}
	}

	return visibility
}

// shouldHideType reports whether a type (enum, struct, interface, ...) is hidden.
func shouldHideType(pkg *packages.Package, typeSpec *ast.TypeSpec, aud Audience) bool {
	// the comments of the type declaration
	var doc *ast.CommentGroup
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == typeSpec.Name.Name {
						doc = genDecl.Doc
						break
					}
				}
			}
		}
	}

	visibility := parseTypeVisibility(doc)

	// decide from the kind of document and the visibility configuration
	switch visibility.Scope {
	case "hidden":
		Logger().Debug("type is hidden by its visibility annotation", "type", typeSpec.Name.Name)
		return true
	case "internal":
		// internal type: hidden in the public document, shown in the internal one
		shouldHide := aud.isPublic()
		return shouldHide
	case "public":
		// public type: shown in both documents (internal is a superset of public)
		return false
	case "custom":
		// custom scope: decided by the allowlist and the blocklist
		return shouldHideCustomType(visibility, aud)
	default:
		// default: stay compatible with the Internal name prefix rule
		return shouldHideByDefault(typeSpec.Name.Name, aud)
	}
}

// shouldHideByDefault is the rule for a type with no //apidoc: directive: it is
// hidden when its name starts with one of the audience's hidden prefixes.
func shouldHideByDefault(typeName string, aud Audience) bool {
	for _, prefix := range aud.HiddenTypePrefixes {
		if strings.HasPrefix(typeName, prefix) {
			return true
		}
	}
	return false
}

// shouldHideField reports whether a field is hidden.
// Visibility can be controlled per field with the apidoc tag.
func shouldHideField(field *ast.Field, aud Audience) bool {
	// the value of the apidoc tag of the field
	apidocValue := getFieldApidocTag(field)
	if apidocValue == "" {
		return false // no apidoc tag: shown
	}

	// a tag value of "-" hides the field
	if apidocValue == "-" {
		return true
	}

	// is it the new visibility format (public, internal, hidden, custom)?
	if isNewVisibilityFormat(apidocValue) {
		// use the new visibility control
		visibility := parseFieldVisibility(apidocValue)
		return shouldHideByVisibility(visibility, aud)
	}

	// Legacy values (such as apidoc:"Staff"): shown only to an audience that lists
	// the value in its LegacyFieldTokens, hidden for everyone else.
	return !aud.listsFieldToken(apidocValue)
}

// isNewVisibilityFormat reports whether the value is in the new visibility format.
func isNewVisibilityFormat(apidocValue string) bool {
	validScopes := []string{"public", "internal", "hidden", "custom"}
	for _, scope := range validScopes {
		if apidocValue == scope || strings.HasPrefix(apidocValue, scope+":") {
			return true
		}
	}
	return false
}

// getFieldApidocTag returns the value of the apidoc tag of a field.
func getFieldApidocTag(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}

	tag := strings.Trim(field.Tag.Value, "`")
	for _, part := range strings.Split(tag, " ") {
		if strings.HasPrefix(part, "apidoc:") {
			value := strings.Trim(strings.TrimPrefix(part, "apidoc:"), `"`)
			return value
		}
	}
	return ""
}

// parseFieldVisibility parses the visibility configuration of a field.
// The formats are: public, internal, hidden, public:value1,value2, internal:-value1,-value2
func parseFieldVisibility(apidocValue string) *Visibility {
	// a colon means there is a list of values
	if strings.Contains(apidocValue, ":") {
		parts := strings.SplitN(apidocValue, ":", 2)
		scope := strings.TrimSpace(parts[0])
		values := strings.TrimSpace(parts[1])

		visibility := &Visibility{
			Scope:     scope,
			Whitelist: []string{},
			Blacklist: []string{},
		}

		// parse the list of values
		if values != "" {
			valueList := strings.Split(values, ",")
			for _, v := range valueList {
				v = strings.TrimSpace(v)
				if strings.HasPrefix(v, "-") {
					visibility.Blacklist = append(visibility.Blacklist, strings.TrimPrefix(v, "-"))
				} else {
					visibility.Whitelist = append(visibility.Whitelist, v)
				}
			}
		}

		return visibility
	}

	// a plain scope
	return &Visibility{
		Scope:     strings.TrimSpace(apidocValue),
		Whitelist: []string{},
		Blacklist: []string{},
	}
}

// shouldHideByVisibility reports whether the visibility configuration hides something.
func shouldHideByVisibility(visibility *Visibility, aud Audience) bool {
	switch visibility.Scope {
	case "hidden":
		return true
	case "public":
		return aud.isInternal()
	case "internal":
		return aud.isPublic()
	case "custom":
		// custom scope: with an allowlist or a blocklist more has to be decided
		// false for now: allowlists and blocklists of fields need more complex logic
		return false
	default:
		return false
	}
}

// shouldHideCustomType reports whether the custom visibility scope hides a type.
func shouldHideCustomType(visibility *Visibility, aud Audience) bool {
	// in the internal document, without an explicit allowlist, it is shown
	if aud.isInternal() && len(visibility.Whitelist) == 0 {
		return false
	}

	// in the public document, without an explicit allowlist, it is hidden
	if aud.isPublic() && len(visibility.Whitelist) == 0 {
		return true
	}

	return false
}

// collectVisibleEnumEntries collects the enum entries that are visible.
func collectVisibleEnumEntries(pkg *packages.Package, typeName string, aud Audience) []EnumEntry {
	allEntries := collectEnumEntries(pkg, typeName)

	// the visibility configuration
	visibility := getEnumVisibility(pkg, typeName)

	// filter by the visibility configuration
	return filterEnumEntries(allEntries, visibility, aud)
}

// getEnumVisibility returns the visibility configuration of an enum type.
func getEnumVisibility(pkg *packages.Package, typeName string) *EnumVisibility {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == typeName {
						return parseTypeVisibility(genDecl.Doc)
					}
				}
			}
		}
	}
	return &Visibility{Scope: "public"}
}

// filterEnumEntries filters the entries of an enum by the visibility configuration.
func filterEnumEntries(entries []EnumEntry, visibility *Visibility, aud Audience) []EnumEntry {
	if visibility.Scope == "hidden" {
		return []EnumEntry{}
	}

	// for the public and internal scopes it depends on the kind of document
	// a public enum is shown in every document
	if visibility.Scope == "public" {
		// a public enum is shown in every document, nothing to filter
	} else if visibility.Scope == "internal" && aud.isPublic() {
		return []EnumEntry{} // hide internal enums in the public document
	}

	// apply the allowlist or the blocklist, if there is one
	if len(visibility.Whitelist) > 0 || len(visibility.Blacklist) > 0 {
		return filterCustomEnumEntries(entries, visibility, aud)
	}

	// default: all entries
	return entries
}

// filterCustomEnumEntries filters the entries of an enum with the custom visibility scope.
func filterCustomEnumEntries(entries []EnumEntry, visibility *Visibility, aud Audience) []EnumEntry {
	var result []EnumEntry

	for _, entry := range entries {
		// check the blocklist
		if isInBlacklist(entry.Name, visibility.Blacklist) {
			continue
		}

		// check the allowlist
		if len(visibility.Whitelist) > 0 {
			if !isInWhitelist(entry.Name, visibility.Whitelist) {
				continue
			}
		}

		result = append(result, entry)
	}

	return result
}

// isInBlacklist reports whether the value is in the blocklist.
func isInBlacklist(name string, blacklist []string) bool {
	for _, pattern := range blacklist {
		if strings.HasSuffix(pattern, "*") {
			// wildcard match
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(name, prefix) {
				return true
			}
		} else if name == pattern {
			// exact match
			return true
		}
	}
	return false
}

// isInWhitelist reports whether the value is in the allowlist.
func isInWhitelist(name string, whitelist []string) bool {
	for _, pattern := range whitelist {
		if strings.HasSuffix(pattern, "*") {
			// wildcard match
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(name, prefix) {
				return true
			}
		} else if name == pattern {
			// exact match
			return true
		}
	}
	return false
}

// generateHiddenSchema makes the schema of a hidden enum: the type is shown, the values are not.
func generateHiddenSchema(underlyingType string) *openapi3.SchemaRef {
	ori := getBasicTypeSchema(underlyingType)
	if ori == nil {
		ori = getBasicTypeSchema("object")
	}

	schema := *ori
	schema.Extensions = map[string]any{}

	return schema.NewRef()
}
