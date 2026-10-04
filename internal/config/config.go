// Package config reads and validates docgen's configuration file.
//
// The file says which documents to generate, for which audience, from which
// packages and with what header information. The decoder is strict: an unknown
// key is an error, so a typo cannot silently fall back to a default.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Audiences a document can be written for.
const (
	AudienceInternal = "internal"
	AudiencePublic   = "public"
)

// DefaultInfoVersion is the info.version of a document that does not set one.
const DefaultInfoVersion = "0.0.1"

// Config is the content of a docgen configuration file.
type Config struct {
	// Version is the schema version of the file; only 1 exists.
	Version int `yaml:"version"`
	// GenericTitles are the full keys of generic types whose oneOf alternatives
	// must not be narrowed by the type arguments in their title.
	GenericTitles []string `yaml:"generic_titles"`
	// TypeMap fixes the schema of types, keyed by full key (the dotted import path
	// followed by the type name), instead of describing them from their source.
	TypeMap map[string]TypeSpec `yaml:"type_map"`
	// Headers says which header type each method gets as a "Headers" parameter.
	Headers Headers `yaml:"headers"`
	// Request lists request types that are not plain JSON bodies.
	Request Request `yaml:"request"`
	// Response lists response types that are not the JSON envelope.
	Response Response `yaml:"response"`
	// Errors says where the catalog of named errors is and how it is keyed.
	Errors Errors `yaml:"errors"`
	// API says how the routes of the operations are formed.
	API API `yaml:"api"`
	// VendorExtensions adds the x- extensions docgen knows to the documents:
	// x-apifox-orders, x-apifox-enum, x-apifox-folder, x-enum-varnames,
	// x-enum-comments, x-display-name, x-primary-property and x-go-interface.
	VendorExtensions bool `yaml:"vendor_extensions"`
	// KeepEmptyTags gives an operation an empty tag for each of @tags and
	// @permission it does not have, as documents generated before this option
	// existed do. Without it empty tags are left out.
	KeepEmptyTags bool `yaml:"keep_empty_tags"`
	// Docs are the documents to generate.
	Docs []Doc `yaml:"docs"`
}

// Doc describes one generated document.
type Doc struct {
	// Name selects the document on the command line; it must be unique.
	Name string `yaml:"name"`
	// Audience is "internal" or "public" and decides what the visibility rules
	// show and which post-processing the document gets.
	Audience string `yaml:"audience"`
	// Output is the file to write, relative to the module directory.
	Output string `yaml:"output"`
	// Info is the OpenAPI info object; version defaults to DefaultInfoVersion.
	Info Info `yaml:"info"`
	// Servers are the OpenAPI servers.
	Servers []Server `yaml:"servers"`
	// Models are the package patterns, relative to the module path, whose types
	// become schemas. In a pattern "*" matches any run of characters, slashes
	// included, and every other character matches itself.
	Models []string `yaml:"models"`
	// Services are the package patterns whose services become operations.
	Services []string `yaml:"services"`
	// LegacyFieldTokens are the values of apidoc:"<value>" struct tags that make a
	// field visible in this document; any other such value hides the field.
	LegacyFieldTokens []string `yaml:"legacy_field_tokens"`
	// ForceKeep lists schema component keys that stay in a public document even
	// when no operation references them, for example webhook payloads.
	ForceKeep []string `yaml:"force_keep"`
	// Overlay lists files of schemas that are put into the document by hand, and
	// the stage each is applied at, in the order they are applied.
	Overlay []OverlayFile `yaml:"overlay"`
	// HideTypePrefixes lists name prefixes: a type whose name starts with one of
	// them is left out of the document, unless its own //apidoc: directive says
	// otherwise.
	HideTypePrefixes []string `yaml:"hide_type_prefixes"`
	// Public refines a document written for the public audience.
	Public *Public `yaml:"public"`
}

// DefaultPublicTag is the tag that makes an operation part of a public document.
const DefaultPublicTag = "public"

