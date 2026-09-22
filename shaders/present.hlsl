// Вывод кадра в swapchain: полноэкранный треугольник без вершинного буфера.
// VSMain/PSMain компилируются отдельно (gfx.ShaderLib.Graphics).
#include "common.hlsli"

Texture2D<float4> Src : register(t0);

struct VSOut
{
    float4 pos : SV_Position;
    float2 uv  : TEXCOORD0;
};

VSOut VSMain(uint id : SV_VertexID)
{
    VSOut o;
    float2 uv = float2((id << 1) & 2, id & 2);
    o.pos = float4(uv * float2(2.0, -2.0) + float2(-1.0, 1.0), 0.0, 1.0);
    o.uv = uv;
    return o;
}

float4 PSMain(VSOut i) : SV_Target
{
    float3 c = Src.SampleLevel(LinearClamp, i.uv, 0).rgb;
    // Маркер кадра в левом верхнем углу: зелёный — реальный, пурпурный — сгенерированный.
    // Снимите экран на камеру с высокой частотой/посмотрите в записи — сразу видно чередование.
    if ((gFlags & FLAG_MARKER) && i.pos.x < 24.0 && i.pos.y < 24.0)
        c = (gFlags & FLAG_GENERATED) ? float3(1.0, 0.0, 1.0) : float3(0.0, 1.0, 0.0);
    return float4(c, 1.0);
}
