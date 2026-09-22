//go:build windows

package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"

	"inbetween/internal/capture"
	"inbetween/internal/config"
	"inbetween/internal/fg"
	"inbetween/internal/gfx"
)

// RunOffline turns a PNG sequence into a PNG sequence with intermediate frames inserted.
// Handy for running the algorithm against a recording of a real game:
//
//	ffmpeg -i clip.mp4 -vf fps=30 in/%06d.png
//	inbetween -mode offline -in in -out out -mult 2 -fps 30
//	ffmpeg -framerate 60 -i out/%06d.png -c:v libx264 -crf 16 result.mp4
func RunOffline(cfg *config.Config) error {
	e, err := newEnv(cfg, max(cfg.Adapter, 0))
	if err != nil {
		return err
	}
	defer e.Close()
	src, err := capture.NewFiles(e.dev, cfg.In, cfg.FPS)
	if err != nil {
		return err
	}
	defer src.Close()
	pipe, err := fg.New(e.dev, e.lib, src.W, src.H, cfg.FGOptions())
	if err != nil {
		return err
	}
	defer pipe.Close()
	log.Printf("offline: %s -> %s, X%d, algo=%s", src.Name(), cfg.Out, cfg.Mult, pipe.Opt.Algo)

	n := 0
	save := func(tex *gfx.Texture) error {
		err := e.dev.SavePNG(tex, filepath.Join(cfg.Out, fmt.Sprintf("%06d.png", n)))
		n++
		return err
	}
	for {
		f, ok, err := src.Poll(0)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		e.beginGPU()
		pipe.Push(f)
		if pipe.Ready() { // intermediate frames first, then the real frame
			for k := 1; k < cfg.Mult; k++ {
				if err := save(pipe.Generate(float32(k) / float32(cfg.Mult))); err != nil {
					return err
				}
			}
		}
		if err := save(pipe.Latest()); err != nil {
			return err
		}
		e.endGPU()
		if f.Seq%50 == 0 {
			log.Printf("offline: %d/%d, GPU: %s", f.Seq, src.Len(), e.gpuSummary())
		}
	}
	log.Printf("готово: %d кадров в %s. Видео: ffmpeg -framerate %g -i %s/%%06d.png -c:v libx264 -crf 16 result.mp4",
		n, cfg.Out, cfg.FPS*float64(cfg.Mult), cfg.Out)
	return nil
}
