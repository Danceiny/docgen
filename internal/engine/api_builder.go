package engine

import (
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/Danceiny/docgen/route"

	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	caser = cases.Title(language.English)
)

// BuildPathItem adds the operation of a method to the document, under its route,
// for each HTTP method its @method names.
func BuildPathItem(doc *openapi3.T, service ServiceInterface, method *Method) {
	method.ServiceName = service.ServiceName
	method.APIPath = route.Resolve(service.ServiceName, method.Name, ExtractTag(method.Doc, "path"), apiPrefix())

	// the HTTP methods an operation can have
	var verbs []string
	for _, httpMethod := range parseHTTPMethod(method.Doc) {
		verb := strings.ToUpper(strings.TrimSpace(httpMethod))
		switch verb {
		case http.MethodPost, http.MethodDelete, http.MethodGet, http.MethodPut,
			http.MethodPatch, http.MethodHead, http.MethodOptions, http.MethodTrace:
			verbs = append(verbs, verb)
		default:
			warnAt("@method names an HTTP method that an operation cannot have, ignoring it",
				"method", method.Name, "httpMethod", httpMethod, "at", method.Pos)
		}
	}
	if len(verbs) == 0 {
		return // nothing to put under the route
	}

	// get or create the PathItem
	pathItem := doc.Paths.Value(method.APIPath)
	if pathItem == nil {
		pathItem = &openapi3.PathItem{}
	}

	operation := buildOperation(method, doc)
	for _, verb := range verbs {
		op := operation
		if len(verbs) > 1 {
			// an operation id names one operation
			cp := *operation
			cp.OperationID = operation.OperationID + "_" + strings.ToLower(verb)
			op = &cp
		}
		if replaced := pathItem.GetOperation(verb); replaced != nil {
			warnAt("two methods answer the same route with the same HTTP method; the later one is documented",
				"route", method.APIPath, "httpMethod", verb, "method", method.Name, "replaces", replaced.Summary, "at", method.Pos)
		}
		pathItem.SetOperation(verb, op)
	}
	disambiguateOperationIDs(pathItem)
	doc.Paths.Set(method.APIPath, pathItem)
}

// disambiguateOperationIDs gives the operations of one route that share an id
// the id with their HTTP method, as two methods of a service that answer one
// route with different verbs do. A route with one operation keeps its id.
func disambiguateOperationIDs(item *openapi3.PathItem) {
	seen := map[string]int{}
	for _, op := range item.Operations() {
		seen[op.OperationID]++
	}
	for verb, op := range item.Operations() {
		if seen[op.OperationID] > 1 {
			op.OperationID += "_" + strings.ToLower(verb)
		}
	}
}

func buildOperation(method *Method, doc *openapi3.T) *openapi3.Operation {
	tags := []string{titleWords(method.ServiceName)}
	for _, t := range ExtractTags(method.Doc, "tags") {
		tags = append(tags, caser.String(t))
	}
	for _, t := range ExtractTags(method.Doc, "permission") {
		tags = append(tags, caser.String(t))
	}
	if !settings.KeepEmptyTags {
		tags = filter(tags, func(t string) bool { return t != "" })
	}
	tags = dedupe(tags)
	sort.Strings(tags)
	firstLineDoc := strings.Split(method.Doc, "\n")
	if firstLineDoc[0] == "" {
		firstLineDoc = []string{method.Name}
	}
	op := &openapi3.Operation{
		OperationID:  strings.TrimPrefix(method.APIPath, apiPrefix()+"/"),
		Summary:      strings.TrimSpace(firstLineDoc[0]),
		Description:  ExtractTag(method.Doc, "desc"),
		Tags:         tags,
		Parameters:   buildParameters(method, doc),
		RequestBody:  buildRequestBody(method, doc),
		Responses:    buildResponses(method, doc),
		ExternalDocs: buildExternalDocs(method, doc),
	}

	// add the x-apifox-folder extension field
	if method.Folder != "" && settings.VendorExtensions {
		if op.Extensions == nil {
			op.Extensions = make(map[string]interface{})
		}
		op.Extensions["x-apifox-folder"] = method.Folder
	}

	return op
}

// buildExternalDocs reads "@doc: <description>: <url>" into the operation's
// external documentation. An annotation without a colon is reported and
// ignored.
func buildExternalDocs(method *Method, doc *openapi3.T) *openapi3.ExternalDocs {
	m := ExtractTag(method.Doc, "doc")
	if m == "" {
		return nil
	}
	description, url, found := strings.Cut(m, ":")
	if !found {
		warnAt("@doc needs a description and a URL separated by a colon, ignoring it",
			"method", method.Name, "annotation", m, "at", method.Pos)
		return nil
	}
	return &openapi3.ExternalDocs{
		Description: strings.TrimSpace(description),
		URL:         strings.TrimSpace(url),
	}
}

func parseHTTPMethod(doc string) []string {
	// parse @method: POST
	if m := ExtractTag(doc, "method"); m != "" {
		return strings.Split(m, ",")
	}
	return []string{"POST"}
}

// titleWords upper-cases the first letter of every word of s. It is what
// strings.Title does, kept as it was because the alternatives draw word
// boundaries differently and the service names in the documents must not change.
func titleWords(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if isWordSeparator(prev) {
			prev = r
			return unicode.ToTitle(r)
		}
		prev = r
		return r
	}, s)
}

// isWordSeparator reports whether r separates words: ASCII letters, digits and
// underscores do not, other letters and digits do not, and of the rest only
// spaces do (and the ASCII punctuation).
func isWordSeparator(r rune) bool {
	if r <= 0x7F {
		switch {
		case '0' <= r && r <= '9', 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', r == '_':
			return false
		}
		return true
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	return unicode.IsSpace(r)
}