// Public refines a document of the public audience. Such a document keeps only
// the operations tagged with the public tag, drops the schemas no operation
// refers to, and shows the last segment of a schema's name as its title.
type Public struct {
	// Tag is the tag that makes an operation public; DefaultPublicTag when empty.
	// Comparison ignores case.
	Tag string `yaml:"tag"`
	// StripTagsContaining lists strings; a tag that contains one of them, in any
	// case, is removed from the operations of the document, except the public tag.
	StripTagsContaining []string `yaml:"strip_tags_containing"`
	// ErrorsLast lists the alternatives of a oneOf, anyOf or allOf whose title
	// contains "err", in any case, after the other alternatives.
	ErrorsLast bool `yaml:"errors_last"`
}

// The stages at which an overlay is applied to a document.
const (
	// StageAfterModels is after the models are generated and before the operations
	// are. A component that exists by then is one that the response of an
	// operation can refer to; the generator emits the reference to a response's
	// data type only when the component is there.
	StageAfterModels = "after_models"
	// StageAfterAPIs is after the operations are generated and before the
	// document is optimized and written.
	StageAfterAPIs = "after_apis"
)

// OverlayFile names an overlay file, which is relative to the module directory,
// and the stage it is applied at.
type OverlayFile struct {
	File  string `yaml:"file"`
	Stage string `yaml:"stage"`
}

// Request lists the request types that are not plain JSON bodies.
type Request struct {
	// Multipart maps a request type's full key to the fields of its
	// multipart/form-data body.
	Multipart map[string][]MultipartField `yaml:"multipart"`
	// Query maps a request type's full key to its URL query parameters; such an
	// operation has no body and no header parameter.
	Query map[string][]QueryField `yaml:"query"`
	// RuntimeOnly lists request bindings no JSON value can satisfy, by API path
	// and request type, such as an interface handed to a setter that is a route.
	RuntimeOnly []RuntimeOnly `yaml:"runtime_only"`
}

// MultipartField is one field of a multipart/form-data request.
type MultipartField struct {
	Name string `yaml:"name"`
	// Kind is "file" or "scalar".
	Kind string `yaml:"kind"`
	// ScalarTo is the OpenAPI type of a scalar: "string" or "integer".
	ScalarTo string `yaml:"scalar_to"`
	Required bool   `yaml:"required"`
}

// QueryField is one URL query parameter.
type QueryField struct {
	Name string `yaml:"name"`
	// Type is the type of the value: string (the default), integer, number or
	// boolean.
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
}

// RuntimeOnly is a request binding no JSON value can satisfy.
type RuntimeOnly struct {
	Path string `yaml:"path"`
	Type string `yaml:"type"`
}

// API says how the route of an operation is formed.
type API struct {
	// Prefix starts every route: "/api" when empty, otherwise a path that starts
	// with a slash and does not end in one, such as "/v1".
	Prefix string `yaml:"prefix"`
	// SkipMethods are names of methods that are never operations, whatever
	// service they belong to. The Name method of a service never is one, and
	// need not be listed.
	SkipMethods []string `yaml:"skip_methods"`
}

// Response says what a response looks like beyond its data type.
type Response struct {
	// Envelope names the properties of the JSON envelope the data is wrapped in.
	// Without one, the data type is the whole body of a response.
	Envelope *Envelope `yaml:"envelope"`
	// DefaultStatuses maps a status code to the description of a response that
	// every operation has besides its own, such as 401 for an API that needs a
	// credential.
	DefaultStatuses map[string]string `yaml:"default_statuses"`
	// Binary maps a response type's full key to the raw file it writes.
	Binary map[string]BinaryResponse `yaml:"binary"`
}

// Envelope names the properties of a response envelope: {code, message, data}.
type Envelope struct {
	// Code is the property that holds the business status code, Message the one
	// that holds the message and Data the one that holds the payload.
	Code    string `yaml:"code"`
	Message string `yaml:"message"`
	Data    string `yaml:"data"`
}

// BinaryResponse describes a response that is a raw file.
type BinaryResponse struct {
	// Description is the description of the 200 response.
	Description string `yaml:"description"`
	// ContentTypes are the media types of the 200 response.
	ContentTypes []string `yaml:"content_types"`
	// Errors maps further status codes to their descriptions.
	Errors map[string]string `yaml:"errors"`
}

