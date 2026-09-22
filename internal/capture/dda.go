//go:build windows

package capture

import (
	"errors"
	"fmt"
	"log"
	"time"
	"unsafe"

	"inbetween/internal/com"
	"inbetween/internal/gfx"
	"inbetween/internal/win"
)

// vtable indices for IDXGIOutput1/IDXGIOutputDuplication, duplicated from gfx's own
// unexported constants so this package stays self-contained.
const (
	vtOutput1DuplicateOutput   = 22
	vtDuplGetDesc              = 7
	vtDuplAcquireNextFrame     = 8
	vtDuplGetFrameDirtyRects   = 9
	vtDuplReleaseFrame         = 14
	recreateIntervalAfterLoss  = 500 * time.Millisecond
	defaultDirtyRectBufferSize = 64
)

// DDA captures a monitor via DXGI Desktop Duplication, cropped to the requested rectangle.
//
// Things to keep in mind:
//   - the device must be on the adapter the monitor is attached to;
//   - a frame only arrives when the desktop actually updated (LastPresentTime != 0);
//   - DXGI_ERROR_ACCESS_LOST (mode switch, UAC, fullscreen transition) means we recreate
//     the duplicator;
//   - our own overlay is excluded from capture via WDA_EXCLUDEFROMCAPTURE (see
//     win.Window.ExcludeFromCapture).
type DDA struct {
	d       *gfx.Device
	output  com.Ptr // IDXGIOutput1
	dup     com.Ptr // IDXGIOutputDuplication
	desktop win.RECT
	crop    win.RECT // in monitor coordinates
	tex     *gfx.Texture
	seq     uint64
	lostAt  time.Time
	dirty   []win.RECT

	// FilterDirty skips updates whose dirty rectangles don't touch crop (e.g. a blinking
	// cursor/clock in another corner of the screen, or our own overlay updating).
	FilterDirty bool
	// Skipped counts updates filtered out via dirty rects.
	Skipped int
}

