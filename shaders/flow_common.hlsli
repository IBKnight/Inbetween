// Shared by flow search: block match cost.
#ifndef FLOW_COMMON_HLSLI
#define FLOW_COMMON_HLSLI
#include "common.hlsli"

Texture2D<float> LumaA : register(t0); // previous frame (level gLevel)
Texture2D<float> LumaB : register(t1); // next frame

#ifndef BLOCK_R
#define BLOCK_R 2 // block (2R+1)^2 = 5x5
#endif

// SYMMETRIC matching: the pixel at moment t=0.5 at point uv came from A(uv - v/2)
// and will go to B(uv + v/2). v is the full A->B displacement in UV. Upside of this
// approach: the flow is already defined on the intermediate frame's grid, with no
// "holes" the way forward warping would leave.
//
// Census cost, not raw luma SAD: each side's samples are compared only against their OWN
// block's center (sign of neighbor-minus-center), then the two sides' sign patterns are
// compared bit by bit. This is invariant to a per-side brightness/contrast difference (a
// blade of grass catching slightly different light in A vs B still SAD-mismatches, but
// keeps the same local sign pattern) and — the bigger win on foliage — it encodes local
// structure (which neighbors are brighter than the center) instead of an averaged
// intensity difference, so two blocks that merely LOOK similarly-toned on average no
// longer look like a cheap match the way plain SAD let them.
float MatchCost(float2 uv, float2 v)
{
    float2 ha = uv - 0.5 * v;
    float2 hb = uv + 0.5 * v;
    float centerA = LumaA.SampleLevel(LinearClamp, ha, 0);
    float centerB = LumaB.SampleLevel(LinearClamp, hb, 0);
    float mismatch = 0;
    [unroll] for (int y = -BLOCK_R; y <= BLOCK_R; ++y)
    [unroll] for (int x = -BLOCK_R; x <= BLOCK_R; ++x)
    {
        if (x == 0 && y == 0) continue;
        float2 o = float2(x, y) * gInvDstSize;
        float a = LumaA.SampleLevel(LinearClamp, ha + o, 0);
        float b = LumaB.SampleLevel(LinearClamp, hb + o, 0);
        mismatch += (a > centerA) != (b > centerB) ? 1.0 : 0.0;
    }
    const float n = float((2 * BLOCK_R + 1) * (2 * BLOCK_R + 1) - 1);
    return mismatch / n;
}

// Penalty for vector v deviating from reference ref (in texels of the current level).
float Smoothness(float2 v, float2 ref)
{
    return gUser.x * length((v - ref) * float2(gDstSize));
}
#endif
