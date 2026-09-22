# Inbetween — a frame generation tool in Go (D3D11, no cgo)

A simplified analog of Lossless Scaling Frame Generation: captures the game's rendered
frames, computes optical flow on the GPU, synthesizes intermediate frames, and outputs
them into a click-through overlay on top of the game window with even frame pacing.

- **Pure Go + standard library.** No cgo, no third-party modules, no neural networks.
- **D3D11/DXGI via hand-written COM calls** (`internal/com`, `internal/gfx`) — `syscall.SyscallN`.
- **HLSL shaders compile at runtime** via `d3dcompiler_47.dll` (present on Windows 10+),
  with **hot reload**: edit a `.hlsl` file and the change shows up immediately, no restart needed.
- Works on any DirectX 11 GPU (an RTX 30-series card has plenty of headroom).

## Releases

Prebuilt Windows binaries are published on the [Releases page](../../releases) — no Go
toolchain needed. Each release is a zip with `inbetween.exe`, `tracestat.exe`, and the
`shaders/` directory next to them (unzip anywhere, double-click `inbetween.exe`), plus a
`.sha256` checksum file.

Releases are built and published automatically by `.github/workflows/release.yml`: push a
tag matching `v*` (e.g. `git tag v0.1.0 && git push origin v0.1.0`), or run the "release"
workflow manually from the Actions tab with a version to build. The workflow cross-compiles
from Linux (`GOOS=windows GOARCH=amd64 go build`) — no cgo means no Windows runner is needed
to build it, only to run it.

## Requirements

- Windows 10 2004+ or Windows 11 (needs `WDA_EXCLUDEFROMCAPTURE`, otherwise the overlay
  gets captured too).
- Go 1.22+ (`winget install GoLang.Go`) — only if building from source; a release zip needs
  nothing but Windows itself.
- The game running in **borderless windowed mode** — exclusive fullscreen isn't covered by
  the overlay.

## Quick start

```bat
build.bat                                   :: vet + tests + build bin\inbetween.exe
bin\inbetween.exe                           :: launcher: pick a window, algorithm, multiplier, hit Start
bin\inbetween.exe -mode list                :: monitors and windows
run-eval.bat                                :: synthetic-scene quality: PSNR, images in dumps\eval
run-synthetic.bat                           :: live mode on a synthetic scene in a regular window
run-game.bat "Cyberpunk"                    :: live mode over the game window (switch to the game within 3s)
go run ./cmd/tracestat game.csv             :: how even the frame pacing was
```

Double-clicking `bin\inbetween.exe` (no arguments) opens a small native launcher window
(`internal/app/gui.go` + `internal/win/dialog.go`): pick the game's window from the
drop-down, pick the algorithm and multiplier, click "Старт". When the game window closes
or you press Ctrl+Alt+Q, the launcher reappears so you can pick another game. It's a thin
wrapper around `-mode live -window "..."` — for anything beyond window/algorithm/multiplier
(flow tuning, `-debug`, `-trace`, ...), use the CLI flags directly.

A good test without a game: open a 30 fps video in the browser and target the browser
window (`-window "YouTube"`) — the effect is immediately visible on smooth pans.

### Hotkeys (live mode)

