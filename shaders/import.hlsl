// Source frame (BGRA8/RGBA8, possibly offset) -> working RGBA8 history frame.
// Future additions could go here: downscaling, HDR->SDR, gamma.
#include "common.hlsli"

Texture2D<float4>         Src : register(t0);
RWTexture2D<unorm float4> Dst : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    uint2 p = min(id.xy + gSrcOffset, gSrcSize - 1);
    float3 c = Src.Load(int3(p, 0)).rgb; // a BGRA texture already returns rgba in the right order
    Dst[id.xy] = float4(c, 1.0);
}
