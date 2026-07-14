package vector

import (
	"math"
	"math/rand"
	"sort"
	"strconv"
	"testing"
)

func TestSetSearch(t *testing.T) {
	c := NewCollection()
	if err := c.Set("a", []float32{1, 0, 0}, "alpha"); err != nil {
		t.Fatal(err)
	}
	c.Set("b", []float32{0, 1, 0}, "beta")
	c.Set("c", []float32{0.9, 0.1, 0}, "gamma")

	res, err := c.Search([]float32{1, 0, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d", len(res))
	}
	if res[0].Item.ID != "a" {
		t.Fatalf("nearest should be a, got %s", res[0].Item.ID)
	}
	if res[0].Score < 0.99 {
		t.Fatalf("identical vector should score ~1, got %f", res[0].Score)
	}
}

func TestDimMismatch(t *testing.T) {
	c := NewCollection()
	c.Set("a", []float32{1, 2, 3}, "")
	if err := c.Set("b", []float32{1, 2}, ""); err != ErrDimMismatch {
		t.Fatalf("want ErrDimMismatch, got %v", err)
	}
}

// bruteCosine recomputes cosine similarity from scratch, independent of the
// collection's cached norms, so it can serve as the ranking oracle.
func bruteCosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// TestSearchMatchesBruteForce proves the min-heap selection and cached-norm
// scoring return exactly the same top-k, in the same order, as a from-scratch
// cosine ranking. The optimisation must cost zero recall.
func TestSearchMatchesBruteForce(t *testing.T) {
	const n, dim, k = 500, 64, 10
	c, query := randVectors(n, dim)

	got, err := c.Search(query, k)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != k {
		t.Fatalf("got %d results, want %d", len(got), k)
	}

	type scored struct {
		id    string
		score float64
	}
	oracle := make([]scored, 0, n)
	for _, it := range c.Items() {
		oracle = append(oracle, scored{it.ID, bruteCosine(query, it.Vec)})
	}
	sort.Slice(oracle, func(i, j int) bool { return oracle[i].score > oracle[j].score })

	for i := 0; i < k; i++ {
		if got[i].Item.ID != oracle[i].id {
			t.Fatalf("rank %d: got %q (%.6f), want %q (%.6f)", i, got[i].Item.ID, got[i].Score, oracle[i].id, oracle[i].score)
		}
		if math.Abs(got[i].Score-oracle[i].score) > 1e-9 {
			t.Fatalf("rank %d score %.12f vs oracle %.12f", i, got[i].Score, oracle[i].score)
		}
		if i > 0 && got[i].Score > got[i-1].Score {
			t.Fatalf("results not in descending order at rank %d", i)
		}
	}
}

// randVectors builds a collection of n random vectors of the given dimension,
// using a fixed seed so the benchmark is reproducible.
func randVectors(n, dim int) (*Collection, []float32) {
	rng := rand.New(rand.NewSource(1))
	c := NewCollection()
	for i := 0; i < n; i++ {
		v := make([]float32, dim)
		for j := range v {
			v[j] = rng.Float32()
		}
		c.Set(strconv.Itoa(i), v, "")
	}
	query := make([]float32, dim)
	for j := range query {
		query[j] = rng.Float32()
	}
	return c, query
}

// BenchmarkVSearch measures a top-10 search over collections of 1k/10k/100k
// vectors at 768 dimensions (a common embedding size).
func BenchmarkVSearch(b *testing.B) {
	const dim = 768
	for _, n := range []int{1000, 10000, 100000} {
		c, query := randVectors(n, dim)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := c.Search(query, 10); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
