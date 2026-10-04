// Package route is the executable specification of docgen's route convention:
// how a service name, a method name and an optional @path annotation become an
// HTTP route, and which methods an "@apidoc: -" annotation hides from both the
// router and the generated documentation.
//
// A router that follows the convention and docgen must agree on these rules or
// the documentation lies about the API. They live here, as plain standard
// library code, so that either side can test itself against them without
// importing the other.
package route

import "strings"

// Resolve returns the route of a service method.
//
// service is the value of the service's Name() (it may contain slashes, as in
// "bi/agent"), method is the Go method name, pathTag is the value of the
// method's @path annotation (empty when absent) and prefix is the route prefix
// (docgen's default is "/api").
//
//   - A pathTag that already starts with prefix is returned unchanged.
//   - Otherwise the route is prefix + "/" + service + "/" + the last path
//     segment name, which is pathTag when present and the method name when not,
//     with its first letter lower-cased.
//   - A leading "/" on pathTag is dropped, and so is each slash-separated part
//     of the service name that pathTag repeats at its start, so both "/stats"
//     and "/user/stats" resolve to ".../user/stats" for service "user".
//
// Resolve does not limit the number of path segments and does not strip
// quotes: those are concerns of the router.
func Resolve(service, method, pathTag, prefix string) string {
	if strings.HasPrefix(pathTag, prefix) {
		return pathTag
	}
	name := pathTag
	if name == "" {
		name = method
	}
	name = strings.TrimPrefix(name, "/")
	for _, part := range strings.Split(service, "/") {
		if strings.HasPrefix(name, part) { // user, user/staff
			name = strings.TrimPrefix(name, part)
			name = strings.TrimPrefix(name, "/")
		}
	}
	return prefix + "/" + service + "/" + lowerFirst(name)
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[0:1]) + s[1:]
}

// Skipped reports whether a method's doc comment hides the method with
// "@apidoc: -". A skipped method is not registered as a route and does not
// appear in the documentation.
//
// commentLines are the lines of the doc comment, with or without their comment
// markers ("// @apidoc: -" and "@apidoc: -" are equivalent). The rules are the
// ones the router has always applied, quirks included:
//
//   - a line that contains "@param" anywhere is a parameter line and is never
//     read as a directive, whatever else it says;
//   - when several @apidoc lines appear, the last one wins;
//   - spaces and double quotes around the value are ignored, so "-" and `"-"`
//     both hide the method.
func Skipped(commentLines []string) bool {
	apidoc := ""
	for _, line := range commentLines {
		text := strings.TrimSpace(line)
		text = strings.TrimPrefix(text, "//")
		text = strings.TrimPrefix(text, "/*")
		text = strings.TrimSuffix(text, "*/")
		text = strings.TrimSpace(text)
		if strings.Contains(text, "@param") {
			continue
		}
		if strings.HasPrefix(text, "@apidoc:") {
			apidoc = strings.Trim(strings.TrimSpace(strings.TrimPrefix(text, "@apidoc:")), `"`)
		}
	}
	return strings.TrimSpace(apidoc) == "-"
}
