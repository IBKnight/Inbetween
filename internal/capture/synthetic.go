//go:build windows

package capture

import (
	"fmt"
	"math/rand"
	"time"

	"inbetween/internal/gfx"
	"inbetween/internal/win"
)

// Synthetic is a procedural scene (shaders/synthetic.hlsl) at a fixed FPS. Its main
// value: the scene can be rendered at ANY point in time, so the ground truth for an
// intermediate frame is known — that's what -mode eval (PSNR) is built on.
type Synthetic struct {
	d       *gfx.Device
	cs      *gfx.Shader
	cb      *gfx.ConstantBuffer
	Tex     *gfx.Texture // RGBA8, the current frame is rendered here
	W, H    int
	FPS     float64
	Jitter  float64 // 0..1: random jitter of frame moments (fraction of the interval) — for testing pacing
	start   int64
	next    int64
	seq     uint64
	rng     *rand.Rand
	sleeper *win.Sleeper
}

func NewSynthetic(d *gfx.Device, lib *gfx.ShaderLib, w, h int, fps float64) (*Synthetic, error) {
	cs, err := lib.Compute("synthetic.hlsl", nil)
	if err != nil {
		return nil, err
	}
	cb, err := d.NewConstantBuffer(112)
	if err != nil {
		return nil, err
	}
	tex, err := d.NewTexture("synthetic", w, h, gfx.FormatRGBA8, gfx.BindShaderResource|gfx.BindUnorderedAccess)
	if err != nil {
		cb.Release()
		return nil, err
	}
	return &Synthetic{d: d, cs: cs, cb: cb, Tex: tex, W: w, H: h, FPS: fps,
		rng: rand.New(rand.NewSource(1)), sleeper: win.NewSleeper()}, nil
}

func (s *Synthetic) Name() string     { return fmt.Sprintf("synthetic %dx%d@%.1f", s.W, s.H, s.FPS) }
func (s *Synthetic) Size() (int, int) { return s.W, s.H }

// RenderAt renders the scene at moment sec (seconds) into dst (RGBA8 with a UAV, size W×H).
func (s *Synthetic) RenderAt(sec float64, dst *gfx.Texture) {
	p := gfx.Params{DstSize: dst.Size(), InvDstSize: [2]float32{1 / float32(dst.W), 1 / float32(dst.H)}, Time: float32(sec)}
	gfx.SetConstants(s.d, s.cb, &p)
	s.d.RunCompute(s.cs, dst.W, dst.H, gfx.Bindings{CB: []*gfx.ConstantBuffer{s.cb}, UAV: []*gfx.Texture{dst}})
}

// Poll produces frames on the FPS schedule (wall-clock time).
func (s *Synthetic) Poll(timeout time.Duration) (Frame, bool, error) {
	freq := win.QPF()
	interval := float64(freq) / s.FPS
	now := win.QPC()
	if s.start == 0 {
		s.start, s.next = now, now
	}
	if now < s.next {
		s.sleeper.SleepUntil(min(s.next, now+win.DurToTicks(timeout)), false)
		if now = win.QPC(); now < s.next {
			return Frame{}, false, nil
		}
	}
	missed := 0
	for now-s.next > int64(interval) { // fell far behind — skip frames
		s.seq++
		s.next = s.scheduled(interval)
		missed++
	}
	sec := float64(s.next-s.start) / float64(freq)
	s.RenderAt(sec, s.Tex)
	f := Frame{Tex: s.Tex, Time: s.next, Seq: s.seq + 1, Missed: missed}
	s.seq++
	s.next = s.scheduled(interval)
	return f, true, nil
}

func (s *Synthetic) scheduled(interval float64) int64 {
	j := 0.0
	if s.Jitter > 0 {
		j = (s.rng.Float64() - 0.5) * s.Jitter * interval
	}
	return s.start + int64(float64(s.seq)*interval+j)
}

func (s *Synthetic) Close() {
	s.Tex.Release()
	s.cb.Release()
	s.sleeper.Close()
}
