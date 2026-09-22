// Flow search at one pyramid level: full search within radius gRadius around the
// prediction (the coarser level's flow, if FLAG_HAS_PRIOR) plus a separate "no motion"
// candidate, then a subpixel refinement pass. Output: xy is the A->B vector in UV, z is
// the match cost (for debugging/confidence).
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
    bool onZero = false;

    // Zero vector: static background and HUD elements shouldn't "swim".
    float zeroCost = MatchCost(uv, float2(0, 0)) - gUser.w + Smoothness(float2(0, 0), prior);
    if (zeroCost < bestCost) { bestCost = zeroCost; best = float2(0, 0); onZero = true; }

    int R = (int)gRadius;
    for (int y = -R; y <= R; ++y)
    {
        for (int x = -R; x <= R; ++x)
        {
            if (x == 0 && y == 0) continue;
            float2 v = prior + float2(x, y) * gInvDstSize;
            float c = MatchCost(uv, v) + Smoothness(v, prior);
            if (c < bestCost) { bestCost = c; best = v; onZero = false; }
        }
    }

    // Subpixel refinement: the search above only tries whole-texel steps, so the result
    // always lands on a grid point even though true motion rarely does. Fit a parabola
    // through the cost at the winner's four axis-aligned neighbors and nudge toward its
    // continuous minimum. Only at the finest level (gLevel == 0): coarser levels just seed
    // the next level's search, which finds its own best offset regardless of how precise
    // the seed was. Skipped for the zero candidate so static content keeps snapping to zero.
    if (!onZero && gLevel == 0)
    {
        float2 step = gInvDstSize;
        float2 vxm = best - float2(step.x, 0), vxp = best + float2(step.x, 0);
        float2 vym = best - float2(0, step.y), vyp = best + float2(0, step.y);
        float cxm = MatchCost(uv, vxm) + Smoothness(vxm, prior);
        float cxp = MatchCost(uv, vxp) + Smoothness(vxp, prior);
        float cym = MatchCost(uv, vym) + Smoothness(vym, prior);
        float cyp = MatchCost(uv, vyp) + Smoothness(vyp, prior);
        float2 denom = float2(cxm - 2.0 * bestCost + cxp, cym - 2.0 * bestCost + cyp);
        float2 frac = float2(0, 0);
        if (denom.x > 1e-6) frac.x = clamp(0.5 * (cxm - cxp) / denom.x, -0.5, 0.5);
        if (denom.y > 1e-6) frac.y = clamp(0.5 * (cym - cyp) / denom.y, -0.5, 0.5);
        best += frac * step;
    }

    FlowOut[id.xy] = float4(best, bestCost, 0);
}
