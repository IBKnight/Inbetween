// Synthesizes an intermediate frame from the flow (backward warping from both frames).
// The flow is estimated at the midpoint (t=0.5); for other t values the same vector is
// reused as an approximation.
#include "common.hlsli"

Texture2D<float4>         A    : register(t0);
Texture2D<float4>         B    : register(t1);
Texture2D<float4>         Flow : register(t2); // xy is the A->B vector in UV (low res), z is its match cost
RWTexture2D<unorm float4> Out  : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float2 uv = PixelUV(id.xy);
    float t = gT;
    float4 flowSample = Flow.SampleLevel(LinearClamp, uv, 0);
    float2 v = flowSample.xy;
    float matchCost = flowSample.z;

    float3 ca = A.SampleLevel(LinearClamp, uv - t * v, 0).rgb;
    float3 cb = B.SampleLevel(LinearClamp, uv + (1.0 - t) * v, 0).rgb;

    // Occlusion handling: two independent signals push toward distrusting the warp and
    // snapping to the nearest real frame instead of blending. (1) the two samples disagree
    // in color once warped along v — cheap, but blind to a structurally wrong match that
    // happens to be similarly colored (e.g. a dark cloak warped over dark rock). (2) the
    // match itself was a poor fit during search (high residual cost from
    // flow_search/flow_refine, already computed, just unused until now) — this catches (1)'s
    // blind spot, since a bad match costs more regardless of color. gUser.y/z tune the color
    // check, gUser2.y/z tune the cost check; either one alone is enough to trigger the snap.
    float colorK = saturate((length(ca - cb) - gUser.y) * gUser.z);
    float costK = saturate((matchCost - gUser2.y) * gUser2.z);
    float k = max(colorK, costK);
    float w = lerp(t, t < 0.5 ? 0.0 : 1.0, k);

    Out[id.xy] = float4(lerp(ca, cb, w), 1.0);
}