// Errors locates the catalog of named errors that @response annotations refer to.
type Errors struct {
	// File is the catalog, a JSON array of {name, code, message, httpCode}, relative
	// to the module directory. Empty means the module has none: no error
	// components are added and the status codes of annotations are not checked.
	File string `yaml:"file"`
	// ComponentPrefix is the prefix of the full key of an error, ending in a dot.
	// Empty means the module path in dotted form followed by ".errors.".
	ComponentPrefix string `yaml:"component_prefix"`
}

// TypeSpec is the schema of a type listed in type_map.
type TypeSpec struct {
	// Type is one of string, integer, number, boolean, object or array.
	Type        string `yaml:"type"`
	Format      string `yaml:"format"`
	Description string `yaml:"description"`
	// Example is an example value of the type, of any YAML type that fits Type.
	Example any `yaml:"example"`
	// Items is the element type of an array.
	Items *TypeSpec `yaml:"items"`
}

// Headers maps @headerType names to header types.
type Headers struct {
	// Default names the entry of Types used when a method has no @headerType or an
	// unknown one. Empty means no header parameter is added.
	Default string `yaml:"default"`
	// Types maps a @headerType name to the full key of the header type.
	Types map[string]string `yaml:"types"`
}

// Info is the OpenAPI info object of a document.
type Info struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

// Server is an OpenAPI server entry.
type Server struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

// Load reads and validates the configuration file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes and validates a configuration file's content.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the configuration is empty: it needs at least \"version: 1\" and a document under \"docs:\"")
		}
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	for i := range cfg.Docs {
		if cfg.Docs[i].Info.Version == "" {
			cfg.Docs[i].Info.Version = DefaultInfoVersion
		}
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("version: want 1, got %d", c.Version)
	}
	if len(c.Docs) == 0 {
		return errors.New("docs: at least one document is required")
	}
	var problems []error
	for key, spec := range c.TypeMap {
		if key == "" {
			problems = append(problems, errors.New("type_map: a key is empty"))
		}
		problems = append(problems, spec.validate("type_map."+key)...)
	}
	problems = append(problems, c.Request.validate()...)
	problems = append(problems, c.Response.validate()...)
	problems = append(problems, c.Errors.validate()...)
	problems = append(problems, c.API.validate()...)
	problems = append(problems, c.Headers.validate()...)
	seen := map[string]bool{}
	for i, d := range c.Docs {
		problems = append(problems, d.validate(i, seen)...)
	}
	return errors.Join(problems...)
}

func (h Headers) validate() []error {
	var problems []error
	for name, key := range h.Types {
		if name == "" || key == "" {
			problems = append(problems, fmt.Errorf("headers.types[%q]: name and header type are both required", name))
		}
	}
	if h.Default != "" {
		if _, ok := h.Types[h.Default]; !ok {
			problems = append(problems, fmt.Errorf("headers.default: %q is not one of headers.types", h.Default))
		}
	}
	return problems
}

// validate checks the i-th document; seen holds the names of the earlier ones.
func (d Doc) validate(i int, seen map[string]bool) []error {
	var problems []error
	at := func(field, msg string) error { return fmt.Errorf("docs[%d].%s: %s", i, field, msg) }
	switch {
	case d.Name == "":
		problems = append(problems, at("name", "required"))
	case seen[d.Name]:
		problems = append(problems, at("name", fmt.Sprintf("duplicate document name %q", d.Name)))
	}
	seen[d.Name] = true
	if d.Audience != AudienceInternal && d.Audience != AudiencePublic {
		problems = append(problems, at("audience", fmt.Sprintf("want %q or %q, got %q", AudienceInternal, AudiencePublic, d.Audience)))
	}
	if d.Output == "" {
		problems = append(problems, at("output", "required"))
	} else if outsideModule(d.Output) {
		problems = append(problems, at("output", "must be a path inside the module directory"))
	}
	if d.Info.Title == "" {
		problems = append(problems, at("info.title", "required"))
	}
	if len(d.Models) == 0 {
		problems = append(problems, at("models", "at least one package pattern is required"))
	}
	if len(d.Services) == 0 {
		problems = append(problems, at("services", "at least one package pattern is required"))
	}
	for j, s := range d.Servers {
		if s.URL == "" {
			problems = append(problems, at(fmt.Sprintf("servers[%d].url", j), "required"))
		}
	}
	for j, prefix := range d.HideTypePrefixes {
		if prefix == "" {
			problems = append(problems, at(fmt.Sprintf("hide_type_prefixes[%d]", j), "must not be empty"))
		}
	}
	if d.Public != nil {
		if d.Audience != AudiencePublic {
			problems = append(problems, at("public", "only a document of the public audience has a public section"))
		}
		for j, s := range d.Public.StripTagsContaining {
			if s == "" {
				problems = append(problems, at(fmt.Sprintf("public.strip_tags_containing[%d]", j), "must not be empty"))
			}
		}
	}
	return append(problems, d.validateOverlays(at)...)
}

