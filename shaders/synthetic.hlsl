// A procedural test scene at moment gTime (seconds). Deterministic: the ground truth can
// be rendered for any intermediate moment — that's what eval (PSNR) is built on.
// Covers typical hard cases: texture scrolling, objects at different speeds, a very fast
// object, rotation, a fine repeating pattern, a static HUD.
#include "common.hlsli"

RWTexture2D<unorm float4> Out : register(u0);

float Disc(float2 p, float2 c, float r) { return saturate(r - length(p - c) + 0.5); }
float Box(float2 p, float2 c, float2 h)  { float2 d = abs(p - c) - h; return saturate(0.5 - max(d.x, d.y)); }

[numthreads(GROUP_X, GROUP_Y, 1)]
void main(uint3 id : SV_DispatchThreadID)
{
    if (OutOfBounds(id.xy)) return;
    float2 p = float2(id.xy) + 0.5;
    float2 S = float2(gDstSize);
    float t = gTime;

    // Background: a checkerboard, scrolling evenly at (120, 30) px/s, plus a vertical gradient.
    float2 bp = p + float2(t * 120.0, t * 30.0);
    float2 cell = floor(bp / 48.0);
    float checker = frac((cell.x + cell.y) * 0.5) * 2.0;
    float3 col = lerp(float3(0.15, 0.17, 0.22), float3(0.30, 0.32, 0.38), checker);
    col *= 0.8 + 0.2 * p.y / S.y;

    // A band with a fine pattern moving faster than the background (tests the aperture
    // problem / repeating patterns).
    float stripes = step(0.5, frac((p.x + p.y * 0.5 - t * 300.0) / 16.0));
    float band = Box(p, float2(S.x * 0.5, S.y * 0.85), float2(S.x * 0.35, S.y * 0.05));
    col = lerp(col, lerp(float3(0.9, 0.8, 0.2), float3(0.2, 0.2, 0.2), stripes), band);

    // Balls moving along Lissajous curves at different speeds.
    [unroll] for (int i = 0; i < 5; ++i)
    {
        float fi = (float)i;
        float sp = 0.4 + 0.35 * fi;
        float2 c = S * 0.5 + S * float2(0.38, 0.30) * float2(sin(t * sp * 1.3 + fi * 1.7), sin(t * sp * 1.9 + fi));
        float r = 22.0 + 10.0 * fi;
        float3 bc = 0.5 + 0.5 * cos(fi * 1.3 + float3(0.0, 2.0, 4.0));
        col = lerp(col, bc, Disc(p, c, r));
    }

    // A very fast object (~0.8 screen widths per second).
    float2 fc = float2(frac(t * 0.8) * (S.x + 100.0) - 50.0, S.y * 0.2);
    col = lerp(col, float3(1.0, 1.0, 1.0), Box(p, fc, float2(10.0, 30.0)));

    // A rotating bar.
    float2 rc = float2(S.x * 0.78, S.y * 0.5);
    float ang = t * 2.0;
    float2 dir = float2(cos(ang), sin(ang));
    float2 rel = p - rc;
    float along = dot(rel, dir);
    float across = dot(rel, float2(-dir.y, dir.x));
    float bar = saturate(0.5 - max(abs(along) - 120.0, abs(across) - 8.0));
    col = lerp(col, float3(0.9, 0.3, 0.3), bar);

    // A static "HUD": a fine high-contrast pattern that should stay still and sharp.
    float2 hp = p - float2(24.0, 24.0);
    if (all(hp >= 0.0) && all(hp < float2(260.0, 60.0)))
    {
        float hud = step(0.5, frac(hp.x / 6.0)) * step(0.5, frac(hp.y / 10.0));
        col = lerp(float3(0.05, 0.05, 0.05), float3(0.95, 0.95, 0.95), hud);
    }

    Out[id.xy] = float4(col, 1.0);
}
