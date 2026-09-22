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
float MatchCost(float2 uv, float2 v)
{
    float2 ha = uv - 0.5 * v;
    float2 hb = uv + 0.5 * v;
    float sum = 0;
    [unroll] for (int y = -BLOCK_R; y <= BLOCK_R; ++y)
    [unroll] for (int x = -BLOCK_R; x <= BLOCK_R; ++x)
    {
        float2 o = float2(x, y) * gInvDstSize;
        sum += abs(LumaA.SampleLevel(LinearClamp, ha + o, 0) - LumaB.SampleLevel(LinearClamp, hb + o, 0));
    }
    return sum / float((2 * BLOCK_R + 1) * (2 * BLOCK_R + 1));
}

// Penalty for vector v deviating from reference ref (in texels of the current level).
float Smoothness(float2 v, float2 ref)
{
    return gUser.x * length((v - ref) * float2(gDstSize));
}
#endif
