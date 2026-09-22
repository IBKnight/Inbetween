//go:build windows

package gfx

import (
	"unsafe"

	"inbetween/internal/com"
	"inbetween/internal/win"
)

// SwapChain is the output window's flip-model swapchain.
type SwapChain struct {
	d        *Device
	SC       com.Ptr // IDXGISwapChain1
	SC2      com.Ptr // IDXGISwapChain2 (may be nil)
	Waitable uintptr // frame latency waitable object (0 = none)
	Back     *Texture
	W, H     int
	Tearing  bool
	flags    uint32
}

// NewSwapChain creates a FLIP_DISCARD swapchain (BGRA8, 2 buffers, max latency 1).
func (d *Device) NewSwapChain(hwnd uintptr, w, h int, allowTearing bool) (*SwapChain, error) {
	flags := swapChainFlagFrameLatencyWaitable
	tearing := allowTearing && d.TearingSupport
	if tearing {
		flags |= swapChainFlagAllowTearing
	}
	desc := SwapChainDesc1{
		Width: uint32(w), Height: uint32(h), Format: FormatBGRA8, SampleDesc: SampleDesc{Count: 1},
		BufferUsage: usageRenderTargetOutput, BufferCount: 2, Scaling: scalingStretch,
		SwapEffect: swapEffectFlipDiscard, AlphaMode: alphaModeUnspecified, Flags: flags,
	}
	var sc unsafe.Pointer
	r := d.Factory.Call(factory2CreateSwapChainForHwnd, d.Dev.U(), hwnd, uintptr(unsafe.Pointer(&desc)), 0, 0, uintptr(unsafe.Pointer(&sc)))
	if err := com.Check(r, "CreateSwapChainForHwnd"); err != nil {
		return nil, err
	}
	d.Factory.Call(factoryMakeWindowAssociation, hwnd, uintptr(mwaNoAltEnter))
	s := &SwapChain{d: d, SC: com.FromRaw(sc), W: w, H: h, Tearing: tearing, flags: flags}
	if sc2, err := s.SC.QueryInterface(&IID_IDXGISwapChain2); err == nil {
		s.SC2 = sc2
		sc2.Call(sc2SetMaximumFrameLatency, 1)
		s.Waitable = sc2.Call(sc2GetFrameLatencyWaitableObject)
	}
	if err := s.createBack(); err != nil {
		s.Release()
		return nil, err
	}
	return s, nil
}

func (s *SwapChain) createBack() error {
	var tex unsafe.Pointer
	r := s.SC.Call(scGetBuffer, 0, uintptr(unsafe.Pointer(&IID_ID3D11Texture2D)), uintptr(unsafe.Pointer(&tex)))
	if err := com.Check(r, "SwapChain.GetBuffer"); err != nil {
		return err
	}
	t := &Texture{Name: "backbuffer", Tex: com.FromRaw(tex), W: s.W, H: s.H, Format: FormatBGRA8}
	if err := s.d.createViews(t, BindRenderTarget); err != nil {
		t.Release()
		return err
	}
	s.Back = t
	return nil
}

// Ready reports whether the next frame can be rendered without blocking in Present (the
// frame latency waitable object is signaled). Each true "spends" one slot — call it only
// right before rendering and Present.
func (s *SwapChain) Ready() bool {
	if s.Waitable == 0 {
		return true
	}
	return win.WaitForSingleObject(s.Waitable, 0) == win.WAIT_OBJECT_0
}

// Present shows the back buffer. vsync=false skips waiting for vblank (with tearing, if supported).
func (s *SwapChain) Present(vsync bool) error {
	sync, flags := uintptr(1), uintptr(0)
	if !vsync {
		sync = 0
		if s.Tearing {
			flags = uintptr(presentAllowTearing)
		}
	}
	return com.Check(s.SC.Call(scPresent, sync, flags), "Present")
}

// Stats returns present statistics (SyncQPCTime is the QPC of the last present's actual vblank).
func (s *SwapChain) Stats() (FrameStatistics, error) {
	var st FrameStatistics
	r := s.SC.Call(scGetFrameStatistics, uintptr(unsafe.Pointer(&st)))
	return st, com.Check(r, "GetFrameStatistics")
}

func (s *SwapChain) Resize(w, h int) error {
	s.Back.Release()
	s.Back = nil
	s.d.Ctx.Call(ctxClearState)
	r := s.SC.Call(scResizeBuffers, 0, uintptr(w), uintptr(h), uintptr(FormatUnknown), uintptr(s.flags))
	if err := com.Check(r, "ResizeBuffers"); err != nil {
		return err
	}
	s.W, s.H = w, h
	return s.createBack()
}

func (s *SwapChain) Release() {
	s.Back.Release()
	win.CloseHandle(s.Waitable)
	s.Waitable = 0
	s.SC2.Release()
	s.SC.Release()
}
