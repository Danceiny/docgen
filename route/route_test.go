package route

import "testing"

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		service, method, tag, pref string
		want                       string
	}{
		{"method name, first letter lower-cased", "pet", "Plain", "", "/api", "/api/pet/plain"},
		{"only the first letter changes", "pet", "APIKeys", "", "/api", "/api/pet/aPIKeys"},
		{"relative path tag", "pet", "RelativePath", "/customName", "/api", "/api/pet/customName"},
		{"path tag without leading slash", "pet", "X", "custom", "/api", "/api/pet/custom"},
		{"path tag that already starts with the prefix is returned as is", "pet", "AbsolutePath", "/api/pet/absolutePath", "/api", "/api/pet/absolutePath"},
		{"that includes a different service", "pet", "M", "/api/other/x", "/api", "/api/other/x"},
		{"path tag repeating the service name", "pet", "PrefixedByService", "/pet/prefixed", "/api", "/api/pet/prefixed"},
		{"service name with slashes", "shop/order", "FindOrder", "", "/api", "/api/shop/order/findOrder"},
		{"path tag repeating every part of a slashed service", "user/staff", "X", "/user/staff/stats", "/api", "/api/user/staff/stats"},
		{"path tag repeating only the last part", "user/staff", "X", "/staff/stats", "/api", "/api/user/staff/stats"},
		{"no segment limit", "pet", "TooDeep", "/a/b/c/d", "/api", "/api/pet/a/b/c/d"},
		{"quotes are kept", "pet", "QuotedPath", `"/quoted"`, "/api", `/api/pet/"/quoted"`},
		{"empty method and tag", "svc", "", "", "/api", "/api/svc/"},
		{"other prefix", "svc", "Do", "", "/internal", "/internal/svc/do"},
		{"empty prefix returns the path tag untouched", "svc", "Do", "/x", "", "/x"},
		{"service name is only a prefix of the tag", "user", "X", "/username", "/api", "/api/user/name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.service, tc.method, tc.tag, tc.pref); got != tc.want {
				t.Fatalf("Resolve(%q, %q, %q, %q) = %q, want %q", tc.service, tc.method, tc.tag, tc.pref, got, tc.want)
			}
		})
	}
}

func TestSkipped(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  bool
	}{
		{"no lines", nil, false},
		{"no directive", []string{"// Plain method.", "// @tags: a"}, false},
		{"hidden, with markers", []string{"// Internal helper", "// @apidoc: -"}, true},
		{"hidden, markers already stripped", []string{"Internal helper", "@apidoc: -"}, true},
		{"hidden, no space after the marker", []string{"//@apidoc: -"}, true},
		{"hidden, block comment", []string{"/* @apidoc: - */"}, true},
		{"hidden, quoted dash", []string{`// @apidoc: "-"`}, true},
		{"hidden, padded dash", []string{"// @apidoc:   -  "}, true},
		{"other value is not hidden", []string{"// @apidoc: public"}, false},
		{"internal is not hidden", []string{"// @apidoc: internal"}, false},
		{"the last line wins: shown after hidden", []string{"// @apidoc: -", "// @apidoc: public"}, false},
		{"the last line wins: hidden after shown", []string{"// @apidoc: public", "// @apidoc: -"}, true},
		{"a parameter line is never a directive", []string{"// @param name string @apidoc: -"}, false},
		{"a directive line that mentions @param is a parameter line", []string{"// @apidoc: - see @param x"}, false},
		{"such a line does not replace an earlier directive", []string{"// @apidoc: public", "// @apidoc: - see @param x"}, false},
		{"nor does it cancel one", []string{"// @apidoc: -", "// @apidoc: public @param x"}, true},
		{"a parameter line does not override an earlier directive", []string{"// @apidoc: -", "// @param x string"}, true},
		{"the directive must start the line", []string{"// note: @apidoc: -"}, false},
		{"a mid-line mention does not cancel an earlier directive", []string{"// @apidoc: -", "// note: @apidoc: public"}, true},
		{"markers inside the text are not stripped", []string{"/// @apidoc: -"}, false},
		{"dash inside a longer value", []string{"// @apidoc: --"}, false},
		{"empty value", []string{"// @apidoc:"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Skipped(tc.lines); got != tc.want {
				t.Fatalf("Skipped(%q) = %v, want %v", tc.lines, got, tc.want)
			}
		})
	}
}
