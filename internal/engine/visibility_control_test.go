package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/packages"
)

// slicesEqual reports whether two string slices are equal.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseTypeVisibility(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected *EnumVisibility
	}{
		{
			name:    "public scope",
			comment: "//apidoc:public",
			expected: &EnumVisibility{
				Scope:     "public",
				Whitelist: []string{},
				Blacklist: []string{},
				Language:  "",
			},
		},
		{
			name:    "internal scope",
			comment: "//apidoc:internal",
			expected: &EnumVisibility{
				Scope:     "internal",
				Whitelist: []string{},
				Blacklist: []string{},
				Language:  "",
			},
		},
		{
			name:    "hidden scope",
			comment: "//apidoc:hidden",
			expected: &EnumVisibility{
				Scope:     "hidden",
				Whitelist: []string{},
				Blacklist: []string{},
				Language:  "",
			},
		},
		{
			name:    "public with chinese language",
			comment: "//apidoc:public,zh",
			expected: &EnumVisibility{
				Scope:     "public",
				Whitelist: []string{},
				Blacklist: []string{},
				Language:  "zh",
			},
		},
		{
			name:    "custom scope with whitelist",
			comment: "//apidoc:public:Region_North,Region_South",
			expected: &EnumVisibility{
				Scope:     "public",
				Whitelist: []string{"Region_North", "Region_South"},
				Blacklist: []string{},
				Language:  "",
			},
		},
		{
			name:    "custom scope with blacklist",
			comment: "//apidoc:internal:-Region_Reserved*,-Region_Sandbox",
			expected: &EnumVisibility{
				Scope:     "internal",
				Whitelist: []string{},
				Blacklist: []string{"Region_Reserved*", "Region_Sandbox"},
				Language:  "",
			},
		},
		{
			name:    "mixed whitelist and blacklist",
			comment: "//apidoc:public:Region_North,-Region_Reserved*",
			expected: &EnumVisibility{
				Scope:     "public",
				Whitelist: []string{"Region_North"},
				Blacklist: []string{"Region_Reserved*"},
				Language:  "",
			},
		},
		{
			name:    "public with language and whitelist",
			comment: "//apidoc:public,zh:Region_North,Region_South",
			expected: &EnumVisibility{
				Scope:     "public",
				Whitelist: []string{"Region_North", "Region_South"},
				Blacklist: []string{},
				Language:  "zh",
			},
		},
		{
			name:    "no comment",
			comment: "",
			expected: &EnumVisibility{
				Scope:     "public",
				Whitelist: []string{},
				Blacklist: []string{},
				Language:  "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc *ast.CommentGroup
			if tt.comment != "" {
				doc = &ast.CommentGroup{
					List: []*ast.Comment{
						{Text: tt.comment},
					},
				}
			}

			result := parseTypeVisibility(doc)
			if result.Scope != tt.expected.Scope {
				t.Errorf("Scope mismatch: got %s, want %s", result.Scope, tt.expected.Scope)
			}
			if len(result.Whitelist) != len(tt.expected.Whitelist) {
				t.Errorf("Whitelist length mismatch: got %d, want %d", len(result.Whitelist), len(tt.expected.Whitelist))
			}
			if len(result.Blacklist) != len(tt.expected.Blacklist) {
				t.Errorf("Blacklist length mismatch: got %d, want %d", len(result.Blacklist), len(tt.expected.Blacklist))
			}
			if result.Language != tt.expected.Language {
				t.Errorf("Language mismatch: got %s, want %s", result.Language, tt.expected.Language)
			}
		})
	}
}

