// Package gfx wraps Direct3D 11 + DXGI via hand-written COM calls: device, textures,
// shaders (compiled at runtime via d3dcompiler_47.dll), swapchain, GPU profiler.
package gfx

// Params is the ONE constant buffer (register b0) shared by every shader in the project.
//
// The layout MUST match `cbuffer Params` in shaders/common.hlsli (HLSL packing rules:
// vectors don't cross a 16-byte boundary). Its size is checked by params_test.go.
// Change one, change the other.
type Params struct {
	SrcOffset  [2]uint32  // c0.xy  offset into the source texture (import)
	SrcSize    [2]uint32  // c0.zw  source texture size
	DstSize    [2]uint32  // c1.xy  size of the texture being written (UAV/RT)
	InvDstSize [2]float32 // c1.zw  1/DstSize
	T          float32    // c2.x   interpolation time 0..1
	Time       float32    // c2.y   seconds (synthetic scene)
	Level      uint32     // c2.z   pyramid level
	Radius     uint32     // c2.w   search radius
	Flags      uint32     // c3.x   pass-specific bit flags
	FrameIndex uint32     // c3.y
	_          [2]uint32  // c3.zw  padding
	User       [4]float32 // c4     tuning: x=reg, y=occ_thr, z=occ_k, w=zero_bias
	User2      [4]float32 // c5     free for experiments (flow_vis: x=max vector length in px)
}

// Params.Flags bits (meaning depends on the pass — see the comments in the .hlsl files).
const (
	FlagHasPrior  = 1 << 0 // flow_search: a coarser-level flow is available
	FlagMarker    = 1 << 0 // present: draw the frame marker
	FlagGenerated = 1 << 1 // present: the frame was generated (magenta marker instead of green)
)
