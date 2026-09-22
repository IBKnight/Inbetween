package stats

import (
	"math"
	"testing"
)

func TestRolling(t *testing.T) {
	var r Rolling
	for _, v := range []float64{1, 2, 3, 4} {
		r.Add(v)
	}
	if r.Mean() != 2.5 || r.Min != 1 || r.Max != 4 {
		t.Fatalf("%+v", r)
	}
	if math.Abs(r.Std()-1.118) > 0.01 {
		t.Fatalf("std %.3f", r.Std())
	}
}

func TestPercentile(t *testing.T) {
	v := []float64{5, 1, 4, 2, 3}
	if Percentile(v, 50) != 3 || Percentile(v, 0) != 1 || Percentile(v, 100) != 5 {
		t.Fatal("percentile")
	}
}
