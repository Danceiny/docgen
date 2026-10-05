# Security

docgen reads Go source and writes YAML; it makes no network requests of its own
and runs no code of the module it reads. It asks the Go tool to load and
type-check the module, which is what `go build` does: that can download the
modules the module needs and a Go toolchain, as `go build` would, and a
`go.mod` can name what it downloads.

If you find a vulnerability, please report it privately through
[GitHub's private vulnerability reporting](https://github.com/Danceiny/docgen/security/advisories/new)
rather than in a public issue, and allow reasonable time for a fix before
disclosing it.
