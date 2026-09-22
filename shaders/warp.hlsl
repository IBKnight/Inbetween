// Synthesizes an intermediate frame from the flow (backward warping from both frames).
// The flow is estimated at the midpoint (t=0.5); for other t values the same vector is
// reused as an approximation.
#include "common.hlsli"

Texture2D<float4>         A    : register(t0);
Texture2D<float4>         B    : register(t1);
Texture2D<float4>         Flow : register(t2); // xy is the A->B vector in UV (low resolution)
RWTexture2D<unorm float4> Out  : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float2 uv = PixelUV(id.xy);
    float t = gT;
    float2 v = Flow.SampleLevel(LinearClamp, uv, 0).xy;

    float3 ca = A.SampleLevel(LinearClamp, uv - t * v, 0).rgb;
    float3 cb = B.SampleLevel(LinearClamp, uv + (1.0 - t) * v, 0).rgb;

    // Occlusion handling (coarse): if the two samples disagree strongly, fall back to
    // whichever frame is nearest in time — a sharp cut beats a translucent halo.
    // gUser.y is the threshold, gUser.z is the sharpness.
    float w = t;
    float k = saturate((length(ca - cb) - gUser.y) * gUser.z);
    w = lerp(w, t < 0.5 ? 0.0 : 1.0, k);

    Out[id.xy] = float4(lerp(ca, cb, w), 1.0);
}
