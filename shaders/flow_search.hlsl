// Flow search at one pyramid level: full search within radius gRadius around the
// prediction (the coarser level's flow, if FLAG_HAS_PRIOR) plus a separate "no motion"
// candidate. Output: xy is the A->B vector in UV, z is the match cost (for debugging/confidence).
#include "flow_common.hlsli"

Texture2D<float4>   Prior   : register(t2);
RWTexture2D<float4> FlowOut : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float2 uv = PixelUV(id.xy);
    float2 prior = (gFlags & FLAG_HAS_PRIOR) ? Prior.SampleLevel(LinearClamp, uv, 0).xy : float2(0, 0);

    float2 best = prior;
    float bestCost = MatchCost(uv, prior);

    // Zero vector: static background and HUD elements shouldn't "swim".
    float zeroCost = MatchCost(uv, float2(0, 0)) - gUser.w + Smoothness(float2(0, 0), prior);
    if (zeroCost < bestCost) { bestCost = zeroCost; best = float2(0, 0); }

    int R = (int)gRadius;
    for (int y = -R; y <= R; ++y)
    {
        for (int x = -R; x <= R; ++x)
        {
            if (x == 0 && y == 0) continue;
            float2 v = prior + float2(x, y) * gInvDstSize;
            float c = MatchCost(uv, v) + Smoothness(v, prior);
            if (c < bestCost) { bestCost = c; best = v; }
        }
    }
    FlowOut[id.xy] = float4(best, bestCost, 0);
}
