// Color frame -> luma at pyramid level 0 (usually 1/2 resolution).
// 4 bilinear samples = a box filter over up to 4x4 source pixels (a proper average when
// downscaling up to 4x).
#include "common.hlsli"

Texture2D<float4>   Color : register(t0);
RWTexture2D<float>  Out   : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float2 uv = PixelUV(id.xy);
    float2 q = 0.25 * gInvDstSize;
    float3 c = Color.SampleLevel(LinearClamp, uv + float2(-q.x, -q.y), 0).rgb
             + Color.SampleLevel(LinearClamp, uv + float2( q.x, -q.y), 0).rgb
             + Color.SampleLevel(LinearClamp, uv + float2(-q.x,  q.y), 0).rgb
             + Color.SampleLevel(LinearClamp, uv + float2( q.x,  q.y), 0).rgb;
    Out[id.xy] = Luma(c * 0.25);
}
