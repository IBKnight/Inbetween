//go:build windows

package gfx

import (
	"fmt"
	"sort"
	"strings"
	"unsafe"

	"inbetween/internal/com"
)

const (
	profSlots     = 8  // how many profiler "frames" are in flight (results are read with a delay)
	profMaxScopes = 32 // measurements per frame
)

type profFrame struct {
	disjoint com.Ptr
	stamps   [profMaxScopes * 2]com.Ptr
	names    [profMaxScopes]string
	n        int
	pending  bool
}

// Profiler times GPU passes using D3D11 timestamp queries.
// Usage: BeginFrame(); ... d.RunCompute(...) (measured automatically) ...; EndFrame().
// Results (ms, smoothed) land in Results; they're read a few frames late, with no GPU stalls.
type Profiler struct {
	d       *Device
	frames  [profSlots]profFrame
	cur     int
	open    bool
	Results map[string]float64
	sums    map[string]float64
}

// NewProfiler creates a profiler and enables it on the device (d.Prof).
func (d *Device) NewProfiler() (*Profiler, error) {
	p := &Profiler{d: d, Results: map[string]float64{}, sums: map[string]float64{}}
	mk := func(kind uint32) (com.Ptr, error) {
		desc := QueryDesc{Query: kind}
		var q unsafe.Pointer
		r := d.Dev.Call(devCreateQuery, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&q)))
		if err := com.Check(r, "CreateQuery"); err != nil {
			return com.Ptr{}, err
		}
		return com.FromRaw(q), nil
	}
	for i := range p.frames {
		f := &p.frames[i]
		var err error
		if f.disjoint, err = mk(QueryTimestampDisjoint); err != nil {
			p.Release()
			return nil, err
		}
		for j := range f.stamps {
			if f.stamps[j], err = mk(QueryTimestamp); err != nil {
				p.Release()
				return nil, err
			}
		}
	}
	d.Prof = p
	return p, nil
}

// BeginFrame starts a new measurement frame.
func (p *Profiler) BeginFrame() {
	if p.open {
		p.EndFrame()
	}
	f := &p.frames[p.cur]
	if f.pending {
		p.collect(f) // the result from profSlots frames ago; dropped if the GPU isn't ready yet
	}
	f.n = 0
	p.d.Ctx.Call(ctxBegin, f.disjoint.U())
	p.open = true
}

// Scope starts a measurement named name; call the returned function when it ends.
// Measurements with the same name in a single frame are summed.
func (p *Profiler) Scope(name string) func() {
	if !p.open {
		return func() {}
	}
	f := &p.frames[p.cur]
	if f.n >= profMaxScopes {
		return func() {}
	}
	i := f.n
	f.n++
	f.names[i] = name
	p.d.Ctx.Call(ctxEnd, f.stamps[2*i].U())
	return func() { p.d.Ctx.Call(ctxEnd, f.stamps[2*i+1].U()) }
}

// EndFrame closes out the measurement frame.
func (p *Profiler) EndFrame() {
	if !p.open {
		return
	}
	f := &p.frames[p.cur]
	p.d.Ctx.Call(ctxEnd, f.disjoint.U())
	f.pending = true
	p.open = false
	p.cur = (p.cur + 1) % profSlots
}

func (p *Profiler) getData(q com.Ptr, dst unsafe.Pointer, size int) bool {
	r := p.d.Ctx.Call(ctxGetData, q.U(), uintptr(dst), uintptr(size), uintptr(AsyncGetDataDoNotFlush))
	return com.HR(r) == com.S_OK
}

func (p *Profiler) collect(f *profFrame) {
	f.pending = false
	var dj QueryDataTimestampDisjoint
	if !p.getData(f.disjoint, unsafe.Pointer(&dj), int(unsafe.Sizeof(dj))) || dj.Disjoint != 0 || dj.Frequency == 0 {
		return
	}
	for k := range p.sums {
		delete(p.sums, k)
	}
	for i := 0; i < f.n; i++ {
		var t0, t1 uint64
		if !p.getData(f.stamps[2*i], unsafe.Pointer(&t0), 8) || !p.getData(f.stamps[2*i+1], unsafe.Pointer(&t1), 8) {
			return
		}
		p.sums[f.names[i]] += float64(t1-t0) * 1000 / float64(dj.Frequency)
	}
	for k, v := range p.sums {
		if old, ok := p.Results[k]; ok {
			p.Results[k] = old + 0.1*(v-old)
		} else {
			p.Results[k] = v
		}
	}
}

// Summary returns a string like "flow_search=0.41 warp=0.12 ..." (ms).
func (p *Profiler) Summary() string {
	keys := make([]string, 0, len(p.Results))
	for k := range p.Results {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s=%.3f ", strings.TrimSuffix(k, ".hlsl"), p.Results[k])
	}
	return strings.TrimSpace(sb.String())
}

func (p *Profiler) Release() {
	for i := range p.frames {
		f := &p.frames[i]
		f.disjoint.Release()
		for j := range f.stamps {
			f.stamps[j].Release()
		}
	}
}
