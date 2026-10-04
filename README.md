# docgen

**OpenAPI 3.0 documents from the Go source of your service.** docgen reads your
packages with `go/packages`, finds your services and the types they take and
return, and writes the YAML. Nothing is generated into your code and nothing is
executed: the documents come from the types, struct tags and doc comments you
already write.

[简体中文](README.zh-CN.md)

```go
// Get returns one pet.
//
// @desc: Looks a pet up by its id.
// @tags: public
// @response:404,NotFoundErr,There is no pet with that id
func (PetService) Get(ctx context.Context, req *protocol.GetPetReq) (*domain.Pet, error) {
```

becomes `POST /api/pet/get`, with a request body, a `200` response whose schema
is `domain.Pet`, a `404` response for `NotFoundErr`, and tags.

- **One module, several documents.** An internal document with everything and a
  public one with the operations and fields you chose, from the same source.
- **Check mode for CI.** `docgen -check` fails when a checked-in document is not
  what the source generates, and says where it differs.
- **Configuration is data.** One `docgen.yaml`, plus optional overlay and error
  catalog files. There is no plug-in API to compile against.
- **Deterministic.** The same source gives the same bytes, so the documents can
  be reviewed in a pull request and diffed.

docgen needs Go 1.25 or later on the machine that runs it. The module it reads
can target any Go version that machine can build.

## Install

```sh
go install github.com/Danceiny/docgen/cmd/docgen@latest
```

Or pin it to your module as a tool (Go 1.24 and later), so that everybody and the
CI run the same version:

```sh
go get -tool github.com/Danceiny/docgen/cmd/docgen@v0.1.0
go tool docgen -version
```

## Quick start

[`examples/petstore`](examples/petstore) is a small module that uses most of what
docgen does. Its two documents are checked in.

```sh
git clone https://github.com/Danceiny/docgen
cd docgen/examples/petstore
docgen            # writes docs/api/internal.yaml and docs/api/public.yaml
docgen -check     # exits 1 if either file is not what the source generates
```

To document your own module, write a `docgen.yaml` in its root:

```yaml
version: 1
docs:
  - name: api
    audience: internal
    output: docs/api/openapi.yaml
    info: {title: My API, version: 1.0.0}
    models: ["*/domain", "*/protocol"]   # packages whose types become schemas
    services: ["*/service"]              # packages whose services become operations
```

and run `docgen` there. Patterns are matched against the package path relative to
the module path; `*` matches any run of characters, slashes included, and every
other character matches itself.

## What docgen looks for

### Services and operations

A **service** is a struct with a method `Name() string` that returns a string
literal or a constant. Its exported methods are the **operations**, except `Name`
itself, the methods marked `@apidoc: -` and the ones listed in `api.skip_methods`.

```go
type PetService struct{}

func (PetService) Name() string { return "pet" }

func (PetService) Get(ctx context.Context, req *protocol.GetPetReq) (*domain.Pet, error)
```

- The first parameter may be a `context.Context`; it is not documented.
- A struct (or pointer to struct) parameter is the **request body**. Parameters
  of other types are described with `@param` annotations.
- A result that is not an `error` is the **`200` response**. An `error` is not
  documented by itself; use `@response` (below).
- The route is `<prefix>/<service>/<method name with its first letter
  lower-cased>`, so `Get` of the service `pet` is `/api/pet/get`. A service name
  may contain slashes (`store/order`). The prefix is `/api` unless `api.prefix`
  says otherwise.
- The default HTTP method is `POST`; `@method` changes it.
- The tags of an operation are the title-cased service name, then those of
  `@tags` and `@permission`.
- The summary is the first line of the doc comment; the description is `@desc`.

The package [`route`](route) is the executable specification of the route rule.
If you have a router of your own, test it against `route.Resolve`.

### Annotations on a method

Written in the doc comment of the method, one per line.

