//go:build windows

package gfx

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"unsafe"

	"inbetween/internal/com"
)

type stagingKey struct {
	w, h   int
	format uint32
}

func (d *Device) stagingFor(w, h int, format uint32) (*Texture, error) {
	k := stagingKey{w, h, format}
	if t, ok := d.staging[k]; ok {
		return t, nil
	}
	desc := Texture2DDesc{
		Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: format,
		SampleDesc: SampleDesc{Count: 1}, Usage: UsageStaging, CPUAccessFlags: CPUAccessRead,
	}
	var tex unsafe.Pointer
	r := d.Dev.Call(devCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&tex)))
	if err := com.Check(r, "CreateTexture2D(staging)"); err != nil {
		return nil, err
	}
	t := &Texture{Name: "staging", Tex: com.FromRaw(tex), W: w, H: h, Format: format}
	d.staging[k] = t
	return t, nil
}

// ReadPixels copies a texture into CPU memory (tightly packed, no padding). Blocks until
// the GPU is ready — for debugging/dumps/eval only, not the hot path.
func (d *Device) ReadPixels(t *Texture) ([]byte, int, error) {
	bpp := BytesPerPixel(t.Format)
	if bpp == 0 {
		return nil, 0, fmt.Errorf("ReadPixels: формат %d не поддержан", t.Format)
	}
	st, err := d.stagingFor(t.W, t.H, t.Format)
	if err != nil {
		return nil, 0, err
	}
	d.Ctx.Call(ctxCopyResource, st.Tex.U(), t.Tex.U())
	var m MappedSubresource
	r := d.Ctx.Call(ctxMap, st.Tex.U(), 0, uintptr(MapRead), 0, uintptr(unsafe.Pointer(&m)))
	if err := com.Check(r, "Map(staging)"); err != nil {
		return nil, 0, err
	}
	defer d.Ctx.Call(ctxUnmap, st.Tex.U(), 0)
	row := t.W * bpp
	out := make([]byte, row*t.H)
	src := unsafe.Slice((*byte)(m.PData), int(m.RowPitch)*(t.H-1)+row)
	for y := 0; y < t.H; y++ {
		copy(out[y*row:(y+1)*row], src[y*int(m.RowPitch):])
	}
	return out, row, nil
}

func (d *Device) ReadImage(t *Texture) (*image.RGBA, error) {
	if t.Format != FormatRGBA8 && t.Format != FormatBGRA8 {
		return nil, fmt.Errorf("ReadImage: ожидался RGBA8/BGRA8, а не %d (%s)", t.Format, t.Name)
	}
	px, row, err := d.ReadPixels(t)
	if err != nil {
		return nil, err
	}
	if t.Format == FormatBGRA8 {
		for i := 0; i+3 < len(px); i += 4 {
			px[i], px[i+2] = px[i+2], px[i]
		}
	}
	for i := 3; i < len(px); i += 4 {
		px[i] = 255
	}
	return &image.RGBA{Pix: px, Stride: row, Rect: image.Rect(0, 0, t.W, t.H)}, nil
}

func (d *Device) ReadFloat32(t *Texture) ([]float32, error) {
	if t.Format != FormatR32F {
		return nil, fmt.Errorf("ReadFloat32: ожидался R32F, а не %d", t.Format)
	}
	px, _, err := d.ReadPixels(t)
	if err != nil {
		return nil, err
	}
	out := make([]float32, len(px)/4)
	for i := range out {
		out[i] = math.Float32frombits(uint32(px[4*i]) | uint32(px[4*i+1])<<8 | uint32(px[4*i+2])<<16 | uint32(px[4*i+3])<<24)
	}
	return out, nil
}

func (d *Device) SavePNG(t *Texture, path string) error {
	img, err := d.ReadImage(t)
	if err != nil {
		return err
	}
	return SaveImage(img, path)
}

func SaveImage(img image.Image, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
