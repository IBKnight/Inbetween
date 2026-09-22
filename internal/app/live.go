//go:build windows

package app

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"inbetween/internal/capture"
	"inbetween/internal/com"
	"inbetween/internal/config"
	"inbetween/internal/fg"
	"inbetween/internal/gfx"
	"inbetween/internal/pacing"
	"inbetween/internal/stats"
	"inbetween/internal/win"
)

const (
	hkToggle = iota + 1
	hkAlgo
	hkVis
	hkDump
	hkQuit
)

// RunLive is the main mode: capture -> pipeline -> presentation on the pacing schedule.
//
// Everything runs on a single thread (locked in main): the D3D11 immediate context isn't
// thread-safe, and the window must pump its messages on the thread that created it. Each
// loop iteration pumps window/hotkey messages, waits for a new source frame without
// blocking past the next scheduled present, pushes any new frame into the pipeline and
// reschedules presents, and presents once the pacing schedule and swapchain are ready.
func RunLive(cfg *config.Config) error {
	if err := win.SetDPIAware(); err != nil {
		log.Printf("warning: DPI awareness: %v", err)
	}
	win.TimeBeginPeriod(1)
	defer win.TimeEndPeriod(1)
	if cfg.HighPriority {
		if err := win.SetThreadPriorityHighest(); err != nil {
			log.Printf("warning: %v", err)
		}
	}

	target, targetHWND, adapter, outputIdx, err := resolveTarget(cfg)
	if err != nil {
		return err
	}
	e, err := newEnv(cfg, adapter)
	if err != nil {
		return err
	}
	defer e.Close()

	var src capture.Source
	if cfg.Source == "dda" {
		d, err := capture.NewDDA(e.dev, outputIdx, target)
		if err != nil {
			return err
		}
		src = d
	} else {
		s, err := capture.NewSynthetic(e.dev, e.lib, cfg.W, cfg.H, cfg.FPS)
		if err != nil {
			return err
		}
		s.Jitter = cfg.Jitter
		src = s
	}
	defer src.Close()
	W, H := src.Size()
	if d, ok := src.(*capture.DDA); ok {
		target = d.ScreenRect() // overlay exactly over the captured area
	} else {
		target = win.RECT{Left: 100, Top: 100, Right: int32(100 + W), Bottom: int32(100 + H)}
	}
	log.Printf("источник: %s", src.Name())

	pipe, err := fg.New(e.dev, e.lib, W, H, cfg.FGOptions())
	if err != nil {
		return err
	}
	defer pipe.Close()
	log.Printf("pipeline: %dx%d algo=%s X%d, уровни потока %v", W, H, pipe.Opt.Algo, cfg.Mult, pipe.Levels())

	wnd, err := win.CreateWindow(win.WindowOptions{
		Title: "inbetween", Client: target, Overlay: cfg.View == "overlay", Layered: cfg.Layered,
	})
	if err != nil {
		return err
	}
	defer wnd.Destroy()
	if err := wnd.ExcludeFromCapture(); err != nil {
		log.Printf("warning: %v — окно попадёт в захват (петля обратной связи)!", err)
	}
	sc, err := e.dev.NewSwapChain(wnd.HWND, W, H, cfg.Tearing && !cfg.VSync)
	if err != nil {
		return err
	}
	defer sc.Release()
	vs, err := e.lib.Graphics("present.hlsl", "VSMain", gfx.StageVertex)
	if err != nil {
		return err
	}
	ps, err := e.lib.Graphics("present.hlsl", "PSMain", gfx.StagePixel)
	if err != nil {
		return err
	}
	presentCB, err := e.dev.NewConstantBuffer(96)
	if err != nil {
		return err
	}
	defer presentCB.Release()

	hotkeys := []struct {
		id   int
		vk   uint32
		name string
	}{
		{hkToggle, 'F', "вкл/выкл"}, {hkAlgo, 'A', "алгоритм"}, {hkVis, 'V', "визуализация потока"},
		{hkDump, 'D', "дамп кадров"}, {hkQuit, 'Q', "выход"},
	}
	for _, h := range hotkeys {
		if err := win.RegisterHotKey(h.id, win.MOD_CONTROL|win.MOD_ALT, h.vk); err != nil {
			log.Printf("warning: Ctrl+Alt+%c (%s): %v", h.vk, h.name, err)
			continue
		}
		defer win.UnregisterHotKey(h.id)
	}

	freq := win.QPF()
	multFor := func(a fg.Algo) int {
		if a == fg.AlgoOff {
			return 1
		}
		return cfg.Mult
	}
	pacer := pacing.New(freq, multFor(pipe.Opt.Algo))
	pacer.Offset = cfg.Offset
	var st stats.Live
	var trace *stats.Trace
	if cfg.Trace != "" {
		if trace, err = stats.NewTrace(cfg.Trace); err != nil {
			return err
		}
		defer trace.Close()
	}
	sleeper := win.NewSleeper()
	defer sleeper.Close()

	var (
		quit        bool
		enabled     = true
		dumpLeft    = cfg.Dump
		dumpNow     bool
		start       = win.QPC()
		lastLog     = start
		lastReload  = start
		lastArrival int64
	)
	onHotkey := func(id int) {
		switch id {
		case hkToggle:
			enabled = !enabled
			wnd.SetVisible(enabled)
			pacer.Reset()
			log.Printf("inbetween: %v", map[bool]string{true: "ВКЛ", false: "ВЫКЛ (оверлей скрыт)"}[enabled])
		case hkAlgo:
			pipe.Opt.Algo = pipe.Opt.Algo.Next()
			pacer.Mult = multFor(pipe.Opt.Algo)
			log.Printf("алгоритм: %s (X%d)", pipe.Opt.Algo, pacer.Mult)
		case hkVis:
			pipe.DebugView = !pipe.DebugView
			log.Printf("визуализация потока: %v", pipe.DebugView)
		case hkDump:
			dumpNow = true
		case hkQuit:
			quit = true
		}
	}
	present := func(tex *gfx.Texture, generated bool) error {
		var flags uint32
		if cfg.Marker {
			flags |= gfx.FlagMarker
		}
		if generated {
			flags |= gfx.FlagGenerated
		}
		prm := gfx.Params{DstSize: [2]uint32{uint32(sc.W), uint32(sc.H)}, Flags: flags}
		if err := gfx.SetConstants(e.dev, presentCB, &prm); err != nil {
			return err
		}
		e.dev.DrawFullscreen(vs, ps, sc.Back.RTV, sc.W, sc.H, gfx.Bindings{
			CB: []*gfx.ConstantBuffer{presentCB}, SRV: []*gfx.Texture{tex}, Samplers: pipe.Samplers(),
		})
		return sc.Present(cfg.VSync)
	}

	log.Printf("старт: view=%s vsync=%v tearing=%v. Ctrl+Alt+Q — выход, Ctrl+Alt+F — вкл/выкл", cfg.View, cfg.VSync, sc.Tearing)

	for !quit {
		if win.PumpMessages(onHotkey) {
			break
		}
		if targetHWND != 0 && !win.IsWindow(targetHWND) {
			log.Printf("окно игры закрыто — выходим")
			break
		}
		now := win.QPC()

		// Wait for a new source frame, but not past the next scheduled present minus 1 ms.
		timeout := 4 * time.Millisecond
		if it, ok := pacer.Peek(); ok && enabled {
			remain := max(it.Due-now-win.MsToTicks(1), 0)
			timeout = min(timeout, time.Duration(remain*int64(time.Second)/freq))
		}
		f, ok, err := src.Poll(timeout)
		if err != nil {
			return err
		}
		if ok {
			arrival := win.QPC()
			st.SrcFrames++
			st.SrcMissed += f.Missed
			e.beginGPU()
			pipe.Push(f)
			e.endGPU()
			lastArrival = arrival
			if pipe.Ready() && enabled {
				pacer.OnSourceFrame(f.Time, arrival, f.Seq, f.Missed)
			}
			if pipe.Ready() && (dumpLeft > 0 || dumpNow) {
				dir := filepath.Join(cfg.DumpDir, fmt.Sprintf("live_%06d", f.Seq))
				if err := pipe.Dump(dir, 0.5); err != nil {
					log.Printf("дамп: %v", err)
				} else {
					log.Printf("дамп: %s", dir)
				}
				if dumpLeft > 0 {
					dumpLeft--
				}
				dumpNow = false
			}
		}

		if it, ok := pacer.Peek(); ok && enabled {
			now = win.QPC()
			if wait := it.Due - now; wait > 0 && wait < win.MsToTicks(1.5) {
				sleeper.SleepUntil(it.Due, true)
				now = win.QPC()
			}
			if now >= it.Due && sc.Ready() {
				t0 := win.QPC()
				e.beginGPU()
				var tex *gfx.Texture
				switch {
				case pipe.DebugView:
					tex = pipe.FlowVis()
				case it.Real:
					tex = pipe.Latest()
				default:
					tex = pipe.Generate(it.T)
				}
				err := present(tex, !it.Real)
				e.endGPU()
				t1 := win.QPC()
				if err != nil {
					if errors.Is(err, com.DXGI_ERROR_DEVICE_REMOVED) || errors.Is(err, com.DXGI_ERROR_DEVICE_RESET) {
						log.Printf("устройство потеряно: %v", e.dev.RemovedReason())
					}
					return err
				}
				pacer.Pop()
				nowMs := win.TicksToMs(t1 - start)
				late := win.TicksToMs(t0 - it.Due)
				st.OnPresent(nowMs, it.Real)
				st.Lateness.Add(late)
				st.GenCPU.Add(win.TicksToMs(t1 - t0))
				if it.Real && lastArrival != 0 {
					st.Latency.Add(win.TicksToMs(t1 - lastArrival))
				}
				trace.Row(nowMs, it.Real, it.T, it.Seq, win.TicksToMs(it.Due-start), late,
					win.TicksToMs(int64(pacer.Interval())), win.TicksToMs(t1-t0))
			}
		}

		now = win.QPC()
		if cfg.HotReload && now-lastReload > freq/2 {
			e.reloadShaders()
			lastReload = now
		}
		if now-lastLog >= win.DurToTicks(cfg.LogEvery) {
			secs := win.TicksToMs(now-lastLog) / 1000
			if d, ok := src.(*capture.DDA); ok {
				st.SrcSkipped, d.Skipped = d.Skipped, 0
			}
			log.Printf("%s | est %.1f fps | stale %d hiccups %d | %s | gpu %s",
				st.Line(secs), pacer.SourceFPS(), pacer.Stale, pacer.Hiccups, pipe.Opt.Algo, e.gpuSummary())
			pacer.Stale, pacer.Hiccups = 0, 0
			st.ResetWindow()
			e.drainDebug()
			lastLog = now
		}
	}
	log.Printf("выход")
	return nil
}

