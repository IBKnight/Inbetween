// Edge-aware smoothing of the finest-level flow field: a small bilateral filter that
// averages neighboring vectors weighted by how similar their luma is, so it doesn't blend
// motion across a strong edge (object boundary) the way a plain box blur would. Runs once,
// after search+refine, only at the finest level (where character-edge artifacts show).
#include "common.hlsli"

Texture2D<float4>   FlowIn  : register(t0);
Texture2D<float>    LumaB   : register(t1); // reference for edge detection (frame B's luma)
RWTexture2D<float4> FlowOut : register(u0);

#ifndef BILATERAL_R
#define BILATERAL_R 1 // (2R+1)^2 = 3x3 neighborhood
#endif

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    int2 p = int2(id.xy);
    int2 maxP = int2(gDstSize) - 1;

    float4 center = FlowIn.Load(int3(p, 0));
    float lumaCenter = LumaB.Load(int3(p, 0));

    float2 sumV = float2(0, 0);
    float sumW = 0;
    [unroll] for (int y = -BILATERAL_R; y <= BILATERAL_R; ++y)
    [unroll] for (int x = -BILATERAL_R; x <= BILATERAL_R; ++x)
    {
        int2 q = clamp(p + int2(x, y), int2(0, 0), maxP);
        float2 v = FlowIn.Load(int3(q, 0)).xy;
        float l = LumaB.Load(int3(q, 0));
        float dl = l - lumaCenter;
        float w = exp(-dl * dl * gUser2.w);
        sumV += v * w;
        sumW += w;
    }
    float2 smoothed = sumW > 1e-5 ? sumV / sumW : center.xy;
    // Match cost isn't recomputed for the smoothed vector — same simplification as the
    // subpixel refinement in flow_search.hlsl; warp.hlsl's occlusion check only needs it
    // directionally right, not exact.
    FlowOut[id.xy] = float4(smoothed, center.z, 0);
}
