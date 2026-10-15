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
- `btree_generic.go`'s doc comments were reformatted by gofmt.

## Tests

Upstream's tests are kept unchanged. `model_test.go` adds model-based
tests: random sequences of every operation (inserts, deletes, lookups,
every kind of range iteration with and without early stops, `Clone`,
`Clear`, new trees) run against up to four trees that share nodes through
`Clone` and share a free list, each compared with a sorted slice after every
operation. The structure is checked as well:

- keys are ordered within and across nodes, leaves are all at one depth,
  node sizes stay within the degree's bounds, and `Len` is right;
- copy-on-write: a node a tree owns, and so modifies in place, is
  reachable from that tree alone;
- no node in the free list is reachable from a tree.

`FuzzModel` drives the same checks from fuzzer input, `TestModelRandom` runs
long sequences over key spaces of 8 to 4,096 keys, and
`TestSharedFreeListConcurrentWrites` writes clones and trees sharing a free
list from several goroutines, for the race detector.

These tests catch copy-on-write and free-list defects that upstream's tests
do not: a `freeNode` that frees nodes it does not own, a merge that takes
ownership of a shared node, and stealing from a sibling without copying it
all pass upstream's tests and fail these.

Upstream's last change (aeba20f, in tag v1.1.3) made a merge in
`growChildAndRemove` copy the right-hand child before freeing it, but only
in the pre-generics `btree.go`, which this fork deleted. The generic code
does not need it: it only reads that child, and `freeNode` frees a node
only if the tree owns it, so a shared node is never cleared. The ownership
check above confirms that no tree ever owns a node another tree can reach.