// resolveTarget resolves the capture rectangle (screen coordinates), the game window,
// and the monitor/adapter to use.
func resolveTarget(cfg *config.Config) (target win.RECT, hwnd uintptr, adapter, output int, err error) {
	adapter = cfg.Adapter
	if cfg.Source != "dda" {
		return win.RECT{}, 0, max(adapter, 0), 0, nil
	}
	if cfg.Delay > 0 {
		log.Printf("жду %v — переключитесь в игру...", cfg.Delay)
		time.Sleep(cfg.Delay)
	}
	outs, err := gfx.ListOutputs()
	if err != nil {
		return
	}
	switch {
	case cfg.Window != "":
		wi, all, ferr := win.FindWindow(cfg.Window)
		if ferr != nil {
			err = ferr
			return
		}
		if len(all) > 1 {
			for _, w := range all[1:] {
				log.Printf("  (ещё подходит: %v)", w)
			}
		}
		log.Printf("окно: %v", wi)
		target, hwnd = wi.Client, wi.HWND
	case cfg.Rect != "":
		x, y, w, h, _ := config.ParseRect(cfg.Rect)
		target = win.RECT{Left: int32(x), Top: int32(y), Right: int32(x + w), Bottom: int32(y + h)}
	default:
		if cfg.Monitor < 0 || cfg.Monitor >= len(outs) {
			err = fmt.Errorf("-monitor %d: всего мониторов %d (см. -mode list)", cfg.Monitor, len(outs))
			return
		}
		target = outs[cfg.Monitor].Desktop
	}
	o, ferr := gfx.FindOutput(target)
	if ferr != nil {
		err = ferr
		return
	}
	if adapter >= 0 && adapter != o.Adapter {
		log.Printf("warning: -adapter %d, но монитор на адаптере %d — Desktop Duplication, скорее всего, не заработает", adapter, o.Adapter)
	} else {
		adapter = o.Adapter
	}
	output = o.Output
	log.Printf("монитор: %v, захват %v", o, target)
	return
}
