// Tracks, per flow-grid pixel, how many consecutive real-frame pairs its luma stayed
// unchanged. warp.hlsl uses this to force flow toward zero on long-static content (HUD,
// crosshair) instead of trusting whatever vector the search happened to find there. Read
// and written in place (RWTexture2D supports both), so no ping-pong resource is needed.
#include "common.hlsli"

Texture2D<float>    LumaA      : register(t0); // previous real frame, finest level
Texture2D<float>    LumaB      : register(t1); // next real frame, finest level
RWTexture2D<float>  StaticHist : register(u0);

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float a = LumaA.Load(int3(id.xy, 0));
    float b = LumaB.Load(int3(id.xy, 0));
    float hist = StaticHist[id.xy];
    hist = abs(a - b) < gUser3.x ? min(hist + 1.0, gUser3.y) : 0.0;
    StaticHist[id.xy] = hist;
}
