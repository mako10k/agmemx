// Package search orders memory hits for a natural-language query.
package search

import (
	"math"
	"sort"
)

// Hit is one ranked record.
type Hit struct {
	ID    string
	Kind  string
	Score float64
}

// Cosine is the cosine similarity of two equal-length vectors.
func Cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// Round6 rounds to six decimal places.
func Round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// Order sorts by descending score and ascending ID.
func Order(hits []Hit) {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID < hits[j].ID
	})
}
