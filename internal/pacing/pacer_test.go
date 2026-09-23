package pacing

import (
	"math"
	"testing"
)

const freq = 10_000_000 // как QPC на большинстве машин

func feed(p *Pacer, fps float64, n int) (last int64) {
	iv := float64(freq) / fps
	for i := 1; i <= n; i++ {
		last = int64(float64(i) * iv)
		p.OnSourceFrame(last, last, uint64(i), 0)
	}
	return last
}

func TestSteadyX2(t *testing.T) {
	p := New(freq, 2)
	now := feed(p, 30, 50)
	if fps := p.SourceFPS(); fps < 29.9 || fps > 30.1 {
		t.Fatalf("fps estimate %.2f", fps)
	}
	a, _ := p.Peek()
	if a.Real || a.T != 0.5 || a.Due != now {
		t.Fatalf("first item %+v", a)
	}
	p.Pop()
	b, _ := p.Peek()
	fr := float64(freq)
	half := int64(fr / 30 / 2)
	if !b.Real || b.T != 1 || abs(b.Due-(now+half)) > 2 {
		t.Fatalf("second item %+v (want due %d)", b, now+half)
	}
}

func TestX3Order(t *testing.T) {
	p := New(freq, 3)
	feed(p, 40, 20)
	var ts []float32
	var prevDue int64 = -1
	for p.Pending() > 0 {
		it, _ := p.Peek()
		if it.Due <= prevDue {
			t.Fatal("due times must increase")
		}
		prevDue = it.Due
		ts = append(ts, it.T)
		p.Pop()
	}
	if len(ts) != 3 || ts[2] != 1 {
		t.Fatalf("got %v", ts)
	}
}

func TestStaleDropped(t *testing.T) {
	p := New(freq, 2)
	feed(p, 60, 10)
	if p.Pending() != 2 {
		t.Fatal("expected 2 pending")
	}
	p.OnSourceFrame(11*freq/60, 11*freq/60, 11, 0) // новый кадр пришёл, старые показы не сделаны
	if p.Stale < 2 {
		t.Fatalf("stale=%d", p.Stale)
	}
}

func TestPauseDoesNotBreakEstimate(t *testing.T) {
	p := New(freq, 2)
	last := feed(p, 60, 30)
	p.OnSourceFrame(last+freq, last+freq, 31, 0) // секундная пауза
	if fps := p.SourceFPS(); fps < 59 || fps > 61 {
		t.Fatalf("estimate broken by pause: %.2f", fps)
	}
	if p.Hiccups == 0 {
		t.Fatal("pause must be counted as hiccup")
	}
}

func TestMissedFramesDontSkewEstimate(t *testing.T) {
	p := New(freq, 2)
	last := feed(p, 60, 30)
	iv := float64(freq) / 60
	// 2 real frames' worth of time passed, but only 1 was captured (1 was coalesced). This
	// gap sits well under the 2.5x hiccup threshold, so an un-normalized delta would silently
	// throw off the EMA instead of being caught as an outlier.
	hiccupsBefore := p.Hiccups
	last += int64(2 * iv)
	p.OnSourceFrame(last, last, 31, 1)
	if fps := p.SourceFPS(); fps < 59 || fps > 61 {
		t.Fatalf("missed frames skewed the estimate: %.2f fps", fps)
	}
	if p.Hiccups != hiccupsBefore {
		t.Fatal("a correctly-reported coalesced gap must not count as a hiccup")
	}
}

func TestVBlankAlignment(t *testing.T) {
	p := New(freq, 2)
	feed(p, 30, 5) // warm up the interval estimate
	refresh := float64(freq) / 165
	anchor := int64(math.Round(3 * refresh)) // an arbitrary real vblank, not aligned with source arrivals
	p.SyncVBlank(anchor, refresh)

	arrival := anchor + int64(2.3*refresh) // arrives partway between two vblanks
	p.OnSourceFrame(arrival, arrival, 100, 0)
	a, _ := p.Peek()

	// a.Due must land on (within rounding) a multiple of the refresh interval from anchor.
	k := math.Round(float64(a.Due-anchor) / refresh)
	want := anchor + int64(math.Round(k*refresh))
	if abs(a.Due-want) > 1 {
		t.Fatalf("Due %d not aligned to a vblank (want %d, anchor=%d refresh=%.1f)", a.Due, want, anchor, refresh)
	}
}

func TestVBlankDisabledByDefault(t *testing.T) {
	p := New(freq, 2)
	last := feed(p, 30, 5)
	a, _ := p.Peek()
	if a.Due != last {
		t.Fatalf("without SyncVBlank, base must stay raw arrival time: got %d want %d", a.Due, last)
	}
}

func TestMult1(t *testing.T) {
	p := New(freq, 1)
	feed(p, 60, 5)
	it, ok := p.Peek()
	if !ok || !it.Real || p.Pending() != 1 {
		t.Fatalf("mult=1 must schedule only the real frame: %+v", it)
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