| Keys | Action |
|---|---|
| Ctrl+Alt+F | toggle on/off (overlay hides so you can compare against the original) |
| Ctrl+Alt+A | cycle algorithm: off → blend → flow |
| Ctrl+Alt+V | show optical flow as color |
| Ctrl+Alt+D | dump the current frames (prev/next/gen/flow) to `dumps\` |
| Ctrl+Alt+Q | quit (Esc only works in the `-view window` debug window) |

## Modes

| Mode | What it does | Why |
|---|---|---|
| `-mode gui` | native launcher: pick a window/algorithm/multiplier, click Start | default when run with no arguments at all |
| `-mode list` | lists monitors/adapters and windows | to pick `-monitor` / `-window` |
| `-mode eval` | synthetic scene, compares the generated frame against **ground truth** (PSNR) for each pair, for the off/blend/flow algorithms | objective metric: improve the shader → the number goes up |
| `-mode offline` | PNG directory → PNG directory with intermediate frames inserted | run the algorithm over a real game recording |
| `-mode live` | capture → generate → overlay | actual usage |

All flags: `bin\inbetween.exe -h`. The main ones: `-mult 2` (X2), `-algo flow|blend|off`,
`-flowscale 0.5`, `-radius 4`, `-vsync`, `-marker` (a colored square in the corner: green —
real frame, magenta — generated), `-trace file.csv`, `-dump N`, `-debug` (D3D11 debug layer).

## How it works

```
 ┌──────────── source (internal/capture) ───────────────┐
 │ DDA: IDXGIOutputDuplication, cropped to the game       │   Synthetic: procedural scene
 │ window; LastPresentTime = frame timestamp (QPC)        │   Files: PNG sequence
 └───────────────────────┬──────────────────────────────┘
                         ▼ Push(frame)
 ┌──────────────── fg.Pipeline (internal/fg + shaders/) ─────────────────┐
 │ import.hlsl → RGBA8 history frame (ring buffer of 3)                   │
 │ luma.hlsl + downsample.hlsl → a luma pyramid for each frame            │
 │ flow_search.hlsl (coarse→fine) + flow_refine.hlsl → flow A→B           │
 │ Generate(t): warp.hlsl (or blend.hlsl) → intermediate frame            │
 └───────────────────────┬───────────────────────────────────────────────┘
                         ▼
 pacing.Pacer: present schedule (t = 1/M … 1) from the estimated source interval
                         ▼
 present.hlsl → SwapChain (flip discard, waitable, max latency 1) → overlay
               (WS_EX_LAYERED|TRANSPARENT|TOPMOST|NOACTIVATE, excluded from capture)
```

Packages:

| Package | Contents |
|---|---|
| `internal/com` | GUID, HRESULT, calling a COM method by vtable index |
| `internal/win` | windows, messages, hotkeys, QPC, precise sleep, window lookup, launcher dialog controls |
| `internal/gfx` | D3D11 device, textures, compute/draw, shader compilation + hot reload, GPU profiler, readback/PNG, swapchain, debug layer |
| `internal/capture` | Desktop Duplication, synthetic source, PNG |
| `internal/fg` | the generation pipeline and its options |
| `internal/pacing` | present scheduler (no Windows dependency, has tests) |
| `internal/stats` | metrics, CSV trace |
| `internal/config` | flags |
| `internal/app` | gui/list/eval/offline/live modes, main loop |
| `cmd/inbetween`, `cmd/tracestat` | entry points |
| `shaders/` | all HLSL: one pass = one file |

## Current algorithm (starting version)

Symmetric block matching: for each pixel of the **intermediate** frame, search for a vector
`v` such that the 5×5 luma block `A(x − v/2)` best matches `B(x + v/2)`. The search runs
over a coarse-to-fine pyramid, with a separate "no motion" candidate (HUD, static areas)
and a smoothness penalty, followed by a neighbor-vector propagation step. The intermediate
frame is sampled from both frames along the vector and blended; where the two disagree
strongly (occlusion), the nearest frame in time is used instead. This is a working baseline
meant to be improved further — see the plan in `CLAUDE.md`.

## Limitations and what to check first

- Overlay: a flip-model swapchain on a layered window. If the output is black or doesn't
  update on your system — run with `-layered=false` (clicks then won't pass through) and
  report it to the agent.
- Desktop Duplication emits a frame on any screen update; updates outside the game's area
  are filtered out via dirty rects. Check `src ... fps` in the log: it should match the
  game's FPS.
- No support for HDR, rotated monitors, or resizing the game window on the fly.
- Latency grows by roughly half a source frame (at X2) — that's inherent to the method.

## Reference open-source projects

- **Magpie** (github.com/Blinue/Magpie) — an open-source analog of Lossless Scaling's
  upscaling on Windows: capture, overlay on top of the game, shader effects. The best
  reference for the surrounding infrastructure.
- **AMD FidelityFX SDK** — FSR 3 Frame Generation, open source (optical flow + interpolation).
- **lsfg-vk** — a Vulkan layer for Linux that reuses Lossless Scaling's own shaders.
