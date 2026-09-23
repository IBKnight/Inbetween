// Package fg is the frame generation itself: frame history, luma pyramids, optical flow
// (coarse-to-fine block matching), and intermediate-frame synthesis.
package fg

import (
	"fmt"
	"strings"
)

// Algo is the algorithm used to generate an intermediate frame.
type Algo int

const (
	AlgoOff   Algo = iota // no generation (the intermediate frame repeats the previous one)
	AlgoBlend             // linear blend of the two frames (ghosting, but no "jelly" warping)
	AlgoFlow              // optical flow + warp (the main algorithm)
)

var algoNames = [...]string{"off", "blend", "flow"}

func (a Algo) String() string {
	if int(a) < len(algoNames) {
		return algoNames[a]
	}
	return fmt.Sprintf("algo(%d)", int(a))
}

// ParseAlgo parses an algorithm name.
func ParseAlgo(s string) (Algo, error) {
	for i, n := range algoNames {
		if strings.EqualFold(s, n) {
			return Algo(i), nil
		}
	}
	return 0, fmt.Errorf("unknown algorithm %q (available: %s)", s, strings.Join(algoNames[:], ", "))
}

// Next cycles to the next algorithm (hotkey).
func (a Algo) Next() Algo { return (a + 1) % Algo(len(algoNames)) }

// Options are the generation parameters. Everything that affects quality lives here.
type Options struct {
	Algo Algo

	FlowScale    float64 // resolution of the finest flow level relative to the frame (0.25..1)
	MinLevelSize int     // the pyramid is built while a level's shorter side is >= this value
	Radius       int     // full-search radius at the coarsest level (texels)
	RefineRadius int     // search radius around the prediction at the other levels
	RefineIters  int     // neighbor-vector propagation iterations per level

	Reg          float32 // penalty for deviating from the prediction (per texel) — flow smoothness
	ZeroBias     float32 // bonus for the zero vector — stability for static content and HUDs
	OccThreshold float32 // warp: color-mismatch threshold above which we assume occlusion
	OccSharpness float32 // warp: how sharply we fall back to the nearest-in-time frame (0 = off)

	// warp: a second, independent occlusion signal based on the match's own residual cost
	// (flow_search/flow_refine's cost, already computed, carried in Flow.z) rather than
	// color. Catches structurally-wrong matches that OccThreshold misses because the two
	// warped samples happen to be similarly colored (e.g. dark cloth over dark rock).
	OccCostThreshold float32
	OccCostSharpness float32

	// Edge-aware bilateral smoothing of the finest-level flow field (flow_smooth.hlsl):
	// higher values fall off faster with luma difference, i.e. respect edges more strictly
	// and smooth flat regions less. 0 would make every neighbor's weight 1 regardless of
	// luma (a plain box blur, bleeding motion across edges) — always left above 0 in practice.
	EdgeSmoothSharpness float32

	// Static/HUD mask: content whose luma stays within StaticDiffThreshold for
	// StaticMaxCount consecutive real-frame pairs is forced to zero flow in warp.hlsl,
	// regardless of what the search found there — catches HUD/crosshair elements getting
	// warped by smoothing bleed from nearby motion.
	StaticDiffThreshold float32
	StaticMaxCount      float32

	VisMaxPx float32 // flow_vis: vector length (px) that reaches full saturation
}

// DefaultOptions is a sensible starting point for 1080p/1440p.
func DefaultOptions() Options {
	return Options{
		Algo: AlgoFlow,
		// flow_search/flow_refine cost is dominated by the finest pyramid level's pixel
		// count; keep this low enough to stay near the ~2-3ms GPU budget (see CLAUDE.md).
		FlowScale:    0.3,
		MinLevelSize: 24,
		Radius:       4,
		RefineRadius: 1,
		RefineIters:  1,
		Reg:          0.002,
		ZeroBias:     0.002,
		OccThreshold: 0.25,
		OccSharpness: 4,
		// MatchCost (see flow_common.hlsli) is a census Hamming distance over a 5x5 block:
		// the fraction of 24 neighbor-vs-center sign comparisons that disagree between A and
		// B, so 0 = identical local structure, ~0.5 = no better than a random block. A good
		// match still isn't exactly 0 (sampling noise flips bits near a neighbor≈center
		// threshold crossing), so the "occluded" threshold sits well above 0 but well below
		// the ~0.5 random floor. First estimate, not measured against a real cost histogram —
		// tune via -occ-cost-thr/-occ-cost-k if mis-set for a given scene.
		OccCostThreshold: 0.15,
		OccCostSharpness: 8,
		// Luma in [0,1]; ~120 keeps weight high (>0.9) for same-surface shading noise
		// (diff up to ~0.03) while cutting sharply past a real edge (diff ~0.1+, weight
		// <0.3). A first estimate, not measured — tune via -edge-smooth-k.
		EdgeSmoothSharpness: 120,
		// Luma is captured straight from the swapchain, so genuinely static content (HUD)
		// should match near-exactly frame to frame; a small threshold just absorbs
		// compression/dithering noise. ~0.25s at 60fps before fully committing to zero
		// flow — long enough that a brief camera pause during real motion doesn't trigger it.
		StaticDiffThreshold: 0.01,
		StaticMaxCount:      15,
		VisMaxPx:            32,
	}
}

// PyramidLevels returns the flow pyramid's level sizes: [0] is the finest.
func PyramidLevels(w, h int, scale float64, minSize int) [][2]int {
	if scale <= 0 || scale > 1 {
		scale = 1
	}
	if minSize < 4 {
		minSize = 4
	}
	lw, lh := max(1, int(float64(w)*scale+0.5)), max(1, int(float64(h)*scale+0.5))
	levels := [][2]int{{lw, lh}}
	for len(levels) < 10 {
		nw, nh := (lw+1)/2, (lh+1)/2
		if min(nw, nh) < minSize {
			break
		}
		lw, lh = nw, nh
		levels = append(levels, [2]int{lw, lh})
	}
	return levels
}
