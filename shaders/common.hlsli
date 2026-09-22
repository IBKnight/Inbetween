// Shared declarations for ALL shaders. Included via #include "common.hlsli".
// cbuffer Params MUST match gfx.Params (internal/gfx/params.go), 112 bytes.
#ifndef COMMON_HLSLI
#define COMMON_HLSLI

// The group size comes from Go (gfx.Shader.GroupX/GroupY) via macros.
#ifndef GROUP_X
#define GROUP_X 8
#endif
#ifndef GROUP_Y
#define GROUP_Y 8
#endif

cbuffer Params : register(b0)
{
    uint2  gSrcOffset;   // c0.xy
    uint2  gSrcSize;     // c0.zw
    uint2  gDstSize;     // c1.xy  size of the texture being written
    float2 gInvDstSize;  // c1.zw
    float  gT;           // c2.x   interpolation time 0..1
    float  gTime;        // c2.y   seconds (synthetic scene)
    uint   gLevel;       // c2.z
    uint   gRadius;      // c2.w
    uint   gFlags;       // c3.x
    uint   gFrameIndex;  // c3.y
    uint2  gPad0;        // c3.zw
    float4 gUser;        // c4: x=reg, y=occ_thr, z=occ_k, w=zero_bias
    float4 gUser2;       // c5: x=flow_vis max px, y=occ_cost_thr, z=occ_cost_k, w=edge_smooth_k
    float4 gUser3;       // c6: x=static_diff_thr, y=static_max_count
};

SamplerState LinearClamp : register(s0);
SamplerState PointClamp  : register(s1);

static const uint FLAG_HAS_PRIOR = 1u; // flow_search
static const uint FLAG_MARKER    = 1u; // present
static const uint FLAG_GENERATED = 2u; // present

float Luma(float3 c) { return dot(c, float3(0.2126, 0.7152, 0.0722)); }

// Center of pixel p in the UV space of the texture being written.
float2 PixelUV(uint2 p) { return (float2(p) + 0.5) * gInvDstSize; }

bool OutOfBounds(uint2 p) { return any(p >= gDstSize); }

#endif