func TestShouldHideType(t *testing.T) {
	// create a simple test package
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", `
package test

//apidoc:public:Region_North,Region_South
type Region int

const (
	Region_North Region = iota
	Region_South
	Region_Internal
)
`, parser.ParseComments)
	if err != nil {
		t.Fatalf("Failed to parse test file: %v", err)
	}

	pkg := &packages.Package{
		Syntax: []*ast.File{file},
	}

	// look for the type declaration
	var typeSpec *ast.TypeSpec
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == "Region" {
					typeSpec = ts
					break
				}
			}
		}
	}

	if typeSpec == nil {
		t.Fatal("Failed to find Region type")
	}

	tests := []struct {
		name     string
		aud      Audience
		expected bool
	}{
		{
			name:     "public doc should not hide",
			aud:      testPublic,
			expected: false,
		},
		{
			name:     "internal doc should not hide",
			aud:      testInternal,
			expected: false, // internal is a superset of public: a public type is shown in the internal document too
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldHideType(pkg, typeSpec, tt.aud)
			if result != tt.expected {
				t.Errorf("shouldHideEnum() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterTypeEntries(t *testing.T) {
	entries := []EnumEntry{
		{Name: "Region_North", Value: "0", Comment: "North region"},
		{Name: "Region_South", Value: "1", Comment: "South region"},
		{Name: "Region_Internal", Value: "2", Comment: "Internal region"},
		{Name: "Region_Reserved1", Value: "3", Comment: "Reserved region"},
	}

	tests := []struct {
		name       string
		visibility *EnumVisibility
		aud        Audience
		expected   int
	}{
		{
			name: "public scope with whitelist",
			visibility: &EnumVisibility{
				Scope:     "public",
				Whitelist: []string{"Region_North", "Region_South"},
				Blacklist: []string{},
			},
			aud:      testPublic,
			expected: 2,
		},
		{
			name: "internal scope with blacklist",
			visibility: &EnumVisibility{
				Scope:     "internal",
				Whitelist: []string{},
				Blacklist: []string{"Region_Reserved*"},
			},
			aud:      testInternal,
			expected: 3,
		},
		{
			name: "hidden scope",
			visibility: &EnumVisibility{
				Scope:     "hidden",
				Whitelist: []string{},
				Blacklist: []string{},
			},
			aud:      testPublic,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterEnumEntries(entries, tt.visibility, tt.aud)
			if len(result) != tt.expected {
				t.Errorf("filterEnumEntries() returned %d entries, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestVisibilityPatternMatching(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		value    string
		expected bool
	}{
		{
			name:     "exact match",
			pattern:  "Region_North",
			value:    "Region_North",
			expected: true,
		},
		{
			name:     "wildcard match",
			pattern:  "Region_Reserved*",
			value:    "Region_Reserved1",
			expected: true,
		},
		{
			name:     "wildcard no match",
			pattern:  "Region_Reserved*",
			value:    "Region_North",
			expected: false,
		},
		{
			name:     "no wildcard no match",
			pattern:  "Region_North",
			value:    "Region_South",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			whitelist := []string{tt.pattern}
			result := isInWhitelist(tt.value, whitelist)
			if result != tt.expected {
				t.Errorf("isInWhitelist(%s, %s) = %v, want %v", tt.value, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestShouldHideField(t *testing.T) {
	tests := []struct {
		name     string
		field    *ast.Field
		aud      Audience
		expected bool
	}{
		{
			name: "no apidoc tag - should show",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\"`"},
			},
			aud:      testPublic,
			expected: false,
		},
		{
			name: "apidoc:- - should hide",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"-\"`"},
			},
			aud:      testPublic,
			expected: true,
		},
		// the new visibility control format
		{
			name: "apidoc:public in public doc - should show",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"public\"`"},
			},
			aud:      testPublic,
			expected: false,
		},
		{
			name: "apidoc:public in internal doc - should hide",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"public\"`"},
			},
			aud:      testInternal,
			expected: true,
		},
		{
			name: "apidoc:internal in internal doc - should show",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"internal\"`"},
			},
			aud:      testInternal,
			expected: false,
		},
		{
			name: "apidoc:internal in public doc - should hide",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"internal\"`"},
			},
			aud:      testPublic,
			expected: true,
		},
		{
			name: "apidoc:hidden - should hide",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"hidden\"`"},
			},
			aud:      testPublic,
			expected: true,
		},
		// A custom value is shown to the audiences that list it and hidden from the rest.
		{
			name: `apidoc:"Staff" in the public document - hidden, it does not list Staff`,
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"Staff\"`"},
			},
			aud:      testPublic,
			expected: true,
		},
		{
			name: `apidoc:"Staff" in the internal document - shown, it lists Staff`,
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"Staff\"`"},
			},
			aud:      testInternal,
			expected: false,
		},
		{
			name: `apidoc:"Partner" in the public document - shown, it lists Partner`,
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"Partner\"`"},
			},
			aud:      testPublic,
			expected: false,
		},
		{
			name: `apidoc:"Partner" in the internal document - hidden, it does not list Partner`,
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"Partner\"`"},
			},
			aud:      testInternal,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldHideField(tt.field, tt.aud)
			if result != tt.expected {
				t.Errorf("shouldHideField() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetFieldApidocTag(t *testing.T) {
	tests := []struct {
		name     string
		field    *ast.Field
		expected string
	}{
		{
			name: "no tag",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
			},
			expected: "",
		},
		{
			name: "no apidoc tag",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\"`"},
			},
			expected: "",
		},
		{
			name: "apidoc tag with value",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"public\"`"},
			},
			expected: "public",
		},
		{
			name: "apidoc tag with dash",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"-\"`"},
			},
			expected: "-",
		},
		{
			name: "apidoc tag with complex value",
			field: &ast.Field{
				Names: []*ast.Ident{{Name: "TestField"}},
				Tag:   &ast.BasicLit{Value: "`json:\"test\" apidoc:\"public:value1,value2\"`"},
			},
			expected: "public:value1,value2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFieldApidocTag(tt.field)
			if result != tt.expected {
				t.Errorf("getFieldApidocTag() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseFieldVisibility(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *Visibility
	}{
		{
			name:  "simple public",
			input: "public",
			expected: &Visibility{
				Scope:     "public",
				Whitelist: []string{},
				Blacklist: []string{},
			},
		},
		{
			name:  "simple internal",
			input: "internal",
			expected: &Visibility{
				Scope:     "internal",
				Whitelist: []string{},
				Blacklist: []string{},
			},
		},
		{
			name:  "simple hidden",
			input: "hidden",
			expected: &Visibility{
				Scope:     "hidden",
				Whitelist: []string{},
				Blacklist: []string{},
			},
		},
		{
			name:  "public with whitelist",
			input: "public:value1,value2",
			expected: &Visibility{
				Scope:     "public",
				Whitelist: []string{"value1", "value2"},
				Blacklist: []string{},
			},
		},
		{
			name:  "internal with blacklist",
			input: "internal:-value1,-value2",
			expected: &Visibility{
				Scope:     "internal",
				Whitelist: []string{},
				Blacklist: []string{"value1", "value2"},
			},
		},
		{
			name:  "mixed whitelist and blacklist",
			input: "custom:value1,-value2,value3",
			expected: &Visibility{
				Scope:     "custom",
				Whitelist: []string{"value1", "value3"},
				Blacklist: []string{"value2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFieldVisibility(tt.input)
			if result.Scope != tt.expected.Scope {
				t.Errorf("parseFieldVisibility(%q).Scope = %q, want %q", tt.input, result.Scope, tt.expected.Scope)
			}
			if !slicesEqual(result.Whitelist, tt.expected.Whitelist) {
				t.Errorf("parseFieldVisibility(%q).Whitelist = %v, want %v", tt.input, result.Whitelist, tt.expected.Whitelist)
			}
			if !slicesEqual(result.Blacklist, tt.expected.Blacklist) {
				t.Errorf("parseFieldVisibility(%q).Blacklist = %v, want %v", tt.input, result.Blacklist, tt.expected.Blacklist)
			}
		})
	}
}

func TestShouldHideByVisibility(t *testing.T) {
	tests := []struct {
		name       string
		visibility *Visibility
		aud        Audience
		expected   bool
	}{
		{
			name: "hidden scope - always hide",
			visibility: &Visibility{
				Scope: "hidden",
			},
			aud:      testPublic,
			expected: true,
		},
		{
			name: "public scope in public doc - show",
			visibility: &Visibility{
				Scope: "public",
			},
			aud:      testPublic,
			expected: false,
		},
		{
			name: "public scope in internal doc - hide",
			visibility: &Visibility{
				Scope: "public",
			},
			aud:      testInternal,
			expected: true,
		},
		{
			name: "internal scope in internal doc - show",
			visibility: &Visibility{
				Scope: "internal",
			},
			aud:      testInternal,
			expected: false,
		},
		{
			name: "internal scope in public doc - hide",
			visibility: &Visibility{
				Scope: "internal",
			},
			aud:      testPublic,
			expected: true,
		},
		{
			name: "custom scope - show (for now)",
			visibility: &Visibility{
				Scope: "custom",
			},
			aud:      testPublic,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldHideByVisibility(tt.visibility, tt.aud)
			if result != tt.expected {
				t.Errorf("shouldHideByVisibility() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsNewVisibilityFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "public scope",
			input:    "public",
			expected: true,
		},
		{
			name:     "internal scope",
			input:    "internal",
			expected: true,
		},
		{
			name:     "hidden scope",
			input:    "hidden",
			expected: true,
		},
		{
			name:     "custom scope",
			input:    "custom",
			expected: true,
		},
		{
			name:     "public with values",
			input:    "public:value1,value2",
			expected: true,
		},
		{
			name:     "internal with values",
			input:    "internal:-value1,-value2",
			expected: true,
		},
		{
			name:     "custom value (old format)",
			input:    "Staff",
			expected: false,
		},
		{
			name:     "Public (old format)",
			input:    "Public",
			expected: false,
		},
		{
			name:     "Internal (old format)",
			input:    "Internal",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNewVisibilityFormat(tt.input)
			if result != tt.expected {
				t.Errorf("isNewVisibilityFormat(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
