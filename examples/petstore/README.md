# petstore

A small module that uses most of what docgen does. Its two documents are checked
in, and the tests of docgen fail when they are not what the tool generates.

```sh
docgen            # writes docs/api/internal.yaml and docs/api/public.yaml
docgen -check     # exits 1 if either differs from what the source generates
```

| Where | What it shows |
|---|---|
| [`docgen.yaml`](docgen.yaml) | Two documents from one module, a type with another wire form, a header type, query and multipart requests, a file response, an error catalog and an overlay. |
| [`pet/service`](pet/service/service.go) | A service, the annotations of a method (`@desc`, `@tags`, `@permission`, `@method`, `@path`, `@doc`, `@response`, `@apidoc: -`) and a request type the models do not cover. |
| [`pet/domain`](pet/domain/pet.go) | Types, an enum with retired values left out of the documents, fields shown to one audience only, a type that contains itself and a type hidden from the public document by its name. |
| [`pet/protocol`](pet/protocol/pet.go) | Requests and responses, including the ones `docgen.yaml` says are read from the query, uploaded or downloaded. |
| [`store/service`](store/service/service.go) | A service whose name has a slash. |
| [`errors.json`](errors.json) | The error catalog `@response` refers to. |
| [`overlay.yaml`](overlay.yaml) | A schema written by hand. |
