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
other character matches itself. So `*/service` matches `pet/service` and
`shop/pet/service`, and does not match a package `service` at the module root: for
a flat layout write `services: ["service"]` (or both). A pattern that matches no
package is reported, since the document it belongs to would be empty without it.

## What docgen looks for

### Services and operations

A **service** is a struct with a method `Name() string` that returns a string
constant: a literal, a named constant (of any package of the module) or a
constant expression. Its exported methods are the **operations**, except `Name`
itself, the methods marked `@apidoc: -` and the ones listed in `api.skip_methods`.

```go
type PetService struct{}

func (PetService) Name() string { return "pet" }

func (PetService) Get(ctx context.Context, req *protocol.GetPetReq) (*domain.Pet, error)
```

- The first parameter may be a `context.Context`; it is not documented.
- The next parameter is the **request**: a struct (or pointer to struct) is the
  JSON request body, a list or a map too, and a parameter of a basic type is a
  body of that type unless a `@param` annotation says it is a query, header,
  path or cookie parameter. Only the first parameter after the context is read:
  docgen warns about a method that has more, since the others have no place in
  the document.
- A result that is not an `error` is the **`200` response**: a struct is a
  reference to its component, a list is an `array` of its element, a map an
  `object`, and an instantiated generic type such as `Page[Pet]` the component of
  `Page`. An `error` is not documented by itself; use `@response` (below).
- The route is `<prefix>/<service>/<method name with its first letter
  lower-cased>`, so `Get` of the service `pet` is `/api/pet/get`. A service name
  may contain slashes (`store/order`). The prefix is `/api` unless `api.prefix`
  says otherwise. `@path` replaces the method name (see below), and a `@path`
  that starts with the service name loses that, as a plain prefix: `@path:
  /orderList` of the service `order` is `/api/order/list`, and so is `@path:
  /order/list`; the method name itself is never stripped (`OrderList` is
  `/api/order/orderList`).
- The default HTTP method is `POST`; `@method` changes it, and several methods
  (`@method: GET, POST`) give an operation each, whose ids have the method in lower
  case added (`pet/get_get`, `pet/get_post`). OpenAPI does not allow a body on `GET` and `DELETE`, but docgen
  documents the request of such an operation as a body all the same, unless the
  request type is listed in `request.query`, which makes it query parameters (see
  the configuration).
- The tags of an operation are the service name, the ones of `@tags` and the ones
  of `@permission`, without repeats, sorted. The first letter of every word of the
  service name is made upper case and the rest is left as it is (`myService/Sub`
  is `MyService/Sub`); the tags of `@tags` and `@permission` are made lower case
  except for the first letter of each word (`OAuth` is `Oauth`, `x-y` is `X-Y`).
- The summary is the first line of the doc comment; the description is `@desc`.

The package [`route`](route) is the executable specification of the route rule.
If you have a router of your own, test it against `route.Resolve`.

### Annotations on a method

Written in the doc comment of the method, one per line. An annotation docgen does
not know is left alone, since other tools read comments too; one that is a letter
away from a known one (`@respone`, `@Tags`) is reported.

