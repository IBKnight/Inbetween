//go:build windows

package app

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"inbetween/internal/config"
	"inbetween/internal/gfx"
)

func SetupLog(path string) func() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("warning: лог-файл %q: %v", path, err)
		return func() {}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return func() { f.Close() }
}

type env struct {
	cfg  *config.Config
	dev  *gfx.Device
	lib  *gfx.ShaderLib
	prof *gfx.Profiler
}

func newEnv(cfg *config.Config, adapter int) (*env, error) {
	dev, err := gfx.NewDevice(adapter, cfg.Debug)
	if err != nil {
		return nil, err
	}
	log.Printf("GPU: %s (adapter %d), feature level 0x%x, tearing=%v, debug layer=%v",
		dev.AdapterName, adapter, dev.FeatureLevel, dev.TearingSupport, dev.DebugLayer)
	dir := shaderDir(cfg)
	lib, err := gfx.NewShaderLib(dev, dir, cfg.ShaderDebug)
	if err != nil {
		dev.Close()
		return nil, err
	}
	lib.Warn = log.Printf
	log.Printf("шейдеры: %s (hot reload=%v)", dir, cfg.HotReload)
	e := &env{cfg: cfg, dev: dev, lib: lib}
	if cfg.Profile {
		if e.prof, err = dev.NewProfiler(); err != nil {
			log.Printf("warning: профайлер GPU недоступен: %v", err)
		}
	}
	return e, nil
}

// beginGPU/endGPU bracket the profiler's "frame"; both are no-ops when there's no profiler.
func (e *env) beginGPU() {
	if e.prof != nil {
		e.prof.BeginFrame()
	}
}

func (e *env) endGPU() {
	if e.prof != nil {
		e.prof.EndFrame()
	}
}

func (e *env) gpuSummary() string {
	if e.prof == nil {
		return "-"
	}
	return e.prof.Summary()
}

func (e *env) reloadShaders() {
	n, errs := e.lib.ReloadChanged()
	for _, err := range errs {
		log.Printf("hot reload: %v", err)
	}
	if n > 0 {
		log.Printf("hot reload: перекомпилировано шейдеров: %d", n)
	}
}

func (e *env) drainDebug() { e.dev.DrainDebugMessages(log.Printf) }

func (e *env) Close() {
	e.drainDebug()
	e.lib.Release()
	e.dev.Close()
}

// shaderDir resolves the shader directory: -shaders, then ./shaders, then next to the
// executable (and one level up, to cover a bin/ layout).
func shaderDir(cfg *config.Config) string {
	if cfg.Shaders != "" {
		return cfg.Shaders
	}
	cands := []string{"shaders"}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, filepath.Join(d, "shaders"), filepath.Join(d, "..", "shaders"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "shaders"
}