// NewDDA creates a capture of monitor outputIndex (on device d's adapter) for the
// rectangle screen (screen coordinates; clipped to the monitor's bounds).
func NewDDA(d *gfx.Device, outputIndex int, screen win.RECT) (*DDA, error) {
	out, desc, err := d.OpenOutput(outputIndex)
	if err != nil {
		return nil, err
	}
	out1, err := out.QueryInterface(&gfx.IID_IDXGIOutput1)
	out.Release()
	if err != nil {
		return nil, fmt.Errorf("IDXGIOutput1: %w", err)
	}
	c := &DDA{d: d, output: out1, desktop: desc.DesktopCoordinates, FilterDirty: true}
	crop := screen.Intersect(desc.DesktopCoordinates)
	if crop.Empty() {
		c.Close()
		return nil, fmt.Errorf("прямоугольник %v не пересекается с монитором %v", screen, desc.DesktopCoordinates)
	}
	c.crop = crop.Offset(-desc.DesktopCoordinates.Left, -desc.DesktopCoordinates.Top)
	if desc.Rotation != gfx.ModeRotationIdentity && desc.Rotation != 0 {
		log.Printf("warning: монитор повёрнут (rotation=%d) — захват повёрнутых мониторов не реализован", desc.Rotation)
	}
	if err := c.duplicate(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *DDA) duplicate() error {
	var dup unsafe.Pointer
	r := c.output.Call(vtOutput1DuplicateOutput, c.d.Dev.U(), uintptr(unsafe.Pointer(&dup)))
	if err := com.Check(r, "DuplicateOutput"); err != nil {
		switch {
		case errors.Is(err, com.DXGI_ERROR_UNSUPPORTED):
			return fmt.Errorf("%w (гибридная графика? устройство должно быть на адаптере монитора)", err)
		case errors.Is(err, com.DXGI_ERROR_NOT_CURRENTLY_AVAILABLE):
			return fmt.Errorf("%w (слишком много приложений захватывают экран)", err)
		case errors.Is(err, com.E_ACCESSDENIED):
			return fmt.Errorf("%w (защищённый рабочий стол: UAC/экран блокировки)", err)
		}
		return err
	}
	c.dup = com.FromRaw(dup)
	var dd gfx.OutDuplDesc
	c.dup.Call(vtDuplGetDesc, uintptr(unsafe.Pointer(&dd)))
	if dd.DesktopImageInSystemMemory != 0 {
		log.Printf("warning: DesktopImageInSystemMemory=1 — нужен путь через MapDesktopSurface (не реализован)")
	}
	format := dd.ModeDesc.Format
	if format == 0 {
		format = gfx.FormatBGRA8
	}
	if c.tex == nil || c.tex.Format != format {
		c.tex.Release()
		t, err := c.d.NewTexture("dda_crop", c.crop.W(), c.crop.H(), format, gfx.BindShaderResource)
		if err != nil {
			return err
		}
		c.tex = t
	}
	return nil
}

// ScreenRect returns the actual capture area in screen coordinates (after clipping to the monitor).
func (c *DDA) ScreenRect() win.RECT { return c.crop.Offset(c.desktop.Left, c.desktop.Top) }

func (c *DDA) Name() string     { return fmt.Sprintf("dda crop=%v", c.crop) }
func (c *DDA) Size() (int, int) { return c.crop.W(), c.crop.H() }

// Poll waits for a desktop update for at most timeout (millisecond precision).
func (c *DDA) Poll(timeout time.Duration) (Frame, bool, error) {
	if c.dup.Nil() {
		if time.Since(c.lostAt) < recreateIntervalAfterLoss {
			time.Sleep(min(timeout, 2*time.Millisecond))
			return Frame{}, false, nil
		}
		if err := c.duplicate(); err != nil {
			log.Printf("DDA: повторное создание не удалось: %v", err)
			c.lostAt = time.Now()
			return Frame{}, false, nil
		}
		log.Printf("DDA: дубликатор пересоздан")
	}
	ms := uint32(timeout / time.Millisecond)
	var info gfx.OutDuplFrameInfo
	var res unsafe.Pointer
	r := c.dup.Call(vtDuplAcquireNextFrame, uintptr(ms), uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&res)))
	switch hr := com.HR(r); {
	case hr == com.DXGI_ERROR_WAIT_TIMEOUT:
		return Frame{}, false, nil
	case hr == com.DXGI_ERROR_ACCESS_LOST:
		log.Printf("DDA: ACCESS_LOST — пересоздаём дубликатор")
		c.dup.Release()
		c.lostAt = time.Now()
		return Frame{}, false, nil
	case hr.Failed():
		return Frame{}, false, fmt.Errorf("AcquireNextFrame: %w", hr)
	}
	defer c.dup.Call(vtDuplReleaseFrame)
	resource := com.FromRaw(res)
	defer resource.Release()

	if info.LastPresentTime == 0 {
		return Frame{}, false, nil // only the cursor moved
	}
	if c.FilterDirty && info.TotalMetadataBufferSize > 0 && !c.cropIsDirty() {
		c.Skipped++
		return Frame{}, false, nil
	}
	tex, err := resource.QueryInterface(&gfx.IID_ID3D11Texture2D)
	if err != nil {
		return Frame{}, false, err
	}
	defer tex.Release()
	box := gfx.Box{
		Left: uint32(c.crop.Left), Top: uint32(c.crop.Top), Front: 0,
		Right: uint32(c.crop.Right), Bottom: uint32(c.crop.Bottom), Back: 1,
	}
	c.d.CopyRegion(c.tex, 0, 0, tex, box)
	c.seq++
	missed := int(info.AccumulatedFrames) - 1
	if missed < 0 {
		missed = 0
	}
	return Frame{Tex: c.tex, Time: info.LastPresentTime, Seq: c.seq, Missed: missed}, true, nil
}

// cropIsDirty reports whether any of the frame's dirty rectangles intersect our area.
// On any error we assume yes (a spurious frame beats a dropped one).
func (c *DDA) cropIsDirty() bool {
	if len(c.dirty) == 0 {
		c.dirty = make([]win.RECT, defaultDirtyRectBufferSize)
	}
	for attempt := 0; attempt < 2; attempt++ {
		var required uint32
		size := uint32(len(c.dirty)) * uint32(unsafe.Sizeof(win.RECT{}))
		r := c.dup.Call(vtDuplGetFrameDirtyRects, uintptr(size), uintptr(unsafe.Pointer(&c.dirty[0])), uintptr(unsafe.Pointer(&required)))
		hr := com.HR(r)
		if hr.Failed() {
			if required > size {
				c.dirty = make([]win.RECT, int(required)/int(unsafe.Sizeof(win.RECT{}))+1)
				continue
			}
			return true
		}
		n := int(required) / int(unsafe.Sizeof(win.RECT{}))
		for _, rc := range c.dirty[:n] {
			if !rc.Intersect(c.crop).Empty() {
				return true
			}
		}
		return n == 0 // no dirty rects at all — unknown, treat it as an update
	}
	return true
}

func (c *DDA) Close() {
	c.dup.Release()
	c.output.Release()
	c.tex.Release()
	c.tex = nil
}
