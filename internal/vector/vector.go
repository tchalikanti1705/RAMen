// Package vector implements a flat (brute-force) vector index with cosine
// similarity. It is deliberately simple: V1 trades query speed for zero
// dependencies and predictable behaviour. An HNSW index is a documented future
// upgrade (see PRD §7) that can replace Collection without changing callers.
package vector

import (
	"errors"
	"math"
	"sort"
)

// ErrDimMismatch is returned when a vector's length does not match the
// collection's established dimension.
var ErrDimMismatch = errors.New("vector dimension mismatch")

// Item is one stored vector, its id, and optional metadata (used by the
// semantic cache to stash the cached response and its expiry).
type Item struct {
	ID   string
	Vec  []float32
	Meta string
	norm float64 // Euclidean norm of Vec, cached at insert time; not persisted
}

// Collection is a flat index of vectors that all share one dimension.
type Collection struct {
	Dim   int
	items map[string]*Item // id -> item
}

// NewCollection returns an empty collection. The dimension is fixed by the
// first vector added.
func NewCollection() *Collection {
	return &Collection{items: map[string]*Item{}}
}

// Set inserts or replaces the vector stored under id. The first vector added
// to an empty collection fixes its dimension; later vectors must match.
func (c *Collection) Set(id string, vec []float32, meta string) error {
	if c.Dim == 0 {
		c.Dim = len(vec)
	} else if len(vec) != c.Dim {
		return ErrDimMismatch
	}
	cp := make([]float32, len(vec))
	copy(cp, vec)
	c.items[id] = &Item{ID: id, Vec: cp, Meta: meta, norm: norm(cp)}
	return nil
}

// norm returns the Euclidean norm (magnitude) of v.
func norm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		f := float64(x)
		sum += f * f
	}
	return math.Sqrt(sum)
}

// Del removes id; it reports whether the id existed.
func (c *Collection) Del(id string) bool {
	if _, ok := c.items[id]; ok {
		delete(c.items, id)
		return true
	}
	return false
}

// Len returns the number of stored vectors.
func (c *Collection) Len() int { return len(c.items) }

// Result is one search hit: an item paired with its cosine similarity score in
// the range [-1, 1] (1 == identical direction).
type Result struct {
	Item  *Item
	Score float64
}

// Search returns the top-k items by cosine similarity to query, highest first.
func (c *Collection) Search(query []float32, k int) ([]Result, error) {
	if c.Dim != 0 && len(query) != c.Dim {
		return nil, ErrDimMismatch
	}
	qn := norm(query)
	results := make([]Result, 0, len(c.items))
	for _, it := range c.items {
		results = append(results, Result{Item: it, Score: cosineNorm(query, qn, it)})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if k > 0 && k < len(results) {
		results = results[:k]
	}
	return results, nil
}

// cosineNorm is cosine similarity between query (with precomputed norm qn) and a
// stored item whose norm is already cached. It only does the dot product in the
// loop; the magnitudes are supplied, not recomputed. A zero-magnitude vector on
// either side yields 0, matching the flat cosine helper.
func cosineNorm(query []float32, qn float64, it *Item) float64 {
	if qn == 0 || it.norm == 0 || len(query) != len(it.Vec) {
		return 0
	}
	var dot float64
	for i := range query {
		dot += float64(query[i]) * float64(it.Vec[i])
	}
	return dot / (qn * it.norm)
}

// Items returns all stored items (used for snapshotting). The slice is freshly
// allocated; callers may not mutate the underlying vectors.
func (c *Collection) Items() []*Item {
	out := make([]*Item, 0, len(c.items))
	for _, it := range c.items {
		out = append(out, it)
	}
	return out
}
