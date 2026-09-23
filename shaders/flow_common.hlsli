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
// Cost is mean luma SAD (continuous — the finest level's subpixel parabola fit needs a
// smooth cost surface to fit against) plus a SMALL weighted census term (sign-of-
// neighbor-vs-center Hamming distance between A's and B's blocks, gUser3.z): a pure
// census replacement was tried and reverted (only 25 possible values across [0,1], vs
// SAD's near-continuous range, broke the parabola fit into fitting quantization noise —
// see CLAUDE.md Stage 3 item 2). Here SAD stays dominant; census only nudges ties toward
// structurally-similar blocks, which is what actually helps on repetitive/self-similar
// texture like grass — without census's coarseness becoming the dominant cost signal.
float MatchCost(float2 uv, float2 v)
{
    float2 ha = uv - 0.5 * v;
    float2 hb = uv + 0.5 * v;
    float centerA = LumaA.SampleLevel(LinearClamp, ha, 0);
    float centerB = LumaB.SampleLevel(LinearClamp, hb, 0);
    float sad = abs(centerA - centerB);
    float mismatch = 0;
    [unroll] for (int y = -BLOCK_R; y <= BLOCK_R; ++y)
    [unroll] for (int x = -BLOCK_R; x <= BLOCK_R; ++x)
    {
        if (x == 0 && y == 0) continue;
        float2 o = float2(x, y) * gInvDstSize;
        float a = LumaA.SampleLevel(LinearClamp, ha + o, 0);
        float b = LumaB.SampleLevel(LinearClamp, hb + o, 0);
        sad += abs(a - b);
        mismatch += (a > centerA) != (b > centerB) ? 1.0 : 0.0;
    }
    const float n = float((2 * BLOCK_R + 1) * (2 * BLOCK_R + 1));
    float sadCost = sad / n;
    float censusCost = mismatch / (n - 1.0);
    return sadCost + gUser3.z * censusCost;
}

// Penalty for vector v deviating from reference ref (in texels of the current level).
float Smoothness(float2 v, float2 ref)
{
    return gUser.x * length((v - ref) * float2(gDstSize));
}
#endif