| Annotation | Meaning |
|---|---|
| `@desc: text` | The description of the operation. It runs to the next annotation, so it may span paragraphs and use Markdown. |
| `@tags: a, b` | Extra tags. |
| `@permission: pet:write` | Added as a tag too; handy to show what a caller needs. |
| `@method: GET` | HTTP methods, comma separated: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`, `TRACE`. |
| `@path: /custom` | Replaces the method name in the route. A value that already starts with the prefix is the whole route. |
| `@doc: Guide: https://example.com/guide` | External documentation, as `description: url`. |
| `@response:404,NotFoundErr,text` | An extra response: the status, the name of an error of the [error catalog](#the-error-catalog), a description. The status is checked against the catalog. |
| `@param:query limit int required "how many"` | A parameter that is not the body: `<in> <name> <type> <required\|optional> ["description"]`, where `in` is `query`, `header`, `path` or `cookie`. `name` is the name of the Go parameter (or `param1`, `param2`, … by position), and is the name in the document. A `path` parameter must be in the `@path`, as `/get/{id}`. |
| `@headerType: Admin` | Chooses the header type of the operation among those of `headers.types`. |
| `@apidoc: -` | The method is not an operation: neither routed, by the convention of `route.Skipped`, nor documented. |

### Types

The named types of the `models` packages become **components**
(`#/components/schemas/<import path with dots>.<Name>`).

- Fields are read as `encoding/json` reads them. Names come from `json` tags;
  `json:"-"` and unexported fields are left out; a declaration of several names
  (`Lat, Lng float64`) is a field for each, and a blank field none. An embedded
  struct is flattened into the struct that embeds it, unless it has a json name
  (``Base `json:"base"` ``), which makes it a field of that name.
- A field is required when its `validate` or `binding` tag has the rule
  `required` (`validate:"required,email"` has it), or it has `required:"true"`.
  `json:"x,nullable"` makes it nullable.
- `example:"..."` and `default:"..."` tags (or `json:"x,default=..."`) give the
  example and the default of a field.
- The comment of a **field**, above it or after it, is its description. The comment
  of a type is not used. A field whose type is a component is a `$ref`, and
  OpenAPI 3.0 does not allow a description next to a `$ref`, so the internal
  document leaves the comment of such a field out.
- Constants of a named type make it an **enum**, whichever way they are declared
  (`iota`, shifts, expressions): the values the compiler computes are listed in
  `enum`, with the names and the comments of the constants in the description. A
  type is an enum when at least one of its constants is declared with the type
  (`A Status = iota`), and a name made with `type Status = string` is a name for
  `string`, which only the constants declared with that name make an enum.
- A map is an `object` whose `additionalProperties` is the schema of its values
  (`true` for a map of any value), `any` and `interface{}` are the empty schema, a
  slice of bytes is a base64 `string` and a byte a number.
- A type that contains itself, directly or through other types, is a `$ref` to
  itself.
- `time.Time` is a `date-time` string and `time.Duration` an `int64` number of
  nanoseconds, as JSON writes them. `type_map` says otherwise.

### Who sees what

A document has an **audience**, `internal` or `public`, which decides what the
visibility rules show.

On a **type**, in its doc comment:

```go
//apidoc:public                           shown in the public document, and so in the internal one
//apidoc:internal                         shown in the internal document only
//apidoc:hidden                           never shown
//apidoc:public:StatusActive,StatusNew    an enum: only these values are shown
//apidoc:public:-StatusLegacy*            an enum: every value but those that start with StatusLegacy
```

There is no space after the slashes (`// apidoc:hidden` is an ordinary comment),
and the values of an enum are the **names of its constants**, not what they are on
the wire. Docgen warns about a directive it cannot read.

On a **field**, as a struct tag:

```go
Notes string `json:"notes" apidoc:"internal"`   // the internal document only
Beta  string `json:"beta"  apidoc:"public"`     // the public document only
Cost  int    `json:"cost"  apidoc:"hidden"`     // never (`apidoc:"-"` too)
```

A document also leaves out every type whose name starts with a prefix of its
`hide_type_prefixes`, unless the type's own `//apidoc:` says otherwise. Anything
with no annotation is shown everywhere.

A hidden type has no schema, and neither has anything that would refer to it: a
field of the type, of a list or map of it, or that embeds it, is left out, and so
is an operation that takes or returns it (with a warning that says which). An
enum is the exception: it is shown, with its values hidden.

An older form, `apidoc:"Staff"`, shows the field only in the documents that list
`Staff` in `legacy_field_tokens`. Any other value of the tag hides the field from
every document, and docgen warns about it, since a typo and a description put in
the wrong tag are the two ways to lose a field that way. The tag
`api.header:"X-Request-Id"` names the HTTP header a field of a header type travels
in.

## Configuration

`docgen.yaml` is strict: an unknown key (with the keys that are there, and the one
it is probably a typo of), a bad value or a type key written with slashes is an
error that names the key; the keys that do not decode are reported together, and
so are the bad values. A key that names a type of the module that the module does
not declare, and a pattern that matches no package, are warnings.
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

compat:                         # only for documents generated by an older tool,
  legacy_operation_types: false #   see "Documents generated before"
  legacy_schema_shapes: false

type_map:                       # fix the schema of a type instead of reading it
  example.com.shop.types.ID: {type: string, description: id}
  example.com.shop.types.IDs: {type: array, items: {type: string}}
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
  query:                        # request types read from the URL query: no body. The
                                #   parameters are listed by hand, for every operation that
                                #   takes the type, and are not checked against its fields
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
`after_apis` overlay is applied after. A schema of an overlay has `$ref`, `title`,
`description`, `type`, `format`, `required`, `properties`, `items`,
`additionalProperties` and `oneOf`; any other key (`enum`, `example`, `nullable`,
`allOf` and the constraints) is rejected with the keys that are understood. To
give a type such values, describe it in Go, or in `type_map`.

## Command line

```
docgen [-C dir] [-config file] [-doc name]... [-check] [-v] [-cpuprofile file] [-memprofile file]
docgen -version
```

| Flag | |
|---|---|
| `-C dir` | Run as if started in `dir`, the root of the module to document. |
| `-config file` | The configuration file, relative to `dir`; `docgen.yaml` by default. |
| `-doc name` | Generate only this document; repeat for several. |
| `-check` | Generate into a temporary directory, compare with the files on disk and exit 1 if one differs. Writes nothing. |
| `-v` | Also log what helps to follow a run, such as each type hidden by its annotation. |
| `-cpuprofile file`, `-memprofile file` | Write a CPU or a memory profile, for finding what makes a run slow. |
| `-version` | Print the version. |

Warnings go to standard error; the ones about code say where it is. They do not
fail the run: they say that something in the source or the configuration does
nothing, or does something else than it looks like, so read them as a to-do list.
`-check` compares the text of the documents and takes a checkout that has turned
line feeds into carriage returns and line feeds (`core.autocrlf`) for current.

In CI:

```sh
go tool docgen -check
```

## Documents generated before

docgen was extracted from the tool that generated the documents of one large
service, and those documents have been reviewed and checked in for years. A few
things the first tool got wrong are fixed in docgen, and a project that has such
documents does not want them rewritten by an upgrade, so each fix that changes
them has a switch that keeps the old reading. A new project should not set either.

```yaml
compat:
  legacy_operation_types: true
  legacy_schema_shapes: true
```

- `legacy_operation_types`: the types of the parameters and results of an operation
  are read as a list being its element type (`[]Pet` is `Pet`), and a map, an
  interface and an instantiated generic type being of an unknown type, so that a
  result of one has no content.
- `legacy_schema_shapes`: of a declaration of several names (`Lat, Lng float64`)
  only the first is a field and a blank field is a property called `_`; an embedded
  struct with a json name is flattened; `validate:"required,email"` does not make a
  field required, only `validate:"required"` does; `any` is an `object` and
  `interface{}` a string, an integer or an object; the values of an enum are the
  constants declared with the type and a literal, so `iota` and expressions give an
  empty value and a constant that repeats the type implicitly is missing; the values
  of a map are lost (`additionalProperties: true`); a slice of bytes is an array of
  strings and a byte a string; a `type_map` entry for an enum type is ignored.

## Known limitations

docgen is young. These are known, and will change in a later release; do not rely
on them.

- **Types of other modules are written out where they are used**, as inline
  schemas, not as components; a type that contains itself there is an `object`
  the second time. Do not use one as the request or response type of an
  operation: wrap it in a type of your module. A custom marshaler is not read, so
  a type that writes itself as something else than its Go shape (`net.IP`,
  `big.Int`, a UUID that is an array of bytes) needs a `type_map` entry.
- **A public document is flat**: a type that a field uses is written out in place
  of a reference, with its name as the title, and only the types that operations
  take or return are components. A deep or diamond-shaped type graph makes it
  large. The marker `pattern: default` can show on a schema that was never filled
  in.
- `json:",string"` is ignored, a pointer is not nullable unless `json:"x,nullable"`
  says so, and `json.RawMessage` is a `string` where it is any JSON.
- **Generics are documented as declared**: a field of a type-parameter type has no
  schema, and a generic type with more than one type parameter is not supported.
- **Query parameters are listed by hand** in `request.query`, not read from the
  struct, and apply to every operation that takes the type.
- The comment of a type is not its description, and the internal document drops
  the comment of a field that is a `$ref`.
- The request and response types of an operation must be declared in a package
  that matches `models`.
- The documents are OpenAPI 3.0; kin-openapi does not write 3.1.
- Every run loads and type-checks the whole module (`./...`), and fails, saying
  why, when a package of it does not build.

## Compatibility

Before 1.0 the command line, the configuration and the output may change in a
minor release. The changes are listed in [CHANGELOG.md](CHANGELOG.md), and the
output of a version does not change within it. Pin the version, and check the
documents in.

The only Go package you can import is [`route`](route), which has no
dependencies. The rest is internal.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short: `go test ./...` must pass and
`go run ./cmd/docgen -C examples/petstore -check` must succeed.

## License

[MIT](LICENSE). The third-party code docgen builds on is listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
