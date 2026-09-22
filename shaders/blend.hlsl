// Простейшая «генерация»: линейное смешивание A и B. База для сравнения в eval.
#include "common.hlsli"

Texture2D<float4>         A   : register(t0);
Texture2D<float4>         B   : register(t1);
RWTexture2D<unorm float4> Out : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float3 a = A.Load(int3(id.xy, 0)).rgb;
    float3 b = B.Load(int3(id.xy, 0)).rgb;
    Out[id.xy] = float4(lerp(a, b, gT), 1.0);
}
