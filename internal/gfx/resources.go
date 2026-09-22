//go:build windows

package gfx

import (
	"fmt"
	"math"
	"unsafe"

	"inbetween/internal/com"
)

// Texture is a 2D texture and its views (created based on the bind flags).
type Texture struct {
	Name   string
	Tex    com.Ptr // ID3D11Texture2D
	SRV    com.Ptr // ID3D11ShaderResourceView (if BindShaderResource)
	UAV    com.Ptr // ID3D11UnorderedAccessView (if BindUnorderedAccess)
	RTV    com.Ptr // ID3D11RenderTargetView (if BindRenderTarget)
	W, H   int
	Format uint32
}

func (t *Texture) String() string {
	if t == nil {
		return "<nil texture>"
	}
	return fmt.Sprintf("%s %dx%d fmt=%d", t.Name, t.W, t.H, t.Format)
}

func (t *Texture) Size() [2]uint32 { return [2]uint32{uint32(t.W), uint32(t.H)} }

func (t *Texture) Release() {
	if t == nil {
		return
	}
	t.SRV.Release()
	t.UAV.Release()
	t.RTV.Release()
	t.Tex.Release()
}

// NewTexture creates a texture (usage DEFAULT, 1 mip) and its views per the bind flags.
func (d *Device) NewTexture(name string, w, h int, format, bind uint32) (*Texture, error) {
	desc := Texture2DDesc{
		Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: format,
		SampleDesc: SampleDesc{Count: 1}, Usage: UsageDefault, BindFlags: bind,
	}
	var tex unsafe.Pointer
	r := d.Dev.Call(devCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&tex)))
	if err := com.Check(r, fmt.Sprintf("CreateTexture2D %s %dx%d fmt=%d", name, w, h, format)); err != nil {
		return nil, err
	}
	t := &Texture{Name: name, Tex: com.FromRaw(tex), W: w, H: h, Format: format}
	if err := d.createViews(t, bind); err != nil {
		t.Release()
		return nil, err
	}
	return t, nil
}

func (d *Device) createViews(t *Texture, bind uint32) error {
	if bind&BindShaderResource != 0 {
		var v unsafe.Pointer
		r := d.Dev.Call(devCreateShaderResourceView, t.Tex.U(), 0, uintptr(unsafe.Pointer(&v)))
		if err := com.Check(r, "CreateShaderResourceView "+t.Name); err != nil {
			return err
		}
		t.SRV = com.FromRaw(v)
	}
	if bind&BindUnorderedAccess != 0 {
		var v unsafe.Pointer
		r := d.Dev.Call(devCreateUnorderedAccessView, t.Tex.U(), 0, uintptr(unsafe.Pointer(&v)))
		if err := com.Check(r, "CreateUnorderedAccessView "+t.Name); err != nil {
			return err
		}
		t.UAV = com.FromRaw(v)
	}
	if bind&BindRenderTarget != 0 {
		var v unsafe.Pointer
		r := d.Dev.Call(devCreateRenderTargetView, t.Tex.U(), 0, uintptr(unsafe.Pointer(&v)))
		if err := com.Check(r, "CreateRenderTargetView "+t.Name); err != nil {
			return err
		}
		t.RTV = com.FromRaw(v)
	}
	return nil
}

func TextureDesc(tex com.Ptr) Texture2DDesc {
	var d Texture2DDesc
	tex.Call(tex2DGetDesc, uintptr(unsafe.Pointer(&d)))
	return d
}

func (d *Device) Upload(t *Texture, data []byte, rowPitch int) {
	if len(data) == 0 {
		return
	}
	d.Ctx.Call(ctxUpdateSubresource, t.Tex.U(), 0, 0, uintptr(unsafe.Pointer(&data[0])), uintptr(rowPitch), 0)
}

// CopyTexture copies the whole resource (size and format must match).
func (d *Device) CopyTexture(dst, src *Texture) {
	d.Ctx.Call(ctxCopyResource, dst.Tex.U(), src.Tex.U())
}

func (d *Device) CopyRegion(dst *Texture, x, y int, src com.Ptr, box Box) {
	d.Ctx.Call(ctxCopySubresourceRegion, dst.Tex.U(), 0, uintptr(x), uintptr(y), 0, src.U(), 0, uintptr(unsafe.Pointer(&box)))
}

// ConstantBuffer is a dynamic constant buffer (updated via Map WRITE_DISCARD).
type ConstantBuffer struct {
	Buf  com.Ptr
	Size int
}

// NewConstantBuffer creates a buffer of size bytes (rounded up to 16).
func (d *Device) NewConstantBuffer(size int) (*ConstantBuffer, error) {
	size = (size + 15) &^ 15
	desc := BufferDesc{ByteWidth: uint32(size), Usage: UsageDynamic, BindFlags: BindConstantBuffer, CPUAccessFlags: CPUAccessWrite}
	var b unsafe.Pointer
	r := d.Dev.Call(devCreateBuffer, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&b)))
	if err := com.Check(r, "CreateBuffer(constant)"); err != nil {
		return nil, err
	}
	return &ConstantBuffer{Buf: com.FromRaw(b), Size: size}, nil
}

func (c *ConstantBuffer) Release() {
	if c != nil {
		c.Buf.Release()
	}
}

func (d *Device) Write(c *ConstantBuffer, src unsafe.Pointer, n int) error {
	if n > c.Size {
		return fmt.Errorf("constant buffer: %d > %d байт", n, c.Size)
	}
	var m MappedSubresource
	r := d.Ctx.Call(ctxMap, c.Buf.U(), 0, uintptr(MapWriteDiscard), 0, uintptr(unsafe.Pointer(&m)))
	if err := com.Check(r, "Map(constant buffer)"); err != nil {
		return err
	}
	copy(unsafe.Slice((*byte)(m.PData), n), unsafe.Slice((*byte)(src), n))
	d.Ctx.Call(ctxUnmap, c.Buf.U(), 0)
	return nil
}

// SetConstants writes struct v into the buffer (v's layout must match the HLSL cbuffer).
func SetConstants[T any](d *Device, c *ConstantBuffer, v *T) error {
	return d.Write(c, unsafe.Pointer(v), int(unsafe.Sizeof(*v)))
}

func (d *Device) NewSampler(filter, address uint32) (com.Ptr, error) {
	desc := SamplerDesc{
		Filter: filter, AddressU: address, AddressV: address, AddressW: address,
		MaxAnisotropy: 1, ComparisonFunc: ComparisonNever, MaxLOD: math.MaxFloat32,
	}
	var s unsafe.Pointer
	r := d.Dev.Call(devCreateSamplerState, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&s)))
	if err := com.Check(r, "CreateSamplerState"); err != nil {
		return com.Ptr{}, err
	}
	return com.FromRaw(s), nil
}
