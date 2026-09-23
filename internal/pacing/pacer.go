// Package pacing schedules the moments at which frames are presented (frame pacing).
//
// The model (like Lossless Scaling's X2/X3 mode): when source frame N arrives, we get a
// pair (N-1, N). We present M frames, spread evenly over the estimated source interval I:
//
//	t = 1/M  at moment arrival                (generated)
//	t = 2/M  at moment arrival + I/M          (generated)
//	...
//	t = 1    at moment arrival + (M-1)·I/M    (real frame N)
//
// The cost is latency ≈ (M-1)/M·I plus processing time. The package has no Windows
// dependency and is covered by tests: the logic can be changed and verified anywhere
// (go test ./internal/pacing).
package pacing

import "math"

// Item is one scheduled present.
type Item struct {
	Due   int64   // present moment, QPC ticks
	T     float32 // position between the pair's frames: (0,1) is generated, 1 is real
	Real  bool    // present the real (latest) frame
	Seq   uint64  // the source frame number N (the right one in the pair)
	Index int     // 1..M
}

// Pacer is the scheduler.
type Pacer struct {
	Freq  int64   // QPC frequency
	Mult  int     // multiplier (1 = no generation)
	Alpha float64 // EMA coefficient for the source interval

	// Offset shifts the ENTIRE schedule by a fraction of the interval (0 = present
	// immediately). >0 trades latency for headroom against uneven frame arrival. Ignored
	// while AdaptiveOffset is true.
	Offset float64

	// AdaptiveOffset, when true, computes the schedule's offset automatically each source
	// frame from recent interval jitter instead of using the fixed Offset field — a source
	// with steadier timing gets less added latency, a jitterier one gets more slack. Gain
	// and cap below are a first estimate, not measured against a real tracestat run.
	AdaptiveOffset bool

	// MinFPS/MaxFPS are sane bounds for the estimated source frequency.
	MinFPS, MaxFPS float64

	interval float64 // ticks, EMA
	jitter   float64 // ticks, EMA of |d - interval| — used only when AdaptiveOffset is true
	lastSrc  int64
	queue    []Item

	// vblank alignment (optional): a known-real vblank instant and the display's refresh
	// interval, from SwapChain.Stats().SyncQPCTime. When set, the schedule's base snaps to
	// the nearest actual vblank instead of raw wall-clock arrival time — otherwise vsync
	// silently rounds every present to its own nearest vblank anyway, and an unaligned base
	// just means that rounding error is uncontrolled instead of zeroed out up front.
	// Zero interval (the default, or -vsync=false where Stats() doesn't advance) disables it.
	vblankAnchor   int64
	vblankInterval float64

	// Stale counts scheduled presents dropped because a new frame arrived first.
	Stale int
	// Hiccups counts source intervals discarded as outliers (a stall/hitch).
	Hiccups int
}

const (
	adaptiveOffsetGain = 2.0 // jitter, as a fraction of the interval, multiplied by this becomes Offset
	maxAdaptiveOffset  = 0.4 // cap, so one bad reading can't blow up latency
)

func New(freq int64, mult int) *Pacer {
	if mult < 1 {
		mult = 1
	}
	return &Pacer{Freq: freq, Mult: mult, Alpha: 0.1, MinFPS: 5, MaxFPS: 1000}
}

// Interval is the current estimate of the source interval in ticks (0 = not known yet).
func (p *Pacer) Interval() float64 { return p.interval }

// SyncVBlank feeds a real vblank timestamp (SwapChain.Stats().SyncQPCTime) and the
// display's refresh interval in ticks, derived from two consecutive readings' delta
// (QPC delta / SyncRefreshCount delta). A non-positive interval disables alignment.
func (p *Pacer) SyncVBlank(qpc int64, interval float64) {
	p.vblankAnchor, p.vblankInterval = qpc, interval
}

// alignToVBlank snaps t to the nearest instant of the form vblankAnchor + k*vblankInterval.
// A no-op (returns t unchanged) until SyncVBlank has been called with a positive interval.
func (p *Pacer) alignToVBlank(t int64) int64 {
	if p.vblankInterval <= 0 {
		return t
	}
	k := math.Round(float64(t-p.vblankAnchor) / p.vblankInterval)
	return p.vblankAnchor + int64(math.Round(k*p.vblankInterval))
}

func (p *Pacer) SourceFPS() float64 {
	if p.interval <= 0 {
		return 0
	}
	return float64(p.Freq) / p.interval
}

// OnSourceFrame is called when a new source frame arrives.
// srcTime is the frame's timestamp at the source (DXGI LastPresentTime), now is when we
// received it, missed is how many source frames were coalesced into this one (capture.Frame.Missed).
func (p *Pacer) OnSourceFrame(srcTime, now int64, seq uint64, missed int) {
	if p.lastSrc != 0 {
		// srcTime-lastSrc spans missed+1 real frames when some were coalesced; divide back
		// down to a per-frame interval, or a coalesced gap reads as one huge (and wrong) one.
		d := float64(srcTime-p.lastSrc) / float64(missed+1)
		lo, hi := float64(p.Freq)/p.MaxFPS, float64(p.Freq)/p.MinFPS
		switch {
		case d < lo || d > hi:
			p.Hiccups++
		case p.interval == 0:
			p.interval = d
		case d > 2.5*p.interval:
			p.Hiccups++ // stall/hitch: don't let it pollute the estimate
		default:
			p.jitter += p.Alpha * (math.Abs(d-p.interval) - p.jitter)
			p.interval += p.Alpha * (d - p.interval)
		}
	}
	p.lastSrc = srcTime

	p.Stale += len(p.queue)
	p.queue = p.queue[:0]

	if p.Mult <= 1 || p.interval == 0 {
		p.queue = append(p.queue, Item{Due: now, T: 1, Real: true, Seq: seq, Index: 1})
		return
	}
	step := p.interval / float64(p.Mult)
	offset := p.Offset
	if p.AdaptiveOffset {
		offset = min(p.jitter/p.interval*adaptiveOffsetGain, maxAdaptiveOffset)
	}
	base := p.alignToVBlank(now + int64(offset*p.interval))
	for k := 1; k <= p.Mult; k++ {
		p.queue = append(p.queue, Item{
			Due:   base + int64(float64(k-1)*step),
			T:     float32(k) / float32(p.Mult),
			Real:  k == p.Mult,
			Seq:   seq,
			Index: k,
		})
	}
}

// Peek returns the nearest scheduled present.
func (p *Pacer) Peek() (Item, bool) {
	if len(p.queue) == 0 {
		return Item{}, false
	}
	return p.queue[0], true
}

// Pop removes the nearest scheduled present (after Present).
func (p *Pacer) Pop() {
	if len(p.queue) > 0 {
		p.queue = p.queue[1:]
	}
}

func (p *Pacer) Pending() int { return len(p.queue) }

// Reset clears the queue and the interval estimate.
func (p *Pacer) Reset() {
	p.queue = p.queue[:0]
	p.interval = 0
	p.jitter = 0
	p.lastSrc = 0
}
