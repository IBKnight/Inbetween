//go:build windows

// Package cuda is a minimal binding to the CUDA Driver API (nvcuda.dll) — device
// enumeration for now, D3D11 interop later (see CLAUDE.md Stage 4, hardware optical flow
// via NVIDIA's OFA through nvofapi64.dll). No CUDA Toolkit and no cgo: nvcuda.dll ships
// with the NVIDIA display driver itself and is called the same way every other Windows
// DLL in this project is — raw syscall. Unlike D3D11/DXGI, the CUDA driver API is plain C
// exports, not COM, so there are no vtables here, just NewProc/Call.
//
// nvcuda.dll only exists with an NVIDIA driver installed, so every entry point here
// reports a clean error instead of panicking when the DLL or a proc is missing — this
// package must fail soft, since hardware optical flow is meant to be optional (see
// CLAUDE.md: "keeping the shader-based one as a fallback").
package cuda

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Numeric CUresult codes worth naming (the rest just print as a number — see
// https://docs.nvidia.com/cuda/cuda-driver-api/group__CUDA__TYPES.html for the full list).
var knownErrors = map[Result]string{
	1:   "CUDA_ERROR_INVALID_VALUE",
	2:   "CUDA_ERROR_OUT_OF_MEMORY",
	3:   "CUDA_ERROR_NOT_INITIALIZED",
	100: "CUDA_ERROR_NO_DEVICE",
	101: "CUDA_ERROR_INVALID_DEVICE",
}

// Result mirrors CUresult (a plain C int).
type Result int32

const success Result = 0

var (
	nvcuda = syscall.NewLazyDLL("nvcuda.dll")

	procInit           = nvcuda.NewProc("cuInit")
	procDeviceGetCount = nvcuda.NewProc("cuDeviceGetCount")
	procDeviceGet      = nvcuda.NewProc("cuDeviceGet")
	procDeviceGetName  = nvcuda.NewProc("cuDeviceGetName")
)

// Available reports whether nvcuda.dll and the functions this package needs can be
// loaded, without calling into any of them. Callers should check this (or just handle a
// non-nil error from Init) before relying on hardware optical flow, and fall back to the
// shader-based flow otherwise.
func Available() error {
	if err := nvcuda.Load(); err != nil {
		return fmt.Errorf("nvcuda.dll: %w", err)
	}
	for _, p := range []*syscall.LazyProc{procInit, procDeviceGetCount, procDeviceGet, procDeviceGetName} {
		if err := p.Find(); err != nil {
			return fmt.Errorf("nvcuda.dll: %s: %w", p.Name, err)
		}
	}
	return nil
}

func check(r uintptr, what string) error {
	res := Result(r)
	if res == success {
		return nil
	}
	if name, ok := knownErrors[res]; ok {
		return fmt.Errorf("%s: %s (%d)", what, name, res)
	}
	return fmt.Errorf("%s: cuda error %d", what, res)
}

// Init must be called once, before any other function in this package.
func Init() error {
	if err := Available(); err != nil {
		return err
	}
	r, _, _ := procInit.Call(0)
	return check(r, "cuInit")
}

// Device is a CUDA device ordinal (not the same numbering as a DXGI adapter index).
type Device int32

// DeviceCount returns the number of CUDA-capable devices.
func DeviceCount() (int, error) {
	var n int32
	r, _, _ := procDeviceGetCount.Call(uintptr(unsafe.Pointer(&n)))
	if err := check(r, "cuDeviceGetCount"); err != nil {
		return 0, err
	}
	return int(n), nil
}

// GetDevice returns the device handle for ordinal (0..DeviceCount()-1).
func GetDevice(ordinal int) (Device, error) {
	var dev int32
	r, _, _ := procDeviceGet.Call(uintptr(unsafe.Pointer(&dev)), uintptr(ordinal))
	if err := check(r, "cuDeviceGet"); err != nil {
		return 0, err
	}
	return Device(dev), nil
}

// Name returns the device's name string (e.g. "NVIDIA GeForce RTX 3060").
func (d Device) Name() (string, error) {
	buf := make([]byte, 256)
	r, _, _ := procDeviceGetName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(d))
	if err := check(r, "cuDeviceGetName"); err != nil {
		return "", err
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n]), nil
}
