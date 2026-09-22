//go:build windows

package fg

import (
	"fmt"

	"inbetween/internal/capture"
	"inbetween/internal/com"
	"inbetween/internal/gfx"
)

const ringSize = 3 // prev, latest, + one spare

type slot struct {
	color *gfx.Texture   // RGBA8 frame at working size
	luma  []*gfx.Texture // R32F luma pyramid, [0] is the finest level
	time  int64
	seq   uint64
}

type shaders struct {
	imp, luma, down, search, refine, smooth, staticMask, blend, warp, vis *gfx.Shader
}

// Pipeline holds all of frame generation's GPU state. One instance per frame size.
//
// Data flow:
//
//	Push(frame): import (crop/convert to RGBA8) -> luma pyramid -> [flow, if Algo=flow]
//	Generate(t): blend or warp(prev, latest, flow, t) -> out
type Pipeline struct {
	d         *gfx.Device
	W, H      int
	Opt       Options
	DebugView bool // Generate returns the flow visualization instead of a frame

	ring   [ringSize]slot
	head   int
	count  int
	pushes uint64

	levels     [][2]int
	flow       []*gfx.Texture // RGBA16F: xy is the A->B vector in UV space, z is the match cost
	flowTmp    []*gfx.Texture
	staticHist *gfx.Texture // R16F: consecutive unchanged-frame count, finest level's grid
	flowFor    uint64       // the pushes value the flow is valid for

	out, vis      *gfx.Texture
	cb            *gfx.ConstantBuffer
	linear, point com.Ptr
	sh            shaders
}

