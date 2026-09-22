// Визуализация потока: оттенок — направление, насыщенность — длина (gUser2.x px = максимум).
#include "common.hlsli"

Texture2D<float4>         Flow : register(t0);
RWTexture2D<unorm float4> Out  : register(u0);

float3 Hue(float h)
{
    float3 k = abs(frac(h + float3(0.0, 2.0 / 3.0, 1.0 / 3.0)) * 6.0 - 3.0) - 1.0;
    return saturate(k);
}

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float2 v = Flow.SampleLevel(LinearClamp, PixelUV(id.xy), 0).xy * float2(gDstSize); // в пикселях кадра
    float mag = length(v);
    float ang = atan2(v.y, v.x) / 6.2831853 + 0.5;
    float s = saturate(mag / max(gUser2.x, 1e-3));
    Out[id.xy] = float4(lerp(float3(1, 1, 1), Hue(ang), s), 1.0);
}
