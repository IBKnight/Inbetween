// Propagation (as in PatchMatch): try neighboring vectors, keep the lowest-cost one.
// Removes isolated outliers and pulls correct vectors into homogeneous regions.
#include "flow_common.hlsli"

Texture2D<float4>   FlowIn  : register(t2);
RWTexture2D<float4> FlowOut : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float2 uv = PixelUV(id.xy);
    int2 p = int2(id.xy);
    int2 maxP = int2(gDstSize) - 1;

    float2 cur = FlowIn.Load(int3(p, 0)).xy;
    float2 best = cur;
    float bestCost = MatchCost(uv, cur);

    [unroll] for (int y = -1; y <= 1; ++y)
    [unroll] for (int x = -1; x <= 1; ++x)
    {
        if (x == 0 && y == 0) continue;
        int2 q = clamp(p + int2(x, y), int2(0, 0), maxP);
        float2 v = FlowIn.Load(int3(q, 0)).xy;
        float c = MatchCost(uv, v) + Smoothness(v, cur);
        if (c < bestCost) { bestCost = c; best = v; }
    }
    FlowOut[id.xy] = float4(best, bestCost, 0);
}
