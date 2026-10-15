// Copyright 2026 The raftseg authors. Licensed under the Apache License,
// Version 2.0.

package btree

// Model-based tests: random sequences of operations run against several
// trees that share nodes through Clone and a free list, and against a sorted
// slice per tree. After every operation the tree must hold exactly what its
// model holds, every call must return what the model returns, and the
// structure must satisfy the B-tree and copy-on-write invariants (see
// checkForest).

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
)

// kv is ordered by k alone, so that ReplaceOrInsert's replacement of an equal
// item, and which item Delete returns, are observable through v.
type kv struct{ k, v int }

func kvLess(a, b kv) bool { return a.k < b.k }

// model is a sorted slice of items with distinct keys.
type model []kv

func (m model) search(k int) (int, bool) {
	i := sort.Search(len(m), func(i int) bool { return m[i].k >= k })
	return i, i < len(m) && m[i].k == k
}

func (m *model) replaceOrInsert(x kv) (kv, bool) {
	i, ok := m.search(x.k)
	if ok {
		old := (*m)[i]
		(*m)[i] = x
		return old, true
	}
	*m = append(*m, kv{})
	copy((*m)[i+1:], (*m)[i:])
	(*m)[i] = x
	return kv{}, false
}

func (m *model) removeAt(i int) kv {
	x := (*m)[i]
	*m = append((*m)[:i], (*m)[i+1:]...)
	return x
}

func (m *model) delete(k int) (kv, bool) {
	i, ok := m.search(k)
	if !ok {
		return kv{}, false
	}
	return m.removeAt(i), true
}

func (m model) clone() model { return append(model(nil), m...) }