func New(d *gfx.Device, lib *gfx.ShaderLib, w, h int, opt Options) (*Pipeline, error) {
	p := &Pipeline{d: d, W: w, H: h, Opt: opt}
	if err := p.init(lib); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func (p *Pipeline) init(lib *gfx.ShaderLib) error {
	var err error
	load := func(dst **gfx.Shader, file string) {
		if err == nil {
			*dst, err = lib.Compute(file, nil)
		}
	}
	load(&p.sh.imp, "import.hlsl")
	load(&p.sh.luma, "luma.hlsl")
	load(&p.sh.down, "downsample.hlsl")
	load(&p.sh.search, "flow_search.hlsl")
	load(&p.sh.refine, "flow_refine.hlsl")
	load(&p.sh.smooth, "flow_smooth.hlsl")
	load(&p.sh.staticMask, "static_mask.hlsl")
	load(&p.sh.blend, "blend.hlsl")
	load(&p.sh.warp, "warp.hlsl")
	load(&p.sh.vis, "flow_vis.hlsl")
	if err != nil {
		return err
	}
	if p.cb, err = p.d.NewConstantBuffer(112); err != nil {
		return err
	}
	if p.linear, err = p.d.NewSampler(gfx.FilterLinear, gfx.AddressClamp); err != nil {
		return err
	}
	if p.point, err = p.d.NewSampler(gfx.FilterPoint, gfx.AddressClamp); err != nil {
		return err
	}

	const rw = gfx.BindShaderResource | gfx.BindUnorderedAccess
	tex := func(name string, w, h int, f uint32) *gfx.Texture {
		if err != nil {
			return nil
		}
		var t *gfx.Texture
		t, err = p.d.NewTexture(name, w, h, f, rw)
		return t
	}
	p.levels = PyramidLevels(p.W, p.H, p.Opt.FlowScale, p.Opt.MinLevelSize)
	for i := range p.ring {
		s := &p.ring[i]
		s.color = tex(fmt.Sprintf("color%d", i), p.W, p.H, gfx.FormatRGBA8)
		for l, sz := range p.levels {
			s.luma = append(s.luma, tex(fmt.Sprintf("luma%d_%d", i, l), sz[0], sz[1], gfx.FormatR32F))
		}
	}
	for l, sz := range p.levels {
		p.flow = append(p.flow, tex(fmt.Sprintf("flow%d", l), sz[0], sz[1], gfx.FormatRGBA16F))
		p.flowTmp = append(p.flowTmp, tex(fmt.Sprintf("flowtmp%d", l), sz[0], sz[1], gfx.FormatRGBA16F))
	}
	p.staticHist = tex("statichist", p.levels[0][0], p.levels[0][1], gfx.FormatR16F)
	p.out = tex("out", p.W, p.H, gfx.FormatRGBA8)
	p.vis = tex("flowvis", p.W, p.H, gfx.FormatRGBA8)
	return err
}

func (p *Pipeline) Levels() [][2]int { return p.levels }

// Samplers returns the linear/point samplers (s0/s1) for external passes.
func (p *Pipeline) Samplers() []com.Ptr { return []com.Ptr{p.linear, p.point} }

// Pass runs a compute pass with the shared constant buffer (b0) and samplers (s0 linear,
// s1 point). DstSize/InvDstSize in prm are filled in from size. Also used from outside
// the package (eval, dumps).
func (p *Pipeline) Pass(sh *gfx.Shader, size [2]int, prm gfx.Params, srv, uav []*gfx.Texture) {
	prm.DstSize = [2]uint32{uint32(size[0]), uint32(size[1])}
	prm.InvDstSize = [2]float32{1 / float32(size[0]), 1 / float32(size[1])}
	if err := gfx.SetConstants(p.d, p.cb, &prm); err != nil {
		return
	}
	p.d.RunCompute(sh, size[0], size[1], gfx.Bindings{
		CB: []*gfx.ConstantBuffer{p.cb}, SRV: srv, UAV: uav, Samplers: []com.Ptr{p.linear, p.point},
	})
}

func (p *Pipeline) tuning() gfx.Params {
	return gfx.Params{
		User:  [4]float32{p.Opt.Reg, p.Opt.OccThreshold, p.Opt.OccSharpness, p.Opt.ZeroBias},
		User2: [4]float32{p.Opt.VisMaxPx, p.Opt.OccCostThreshold, p.Opt.OccCostSharpness, p.Opt.EdgeSmoothSharpness},
		User3: [4]float32{p.Opt.StaticDiffThreshold, p.Opt.StaticMaxCount, 0, 0},
	}
}

func (p *Pipeline) full() [2]int { return [2]int{p.W, p.H} }

func (p *Pipeline) latestSlot() *slot { return &p.ring[p.head] }
func (p *Pipeline) prevSlot() *slot   { return &p.ring[(p.head+ringSize-1)%ringSize] }

// Ready reports whether there's a frame pair available to generate from.
func (p *Pipeline) Ready() bool { return p.count >= 2 }

// Latest returns the most recent frame (RGBA8).
func (p *Pipeline) Latest() *gfx.Texture { return p.latestSlot().color }

// Prev returns the frame before Latest (RGBA8).
func (p *Pipeline) Prev() *gfx.Texture { return p.prevSlot().color }

func (p *Pipeline) LatestSeq() uint64 { return p.latestSlot().seq }

// Push accepts a new source frame: copies it into the history and precomputes everything
// that only needs to happen once per pair (pyramid, flow).
func (p *Pipeline) Push(f capture.Frame) {
	p.head = (p.head + 1) % ringSize
	s := p.latestSlot()
	prm := gfx.Params{SrcOffset: [2]uint32{uint32(f.Offset[0]), uint32(f.Offset[1])}, SrcSize: f.Tex.Size()}
	p.Pass(p.sh.imp, p.full(), prm, []*gfx.Texture{f.Tex}, []*gfx.Texture{s.color})
	p.buildPyramid(s)
	s.time, s.seq = f.Time, f.Seq
	p.count++
	p.pushes++
	if p.Ready() && p.Opt.Algo == AlgoFlow {
		p.EstimateFlow()
	}
}

func (p *Pipeline) buildPyramid(s *slot) {
	p.Pass(p.sh.luma, p.levels[0], gfx.Params{}, []*gfx.Texture{s.color}, []*gfx.Texture{s.luma[0]})
	for l := 1; l < len(p.levels); l++ {
		p.Pass(p.sh.down, p.levels[l], gfx.Params{Level: uint32(l)}, []*gfx.Texture{s.luma[l-1]}, []*gfx.Texture{s.luma[l]})
	}
}

// EstimateFlow computes the flow between Prev and Latest, coarse level to fine, then
// bilaterally smooths the finest level. The result is p.flow[0] (the A->B vector in UV
// units, estimated at the midpoint, t=0.5).
func (p *Pipeline) EstimateFlow() {
	if !p.Ready() {
		return
	}
	a, b := p.prevSlot(), p.latestSlot()
	top := len(p.levels) - 1
	for l := top; l >= 0; l-- {
		prm := p.tuning()
		prm.Level = uint32(l)
		var prior *gfx.Texture
		if l < top {
			prior = p.flow[l+1]
			prm.Flags = gfx.FlagHasPrior
			prm.Radius = uint32(p.Opt.RefineRadius)
		} else {
			prm.Radius = uint32(p.Opt.Radius)
		}
		p.Pass(p.sh.search, p.levels[l], prm, []*gfx.Texture{a.luma[l], b.luma[l], prior}, []*gfx.Texture{p.flow[l]})
		for i := 0; i < p.Opt.RefineIters; i++ {
			p.Pass(p.sh.refine, p.levels[l], prm, []*gfx.Texture{a.luma[l], b.luma[l], p.flow[l]}, []*gfx.Texture{p.flowTmp[l]})
			p.flow[l], p.flowTmp[l] = p.flowTmp[l], p.flow[l]
		}
	}
	// Edge-aware smoothing, finest level only — reuses flowTmp[0] as the ping-pong target,
	// same pattern as the refine loop above, no new resources.
	p.Pass(p.sh.smooth, p.levels[0], p.tuning(), []*gfx.Texture{p.flow[0], b.luma[0]}, []*gfx.Texture{p.flowTmp[0]})
	p.flow[0], p.flowTmp[0] = p.flowTmp[0], p.flow[0]
	// Static-history counter: independent of the search above, read-modify-write in place.
	p.Pass(p.sh.staticMask, p.levels[0], p.tuning(), []*gfx.Texture{a.luma[0], b.luma[0]}, []*gfx.Texture{p.staticHist})
	p.flowFor = p.pushes
}

// Generate builds a frame at moment t ∈ (0,1) between Prev and Latest.
// The returned texture is valid until the next Generate/Push.
func (p *Pipeline) Generate(t float32) *gfx.Texture {
	if !p.Ready() {
		return p.Latest()
	}
	a, b := p.prevSlot(), p.latestSlot()
	prm := p.tuning()
	prm.T = t
	switch p.Opt.Algo {
	case AlgoOff:
		return a.color
	case AlgoBlend:
		p.Pass(p.sh.blend, p.full(), prm, []*gfx.Texture{a.color, b.color}, []*gfx.Texture{p.out})
	case AlgoFlow:
		if p.flowFor != p.pushes {
			p.EstimateFlow()
		}
		if p.DebugView {
			return p.FlowVis()
		}
		p.Pass(p.sh.warp, p.full(), prm, []*gfx.Texture{a.color, b.color, p.flow[0], p.staticHist}, []*gfx.Texture{p.out})
	}
	return p.out
}

// FlowVis renders the current flow as color (direction -> hue, length -> saturation).
func (p *Pipeline) FlowVis() *gfx.Texture {
	if p.flowFor != p.pushes {
		p.EstimateFlow()
	}
	p.Pass(p.sh.vis, p.full(), p.tuning(), []*gfx.Texture{p.flow[0]}, []*gfx.Texture{p.vis})
	return p.vis
}

// Dump saves the prev/next/generated frame and the flow visualization into dir.
func (p *Pipeline) Dump(dir string, t float32) error {
	if !p.Ready() {
		return fmt.Errorf("нет пары кадров")
	}
	save := func(tex *gfx.Texture, name string) error {
		return p.d.SavePNG(tex, fmt.Sprintf("%s/%s.png", dir, name))
	}
	if err := save(p.Prev(), "a_prev"); err != nil {
		return err
	}
	if err := save(p.Latest(), "b_next"); err != nil {
		return err
	}
	if err := save(p.Generate(t), fmt.Sprintf("gen_%s_t%.2f", p.Opt.Algo, t)); err != nil {
		return err
	}
	return save(p.FlowVis(), "flow_vis")
}

func (p *Pipeline) Close() {
	for i := range p.ring {
		p.ring[i].color.Release()
		for _, t := range p.ring[i].luma {
			t.Release()
		}
	}
	for _, t := range p.flow {
		t.Release()
	}
	for _, t := range p.flowTmp {
		t.Release()
	}
	p.staticHist.Release()
	p.out.Release()
	p.vis.Release()
	p.cb.Release()
	p.linear.Release()
	p.point.Release()
}
