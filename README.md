# btree

A fork of [Google's btree](https://github.com/google/btree) at v1.1.3, kept
free of dependencies: an in-memory B-tree for ordered data, generic over
the item type.

- **Module:** `github.com/raftseg/btree`.
- **No dependencies:** `go.mod` requires nothing, and every package, tests
  included, imports only the Go standard library and this module.
  `internal/depcheck` fails the tests otherwise.
- **Generics only:** the pre-generics implementation is gone. `BTreeG[T]`
  is the implementation, and `BTree` (items with a `Less` method) is
  `BTreeG[Item]` with upstream's API, as it already was on Go 1.18 and
  later.
- **Same API as upstream v1.1.3.** [FORK.md](FORK.md) lists every change.
- **Model-based tests:** random operations on trees that share nodes
  through `Clone`, checked against a model and the B-tree and
  copy-on-write invariants after every step.

Upstream's README is [UPSTREAM-README.md](UPSTREAM-README.md). The license
is upstream's, Apache 2.0 ([LICENSE](LICENSE)); [NOTICE](NOTICE) lists what
this fork changed.

## Relation to upstream

This fork's history starts at upstream's tag v1.1.3 (commit `aeba20f`,
upstream's `master`), with upstream's full history below it. The branch
`upstream-main` mirrors upstream's default branch. Upstream fixes are taken
with `git cherry-pick -x`, so each commit names the upstream commit it came
from, and are listed in [upstream.json](upstream.json), which SBOM tools
read for this fork's pedigree.

## Testing

```
go test ./...
go test -short -race ./...
go test -run XXX -fuzz FuzzModel .
```

btree is used by [raftseg](https://github.com/raftseg/raftseg).
