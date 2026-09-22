//go:build windows

package capture

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"inbetween/internal/gfx"
	"inbetween/internal/win"
)

// Files is a PNG sequence read from a directory (sorted by name) — e.g. frames dumped
// from a video: ffmpeg -i clip.mp4 in/%06d.png. Used by the offline mode.
type Files struct {
	d     *gfx.Device
	paths []string
	i     int
	tex   *gfx.Texture
	W, H  int
	FPS   float64
}

func NewFiles(d *gfx.Device, dir string, fps float64) (*Files, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		return nil, err
	}
	if len(paths) < 2 {
		return nil, fmt.Errorf("в %q меньше двух PNG", dir)
	}
	sort.Strings(paths)
	first, err := loadRGBA(paths[0])
	if err != nil {
		return nil, err
	}
	w, h := first.Rect.Dx(), first.Rect.Dy()
	tex, err := d.NewTexture("files", w, h, gfx.FormatRGBA8, gfx.BindShaderResource)
	if err != nil {
		return nil, err
	}
	return &Files{d: d, paths: paths, tex: tex, W: w, H: h, FPS: fps}, nil
}

func loadRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if rgba, ok := img.(*image.RGBA); ok && rgba.Rect.Min == (image.Point{}) {
		return rgba, nil
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Rect, img, b.Min, draw.Src)
	return rgba, nil
}

func (s *Files) Name() string     { return fmt.Sprintf("files %d×png %dx%d", len(s.paths), s.W, s.H) }
func (s *Files) Size() (int, int) { return s.W, s.H }

func (s *Files) Len() int { return len(s.paths) }

// Poll returns the next frame immediately; io.EOF once the frames run out.
func (s *Files) Poll(time.Duration) (Frame, bool, error) {
	if s.i >= len(s.paths) {
		return Frame{}, false, io.EOF
	}
	img, err := loadRGBA(s.paths[s.i])
	if err != nil {
		return Frame{}, false, err
	}
	if img.Rect.Dx() != s.W || img.Rect.Dy() != s.H {
		return Frame{}, false, fmt.Errorf("%s: размер %dx%d, ожидался %dx%d", s.paths[s.i], img.Rect.Dx(), img.Rect.Dy(), s.W, s.H)
	}
	s.d.Upload(s.tex, img.Pix, img.Stride)
	s.i++
	t := int64(float64(s.i-1) * float64(win.QPF()) / s.FPS)
	return Frame{Tex: s.tex, Time: t, Seq: uint64(s.i)}, true, nil
}

func (s *Files) Close() { s.tex.Release() }
