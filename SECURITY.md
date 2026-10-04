# Security

docgen reads Go source and writes YAML; it makes no network requests and runs
no code of the module it reads (it asks the Go tool to load and type-check the
module, which is what `go build` does).

If you find a vulnerability, please report it privately through
[GitHub's private vulnerability reporting](https://github.com/Danceiny/docgen/security/advisories/new)
rather than in a public issue, and allow reasonable time for a fix before
disclosing it.
