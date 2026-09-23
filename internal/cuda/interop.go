//go:build windows

package cuda

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	procD3D11GetDevice   = nvcuda.NewProc("cuD3D11GetDevice")
	procCtxCreate        = nvcuda.NewProc("cuCtxCreate_v2")
	procCtxDestroy       = nvcuda.NewProc("cuCtxDestroy_v2")
	procCtxSynchronize   = nvcuda.NewProc("cuCtxSynchronize")
	procGraphicsD3D11Reg = nvcuda.NewProc("cuGraphicsD3D11RegisterResource")
	procGraphicsMap      = nvcuda.NewProc("cuGraphicsMapResources")
	procGraphicsUnmap    = nvcuda.NewProc("cuGraphicsUnmapResources")
	procGraphicsSubArray = nvcuda.NewProc("cuGraphicsSubResourceGetMappedArray")
	procGraphicsUnreg    = nvcuda.NewProc("cuGraphicsUnregisterResource")
)

// AvailableInterop reports whether the D3D11 interop functions can be loaded, beyond what
// Available() already checks. Kept separate so a driver missing these (unlikely, but why
// assume) doesn't also fail plain device enumeration.
func AvailableInterop() error {
	procs := []*syscall.LazyProc{
		procD3D11GetDevice, procCtxCreate, procCtxDestroy, procCtxSynchronize,
		procGraphicsD3D11Reg, procGraphicsMap, procGraphicsUnmap, procGraphicsSubArray, procGraphicsUnreg,
	}
	for _, p := range procs {
		if err := p.Find(); err != nil {
			return fmt.Errorf("nvcuda.dll: %s: %w", p.Name, err)
		}
	}
	return nil
}

// GetD3D11Device returns the CUDA device corresponding to a D3D11 adapter (its raw
// IDXGIAdapter COM pointer, e.g. gfx.Device.Adapter.U()).
func GetD3D11Device(adapter uintptr) (Device, error) {
	var dev int32
	r, _, _ := procD3D11GetDevice.Call(uintptr(unsafe.Pointer(&dev)), adapter)
	if err := check(r, "cuD3D11GetDevice"); err != nil {
		return 0, err
	}
	return Device(dev), nil
}

// Context is a CUcontext.
type Context uintptr

// CreateContext creates a CUDA context on dev.
func CreateContext(dev Device) (Context, error) {
	var ctx uintptr
	r, _, _ := procCtxCreate.Call(uintptr(unsafe.Pointer(&ctx)), 0, uintptr(dev))
	if err := check(r, "cuCtxCreate_v2"); err != nil {
		return 0, err
	}
	return Context(ctx), nil
}

// Destroy releases the context.
func (c Context) Destroy() error {
	r, _, _ := procCtxDestroy.Call(uintptr(c))
	return check(r, "cuCtxDestroy_v2")
}

// Synchronize blocks until all preceding work on the current context finishes.
func Synchronize() error {
	r, _, _ := procCtxSynchronize.Call()
	return check(r, "cuCtxSynchronize")
}

// GraphicsResource is a CUgraphicsResource: a D3D11 resource registered for CUDA access.
type GraphicsResource uintptr

// RegisterD3D11Resource registers a D3D11 resource (e.g. gfx.Texture.Tex.U()) — created on
// the same adapter as the current CUDA context — for CUDA access. The caller must
// Unregister it when done.
func RegisterD3D11Resource(resource uintptr) (GraphicsResource, error) {
	var res uintptr
	r, _, _ := procGraphicsD3D11Reg.Call(uintptr(unsafe.Pointer(&res)), resource, 0)
	if err := check(r, "cuGraphicsD3D11RegisterResource"); err != nil {
		return 0, err
	}
	return GraphicsResource(res), nil
}

// Map makes the resource accessible to CUDA on the default stream. Must precede
// MappedArray and be matched with Unmap.
func (g GraphicsResource) Map() error {
	res := uintptr(g)
	r, _, _ := procGraphicsMap.Call(1, uintptr(unsafe.Pointer(&res)), 0)
	return check(r, "cuGraphicsMapResources")
}

// Unmap releases CUDA's access to the resource (D3D11 can use it again).
func (g GraphicsResource) Unmap() error {
	res := uintptr(g)
	r, _, _ := procGraphicsUnmap.Call(1, uintptr(unsafe.Pointer(&res)), 0)
	return check(r, "cuGraphicsUnmapResources")
}

// MappedArray returns the CUarray view of subresource 0, mip 0. Only valid while mapped.
func (g GraphicsResource) MappedArray() (Array, error) {
	var arr uintptr
	r, _, _ := procGraphicsSubArray.Call(uintptr(unsafe.Pointer(&arr)), uintptr(g), 0, 0)
	if err := check(r, "cuGraphicsSubResourceGetMappedArray"); err != nil {
		return 0, err
	}
	return Array(arr), nil
}

// Unregister releases the CUDA registration entirely (not just Unmap).
func (g GraphicsResource) Unregister() error {
	r, _, _ := procGraphicsUnreg.Call(uintptr(g))
	return check(r, "cuGraphicsUnregisterResource")
}

// Array is a CUarray.
type Array uintptr
