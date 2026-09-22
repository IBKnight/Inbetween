//go:build windows

package gfx

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"inbetween/internal/com"
	"inbetween/internal/win"
)

var (
	modD3D11 = syscall.NewLazyDLL("d3d11.dll")
	modDXGI  = syscall.NewLazyDLL("dxgi.dll")

	procD3D11CreateDevice  = modD3D11.NewProc("D3D11CreateDevice")
	procCreateDXGIFactory1 = modDXGI.NewProc("CreateDXGIFactory1")
)

// Device is the D3D11 device, immediate context, and DXGI factory. Used from a SINGLE thread.
type Device struct {
	Dev     com.Ptr // ID3D11Device
	Ctx     com.Ptr // ID3D11DeviceContext (immediate)
	Factory com.Ptr // IDXGIFactory2
	Adapter com.Ptr // IDXGIAdapter1

	AdapterIndex   int
	AdapterName    string
	FeatureLevel   uint32
	TearingSupport bool
	DebugLayer     bool

	// Prof, if non-nil, makes RunCompute/DrawFullscreen automatically time each pass on the GPU.
	Prof *Profiler

	info    com.Ptr // ID3D11InfoQueue (only with -debug)
	staging map[stagingKey]*Texture
}

// OutputInfo describes a monitor attached to an adapter.
type OutputInfo struct {
	Adapter, Output int
	AdapterName     string
	DeviceName      string
	Desktop         win.RECT // coordinates on the virtual desktop
	Rotation        uint32
}

func (o OutputInfo) String() string {
	return fmt.Sprintf("adapter %d output %d: %s %v [%s]", o.Adapter, o.Output, o.DeviceName, o.Desktop, o.AdapterName)
}

func createFactory() (com.Ptr, error) {
	var f unsafe.Pointer
	r, _, _ := procCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&IID_IDXGIFactory1)), uintptr(unsafe.Pointer(&f)))
	if err := com.Check(r, "CreateDXGIFactory1"); err != nil {
		return com.Ptr{}, err
	}
	return com.FromRaw(f), nil
}

func enumAdapter(f com.Ptr, i int) (com.Ptr, bool) {
	var a unsafe.Pointer
	r := f.Call(factoryEnumAdapters1, uintptr(i), uintptr(unsafe.Pointer(&a)))
	if com.HR(r).Failed() {
		return com.Ptr{}, false
	}
	return com.FromRaw(a), true
}

func adapterName(a com.Ptr) string {
	var d AdapterDesc1
	a.Call(adapterGetDesc1, uintptr(unsafe.Pointer(&d)))
	return strings.TrimSpace(syscall.UTF16ToString(d.Description[:]))
}

func enumOutput(a com.Ptr, i int) (com.Ptr, OutputDesc, bool) {
	var o unsafe.Pointer
	r := a.Call(adapterEnumOutputs, uintptr(i), uintptr(unsafe.Pointer(&o)))
	if com.HR(r).Failed() {
		return com.Ptr{}, OutputDesc{}, false
	}
	out := com.FromRaw(o)
	var d OutputDesc
	out.Call(outputGetDesc, uintptr(unsafe.Pointer(&d)))
	return out, d, true
}

// ListOutputs enumerates every monitor on every adapter.
func ListOutputs() ([]OutputInfo, error) {
	f, err := createFactory()
	if err != nil {
		return nil, err
	}
	defer f.Release()
	var res []OutputInfo
	for ai := 0; ; ai++ {
		a, ok := enumAdapter(f, ai)
		if !ok {
			break
		}
		name := adapterName(a)
		for oi := 0; ; oi++ {
			out, d, ok := enumOutput(a, oi)
			if !ok {
				break
			}
			res = append(res, OutputInfo{
				Adapter: ai, Output: oi, AdapterName: name,
				DeviceName: syscall.UTF16ToString(d.DeviceName[:]),
				Desktop:    d.DesktopCoordinates, Rotation: d.Rotation,
			})
			out.Release()
		}
		a.Release()
	}
	return res, nil
}

// FindOutput returns the monitor containing the center of rectangle r (screen coordinates).
func FindOutput(r win.RECT) (OutputInfo, error) {
	outs, err := ListOutputs()
	if err != nil {
		return OutputInfo{}, err
	}
	cx, cy := r.Left+int32(r.W()/2), r.Top+int32(r.H()/2)
	for _, o := range outs {
		if o.Desktop.Contains(cx, cy) {
			return o, nil
		}
	}
	return OutputInfo{}, fmt.Errorf("ни один монитор не содержит точку (%d,%d)", cx, cy)
}

