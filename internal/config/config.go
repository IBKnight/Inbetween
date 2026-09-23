// Package config holds the command-line parameters.
package config

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"inbetween/internal/fg"
)

type Config struct {
	Mode string // live | eval | offline | list

	// Source (live)
	Source  string        // dda | synthetic
	Window  string        // substring of the game window's title (dda)
	Monitor int           // monitor index from -mode list (dda, if no -window/-rect)
	Rect    string        // x,y,w,h in screen coordinates (dda)
	Delay   time.Duration // pause before looking for the window (time to switch to the game)
	Adapter int           // -1 = auto (the monitor's adapter)

	// Synthetic / files
	Size   string // WxH of the synthetic scene
	W, H   int
	FPS    float64 // FPS of the synthetic source, and the "FPS" of the PNG sequence
	Jitter float64 // jitter of synthetic frame moments (fraction of the interval)
	In     string  // input PNG directory (offline)
	Out    string  // output directory (offline)

	// Generation
	Mult          int
	Algo          string
	FlowScale     float64
	MinLevel      int
	Radius        int
	RefineRadius  int
	RefineIters   int
	Reg           float64
	ZeroBias      float64
	OccThr        float64
	OccK          float64
	OccCostThr    float64
	OccCostK      float64
	EdgeSmoothK   float64
	StaticDiffThr float64
	StaticMaxCnt  float64

	// Output
	View           string // overlay | window
	Layered        bool   // overlay: click-through (WS_EX_LAYERED|WS_EX_TRANSPARENT)
	VSync          bool
	Tearing        bool
	Marker         bool
	Offset         float64 // pacing schedule shift (fraction of the interval)
	AdaptiveOffset bool    // compute the shift automatically from observed source jitter

	// Debug
	Debug        bool
	ShaderDebug  bool
	HotReload    bool
	Shaders      string
	Profile      bool
	HighPriority bool
	Dump         int
	DumpDir      string
	Trace        string
	EvalFrames   int
	LogEvery     time.Duration
	LogFile      string
}

const usageHead = `inbetween — frame generation «снаружи» (как Lossless Scaling) на Go + D3D11.

Режимы (-mode):
  gui      окно выбора игры и запуска live-режима (по умолчанию при запуске без аргументов)
  list     показать мониторы/адаптеры и окна
  live     захват -> генерация -> вывод в оверлей (или окно)
  eval     офлайн-проверка качества на синтетической сцене: PSNR против «правды»
  offline  PNG-последовательность -> PNG с промежуточными кадрами

Примеры:
  inbetween
  inbetween -mode list
  inbetween -mode eval -size 1280x720 -fps 30 -eval 60
  inbetween -source synthetic -view window -fps 30 -mult 2 -marker
  inbetween -window "Cyberpunk" -delay 3s -mult 2 -trace trace.csv
  inbetween -mode offline -in frames -out out -mult 2

Горячие клавиши (live): Ctrl+Alt+F вкл/выкл, Ctrl+Alt+A алгоритм, Ctrl+Alt+V визуализация потока,
Ctrl+Alt+D дамп кадров, Ctrl+Alt+Q выход (Esc — в окне отладки).

Флаги:
`

