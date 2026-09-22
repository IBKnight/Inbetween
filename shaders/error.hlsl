// eval: поэлементная квадратичная ошибка между сгенерированным (A) и истинным (B) кадром.
#include "common.hlsli"

Texture2D<float4>  A   : register(t0);
Texture2D<float4>  B   : register(t1);
RWTexture2D<float> Err : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float3 d = A.Load(int3(id.xy, 0)).rgb - B.Load(int3(id.xy, 0)).rgb;
    Err[id.xy] = dot(d, d) / 3.0;
}
