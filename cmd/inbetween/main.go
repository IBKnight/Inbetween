//go:build windows

// Command inbetween is the entry point. All modes: see `inbetween -h`.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"inbetween/internal/app"
	"inbetween/internal/config"
)

// The D3D11 immediate context and Win32 windows are thread-affine, so the whole
// runtime lives on the main OS thread.
func init() { runtime.LockOSThread() }

func main() { os.Exit(run()) }

func run() int {
	cfg, err := config.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return 2
	}
	defer app.SetupLog(cfg.LogFile)()

	switch cfg.Mode {
	case "list":
		err = app.RunList(cfg)
	case "eval":
		err = app.RunEval(cfg)
	case "offline":
		err = app.RunOffline(cfg)
	default:
		err = app.RunLive(cfg)
	}
	if err != nil {
		log.Printf("ОШИБКА: %v", err)
		return 1
	}
	return 0
}