func Parse(args []string, out io.Writer) (*Config, error) {
	c := &Config{}
	fs := flag.NewFlagSet("inbetween", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprint(out, usageHead)
		fs.PrintDefaults()
	}
	d := fg.DefaultOptions()

	fs.StringVar(&c.Mode, "mode", "live", "gui | live | eval | offline | list")
	fs.StringVar(&c.Source, "source", "dda", "источник для live: dda | synthetic")
	fs.StringVar(&c.Window, "window", "", "захват: подстрока заголовка окна (клиентская область)")
	fs.IntVar(&c.Monitor, "monitor", 0, "захват: индекс монитора (если не задан -window/-rect)")
	fs.StringVar(&c.Rect, "rect", "", "захват: прямоугольник x,y,w,h в экранных координатах")
	fs.DurationVar(&c.Delay, "delay", 0, "пауза перед поиском окна, например 3s")
	fs.IntVar(&c.Adapter, "adapter", -1, "индекс адаптера DXGI (-1 = авто)")

	fs.StringVar(&c.Size, "size", "1280x720", "размер синтетической сцены WxH")
	fs.Float64Var(&c.FPS, "fps", 30, "FPS синтетики / PNG-последовательности")
	fs.Float64Var(&c.Jitter, "jitter", 0, "дрожание синтетических кадров (доля интервала, 0..1)")
	fs.StringVar(&c.In, "in", "", "offline: каталог входных PNG")
	fs.StringVar(&c.Out, "out", "out", "offline: каталог результата")

	fs.IntVar(&c.Mult, "mult", 2, "множитель кадров (2 = X2)")
	fs.StringVar(&c.Algo, "algo", d.Algo.String(), "алгоритм: off | blend | flow")
	fs.Float64Var(&c.FlowScale, "flowscale", d.FlowScale, "разрешение потока относительно кадра")
	fs.IntVar(&c.MinLevel, "minlevel", d.MinLevelSize, "минимальная сторона самого грубого уровня пирамиды")
	fs.IntVar(&c.Radius, "radius", d.Radius, "радиус перебора на грубом уровне")
	fs.IntVar(&c.RefineRadius, "refine-radius", d.RefineRadius, "радиус поиска на остальных уровнях")
	fs.IntVar(&c.RefineIters, "refine-iters", d.RefineIters, "итераций распространения на уровень")
	fs.Float64Var(&c.Reg, "reg", float64(d.Reg), "штраф за негладкость потока")
	fs.Float64Var(&c.ZeroBias, "zero-bias", float64(d.ZeroBias), "бонус нулевому вектору (статика/HUD)")
	fs.Float64Var(&c.OccThr, "occ-thr", float64(d.OccThreshold), "порог окклюзии в warp (по цвету)")
	fs.Float64Var(&c.OccK, "occ-k", float64(d.OccSharpness), "крутизна обработки окклюзий по цвету (0 = выкл.)")
	fs.Float64Var(&c.OccCostThr, "occ-cost-thr", float64(d.OccCostThreshold), "порог окклюзии в warp (по стоимости совпадения потока)")
	fs.Float64Var(&c.OccCostK, "occ-cost-k", float64(d.OccCostSharpness), "крутизна обработки окклюзий по стоимости (0 = выкл.)")
	fs.Float64Var(&c.EdgeSmoothK, "edge-smooth-k", float64(d.EdgeSmoothSharpness), "резкость edge-aware сглаживания потока (больше = меньше размытие через границы)")
	fs.Float64Var(&c.StaticDiffThr, "static-diff-thr", float64(d.StaticDiffThreshold), "порог различия яркости для счётчика статичности (HUD-маска)")
	fs.Float64Var(&c.StaticMaxCnt, "static-max-count", float64(d.StaticMaxCount), "число подряд неизменных пар кадров до полного подавления потока (HUD-маска)")

	fs.StringVar(&c.View, "view", "overlay", "вывод: overlay (поверх игры) | window (обычное окно)")
	fs.BoolVar(&c.Layered, "layered", true, "overlay: клики проходят насквозь (layered+transparent)")
	fs.BoolVar(&c.VSync, "vsync", true, "Present с vsync")
	fs.BoolVar(&c.Tearing, "tearing", false, "разрешить tearing при -vsync=false")
	fs.BoolVar(&c.Marker, "marker", false, "маркер в углу: зелёный — реальный кадр, пурпурный — сгенерированный")
	fs.Float64Var(&c.Offset, "pacing-offset", 0, "сдвиг расписания показа (доля интервала источника)")
	fs.BoolVar(&c.AdaptiveOffset, "adaptive-offset", false, "сдвиг расписания вычислять автоматически по дрожанию источника (игнорирует -pacing-offset)")

	fs.BoolVar(&c.Debug, "debug", false, "debug-слой D3D11 (нужен компонент Graphics Tools)")
	fs.BoolVar(&c.ShaderDebug, "shader-debug", false, "компилировать шейдеры без оптимизаций, с отладочной информацией")
	fs.BoolVar(&c.HotReload, "hot-reload", true, "перекомпилировать изменённые .hlsl на лету")
	fs.StringVar(&c.Shaders, "shaders", "", "каталог шейдеров (по умолчанию ./shaders или рядом с exe)")
	fs.BoolVar(&c.Profile, "profile", true, "замер времени проходов на GPU")
	fs.BoolVar(&c.HighPriority, "high-priority", true, "повышенный приоритет главного потока")
	fs.IntVar(&c.Dump, "dump", 0, "сохранить N первых пар кадров (prev/next/gen/flow) в -dumpdir")
	fs.StringVar(&c.DumpDir, "dumpdir", "dumps", "каталог дампов")
	fs.StringVar(&c.Trace, "trace", "", "CSV-трейс каждого Present (анализ: go run ./cmd/tracestat file.csv)")
	fs.IntVar(&c.EvalFrames, "eval", 60, "eval: число пар кадров")
	fs.DurationVar(&c.LogEvery, "log-every", time.Second, "период строки статистики")
	fs.StringVar(&c.LogFile, "logfile", "inbetween.log", "лог-файл (пусто = только консоль)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("лишние аргументы: %v", fs.Args())
	}
	return c, c.validate()
}