// filter returns the items whose keys satisfy keep, ascending or descending.
func (m model) filter(keep func(k int) bool, descending bool) []kv {
	var out []kv
	for _, x := range m {
		if keep(x.k) {
			out = append(out, x)
		}
	}
	if descending {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

// forest is a set of trees with their models. Trees made by Clone share
// nodes and a free list; trees made by newTree share only the free list.
type forest struct {
	trees  []*BTreeG[kv]
	models []model
	free   *FreeListG[kv]
	degree int
	cur    int
	next   int // the next value to store, so that every stored item differs
	ops    int
}

const maxTrees = 4

func newForest(degree, freeListSize int) *forest {
	f := &forest{free: NewFreeListG[kv](freeListSize), degree: degree}
	f.trees = []*BTreeG[kv]{NewWithFreeListG[kv](degree, kvLess, f.free)}
	f.models = []model{nil}
	return f
}

// collect runs an iteration that stops after limit items (0: no limit).
func collect(limit int, iterate func(ItemIteratorG[kv])) []kv {
	var got []kv
	iterate(func(x kv) bool {
		got = append(got, x)
		return limit == 0 || len(got) < limit
	})
	return got
}

func prefix(want []kv, limit int) []kv {
	if limit > 0 && len(want) > limit {
		return want[:limit]
	}
	return want
}

func sameItems(a, b []kv) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// step applies one operation, chosen and parameterized by op, a, b and c, to
// the current tree and its model, and returns a description of it, or an
// error if the tree and the model disagree.
func (f *forest) step(op, a, b, c int) (string, error) {
	t, m := f.trees[f.cur], &f.models[f.cur]
	f.ops++
	k := a
	switch op {
	case 0, 1, 2: // inserts are the most common operation, so that trees grow
		f.next++
		x := kv{k, f.next}
		got, gotOK := t.ReplaceOrInsert(x)
		want, wantOK := m.replaceOrInsert(x)
		if got != want || gotOK != wantOK {
			return "", fmt.Errorf("ReplaceOrInsert(%v) = %v, %v; want %v, %v", x, got, gotOK, want, wantOK)
		}
		return fmt.Sprintf("ReplaceOrInsert(%v)", x), nil
	case 3, 4:
		got, gotOK := t.Delete(kv{k: k})
		want, wantOK := m.delete(k)
		if got != want || gotOK != wantOK {
			return "", fmt.Errorf("Delete(%d) = %v, %v; want %v, %v", k, got, gotOK, want, wantOK)
		}
		return fmt.Sprintf("Delete(%d)", k), nil
	case 5:
		got, gotOK := t.DeleteMin()
		var want kv
		wantOK := len(*m) > 0
		if wantOK {
			want = m.removeAt(0)
		}
		if got != want || gotOK != wantOK {
			return "", fmt.Errorf("DeleteMin() = %v, %v; want %v, %v", got, gotOK, want, wantOK)
		}
		return "DeleteMin()", nil
	case 6:
		got, gotOK := t.DeleteMax()
		var want kv
		wantOK := len(*m) > 0
		if wantOK {
			want = m.removeAt(len(*m) - 1)
		}
		if got != want || gotOK != wantOK {
			return "", fmt.Errorf("DeleteMax() = %v, %v; want %v, %v", got, gotOK, want, wantOK)
		}
		return "DeleteMax()", nil
	case 7:
		got, gotOK := t.Get(kv{k: k})
		i, wantOK := m.search(k)
		var want kv
		if wantOK {
			want = (*m)[i]
		}
		if got != want || gotOK != wantOK {
			return "", fmt.Errorf("Get(%d) = %v, %v; want %v, %v", k, got, gotOK, want, wantOK)
		}
		if has := t.Has(kv{k: k}); has != wantOK {
			return "", fmt.Errorf("Has(%d) = %v; want %v", k, has, wantOK)
		}
		return fmt.Sprintf("Get(%d)", k), nil
	case 8:
		var want, wantMax kv
		ok := len(*m) > 0
		if ok {
			want, wantMax = (*m)[0], (*m)[len(*m)-1]
		}
		if got, gotOK := t.Min(); got != want || gotOK != ok {
			return "", fmt.Errorf("Min() = %v, %v; want %v, %v", got, gotOK, want, ok)
		}
		if got, gotOK := t.Max(); got != wantMax || gotOK != ok {
			return "", fmt.Errorf("Max() = %v, %v; want %v, %v", got, gotOK, wantMax, ok)
		}
		return "Min(), Max()", nil
	case 9: // Clone the current tree into a new slot, or over another one.
		dst := len(f.trees)
		if dst == maxTrees {
			dst = b % maxTrees
			if dst == f.cur {
				dst = (dst + 1) % maxTrees
			}
		} else {
			f.trees = append(f.trees, nil)
			f.models = append(f.models, nil)
		}
		f.trees[dst] = t.Clone()
		f.models[dst] = m.clone()
		f.cur = b % len(f.trees) // carry on with either tree, or another one
		return fmt.Sprintf("Clone() into tree %d, continue with tree %d", dst, f.cur), nil
	case 10:
		f.cur = b % len(f.trees)
		return fmt.Sprintf("switch to tree %d", f.cur), nil
	case 11: // a new, empty tree sharing the free list
		dst := b % maxTrees
		if dst >= len(f.trees) {
			dst = len(f.trees)
			f.trees = append(f.trees, nil)
			f.models = append(f.models, nil)
		}
		f.trees[dst] = NewWithFreeListG[kv](f.degree, kvLess, f.free)
		f.models[dst] = nil
		return fmt.Sprintf("new tree %d", dst), nil
	case 12:
		free := b%2 == 1
		t.Clear(free)
		*m = nil
		return fmt.Sprintf("Clear(%v)", free), nil
	}
	// Iterations: op 13 and above.
	lo, hi, limit := a, b, c%5
	var name string
	var got, want []kv
	switch op % 10 {
	case 0:
		name = fmt.Sprintf("AscendRange(%d, %d)", lo, hi)
		got = collect(limit, func(it ItemIteratorG[kv]) { t.AscendRange(kv{k: lo}, kv{k: hi}, it) })
		want = m.filter(func(k int) bool { return lo <= k && k < hi }, false)
	case 1:
		name = fmt.Sprintf("DescendRange(%d, %d)", hi, lo)
		got = collect(limit, func(it ItemIteratorG[kv]) { t.DescendRange(kv{k: hi}, kv{k: lo}, it) })
		want = m.filter(func(k int) bool { return lo < k && k <= hi }, true)
	case 2:
		name = fmt.Sprintf("AscendLessThan(%d)", lo)
		got = collect(limit, func(it ItemIteratorG[kv]) { t.AscendLessThan(kv{k: lo}, it) })
		want = m.filter(func(k int) bool { return k < lo }, false)
	case 3:
		name = fmt.Sprintf("AscendGreaterOrEqual(%d)", lo)
		got = collect(limit, func(it ItemIteratorG[kv]) { t.AscendGreaterOrEqual(kv{k: lo}, it) })
		want = m.filter(func(k int) bool { return k >= lo }, false)
	case 4:
		name = fmt.Sprintf("DescendLessOrEqual(%d)", lo)
		got = collect(limit, func(it ItemIteratorG[kv]) { t.DescendLessOrEqual(kv{k: lo}, it) })
		want = m.filter(func(k int) bool { return k <= lo }, true)
	case 5:
		name = fmt.Sprintf("DescendGreaterThan(%d)", lo)
		got = collect(limit, func(it ItemIteratorG[kv]) { t.DescendGreaterThan(kv{k: lo}, it) })
		want = m.filter(func(k int) bool { return k > lo }, true)
	case 6, 7:
		name = "Ascend()"
		got = collect(limit, t.Ascend)
		want = m.filter(func(int) bool { return true }, false)
	default:
		name = "Descend()"
		got = collect(limit, t.Descend)
		want = m.filter(func(int) bool { return true }, true)
	}
	if limit > 0 {
		name += fmt.Sprintf(", stopping after %d", limit)
	}
	if want = prefix(want, limit); !sameItems(got, want) {
		return "", fmt.Errorf("%s = %v; want %v", name, got, want)
	}
	return name, nil
}

// checkForest checks every tree against its model, and the invariants:
//   - keys strictly increase in order, and every node's keys lie within
//     the bounds its ancestors give it;
//   - every leaf is at the same depth; an internal node has one more child
//     than items; every node but the root holds between degree-1 and
//     2*degree-1 items, and the root holds at least one;
//   - Len is the number of items;
//   - copy-on-write: a node owned by a tree (its cow is the tree's) is
//     reachable from that tree alone, since the tree modifies it in place;
//   - no node in the free list is reachable from any tree, since the next
//     newNode hands it out for reuse.
func (f *forest) checkForest() error {
	owner := map[*node[kv]]int{}
	reach := map[*node[kv]][]int{}
	for i, t := range f.trees {
		if err := checkTree(t, f.models[i], i, owner, reach); err != nil {
			return fmt.Errorf("tree %d: %w", i, err)
		}
	}
	for n, ts := range reach {
		if o, ok := owner[n]; ok && len(ts) > 1 {
			return fmt.Errorf("node %p is owned by tree %d but reachable from trees %v", n, o, ts)
		}
	}
	f.free.mu.Lock()
	defer f.free.mu.Unlock()
	for _, n := range f.free.freelist {
		if ts, ok := reach[n]; ok {
			return fmt.Errorf("node %p is in the free list and reachable from trees %v", n, ts)
		}
		if len(n.items) != 0 || len(n.children) != 0 || n.cow != nil {
			return fmt.Errorf("node %p in the free list was not cleared", n)
		}
	}
	return nil
}

func checkTree(t *BTreeG[kv], m model, id int, owner map[*node[kv]]int, reach map[*node[kv]][]int) error {
	if t.Len() != len(m) {
		return fmt.Errorf("Len() = %d; want %d", t.Len(), len(m))
	}
	all := collect(0, t.Ascend)
	if !sameItems(all, m) {
		return fmt.Errorf("contents %v; want %v", all, m)
	}
	if t.root == nil {
		return nil
	}
	if len(t.root.items) == 0 && (len(t.root.children) > 0 || t.length > 0) {
		return fmt.Errorf("empty root with %d children and length %d", len(t.root.children), t.length)
	}
	leafDepth := -1
	count := 0
	var walk func(n *node[kv], depth int, lo, hi *int) error
	walk = func(n *node[kv], depth int, lo, hi *int) error {
		if ts := reach[n]; len(ts) == 0 || ts[len(ts)-1] != id {
			reach[n] = append(ts, id)
		} else {
			return fmt.Errorf("node %p appears twice in the tree", n)
		}
		if n.cow == t.cow {
			owner[n] = id
		}
		if n != t.root && (len(n.items) < t.minItems() || len(n.items) > t.maxItems()) {
			return fmt.Errorf("node at depth %d holds %d items; want %d to %d", depth, len(n.items), t.minItems(), t.maxItems())
		}
		if len(n.items) > t.maxItems() {
			return fmt.Errorf("root holds %d items; at most %d", len(n.items), t.maxItems())
		}
		for i, x := range n.items {
			if (lo != nil && x.k <= *lo) || (hi != nil && x.k >= *hi) || (i > 0 && n.items[i-1].k >= x.k) {
				return fmt.Errorf("key %d out of order at depth %d", x.k, depth)
			}
		}
		count += len(n.items)
		if len(n.children) == 0 {
			if leafDepth == -1 {
				leafDepth = depth
			} else if depth != leafDepth {
				return fmt.Errorf("leaves at depths %d and %d", leafDepth, depth)
			}
			return nil
		}
		if len(n.children) != len(n.items)+1 {
			return fmt.Errorf("node with %d items has %d children", len(n.items), len(n.children))
		}
		for i, c := range n.children {
			clo, chi := lo, hi
			if i > 0 {
				clo = &n.items[i-1].k
			}
			if i < len(n.items) {
				chi = &n.items[i].k
			}
			if err := walk(c, depth+1, clo, chi); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(t.root, 0, nil, nil); err != nil {
		return err
	}
	if count != t.length {
		return fmt.Errorf("nodes hold %d items; length is %d", count, t.length)
	}
	return nil
}

// opTable maps a byte to an operation for step, weighted so that trees
// grow: inserts are common, and Clear and new trees rare.
var opTable = func() [256]int {
	weights := []struct{ op, n int }{
		{0, 100}, // ReplaceOrInsert
		{3, 40},  // Delete
		{5, 8},   // DeleteMin
		{6, 8},   // DeleteMax
		{7, 20},  // Get, Has
		{8, 6},   // Min, Max
		{9, 8},   // Clone
		{10, 8},  // switch tree
		{11, 1},  // new tree
		{12, 2},  // Clear
	}
	var t [256]int
	i := 0
	for _, w := range weights {
		for j := 0; j < w.n; j++ {
			t[i] = w.op
			i++
		}
	}
	for op := 13; i < 256; i++ { // the rest: iterations, op 13 to 22
		t[i] = op
		if op++; op > 22 {
			op = 13
		}
	}
	return t
}()

// run decodes data into operations, four bytes each, and applies them,
// checking the forest as it goes: after every operation while the trees are
// small, less often as they grow, and at the end. Keys range over keySpace
// values, so that operations collide often.
func run(t testing.TB, data []byte, keySpace int) {
	if len(data) < 2 {
		return
	}
	degree := 2 + int(data[0]%6)
	f := newForest(degree, int(data[1]%8))
	var log []string
	sinceCheck := 0
	for p := 2; p+4 <= len(data); p += 4 {
		a := int(data[p+1]) | int(data[p+2])<<8
		b := int(data[p+2]) | int(data[p+3])<<8
		desc, err := f.step(opTable[data[p]], a%keySpace, b%keySpace, int(data[p+3]))
		if sinceCheck++; err == nil && sinceCheck*64 > f.size() {
			sinceCheck = 0
			err = f.checkForest()
		}
		if err != nil {
			t.Fatalf("degree %d, after %v:\n%v", degree, tail(log, 20), err)
		}
		log = append(log, fmt.Sprintf("tree %d: %s", f.cur, desc))
	}
	if err := f.checkForest(); err != nil {
		t.Fatalf("degree %d, at the end: %v", degree, err)
	}
}

// size is the number of items in all the trees.
func (f *forest) size() int {
	n := 0
	for _, m := range f.models {
		n += len(m)
	}
	return n
}

func tail(s []string, n int) []string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func FuzzModel(f *testing.F) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 16; i++ {
		b := make([]byte, 2+4*(50+rng.Intn(400)))
		rng.Read(b)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) { run(t, data, 64+int(len(data)%3)*448) })
}

// TestModelRandom runs long random sequences with key spaces from a few
// keys, where operations collide constantly, to thousands, where trees grow
// several levels deep.
func TestModelRandom(t *testing.T) {
	runs, ops := 240, 6000
	if testing.Short() {
		runs, ops = 40, 3000
	}
	for seed := 0; seed < runs; seed++ {
		rng := rand.New(rand.NewSource(int64(seed)))
		data := make([]byte, 2+4*ops)
		rng.Read(data)
		keySpace := []int{8, 64, 512, 4096}[seed%4]
		t.Run(fmt.Sprint(seed), func(t *testing.T) { run(t, data, keySpace) })
	}
}

// TestCloneIsolation fills a tree, clones it repeatedly, and empties each
// clone in a different way while checking that the others stay intact.
func TestCloneIsolation(t *testing.T) {
	for degree := 2; degree <= 5; degree++ {
		f := newForest(degree, 32)
		for k := 0; k < 500; k++ {
			if _, err := f.step(0, k, 0, 0); err != nil {
				t.Fatal(err)
			}
		}
		for i := 1; i < maxTrees; i++ {
			f.cur = 0
			if _, err := f.step(9, 0, 0, 0); err != nil { // clone tree 0, keep using it
				t.Fatal(err)
			}
		}
		for i := range f.trees {
			f.cur = i
			for k := 0; k < 500; k++ {
				var op, key int
				switch i {
				case 0:
					op, key = 3, k // delete ascending
				case 1:
					op, key = 3, 499-k // delete descending
				case 2:
					op = 5 + k%2 // alternate DeleteMin and DeleteMax
				default:
					op, key = 0, k // overwrite every item
				}
				if _, err := f.step(op, key, 0, 0); err != nil {
					t.Fatalf("degree %d tree %d step %d: %v", degree, i, k, err)
				}
				if k%50 == 0 {
					if err := f.checkForest(); err != nil {
						t.Fatalf("degree %d tree %d step %d: %v", degree, i, k, err)
					}
				}
			}
		}
		if err := f.checkForest(); err != nil {
			t.Fatalf("degree %d: %v", degree, err)
		}
	}
}

// TestSharedFreeListConcurrentWrites checks the documented guarantee that
// trees sharing a free list, clones included, may be written concurrently.
// Run it with -race.
func TestSharedFreeListConcurrentWrites(t *testing.T) {
	free := NewFreeListG[kv](16)
	base := NewWithFreeListG[kv](3, kvLess, free)
	var want model
	for k := 0; k < 2000; k++ {
		base.ReplaceOrInsert(kv{k, k})
		want.replaceOrInsert(kv{k, k})
	}
	const writers = 8
	trees := make([]*BTreeG[kv], writers)
	for i := range trees {
		if i%2 == 0 {
			trees[i] = base.Clone()
		} else {
			trees[i] = NewWithFreeListG[kv](3, kvLess, free)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range trees {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr := trees[i]
			var m model
			if i%2 == 0 {
				m = want.clone()
			}
			rng := rand.New(rand.NewSource(int64(i)))
			for j := 0; j < 20000; j++ {
				k := rng.Intn(3000)
				switch rng.Intn(4) {
				case 0, 1:
					x := kv{k, i*100000 + j}
					got, gotOK := tr.ReplaceOrInsert(x)
					if w, wOK := m.replaceOrInsert(x); got != w || gotOK != wOK {
						errs[i] = fmt.Errorf("writer %d: ReplaceOrInsert(%v) = %v, %v; want %v, %v", i, x, got, gotOK, w, wOK)
						return
					}
				case 2:
					got, gotOK := tr.Delete(kv{k: k})
					if w, wOK := m.delete(k); got != w || gotOK != wOK {
						errs[i] = fmt.Errorf("writer %d: Delete(%d) = %v, %v; want %v, %v", i, k, got, gotOK, w, wOK)
						return
					}
				default:
					if j%1000 == 0 {
						tr.Clear(true)
						m = nil
					}
				}
			}
			if got := collect(0, tr.Ascend); !sameItems(got, m) {
				errs[i] = fmt.Errorf("writer %d: contents differ from the model", i)
			}
		}(i)
	}
	// Reads of the original are safe while its clones are written.
	for j := 0; j < 200; j++ {
		if got := collect(0, base.Ascend); !sameItems(got, want) {
			t.Fatal("the original changed while its clones were written")
		}
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

// TestBadDegree checks that degrees below 2 are refused.
func TestBadDegree(t *testing.T) {
	for _, d := range []int{-1, 0, 1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewG(%d) did not panic", d)
				}
			}()
			NewG[int](d, Less[int]())
		}()
	}
}
