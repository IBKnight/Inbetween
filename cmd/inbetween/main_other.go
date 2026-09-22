//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "inbetween работает только на Windows (D3D11/DXGI). Для логики без GPU: go test ./...")
	os.Exit(1)
}
