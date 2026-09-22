// The next luma pyramid level: half resolution (a centered bilinear sample = a 2x2 box average).
#include "common.hlsli"

Texture2D<float>   Src : register(t0);
RWTexture2D<float> Dst : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    Dst[id.xy] = Src.SampleLevel(LinearClamp, PixelUV(id.xy), 0);
}
