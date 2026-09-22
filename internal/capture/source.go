//go:build windows

package capture

import (
	"time"

	"inbetween/internal/gfx"
)

// Frame is a new frame from a source. Tex is only valid until the next Poll: the
// consumer (fg.Pipeline.Push) copies the area it needs right away.
type Frame struct {
	Tex    *gfx.Texture // source texture (BGRA8 or RGBA8)
	Offset [2]int       // top-left corner of the usable area within Tex
	Time   int64        // QPC timestamp of when the frame arrived at the source
	Seq    uint64       // the source's frame number (1, 2, ...)
	Missed int          // how many source frames were dropped before this one
}

// Source is a frame source. All methods are called from the main thread.
type Source interface {
	Name() string
	Size() (w, h int) // size of the frame's usable area
	// Poll waits for a new frame for at most timeout. ok=false means no frame yet.
	// io.EOF means the source is exhausted (Files).
	Poll(timeout time.Duration) (f Frame, ok bool, err error)
	Close()
}
