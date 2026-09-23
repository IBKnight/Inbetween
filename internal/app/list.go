//go:build windows

package app

import (
	"fmt"

	"inbetween/internal/config"
	"inbetween/internal/cuda"
	"inbetween/internal/gfx"
	"inbetween/internal/win"
)

func RunList(cfg *config.Config) error {
	win.SetDPIAware()
	outs, err := gfx.ListOutputs()
	if err != nil {
		return err
	}
	fmt.Println("Мониторы (номер — для -monitor N):")
	for i, o := range outs {
		fmt.Printf("  [%d] %v\n", i, o)
	}
	fmt.Println("\nОкна (подстрока заголовка — для -window \"...\"):")
	for _, w := range win.ListWindows() {
		fmt.Printf("  %v\n", w)
	}
	if printCUDA() {
		printCUDAInterop(cfg)
	}
	return nil
}

// printCUDA is a diagnostic checkpoint for Stage 4 (hardware optical flow, in progress —
// see CLAUDE.md): confirms nvcuda.dll loads and the driver enumerates a device, before
// any interop/NVOFA work depends on it. Never fails RunList — hardware flow is optional.
// Reports whether at least one device was found (worth then trying interop).
func printCUDA() bool {
	fmt.Println("\nCUDA (для аппаратного optical flow, см. Stage 4 в CLAUDE.md):")
	if err := cuda.Init(); err != nil {
		fmt.Printf("  недоступно: %v\n", err)
		return false
	}
	n, err := cuda.DeviceCount()
	if err != nil {
		fmt.Printf("  cuDeviceGetCount: %v\n", err)
		return false
	}
	if n == 0 {
		fmt.Println("  устройств не найдено")
		return false
	}
	found := false
	for i := 0; i < n; i++ {
		dev, err := cuda.GetDevice(i)
		if err != nil {
			fmt.Printf("  [%d] cuDeviceGet: %v\n", i, err)
			continue
		}
		name, err := dev.Name()
		if err != nil {
			fmt.Printf("  [%d] cuDeviceGetName: %v\n", i, err)
			continue
		}
		fmt.Printf("  [%d] %s\n", i, name)
		found = true
	}
	return found
}

// printCUDAInterop is the next checkpoint after printCUDA: registers a throwaway D3D11
// texture (on the same adapter live/eval mode would actually use) as a CUDA graphics
// resource and maps it, without doing anything with the content. Validates the whole
// D3D11<->CUDA interop chain before any NVOFA work depends on it.
func printCUDAInterop(cfg *config.Config) {
	adapterIndex := cfg.Adapter
	if adapterIndex < 0 {
		adapterIndex = 0
	}
	if err := cuda.AvailableInterop(); err != nil {
		fmt.Printf("  interop: %v\n", err)
		return
	}
	d, err := gfx.NewDevice(adapterIndex, false)
	if err != nil {
		fmt.Printf("  interop: D3D11-устройство на адаптере %d: %v\n", adapterIndex, err)
		return
	}
	defer d.Close()

	cdev, err := cuda.GetD3D11Device(d.Adapter.U())
	if err != nil {
		fmt.Printf("  interop: cuD3D11GetDevice: %v\n", err)
		return
	}
	ctx, err := cuda.CreateContext(cdev)
	if err != nil {
		fmt.Printf("  interop: cuCtxCreate: %v\n", err)
		return
	}
	defer ctx.Destroy()

	tex, err := d.NewTexture("cuda-interop-test", 64, 64, gfx.FormatRGBA8, gfx.BindShaderResource|gfx.BindUnorderedAccess)
	if err != nil {
		fmt.Printf("  interop: тестовая текстура: %v\n", err)
		return
	}
	defer tex.Release()

	res, err := cuda.RegisterD3D11Resource(tex.Tex.U())
	if err != nil {
		fmt.Printf("  interop: cuGraphicsD3D11RegisterResource: %v\n", err)
		return
	}
	defer res.Unregister()

	if err := res.Map(); err != nil {
		fmt.Printf("  interop: cuGraphicsMapResources: %v\n", err)
		return
	}
	defer res.Unmap()

	if _, err := res.MappedArray(); err != nil {
		fmt.Printf("  interop: cuGraphicsSubResourceGetMappedArray: %v\n", err)
		return
	}

	fmt.Println("  interop: OK (текстура зарегистрирована и замаплена в CUDA)")
}
