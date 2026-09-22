//go:build windows

package app

import (
	"fmt"

	"inbetween/internal/config"
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
	return nil
}
