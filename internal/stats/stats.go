// Package stats provides live-mode metrics and a present CSV trace (for cmd/tracestat).
package stats

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"sort"
)

// Rolling is simple running statistics over a series of values.
type Rolling struct {
	N          int
	Sum, SumSq float64
	Min, Max   float64
}

func (r *Rolling) Add(v float64) {
	if r.N == 0 || v < r.Min {
		r.Min = v
	}
	if r.N == 0 || v > r.Max {
		r.Max = v
	}
	r.N++
	r.Sum += v
	r.SumSq += v * v
}

func (r *Rolling) Mean() float64 {
	if r.N == 0 {
		return 0
	}
	return r.Sum / float64(r.N)
}

func (r *Rolling) Std() float64 {
	if r.N < 2 {
		return 0
	}
	m := r.Mean()
	return math.Sqrt(math.Max(0, r.SumSq/float64(r.N)-m*m))
}

func (r *Rolling) Reset() { *r = Rolling{} }

func (r *Rolling) String() string {
	if r.N == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f±%.2f[%.2f..%.2f]", r.Mean(), r.Std(), r.Min, r.Max)
}

// Percentile returns the p-th percentile (0..100), operating on a copy of v.
func Percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	i := int(math.Round(p / 100 * float64(len(s)-1)))
	return s[max(0, min(len(s)-1, i))]
}

// Live holds counters for the current logging window (usually 1 s).
type Live struct {
	SrcFrames, SrcMissed, SrcSkipped int
	Real, Generated                  int
	PresentInterval                  Rolling // ms between consecutive Present calls
	Lateness                         Rolling // ms: how much later than scheduled a Present was
	Latency                          Rolling // ms: from a source frame's arrival to its real frame's Present
	GenCPU                           Rolling // ms of CPU time spent preparing a frame (recording commands)
	lastPresent                      int64
}

func (s *Live) OnPresent(nowMs float64, real bool) {
	if real {
		s.Real++
	} else {
		s.Generated++
	}
	if s.lastPresent != 0 {
		s.PresentInterval.Add(nowMs - float64(s.lastPresent)/1e6)
	}
	s.lastPresent = int64(nowMs * 1e6)
}

func (s *Live) Line(seconds float64) string {
	return fmt.Sprintf("src %.1f fps (missed %d, skipped %d) | out %.1f fps (real %d gen %d) | interval %s ms | late %s ms | latency %s ms | cpu %s ms",
		float64(s.SrcFrames)/seconds, s.SrcMissed, s.SrcSkipped,
		float64(s.Real+s.Generated)/seconds, s.Real, s.Generated,
		s.PresentInterval.String(), s.Lateness.String(), s.Latency.String(), s.GenCPU.String())
}

// ResetWindow clears the window's counters (lastPresent is preserved).
func (s *Live) ResetWindow() {
	lp := s.lastPresent
	*s = Live{}
	s.lastPresent = lp
}

// Trace is a CSV file with one row per Present.
type Trace struct {
	f *os.File
	w *bufio.Writer
}

// TraceHeader lists the CSV columns.
const TraceHeader = "t_ms,kind,t,seq,due_ms,late_ms,src_interval_ms,cpu_ms"

func NewTrace(path string) (*Trace, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriterSize(f, 1<<16)
	fmt.Fprintln(w, TraceHeader)
	return &Trace{f: f, w: w}, nil
}

func (t *Trace) Row(tMs float64, real bool, tt float32, seq uint64, dueMs, lateMs, srcIntervalMs, cpuMs float64) {
	if t == nil {
		return
	}
	kind := "gen"
	if real {
		kind = "real"
	}
	fmt.Fprintf(t.w, "%.3f,%s,%.3f,%d,%.3f,%.3f,%.3f,%.3f\n", tMs, kind, tt, seq, dueMs, lateMs, srcIntervalMs, cpuMs)
}

func (t *Trace) Close() error {
	if t == nil {
		return nil
	}
	t.w.Flush()
	return t.f.Close()
}
