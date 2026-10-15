# How this btree differs from Google's btree v1.1.3

This package is a fork of [btree](https://github.com/google/btree) at tag
v1.1.3 (Apache License 2.0, see LICENSE). This file lists every change and
why it was made. Every modified file says so in its header.

## Module and layout

- The module path is `github.com/raftseg/btree`, and `go.mod` requires Go
  1.24 or later.
- Upstream kept two implementations: `btree.go` for Go releases before
  1.18, and `btree_generic.go` for 1.18 and later. Only the second could
  ever build under this module's Go version, so the first was deleted and
  the build constraints removed. `BTree` remains `BTreeG[Item]`, so the
  API, generic and not, is upstream's.
- `btree_mem.go`, a benchmark program behind `// +build ignore` that
  imported `github.com/petar/GoLLRB`, was deleted: it was never built, but
  it was the only reference to a third-party module.
- `internal/depcheck` fails the tests if `go.mod` requires anything or any
  Go file imports anything outside the standard library and this module.