func (c *Config) validate() error {
	switch c.Mode {
	case "gui", "live", "eval", "offline", "list":
	default:
		return fmt.Errorf("-mode: %q (ожидалось gui|live|eval|offline|list)", c.Mode)
	}
	switch c.Source {
	case "dda", "synthetic":
	default:
		return fmt.Errorf("-source: %q (ожидалось dda|synthetic)", c.Source)
	}
	switch c.View {
	case "overlay", "window":
	default:
		return fmt.Errorf("-view: %q (ожидалось overlay|window)", c.View)
	}
	if _, err := fg.ParseAlgo(c.Algo); err != nil {
		return err
	}
	w, h, err := ParseSize(c.Size)
	if err != nil {
		return err
	}
	c.W, c.H = w, h
	if c.Mult < 1 || c.Mult > 8 {
		return fmt.Errorf("-mult: %d (1..8)", c.Mult)
	}
	if c.FPS <= 0 {
		return fmt.Errorf("-fps должен быть > 0")
	}
	if c.Rect != "" {
		if _, _, _, _, err := ParseRect(c.Rect); err != nil {
			return err
		}
	}
	if c.Mode == "offline" && c.In == "" {
		return fmt.Errorf("-mode offline требует -in <каталог с PNG>")
	}
	return nil
}

func (c *Config) FGOptions() fg.Options {
	o := fg.DefaultOptions()
	o.Algo, _ = fg.ParseAlgo(c.Algo)
	o.FlowScale = c.FlowScale
	o.MinLevelSize = c.MinLevel
	o.Radius = c.Radius
	o.RefineRadius = c.RefineRadius
	o.RefineIters = c.RefineIters
	o.Reg = float32(c.Reg)
	o.ZeroBias = float32(c.ZeroBias)
	o.OccThreshold = float32(c.OccThr)
	o.OccSharpness = float32(c.OccK)
	o.OccCostThreshold = float32(c.OccCostThr)
	o.OccCostSharpness = float32(c.OccCostK)
	o.EdgeSmoothSharpness = float32(c.EdgeSmoothK)
	o.StaticDiffThreshold = float32(c.StaticDiffThr)
	o.StaticMaxCount = float32(c.StaticMaxCnt)
	return o
}

// ParseSize parses "1280x720".
func ParseSize(s string) (int, int, error) {
	p := strings.Split(strings.ToLower(s), "x")
	if len(p) != 2 {
		return 0, 0, fmt.Errorf("размер %q: ожидалось WxH", s)
	}
	w, err1 := strconv.Atoi(p[0])
	h, err2 := strconv.Atoi(p[1])
	if err1 != nil || err2 != nil || w < 16 || h < 16 || w > 16384 || h > 16384 {
		return 0, 0, fmt.Errorf("размер %q некорректен", s)
	}
	return w, h, nil
}

// ParseRect parses "x,y,w,h".
func ParseRect(s string) (x, y, w, h int, err error) {
	p := strings.Split(s, ",")
	if len(p) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("прямоугольник %q: ожидалось x,y,w,h", s)
	}
	v := make([]int, 4)
	for i := range p {
		if v[i], err = strconv.Atoi(strings.TrimSpace(p[i])); err != nil {
			return 0, 0, 0, 0, fmt.Errorf("прямоугольник %q: %w", s, err)
		}
	}
	if v[2] <= 0 || v[3] <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("прямоугольник %q: пустой", s)
	}
	return v[0], v[1], v[2], v[3], nil
}
