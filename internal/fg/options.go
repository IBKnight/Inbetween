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
		// MatchCost (see flow_common.hlsli) is a mean absolute luma difference over a 5x5
		// block, so it's bounded to [0,1] but well-matched blocks usually land well under
		// 0.05. These are a first estimate, not measured against a real cost histogram —
		// tune via -occ-cost-thr/-occ-cost-k if they're mis-set for a given scene.
		OccCostThreshold: 0.05,
		OccCostSharpness: 20,
		VisMaxPx:         32,
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