| Annotation | Meaning |
|---|---|
| `@desc: text` | The description of the operation. It runs to the next annotation, so it may span paragraphs and use Markdown. |
| `@tags: a, b` | Extra tags. |
| `@permission: pet:write` | Added as a tag too; handy to show what a caller needs. |
| `@method: GET` | HTTP methods, comma separated: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`, `TRACE`. |
| `@path: /custom` | Replaces the method name in the route. A value that already starts with the prefix is the whole route. |
| `@doc: Guide: https://example.com/guide` | External documentation, as `description: url`. |
| `@response:404,NotFoundErr,text` | An extra response: the status, the name of an error of the [error catalog](#the-error-catalog), a description. The status is checked against the catalog. |
| `@param:query limit int required "how many"` | A parameter that is not the body: `<in> <name> <type> <required\|optional> ["description"]`, where `in` is `query`, `header`, `path` or `cookie`. |
| `@headerType: Admin` | Chooses the header type of the operation among those of `headers.types`. |
| `@apidoc: -` | The method is not an operation: neither routed, by the convention of `route.Skipped`, nor documented. |

### Types

The named types of the `models` packages become **components**
(`#/components/schemas/<import path with dots>.<Name>`).

- Field names come from `json` tags. `json:"-"` and unexported fields are left
  out. Embedded structs are flattened.
- A field is required when it has `validate:"required"`, `binding:"required"` or
  `required:"true"`. `json:"x,nullable"` makes it nullable.
- `example:"..."` and `default:"..."` tags (or `json:"x,default=..."`) give the
  example and the default of a field.
- The doc comment of a type or a field is its description.
- Constants of a named type make it an **enum**: the values and their comments
  are listed in `enum` and in the description.
- A type that contains itself, directly or through other types, is a `$ref` to
  itself.
- `time.Time` is a `date-time` string and `time.Duration` an `int64` number of
  nanoseconds, as JSON writes them. `type_map` says otherwise.

### Who sees what

A document has an **audience**, `internal` or `public`, which decides what the
visibility rules show.

On a **type**, in its doc comment:

```go
//apidoc:public                    shown in the public document, and so in the internal one
//apidoc:internal                  shown in the internal document only
//apidoc:hidden                    never shown
//apidoc:public:Active,Pending     an enum: only these values are shown
//apidoc:public:-Legacy*           an enum: every value but those that start with Legacy
```

On a **field**, as a struct tag:

```go
Notes string `json:"notes" apidoc:"internal"`   // the internal document only
Beta  string `json:"beta"  apidoc:"public"`     // the public document only
Cost  int    `json:"cost"  apidoc:"hidden"`     // never (`apidoc:"-"` too)
```

A public document also leaves out every type whose name starts with a prefix of
`hide_type_prefixes`, unless its own `//apidoc:` says otherwise. Anything with no
annotation is shown everywhere.

An older form, `apidoc:"Staff"`, shows the field only in the documents that list
`Staff` in `legacy_field_tokens`. The tag `api.header:"X-Request-Id"` names the
HTTP header a field of a header type travels in.

## Configuration

`docgen.yaml` is strict: an unknown key, a bad value or a pattern that cannot
work is an error that names the key, and all the problems are reported at once.
The [petstore configuration](examples/petstore/docgen.yaml) is a complete
example; this is the reference.

```yaml
version: 1                      # the only version

api:
  prefix: /api                  # starts every route; default /api
  skip_methods: [GetChildren]   # never operations, besides Name

vendor_extensions: false        # add x-apifox-*, x-enum-varnames, x-enum-comments,
                                # x-display-name, x-primary-property, x-go-interface
keep_empty_tags: false          # keep the empty tag of an operation without @tags

type_map:                       # fix the schema of a type instead of reading it
  example.com.shop.types.ID: {type: string, description: id}
  github.com.shopspring.decimal.Decimal: {type: number}
  time.Duration: {type: string, format: duration, example: 1h30m}

headers:                        # the header type each operation takes, as its
  default: Auth                 #   "Headers" parameter
  types: {Auth: example.com.shop.protocol.AuthHeader}

request:
  multipart:                    # request types that are multipart/form-data
    example.com.shop.protocol.UploadReq:
      - {name: file, kind: file, required: true}
      - {name: caption, kind: scalar, scalar_to: string}
  query:                        # request types read from the URL query: no body
    example.com.shop.protocol.ListReq:
      - {name: status, type: string, description: Limit the list}
  runtime_only:                 # bindings no JSON value can satisfy
    - {path: /api/shop/setHook, type: example.com.shop.Hook}

response:
  envelope: {code: code, message: message, data: data}   # wrap the data; without
                                                         #   one, the data is the body
  default_statuses: {"401": Unauthorized}                # on every operation
  binary:                       # response types that are files
    example.com.shop.protocol.DownloadResp:
      description: The file
      content_types: [image/png]
      errors: {"404": Not found}

errors:                         # the error catalog @response refers to
  file: errors.json
  component_prefix: example.com.shop.errors.

generic_titles: []              # full keys of generic types whose oneOf is not narrowed

docs:
  - name: internal              # selects the document with -doc
    audience: internal          # internal | public
    output: docs/api/internal.yaml
    info: {title: Shop API, version: 1.0.0, description: ...}
    servers: [{url: "http://localhost:8080", description: Local}]
    models: ["*/domain", "*/protocol"]
    services: ["*/service"]
    legacy_field_tokens: []     # values of apidoc:"..." tags shown in this document
    hide_type_prefixes: []      # types whose name starts with one are left out
    overlay:                    # hand-written schemas, see Overlays
      - {file: overlay.yaml, stage: after_models}

  - name: public
    audience: public
    output: docs/api/public.yaml
    info: {title: Shop API, version: 1.0.0}
    models: ["*/domain", "*/protocol"]
    services: ["*/service"]
    force_keep: []              # components kept though no operation uses them
    public:                     # only for audience: public
      tag: public               # the operations with this tag are the public ones
      strip_tags_containing: [] # remove the tags that contain one of these strings
      errors_last: false        # put error alternatives of oneOf/anyOf/allOf last
```

`models` must contain the package of every type an operation takes or returns, or
the run fails with the reference that has no schema. docgen loads `./...` and
fails, saying why, when any package of the module does not build.

### The error catalog

`errors.json` lists the errors a service can answer with:

```json
[
  {"name": "NotFoundErr", "code": 40401, "message": "not found", "httpCode": 404}
]
```

Each becomes a component (named from `errors.component_prefix`), and
`@response:404,NotFoundErr,text` refers to it by name. If the status of the
annotation is not the `httpCode` of the error, docgen warns and uses the error's.
Generate the file from your own error definitions with a few lines of Go, and
check it in with the documents.

### Overlays

Some schemas cannot be read from Go: a type declared outside the packages you
document, a webhook payload, a union written by hand. An overlay file adds them,
or replaces what was generated:

```yaml
version: 1
schemas:
  example.com.shop.protocol.ImportReq:
    type: object
    required: [source]
    properties:
      source: {type: string, description: Where the pets come from}
      pets:
        type: array
        items: {$ref: '#/components/schemas/example.com.shop.domain.Pet'}
```

An `after_models` overlay is applied before the operations are generated, which
is what lets an operation refer to a schema that only the overlay has; an
`after_apis` overlay is applied after. An overlay understands the part of JSON
Schema that OpenAPI 3.0 documents use, and rejects the rest.

## Command line

```
docgen [-C dir] [-config file] [-doc name]... [-check] [-v]
docgen -version
```

| Flag | |
|---|---|
| `-C dir` | Run as if started in `dir`, the root of the module to document. |
| `-config file` | The configuration file, relative to `dir`; `docgen.yaml` by default. |
| `-doc name` | Generate only this document; repeat for several. |
| `-check` | Generate into a temporary directory, compare with the files on disk and exit 1 if one differs. Writes nothing. |
| `-v` | Also log what helps to follow a run, such as each type hidden by its annotation. |
| `-version` | Print the version. |

Warnings go to standard error; the ones about code say where it is.

In CI:

```sh
go tool docgen -check
```

## Known limitations

docgen is young, and the documents of the service it was written for fixed much
of its behaviour. These are known and will change in a later release; do not rely
on them.

- **Maps lose their value type**: `map[string]Pet` is `additionalProperties: true`.
- **Enums are read from literals.** A constant declared with `iota` or with an
  expression has an empty value, and in a block that repeats the type implicitly
  only the first constant is listed.
- `[]byte` is an array of strings (JSON writes a base64 string), `byte` is a
  string and `json:",string"` is ignored.
- **Types of other modules are written out where they are used**, as inline
  schemas, not as components; a type that contains itself there is an `object`
  the second time. Do not use one as the request or response type of an
  operation: wrap it in a type of your module.
- A reference that has a description is expanded in place instead of being a
  `$ref` with a sibling, which OpenAPI 3.0 forbids; the description of the field
  is kept and the schema is copied. The marker `pattern: default` can show on a
  schema that was never filled in.
- **Generics are documented as declared**: a field of a type-parameter type has
  no schema, and a generic type with more than one type parameter is not
  supported.
- The documents are OpenAPI 3.0; kin-openapi does not write 3.1.
- The request and response types of an operation must be declared in a package
  that matches `models`.

## Compatibility

Before 1.0 the command line, the configuration and the output may change in a
minor release. The changes are listed in [CHANGELOG.md](CHANGELOG.md), and the
output of a version does not change within it. Pin the version, and check the
documents in.

The only Go package you can import is [`route`](route), which has no
dependencies. The rest is internal.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short: `go test ./...` must pass and
`go run ./cmd/docgen -C examples/petstore -check` must be quiet.

## License

[MIT](LICENSE). The third-party code docgen builds on is listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