// NewDevice creates a device on adapter adapterIndex (DXGI order; 0 is the primary one).
// For Desktop Duplication, the device MUST be on the adapter the monitor is attached to.
func NewDevice(adapterIndex int, debug bool) (*Device, error) {
	f1, err := createFactory()
	if err != nil {
		return nil, err
	}
	f2, err := f1.QueryInterface(&IID_IDXGIFactory2)
	f1.Release()
	if err != nil {
		return nil, fmt.Errorf("нужен DXGI 1.2+ (Windows 8+): %w", err)
	}
	a, ok := enumAdapter(f2, adapterIndex)
	if !ok {
		f2.Release()
		return nil, fmt.Errorf("адаптер %d не найден", adapterIndex)
	}
	d := &Device{Factory: f2, Adapter: a, AdapterIndex: adapterIndex, AdapterName: adapterName(a), staging: map[stagingKey]*Texture{}}

	create := func(flags uint32) error {
		levels := [2]uint32{featureLevel11_1, featureLevel11_0}
		var dev, ctx unsafe.Pointer
		var fl uint32
		r, _, _ := procD3D11CreateDevice.Call(a.U(), driverTypeUnknown, 0, uintptr(flags),
			uintptr(unsafe.Pointer(&levels[0])), uintptr(len(levels)), d3d11SDKVersion,
			uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(&fl)), uintptr(unsafe.Pointer(&ctx)))
		if err := com.Check(r, "D3D11CreateDevice"); err != nil {
			return err
		}
		d.Dev, d.Ctx, d.FeatureLevel = com.FromRaw(dev), com.FromRaw(ctx), fl
		return nil
	}
	flags := uint32(createDeviceBGRA)
	if debug {
		if err := create(flags | createDeviceDebug); err != nil {
			// The debug layer requires the Windows "Graphics Tools" optional feature.
			fmt.Printf("warning: debug layer недоступен (%v) — Settings > Optional features > Graphics Tools\n", err)
		} else {
			d.DebugLayer = true
		}
	}
	if d.Dev.Nil() {
		if err := create(flags); err != nil {
			d.Close()
			return nil, err
		}
	}

	// Don't let the driver queue frames ahead of us: keep latency minimal.
	if dx, err := d.Dev.QueryInterface(&IID_IDXGIDevice1); err == nil {
		dx.Call(dxgiDevice1SetMaximumFrameLatency, 1)
		dx.Release()
	}
	// Tearing support (VRR / no-vsync in windowed mode).
	if f5, err := d.Factory.QueryInterface(&IID_IDXGIFactory5); err == nil {
		var allow int32
		r := f5.Call(factory5CheckFeatureSupport, featurePresentAllowTearing, uintptr(unsafe.Pointer(&allow)), 4)
		d.TearingSupport = !com.HR(r).Failed() && allow != 0
		f5.Release()
	}
	if d.DebugLayer {
		if iq, err := d.Dev.QueryInterface(&IID_ID3D11InfoQueue); err == nil {
			d.info = iq
			iq.Call(infoSetMessageCountLimit, 4096)
		}
	}
	return d, nil
}

// OpenOutput returns the IDXGIOutput for the monitor at index on the device's adapter.
func (d *Device) OpenOutput(index int) (com.Ptr, OutputDesc, error) {
	o, desc, ok := enumOutput(d.Adapter, index)
	if !ok {
		return com.Ptr{}, OutputDesc{}, fmt.Errorf("output %d не найден на адаптере %d", index, d.AdapterIndex)
	}
	return o, desc, nil
}

// SetGPUThreadPriority sets this device's GPU scheduling priority (-7..7). May require privileges.
func (d *Device) SetGPUThreadPriority(p int) error {
	dx, err := d.Dev.QueryInterface(&IID_IDXGIDevice1)
	if err != nil {
		return err
	}
	defer dx.Release()
	return com.Check(dx.Call(dxgiDeviceSetGPUThreadPriority, uintptr(int32(p))), "SetGPUThreadPriority")
}

// RemovedReason returns why the device was lost (nil if everything's fine).
func (d *Device) RemovedReason() error {
	return com.Check(d.Dev.Call(devGetDeviceRemovedReason), "device removed")
}

func (d *Device) Flush() { d.Ctx.Call(ctxFlush) }

func (d *Device) Close() {
	for k, t := range d.staging {
		t.Release()
		delete(d.staging, k)
	}
	if d.Prof != nil {
		d.Prof.Release()
		d.Prof = nil
	}
	d.info.Release()
	if !d.Ctx.Nil() {
		d.Ctx.Call(ctxClearState)
		d.Ctx.Call(ctxFlush)
	}
	d.Ctx.Release()
	d.Dev.Release()
	d.Adapter.Release()
	d.Factory.Release()
}
