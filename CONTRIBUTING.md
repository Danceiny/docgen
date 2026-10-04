# Contributing

Thanks for looking. docgen is small enough that a change is usually a pull
request with a test.

## Build and test

You need Go 1.25 or later.

```sh
go build ./...
go vet ./...
go test ./...
go run ./cmd/docgen -C examples/petstore -check   # the example documents are current
```

`golangci-lint run ./...` (version 2) must report nothing. CI runs all of these
on Linux, macOS and Windows, with the Go that `go.mod` names and, on Linux, the
newest.

## The example is a test

`examples/petstore` is a separate module whose generated documents are checked
in, and `go test ./internal/pipeline` fails when they are not what the tool
generates. When a change is meant to change them, regenerate and review the diff:

```sh
go run ./cmd/docgen -C examples/petstore
git diff examples
```

The same goes for the fixtures under `internal/*/testdata`: they are small Go
packages the tests load, so give a new behaviour a type in one of them rather than
a mock.

## What a change should look like

- **Output is the contract.** Generation must give the same bytes for the same
  source, whatever the order a map iterates in, so a change that alters the
  generated documents says so in the pull request and in `CHANGELOG.md`. Anything
  that iterates a map and writes the result needs sorted keys.
- **A bug fix has a test that fails without it.** Break the fix on purpose and
  look at the test fail before you send it.
- **Say what is wrong, where and what to do.** A diagnostic names the source
  position, the type or the key, and, when there is one, the way out. A field
  that disappears from a document without a word is the worst kind of bug.
- **Keep the surface small.** The configuration file, the annotations and the
  package `route` are the public interface; the rest is `internal`. A new
  configuration key needs a validation, a line in the README and a test of both.
- **No dependency without a reason, and none newer than needed.** The module has
  few; each one is in `THIRD_PARTY_NOTICES.md` with its license text, which
  `go test ./cmd/docgen` checks. What `go.mod` requires is required of everybody
  who runs docgen with `go tool`, whose own versions are raised to match, so even
  a test dependency is not updated without a reason.
- **Four dependencies are updated by hand.** kin-openapi and yaml.v3 write the
  YAML, x/tools loads the packages and x/text cases the tags, so a new version of
  any of them can change the generated documents. Update one in a pull request of
  its own, with the example documents regenerated, the notices updated and the
  difference in `CHANGELOG.md`. Dependabot leaves them alone for that reason.

## Commits and pull requests

Use a short imperative subject (`Keep a reference to a component that is being
expanded`), and a body that says why. One logical change to a pull request.

## Reporting problems

An issue is most useful with the smallest module that shows it: a `docgen.yaml`,
a Go file or two, what you expected and the output. For security problems see
[SECURITY.md](SECURITY.md).
