//go:build windows

package app

import (
	"fmt"

	"inbetween/internal/config"
	"inbetween/internal/cuda"
	"inbetween/internal/gfx"
	"inbetween/internal/win"
)

func RunList(*config.Config) error {
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
	printCUDA()
	return nil
}

// printCUDA is a diagnostic checkpoint for Stage 4 (hardware optical flow, in progress —
// see CLAUDE.md): confirms nvcuda.dll loads and the driver enumerates a device, before
// any interop/NVOFA work depends on it. Never fails RunList — hardware flow is optional.
func printCUDA() {
	fmt.Println("\nCUDA (для аппаратного optical flow, см. Stage 4 в CLAUDE.md):")
	if err := cuda.Init(); err != nil {
		fmt.Printf("  недоступно: %v\n", err)
		return
	}
	n, err := cuda.DeviceCount()
	if err != nil {
		fmt.Printf("  cuDeviceGetCount: %v\n", err)
		return
	}
	if n == 0 {
		fmt.Println("  устройств не найдено")
		return
	}
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
	}
}
