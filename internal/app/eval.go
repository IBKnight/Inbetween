//go:build windows

package app

import (
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strings"

	"inbetween/internal/capture"
	"inbetween/internal/config"
	"inbetween/internal/fg"
	"inbetween/internal/gfx"
)

// RunEval is an objective quality check that needs neither a screen nor a game.
//
// The synthetic scene is rendered at moments i/fps (source frames) and at intermediate
// moments (ground truth). Each algorithm generates an intermediate frame, and we compute
// MSE/PSNR against the ground truth. -dump N also writes the images to <dumpdir>/eval/.
func RunEval(cfg *config.Config) error {
	e, err := newEnv(cfg, max(cfg.Adapter, 0))
	if err != nil {
		return err
	}
	defer e.Close()
	W, H := cfg.W, cfg.H
	syn, err := capture.NewSynthetic(e.dev, e.lib, W, H, cfg.FPS)
	if err != nil {
		return err
	}
	defer syn.Close()
	opt := cfg.FGOptions()
	pipe, err := fg.New(e.dev, e.lib, W, H, opt)
	if err != nil {
		return err
	}
	defer pipe.Close()
	const rw = gfx.BindShaderResource | gfx.BindUnorderedAccess
	gt, err := e.dev.NewTexture("truth", W, H, gfx.FormatRGBA8, rw)
	if err != nil {
		return err
	}
	defer gt.Release()
	errTex, err := e.dev.NewTexture("err", W, H, gfx.FormatR32F, rw)
	if err != nil {
		return err
	}
	defer errTex.Release()
	errSh, err := e.lib.Compute("error.hlsl", nil)
	if err != nil {
		return err
	}

	algos := []fg.Algo{fg.AlgoOff, fg.AlgoBlend, fg.AlgoFlow}
	type acc struct {
		sum, min float64
		n        int
		worstI   int
		worstT   float32
	}
	res := make([]acc, len(algos))
	for i := range res {
		res[i].min = math.Inf(1)
	}
	mse := func(tex *gfx.Texture) (float64, error) {
		pipe.Pass(errSh, [2]int{W, H}, gfx.Params{}, []*gfx.Texture{tex, gt}, []*gfx.Texture{errTex})
		v, err := e.dev.ReadFloat32(errTex)
		if err != nil {
			return 0, err
		}
		s := 0.0
		for _, x := range v {
			s += float64(x)
		}
		return s / float64(len(v)), nil
	}
	psnr := func(m float64) float64 {
		if m < 1e-10 {
			return 100
		}
		return 10 * math.Log10(1/m)
	}
	M := max(cfg.Mult, 2)
	dt := 1 / cfg.FPS
	log.Printf("eval: %dx%d, %.1f fps, X%d, %d пар, уровни потока %v", W, H, cfg.FPS, M, cfg.EvalFrames, pipe.Levels())

	for i := 0; i <= cfg.EvalFrames; i++ {
		e.beginGPU()
		syn.RenderAt(float64(i)*dt, syn.Tex)
		pipe.Opt.Algo = opt.Algo
		pipe.Push(capture.Frame{Tex: syn.Tex, Time: int64(i), Seq: uint64(i + 1)})
		if i == 0 {
			e.endGPU()
			continue
		}
		dumpDir := ""
		if i <= cfg.Dump {
			dumpDir = filepath.Join(cfg.DumpDir, "eval", fmt.Sprintf("pair_%03d", i))
			e.dev.SavePNG(pipe.Prev(), filepath.Join(dumpDir, "a_prev.png"))
			e.dev.SavePNG(pipe.Latest(), filepath.Join(dumpDir, "b_next.png"))
		}
		for k := 1; k < M; k++ {
			t := float32(k) / float32(M)
			syn.RenderAt((float64(i-1)+float64(t))*dt, gt)
			for ai, a := range algos {
				pipe.Opt.Algo = a
				out := pipe.Generate(t)
				m, err := mse(out)
				if err != nil {
					return err
				}
				p := psnr(m)
				r := &res[ai]
				r.sum += p
				r.n++
				if p < r.min {
					r.min, r.worstI, r.worstT = p, i, t
				}
				if dumpDir != "" {
					e.dev.SavePNG(out, filepath.Join(dumpDir, fmt.Sprintf("gen_%s_t%.2f.png", a, t)))
				}
			}
			if dumpDir != "" {
				e.dev.SavePNG(gt, filepath.Join(dumpDir, fmt.Sprintf("truth_t%.2f.png", t)))
				e.dev.SavePNG(pipe.FlowVis(), filepath.Join(dumpDir, "flow_vis.png"))
			}
		}
		e.endGPU()
	}

	fmt.Println()
	fmt.Printf("%-6s %10s %10s   %s\n", "algo", "PSNR avg", "PSNR min", "худший случай")
	for ai, a := range algos {
		r := res[ai]
		fmt.Printf("%-6s %10.2f %10.2f   пара %d, t=%.2f\n", a, r.sum/float64(r.n), r.min, r.worstI, r.worstT)
	}
	fmt.Printf("\nGPU, мс (EMA): %s\n", e.gpuSummary())
	fmt.Printf("параметры: flowscale=%g radius=%d refine-radius=%d refine-iters=%d reg=%g zero-bias=%g occ-thr=%g occ-k=%g\n",
		opt.FlowScale, opt.Radius, opt.RefineRadius, opt.RefineIters, opt.Reg, opt.ZeroBias, opt.OccThreshold, opt.OccSharpness)
	var sb strings.Builder
	for ai, a := range algos {
		fmt.Fprintf(&sb, " %s=%.3f", a, res[ai].sum/float64(res[ai].n))
	}
	fmt.Printf("EVAL_RESULT%s\n", sb.String())
	if cfg.Dump > 0 {
		fmt.Printf("картинки: %s\n", filepath.Join(cfg.DumpDir, "eval"))
	}
	return nil
}