func (d Doc) validateOverlays(at func(field, msg string) error) []error {
	var problems []error
	applied := map[OverlayFile]bool{}
	for j, o := range d.Overlay {
		oat := fmt.Sprintf("overlay[%d]", j)
		switch {
		case o.File == "":
			problems = append(problems, at(oat+".file", "required"))
		case outsideModule(o.File):
			problems = append(problems, at(oat+".file", "must be a path inside the module directory"))
		}
		if o.Stage != StageAfterModels && o.Stage != StageAfterAPIs {
			problems = append(problems, at(oat+".stage", fmt.Sprintf("want %q or %q, got %q", StageAfterModels, StageAfterAPIs, o.Stage)))
		}
		if applied[o] {
			problems = append(problems, at(oat, fmt.Sprintf("%s is already applied at %s", o.File, o.Stage)))
		}
		applied[o] = true
	}
	return problems
}

// outsideModule reports whether a path from the configuration leaves the module directory.
func outsideModule(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(filepath.Clean(path), "..")
}

func (e Errors) validate() []error {
	var problems []error
	if e.File != "" && outsideModule(e.File) {
		problems = append(problems, errors.New("errors.file: must be a path inside the module directory"))
	}
	if e.File == "" && e.ComponentPrefix != "" {
		problems = append(problems, errors.New("errors.component_prefix: has no effect without errors.file"))
	}
	if e.ComponentPrefix != "" && (len(e.ComponentPrefix) < 2 || !strings.HasSuffix(e.ComponentPrefix, ".")) {
		problems = append(problems, fmt.Errorf("errors.component_prefix: %q must name a package and end in a dot", e.ComponentPrefix))
	}
	return problems
}

func (r Request) validate() []error {
	var problems []error
	for key, fields := range r.Multipart {
		at := "request.multipart." + key
		if len(fields) == 0 {
			problems = append(problems, fmt.Errorf("%s: at least one field is required", at))
		}
		names := map[string]bool{}
		for i, f := range fields {
			fat := fmt.Sprintf("%s[%d]", at, i)
			switch {
			case f.Name == "":
				problems = append(problems, fmt.Errorf("%s.name: required", fat))
			case names[f.Name]:
				problems = append(problems, fmt.Errorf("%s.name: duplicate field %q", fat, f.Name))
			}
			names[f.Name] = true
			switch f.Kind {
			case "file":
				if f.ScalarTo != "" {
					problems = append(problems, fmt.Errorf("%s.scalar_to: only a scalar has one", fat))
				}
			case "scalar":
				if f.ScalarTo != "string" && f.ScalarTo != "integer" {
					problems = append(problems, fmt.Errorf("%s.scalar_to: want string or integer, got %q", fat, f.ScalarTo))
				}
			default:
				problems = append(problems, fmt.Errorf("%s.kind: want file or scalar, got %q", fat, f.Kind))
			}
		}
	}
	for key, fields := range r.Query {
		at := "request.query." + key
		if len(fields) == 0 {
			problems = append(problems, fmt.Errorf("%s: at least one parameter is required", at))
		}
		for i, f := range fields {
			if f.Name == "" {
				problems = append(problems, fmt.Errorf("%s[%d].name: required", at, i))
			}
			switch f.Type {
			case "", "string", "integer", "number", "boolean":
			default:
				problems = append(problems, fmt.Errorf("%s[%d].type: want string, integer, number or boolean, got %q", at, i, f.Type))
			}
		}
	}
	seen := map[RuntimeOnly]bool{}
	for i, ro := range r.RuntimeOnly {
		if ro.Path == "" || ro.Type == "" {
			problems = append(problems, fmt.Errorf("request.runtime_only[%d]: path and type are both required", i))
		}
		if seen[ro] {
			problems = append(problems, fmt.Errorf("request.runtime_only[%d]: duplicate %s %s", i, ro.Path, ro.Type))
		}
		seen[ro] = true
	}
	return problems
}

