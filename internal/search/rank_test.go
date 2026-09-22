package search

import "testing"

func TestOrderTiesBreakByID(t *testing.T) {
	hits := []Hit{{ID: "b", Score: 1}, {ID: "a", Score: 1}, {ID: "c", Score: 0}}
	Order(hits)
	if hits[0].ID != "a" || hits[1].ID != "b" || hits[2].ID != "c" {
		t.Fatalf("%v", hits)
	}
}

func TestCosine(t *testing.T) {
	if Cosine([]float64{1, 0}, []float64{1, 0}) != 1 {
		t.Fatal("parallel")
	}
	if Cosine([]float64{1, 0}, []float64{0, 1}) != 0 {
		t.Fatal("orthogonal")
	}
	if Cosine([]float64{0, 0}, []float64{1, 0}) != 0 {
		t.Fatal("zero")
	}
}
