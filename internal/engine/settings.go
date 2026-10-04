package engine

import (
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Danceiny/docgen/internal/errcat"
)

// Settings are facts about the documented repository that the engine cannot
// read from its source.
type Settings struct {
	// TypeMap fixes the schema of the types it lists, keyed by full key (the
	// dotted import path followed by the type name), instead of describing them
	// from their declarations.
	TypeMap map[string]*openapi3.Schema
	// HeaderTypes maps a @headerType name to the full key of the header type that
	// becomes a "Headers" parameter. DefaultHeader names the one used when a
	// method has no @headerType or an unknown one; when it is empty no header
	// parameter is added.
	HeaderTypes   map[string]string
	DefaultHeader string
	// Multipart maps the full key of a request type to the fields of its
	// multipart/form-data body. Such a type is documented as an upload instead of
	// a JSON body; the router's decoders enforce the bounds and the types.
	Multipart map[string][]MultipartField
	// QueryString maps the full key of a request type that the router binds from
	// the URL query string to its query parameters. Such an operation has no
	// request body and no header parameter.
	QueryString map[string][]QueryField
	// RuntimeOnly lists request bindings that no JSON value can satisfy, such as
	// an interface or a function handed to a setter that is reachable as a route.
	// Each is documented as an unsatisfiable input. The API path is part of the
	// key: a new method with the same type fails closed until it is listed.
	RuntimeOnly map[RuntimeInputKey]struct{}
	// BinaryResponses maps the full key of a response type that writes raw bytes
	// instead of the JSON envelope to the response it stands for.
	BinaryResponses map[string]BinaryResponse
	// Errors is the catalog of named errors that @response annotations refer to.
	// With none, no error components are added and the status codes of
	// annotations are not checked.
	Errors *errcat.Catalog
	// ErrorPrefix is the prefix of the full key of an error, ending in a dot.
	// Empty means the module path in dotted form followed by ".errors.".
	ErrorPrefix string
	// APIPrefix starts the route of every operation. Empty means "/api".
	APIPrefix string
	// SkipMethods are names of methods that are never operations. The Name
	// method of a service never is one, and need not be listed.
	SkipMethods []string
	// Envelope names the properties of the JSON envelope that wraps the data of
	// a response. Nil means there is none: the data type is the whole body.
	Envelope *Envelope
	// DefaultStatuses maps a status code to the description of a response that
	// every operation has besides its own, such as 401 for an API that needs a
	// credential. Nil means there are none.
	DefaultStatuses map[string]string
	// VendorExtensions adds the x- extensions docgen knows to the document:
	// x-apifox-orders, x-apifox-enum, x-apifox-folder, x-enum-varnames,
	// x-enum-comments, x-display-name, x-primary-property and x-go-interface.
	VendorExtensions bool
	// CompatLegacyOperationTypes reads the types of the parameters and results of
	// an operation as documents always read them: a list is its element type, and
	// a map, an interface and an instantiated generic type are unknown.
	CompatLegacyOperationTypes bool
	// CompatLegacyFieldShapes reads the fields of a struct as documents always
	// read them: the first name of a declaration with several, an embedded field
	// that has a json name flattened, "required" only as the whole of a validate
	// or binding tag, and any as an object and interface{} as one of a string, an
	// integer and an object.
	CompatLegacyFieldShapes bool
	// KeepEmptyTags keeps the empty tags an operation gets from a missing @tags
	// or @permission; without it they are dropped.
	KeepEmptyTags bool
}

// Envelope names the properties of a response envelope.
type Envelope struct {
	// Code holds the business status code, Message the message and Data the
	// payload.
	Code, Message, Data string
}

// MultipartField is one field of a multipart/form-data request.
type MultipartField struct {
	Name string
	// Kind is "file" (a binary part) or "scalar".
	Kind string
	// ScalarTo is the OpenAPI type of a scalar: "string" or "integer".
	ScalarTo string
	Required bool
}

// QueryField is one URL query parameter of a request bound from the query string.
type QueryField struct {
	Name string
	// Type is string, integer, number or boolean; empty means string.
	Type        string
	Required    bool
	Description string
}

// BinaryResponse describes a response that is a raw file.
type BinaryResponse struct {
	// Description is the description of the 200 response.
	Description string
	// ContentTypes are the media types the 200 response can have.
	ContentTypes []string
	// Errors maps further status codes to their descriptions.
	Errors map[string]string
}

// settings holds the settings of the current run. docgen is a command-line tool
// that generates one configuration at a time, so they are process-wide.
var settings Settings

// Configure sets the repository-specific settings for the run.
func Configure(s Settings) { settings = s }

// headerTypeKey returns the full key of the header type a method with the given
// @headerType gets: the configured one, else the default one, else "" for none.
func headerTypeKey(name string) string {
	if key, ok := settings.HeaderTypes[name]; ok {
		return key
	}
	return settings.HeaderTypes[settings.DefaultHeader]
}

// defaultAPIPrefix is the route prefix of a module that does not set one.
const defaultAPIPrefix = "/api"

// apiPrefix is the route prefix of every operation.
func apiPrefix() string {
	if settings.APIPrefix != "" {
		return settings.APIPrefix
	}
	return defaultAPIPrefix
}

// errorPrefix is the prefix of the full key of a catalog error: the configured
// one, else the module's key prefix followed by "errors.".
func errorPrefix() string {
	if settings.ErrorPrefix != "" {
		return settings.ErrorPrefix
	}
	return ownKeyPrefix() + "errors."
}

// errorKey is the full key of the catalog error an annotation names.
func errorKey(name string) string { return errorPrefix() + name }

// ownKeyPrefix is the prefix of the component keys of the documented module's
// own packages: its module path with the slashes turned into dots, and a dot.
func ownKeyPrefix() string { return replaceSlashes(ModuleName) + "." }

// isOwnImportPath reports whether an import path belongs to the documented module.
func isOwnImportPath(path string) bool {
	return path == ModuleName || strings.HasPrefix(path, ModuleName+"/")
}
