package fg

import "testing"

func TestPyramidLevels(t *testing.T) {
	l := PyramidLevels(1920, 1080, 0.5, 24)
	if l[0] != [2]int{960, 540} {
		t.Fatalf("level0 %v", l[0])
	}
	for i := 1; i < len(l); i++ {
		if l[i][0] != (l[i-1][0]+1)/2 {
			t.Fatalf("level %d not halved: %v", i, l)
		}
	}
	last := l[len(l)-1]
	if min(last[0], last[1]) < 24 {
		t.Fatalf("coarsest level too small: %v", last)
	}
	if len(l) != 5 { // 960x540 480x270 240x135 120x68 60x34
		t.Fatalf("levels %v", l)
	}
}

func TestParseAlgo(t *testing.T) {
	for _, n := range []string{"off", "BLEND", "flow"} {
		if _, err := ParseAlgo(n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ParseAlgo("dlss"); err == nil {
		t.Fatal("expected error")
	}
	if AlgoFlow.Next() != AlgoOff {
		t.Fatal("Next must wrap")
	}
}
