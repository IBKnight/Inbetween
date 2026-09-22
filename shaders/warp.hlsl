// Синтез промежуточного кадра по потоку (backward warping из обоих кадров).
// Поток оценён в середине пути (t=0.5); для других t используется тот же вектор — приближение.
#include "common.hlsli"

Texture2D<float4>         A    : register(t0);
Texture2D<float4>         B    : register(t1);
Texture2D<float4>         Flow : register(t2); // xy — вектор A->B в UV (низкое разрешение)
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

    // Окклюзии (грубо): если выборки сильно расходятся, берём кадр, ближайший по времени —
    // лучше «резкий скачок», чем полупрозрачный ореол. gUser.y — порог, gUser.z — крутизна.
    float w = t;
    float k = saturate((length(ca - cb) - gUser.y) * gUser.z);
    w = lerp(w, t < 0.5 ? 0.0 : 1.0, k);

    Out[id.xy] = float4(lerp(ca, cb, w), 1.0);
}