// isOtherStatus reports whether s is a three-digit HTTP status from 100 to 599
// other than 200, the one a response always has.
func isOtherStatus(s string) bool {
	return len(s) == 3 && s[0] >= '1' && s[0] <= '5' && strings.Trim(s, "0123456789") == "" && s != "200"
}

func (a API) validate() []error {
	var problems []error
	if p := a.Prefix; p != "" {
		if !strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") || strings.ContainsAny(p, " \t?#") {
			problems = append(problems, fmt.Errorf("api.prefix: %q must start with a slash, must not end with one and must be a plain path such as /api", p))
		}
	}
	for _, name := range a.SkipMethods {
		if !token.IsIdentifier(name) {
			problems = append(problems, fmt.Errorf("api.skip_methods: %q is not the name of a Go method", name))
		}
	}
	return problems
}

func (e Envelope) validate() []error {
	var problems []error
	names := map[string]string{"code": e.Code, "message": e.Message, "data": e.Data}
	for _, key := range []string{"code", "message", "data"} {
		if names[key] == "" {
			problems = append(problems, fmt.Errorf("response.envelope.%s: required", key))
		}
	}
	if e.Code != "" && (e.Code == e.Message || e.Code == e.Data) || e.Message != "" && e.Message == e.Data {
		problems = append(problems, errors.New("response.envelope: code, message and data must be three different properties"))
	}
	return problems
}

func (r Response) validate() []error {
	var problems []error
	if r.Envelope != nil {
		problems = append(problems, r.Envelope.validate()...)
	}
	for status, description := range r.DefaultStatuses {
		if !isOtherStatus(status) {
			problems = append(problems, fmt.Errorf("response.default_statuses[%q]: want a status code other than 200", status))
		}
		if description == "" {
			problems = append(problems, fmt.Errorf("response.default_statuses[%q]: description required", status))
		}
	}
	for key, b := range r.Binary {
		at := "response.binary." + key
		if b.Description == "" {
			problems = append(problems, fmt.Errorf("%s.description: required", at))
		}
		if len(b.ContentTypes) == 0 {
			problems = append(problems, fmt.Errorf("%s.content_types: at least one media type is required", at))
		}
		for status, description := range b.Errors {
			if !isOtherStatus(status) {
				problems = append(problems, fmt.Errorf("%s.errors[%q]: want a status code other than 200", at, status))
			}
			if description == "" {
				problems = append(problems, fmt.Errorf("%s.errors[%q]: description required", at, status))
			}
		}
	}
	return problems
}

func (t TypeSpec) validate(at string) []error {
	switch t.Type {
	case "string", "integer", "number", "boolean", "object":
		if t.Items != nil {
			return []error{fmt.Errorf("%s.items: only an array has items", at)}
		}
	case "array":
		if t.Items == nil {
			return []error{fmt.Errorf("%s.items: an array needs its element type", at)}
		}
		return t.Items.validate(at + ".items")
	default:
		return []error{fmt.Errorf("%s.type: want string, integer, number, boolean, object or array, got %q", at, t.Type)}
	}
	return nil
}

// Select returns the documents with the given names, in the order of the file.
// With no names it returns all of them. An unknown name is an error that lists
// the available ones.
func (c *Config) Select(names []string) ([]Doc, error) {
	if len(names) == 0 {
		return c.Docs, nil
	}
	var out []Doc
	for _, d := range c.Docs {
		if slices.Contains(names, d.Name) {
			out = append(out, d)
		}
	}
	for _, n := range names {
		if !slices.ContainsFunc(c.Docs, func(d Doc) bool { return d.Name == n }) {
			avail := make([]string, 0, len(c.Docs))
			for _, d := range c.Docs {
				avail = append(avail, d.Name)
			}
			return nil, fmt.Errorf("no document named %q (available: %s)", n, strings.Join(avail, ", "))
		}
	}
	return out, nil
}
