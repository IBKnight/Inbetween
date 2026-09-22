// Общее для поиска потока: стоимость совпадения блоков.
#ifndef FLOW_COMMON_HLSLI
#define FLOW_COMMON_HLSLI
#include "common.hlsli"

Texture2D<float> LumaA : register(t0); // предыдущий кадр (уровень gLevel)
Texture2D<float> LumaB : register(t1); // следующий кадр

#ifndef BLOCK_R
#define BLOCK_R 2 // блок (2R+1)^2 = 5x5
#endif

// СИММЕТРИЧНОЕ сопоставление: пиксель в момент t=0.5 в точке uv пришёл из A(uv - v/2)
// и уйдёт в B(uv + v/2). v — полное смещение A->B в UV. Плюс такого подхода: поток сразу
// задан на сетке промежуточного кадра, без «дыр» прямого варпинга.
float MatchCost(float2 uv, float2 v)
{
    float2 ha = uv - 0.5 * v;
    float2 hb = uv + 0.5 * v;
    float sum = 0;
    [unroll] for (int y = -BLOCK_R; y <= BLOCK_R; ++y)
    [unroll] for (int x = -BLOCK_R; x <= BLOCK_R; ++x)
    {
        float2 o = float2(x, y) * gInvDstSize;
        sum += abs(LumaA.SampleLevel(LinearClamp, ha + o, 0) - LumaB.SampleLevel(LinearClamp, hb + o, 0));
    }
    return sum / float((2 * BLOCK_R + 1) * (2 * BLOCK_R + 1));
}

// Штраф за отклонение вектора v от опорного ref (в текселях текущего уровня).
float Smoothness(float2 v, float2 ref)
{
    return gUser.x * length((v - ref) * float2(gDstSize));
}
#endif
